package knowledge

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"image"
	"image/gif"
	_ "image/jpeg"
	"image/png"
	"sort"
	"strings"
	"sync"
	"time"
	"unicode"
	"unicode/utf8"
)

// ImageDescriber transcribes one document image into retrievable text.
// ErrUnavailable marks a transient failure that may be retried.
type ImageDescriber interface {
	Model() string
	DescribeImage(ctx context.Context, mime string, data []byte) (text string, truncated bool, err error)
}

const (
	maxRecognizedImages = 200
	visionWorkers       = 4
	maxImageText        = 4000
	decorativeMarker    = "[装饰图片]"
)

// visionRetryDelay is the base backoff between transient vision failures.
var visionRetryDelay = 2 * time.Second

type imageOutcome struct {
	text                  string
	decorative, truncated bool
	problem               string
}

// resolveImages replaces DOCX image placeholders with model transcriptions
// labelled as model generated. Images that cannot be recognized keep their
// alternative text when present and are reported as non-blocking warnings;
// the rest of the document is still indexed. Cancellation returns an error so
// a partially recognized document is never published.
func (s *Service) resolveImages(ctx context.Context, p *Preview) error {
	if len(p.images) == 0 {
		return nil
	}
	blocked := p.blocked()
	useModel := s.Vision != nil && !blocked
	outcomes := map[string]*imageOutcome{}
	type job struct {
		outcome *imageOutcome
		mime    string
		data    []byte
	}
	jobs := []job{}
	for _, img := range p.images {
		if img.problem != "" || outcomes[img.key] != nil {
			continue
		}
		o := &imageOutcome{}
		outcomes[img.key] = o
		mime, data, problem, decorative := prepareImage(img.data)
		switch {
		case decorative:
			o.decorative = true
		case problem != "":
			o.problem = problem
		case !useModel:
			o.problem = "平台未配置视觉模型"
		case len(jobs) >= maxRecognizedImages:
			o.problem = fmt.Sprintf("超过单个文档 %d 张图片的识别上限", maxRecognizedImages)
		default:
			jobs = append(jobs, job{o, mime, data})
		}
	}
	if len(jobs) > 0 {
		sem := make(chan struct{}, visionWorkers)
		var wg sync.WaitGroup
		for _, j := range jobs {
			wg.Add(1)
			go func() {
				defer wg.Done()
				sem <- struct{}{}
				defer func() { <-sem }()
				text, truncated, err := s.describe(ctx, j.mime, j.data)
				if err != nil {
					j.outcome.problem = "视觉模型识别失败"
					return
				}
				text, cut := cleanImageText(text)
				switch {
				case text == "":
					j.outcome.problem = "视觉模型未返回内容"
				case strings.Trim(text, " 。.") == decorativeMarker:
					j.outcome.decorative = true
				default:
					j.outcome.text, j.outcome.truncated = text, truncated || cut
				}
			}()
		}
		wg.Wait()
		if ctx.Err() != nil {
			return ctx.Err()
		}
	}
	problems := map[string][]int{}
	recognized, truncated := 0, []int{}
	blocks := make([]Block, 0, len(p.Blocks))
	for _, b := range p.Blocks {
		if b.image == 0 {
			blocks = append(blocks, b)
			continue
		}
		img := p.images[b.image-1]
		o := &imageOutcome{problem: img.problem}
		if img.problem == "" {
			o = outcomes[img.key]
		}
		b.image = 0
		switch {
		case o.decorative:
			continue
		case o.text != "":
			recognized++
			if o.truncated {
				truncated = append(truncated, img.index)
			}
			b.Content = fmt.Sprintf("[图片 %d · 视觉模型识别]\n", img.index)
			if img.alt != "" {
				b.Content += "替代文字：" + img.alt + "\n"
			}
			b.Content += o.text
		default:
			problems[o.problem] = append(problems[o.problem], img.index)
			if img.alt == "" {
				continue
			}
			b.Content = fmt.Sprintf("[图片 %d · 替代文字]\n%s", img.index, img.alt)
		}
		blocks = append(blocks, b)
	}
	p.Blocks, p.images = blocks, nil
	if blocked {
		return nil
	}
	if recognized > 0 {
		diagnostic(p, "image_recognized", fmt.Sprintf("已用视觉模型 %s 识别 %d 张图片；识别内容由模型生成，可能有误，请在预览中核对", s.Vision.Model(), recognized), 0, false)
	}
	if len(truncated) > 0 {
		diagnostic(p, "image_text_truncated", "图片 "+joinIndexes(truncated)+" 识别内容过长，仅保留前段", 0, false)
	}
	reasons := make([]string, 0, len(problems))
	for reason := range problems {
		reasons = append(reasons, reason)
	}
	sort.Strings(reasons)
	for _, reason := range reasons {
		diagnostic(p, "image_not_recognized", "图片 "+joinIndexes(problems[reason])+" 未识别（"+reason+"）；图片内容未纳入检索，有替代文字的仅保留替代文字", 0, false)
	}
	total := 0
	for _, b := range p.Blocks {
		total += len(b.Content)
	}
	if total == 0 {
		diagnostic(p, "empty_text", "没有可可靠提取的正文；扫描件 OCR 暂不支持", 0, true)
	}
	if total > MaxText {
		diagnostic(p, "text_too_large", "提取文本（含图片识别内容）超过 2 MiB 上限", 0, true)
	}
	return nil
}

func (s *Service) describe(ctx context.Context, mime string, data []byte) (string, bool, error) {
	var err error
	for attempt := 0; attempt < 3; attempt++ {
		if attempt > 0 {
			select {
			case <-ctx.Done():
				return "", false, ctx.Err()
			case <-time.After(time.Duration(attempt) * visionRetryDelay):
			}
		}
		var text string
		var truncated bool
		text, truncated, err = s.Vision.DescribeImage(ctx, mime, data)
		if err == nil || !errors.Is(err, ErrUnavailable) || ctx.Err() != nil {
			return text, truncated, err
		}
	}
	return "", false, err
}

// prepareImage checks the formats and dimensions accepted by vision models.
// GIF is converted to PNG; EMF/WMF/SVG and other vector formats are rejected.
func prepareImage(data []byte) (mime string, out []byte, problem string, decorative bool) {
	mime = imageMIME(data)
	if mime == "" {
		return "", nil, "格式不受支持，如 EMF/WMF 矢量图", false
	}
	out = data
	if mime == "image/gif" {
		img, err := gif.Decode(bytes.NewReader(data))
		var buf bytes.Buffer
		if err != nil || png.Encode(&buf, img) != nil {
			return "", nil, "图片损坏", false
		}
		mime, out = "image/png", buf.Bytes()
	}
	if mime == "image/png" || mime == "image/jpeg" {
		cfg, _, err := image.DecodeConfig(bytes.NewReader(out))
		if err != nil {
			return "", nil, "图片损坏", false
		}
		w, h := cfg.Width, cfg.Height
		if w <= 10 || h <= 10 || (w < 32 && h < 32) {
			return "", nil, "", true
		}
		if max(w, h) > 200*min(w, h) {
			return "", nil, "宽高比超出模型限制", false
		}
	}
	if len(out) > maxImageBytes {
		return "", nil, "图片超过 10 MiB", false
	}
	return mime, out, "", false
}

func imageMIME(data []byte) string {
	switch {
	case bytes.HasPrefix(data, []byte("\x89PNG\r\n\x1a\n")):
		return "image/png"
	case bytes.HasPrefix(data, []byte{0xff, 0xd8, 0xff}):
		return "image/jpeg"
	case bytes.HasPrefix(data, []byte("GIF87a")), bytes.HasPrefix(data, []byte("GIF89a")):
		return "image/gif"
	case bytes.HasPrefix(data, []byte("BM")) && len(data) > 26:
		return "image/bmp"
	case len(data) > 12 && bytes.HasPrefix(data, []byte("RIFF")) && string(data[8:12]) == "WEBP":
		return "image/webp"
	case bytes.HasPrefix(data, []byte("II*\x00")), bytes.HasPrefix(data, []byte("MM\x00*")):
		return "image/tiff"
	}
	return ""
}

// cleanImageText removes control characters and bounds stored model output.
func cleanImageText(s string) (string, bool) {
	s = strings.Map(func(r rune) rune {
		if r == utf8.RuneError || (unicode.IsControl(r) && r != '\n' && r != '\t') {
			return -1
		}
		return r
	}, strings.ReplaceAll(s, "\r\n", "\n"))
	s = strings.TrimSpace(s)
	if r := []rune(s); len(r) > maxImageText {
		return strings.TrimSpace(string(r[:maxImageText])), true
	}
	return s, false
}
