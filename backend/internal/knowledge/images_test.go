package knowledge

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"image"
	"image/png"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"
)

func pngFixture(t *testing.T, w, h int) []byte {
	t.Helper()
	var b bytes.Buffer
	if e := png.Encode(&b, image.NewGray(image.Rect(0, 0, w, h))); e != nil {
		t.Fatal(e)
	}
	return b.Bytes()
}

func drawing(id, alt string) string {
	return `<w:r><w:drawing><wp:inline><wp:docPr id="1" name="p" descr="` + alt + `"/><a:graphic><a:graphicData><a:blip r:embed="` + id + `"/></a:graphicData></a:graphic></wp:inline></w:drawing></w:r>`
}

func relations(targets map[string]string) []byte {
	var b strings.Builder
	b.WriteString(`<Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships">`)
	for id, target := range targets {
		mode := ""
		if strings.HasPrefix(target, "http") {
			mode = ` TargetMode="External"`
		}
		fmt.Fprintf(&b, `<Relationship Id="%s" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/image" Target="%s"%s/>`, id, target, mode)
	}
	b.WriteString(`</Relationships>`)
	return []byte(b.String())
}

type fakeVision struct {
	mu        sync.Mutex
	calls     int
	transient int
	err       error
	text      string
}

func (*fakeVision) Model() string { return "fixture-vision" }
func (f *fakeVision) DescribeImage(ctx context.Context, mime string, data []byte) (string, bool, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls++
	if mime != "image/png" || len(data) == 0 {
		return "", false, ErrInvalid
	}
	if f.transient > 0 {
		f.transient--
		return "", false, ErrUnavailable
	}
	if f.err != nil {
		return "", false, f.err
	}
	return f.text, false, nil
}

func hasDiagnostic(p Preview, code string) bool {
	for _, d := range p.Diagnostics {
		if d.Code == code {
			return true
		}
	}
	return false
}

func TestMarkdownTableSplitsByRowsWithRepeatedHeader(t *testing.T) {
	var src strings.Builder
	src.WriteString("# 服务器清单\n\n|主机|地址|用途|\n|---|---|---|\n")
	for i := 1; i <= 60; i++ {
		fmt.Fprintf(&src, "|wms-%02d|10.0.0.%d|仓储 WMS 接口服务节点|\n", i, i)
	}
	src.WriteString("\n```bash\nps aux | grep wms | awk '{print $2}'\n```\n")
	p := Parse(context.Background(), "ops.md", []byte(src.String()))
	blocks, err := splitWithInputLimit(p, 768)
	if err != nil || len(blocks) < 3 {
		t.Fatal(err, len(blocks))
	}
	seen := map[string]int{}
	for _, b := range blocks {
		if len(b.Locator.Heading+"\n"+b.Content) > 768 {
			t.Fatal("budget exceeded", b.Locator)
		}
		if !strings.Contains(b.Content, "|wms-") {
			continue
		}
		if !strings.HasPrefix(b.Content, "|主机|地址|用途|\n|---|---|---|\n") {
			t.Fatal("header not repeated", b.Content)
		}
		rows := strings.Split(b.Content, "\n")[2:]
		if b.Locator.LineEnd-b.Locator.Line+1 != len(rows) {
			t.Fatal("row lines do not match locator", b.Locator, len(rows))
		}
		for _, r := range rows {
			seen[r]++
		}
	}
	if len(seen) != 60 {
		t.Fatal("rows lost or merged", len(seen))
	}
	for r, n := range seen {
		if n != 1 {
			t.Fatal("row duplicated", r)
		}
	}
	// A single row longer than the budget is cut, and every piece keeps the header.
	p = Parse(context.Background(), "long.md", []byte("|项|说明|\n|---|---|\n|a|"+strings.Repeat("很长的说明", 120)+"|\n|b|短|"))
	blocks, err = splitWithInputLimit(p, 768)
	if err != nil || len(blocks) < 3 {
		t.Fatal(err)
	}
	for _, b := range blocks {
		if !strings.HasPrefix(b.Content, "|项|说明|\n|---|---|\n") || len(b.Content)+1 > 768 {
			t.Fatal("long row piece lost header or budget", b.Content)
		}
	}
}

func TestSplitTextCutsAtLineBreaks(t *testing.T) {
	lines := []string{}
	for i := 1; i <= 40; i++ {
		lines = append(lines, fmt.Sprintf("taxIncludedUnitPrice%02d（含税单价） | 10.0 | 11.6600 | 单价高 1.66", i))
	}
	text := strings.Join(lines, "\n")
	pieces := splitText(text, chunkRunes, 768-len("标题")-1)
	if len(pieces) < 3 {
		t.Fatal(len(pieces))
	}
	covered := map[string]bool{}
	for i, piece := range pieces {
		if len(piece) > 768-len("标题")-1 || !strings.Contains(text, piece) {
			t.Fatal("budget exceeded or text altered", i)
		}
		if i > 0 && !strings.HasPrefix(piece, "taxIncludedUnitPrice") {
			t.Fatalf("piece %d starts mid-line: %q", i, piece[:20])
		}
		if i < len(pieces)-1 && !strings.HasSuffix(piece, "\n") {
			t.Fatalf("piece %d ends mid-line", i)
		}
		for _, line := range strings.Split(strings.TrimSuffix(piece, "\n"), "\n") {
			covered[line] = true
		}
	}
	for _, line := range lines {
		if !covered[line] {
			t.Fatal("line lost", line)
		}
	}
}

func TestDOCXMergedCellsAndHeaderRowsExpand(t *testing.T) {
	cell := func(props, text string) string {
		return `<w:tc><w:tcPr>` + props + `</w:tcPr><w:p><w:r><w:t>` + text + `</w:t></w:r></w:p></w:tc>`
	}
	body := `<w:tbl>` +
		`<w:tr><w:trPr><w:tblHeader/></w:trPr>` + cell("", "系统") + cell(`<w:gridSpan w:val="2"/>`, "联系人") + `</w:tr>` +
		`<w:tr>` + cell(`<w:vMerge w:val="restart"/>`, "WMS") + cell("", "张三") + cell("", "8001") + `</w:tr>` +
		`<w:tr>` + cell(`<w:vMerge/>`, "") + cell("", "李四") + cell("", "8002") + `</w:tr>` +
		`</w:tbl>`
	p := Parse(context.Background(), "a.docx", docxFixture(t, body))
	if !p.Valid() || len(p.Blocks) != 1 {
		t.Fatal(p)
	}
	want := "系统 | 联系人 | 联系人\nWMS | 张三 | 8001\nWMS | 李四 | 8002"
	if p.Blocks[0].Content != want || p.Blocks[0].table == nil || len(p.Blocks[0].table.rows) != 2 {
		t.Fatalf("%q", p.Blocks[0].Content)
	}
}

func TestDOCXTextboxReadOnceAndChartsWarn(t *testing.T) {
	textbox := `<w:txbxContent><w:p><w:r><w:t>入库 → 上架</w:t></w:r></w:p></w:txbxContent>`
	body := `<w:p><w:r><w:t>流程：</w:t></w:r><w:r><mc:AlternateContent><mc:Choice Requires="wps"><w:drawing><wp:anchor>` + textbox + `</wp:anchor></w:drawing></mc:Choice><mc:Fallback><w:pict><v:textbox>` + textbox + `</v:textbox></w:pict></mc:Fallback></mc:AlternateContent></w:r></w:p>` +
		`<w:p><w:r><w:drawing><wp:inline><a:graphic><a:graphicData><c:chart xmlns:c="urn:chart" r:id="rId9"/></a:graphicData></a:graphic></wp:inline></w:drawing></w:r></w:p>`
	p := Parse(context.Background(), "a.docx", docxFixture(t, body))
	if !p.Valid() || len(p.Blocks) != 1 || p.Blocks[0].Content != "流程：\n入库 → 上架" {
		t.Fatal(p)
	}
	if !hasDiagnostic(p, "chart_not_extracted") {
		t.Fatal("chart not reported", p.Diagnostics)
	}
}

func imageDOCX(t *testing.T, body string, media map[string][]byte, rels map[string]string) []byte {
	extra := map[string][]byte{"word/_rels/document.xml.rels": relations(rels)}
	for name, data := range media {
		extra[name] = data
	}
	return docxPackage(t, body, extra)
}

func TestDOCXImagesRecognizedAndLabelled(t *testing.T) {
	shot := pngFixture(t, 640, 360)
	body := `<w:p><w:pPr><w:pStyle w:val="Heading1"/></w:pPr><w:r><w:t>上架异常</w:t></w:r></w:p>` +
		`<w:p><w:r><w:t>出现如下报错：</w:t></w:r>` + drawing("rId1", "报错截图") + `</w:p>` +
		`<w:p>` + drawing("rId1", "") + `</w:p>` +
		`<w:p>` + drawing("rId2", "") + `</w:p>` +
		`<w:p>` + drawing("rId3", "") + `</w:p>` +
		`<w:tbl><w:tr><w:tc><w:p><w:r><w:t>步骤</w:t></w:r></w:p></w:tc></w:tr><w:tr><w:tc><w:p>` + drawing("rId4", "") + `</w:p></w:tc></w:tr></w:tbl>`
	data := imageDOCX(t, body, map[string][]byte{
		"word/media/shot.png": shot,
		"word/media/logo.png": pngFixture(t, 16, 16),
		"word/media/flow.emf": []byte("\x01\x00\x00\x00 EMF vector"),
	}, map[string]string{"rId1": "media/shot.png", "rId2": "media/logo.png", "rId3": "media/flow.emf", "rId4": "/word/media/shot.png"})
	p := Parse(context.Background(), "a.docx", data)
	if !p.Valid() || len(p.images) != 5 {
		t.Fatal(p)
	}
	old := visionRetryDelay
	visionRetryDelay = time.Millisecond
	defer func() { visionRetryDelay = old }()
	v := &fakeVision{transient: 1, text: "ERROR LOC-404 库位不存在\n界面截图：上架确认页报错"}
	s := &Service{Vision: v}
	if e := s.resolveImages(context.Background(), &p); e != nil {
		t.Fatal(e)
	}
	// One unique recognizable image, one transient retry; logo and EMF are never sent.
	if v.calls != 2 {
		t.Fatal("calls", v.calls)
	}
	recognized := []Block{}
	for _, b := range p.Blocks {
		if b.image != 0 {
			t.Fatal("placeholder left", b)
		}
		if strings.Contains(b.Content, "视觉模型识别") {
			recognized = append(recognized, b)
		}
	}
	if len(recognized) != 3 || !strings.Contains(recognized[0].Content, "替代文字：报错截图\nERROR LOC-404") || recognized[0].Locator.Heading != "上架异常" || recognized[0].Locator.Paragraph != 2 {
		t.Fatal(recognized)
	}
	tableMarked := false
	for _, b := range p.Blocks {
		tableMarked = tableMarked || b.Content == "步骤\n[图片 5]"
	}
	if !tableMarked {
		t.Fatal("table cell marker missing", p.Blocks)
	}
	if !hasDiagnostic(p, "image_recognized") || !hasDiagnostic(p, "image_not_recognized") || !p.Valid() {
		t.Fatal(p.Diagnostics)
	}
	blocks, err := splitWithInputLimit(p, 768)
	if err != nil || len(blocks) == 0 {
		t.Fatal(err)
	}
}

func TestDOCXImagesDegradeWithoutBlocking(t *testing.T) {
	body := `<w:p><w:r><w:t>正文</w:t></w:r>` + drawing("rId1", "仓库平面图") + drawing("rId2", "") + drawing("rId3", "") + `</w:p>`
	data := imageDOCX(t, body, map[string][]byte{"word/media/a.png": pngFixture(t, 200, 100)}, map[string]string{"rId1": "media/a.png", "rId2": "media/a.png", "rId3": "https://example.com/x.png"})
	for name, s := range map[string]*Service{"not configured": {}, "rejected": {Vision: &fakeVision{err: ErrInvalid}}} {
		p := Parse(context.Background(), "a.docx", data)
		if e := s.resolveImages(context.Background(), &p); e != nil {
			t.Fatal(name, e)
		}
		if !p.Valid() || len(p.Blocks) != 2 || p.Blocks[1].Content != "[图片 1 · 替代文字]\n仓库平面图" || !hasDiagnostic(p, "image_not_recognized") {
			t.Fatal(name, p)
		}
		for _, d := range p.Diagnostics {
			if d.Blocking {
				t.Fatal(name, d)
			}
		}
	}
	// Without any text, an unrecognized image-only document is empty.
	p := Parse(context.Background(), "a.docx", imageDOCX(t, `<w:p>`+drawing("rId1", "")+`</w:p>`, map[string][]byte{"word/media/a.png": pngFixture(t, 200, 100)}, map[string]string{"rId1": "media/a.png"}))
	if e := (&Service{}).resolveImages(context.Background(), &p); e != nil || p.Valid() || !hasDiagnostic(p, "empty_text") {
		t.Fatal(e, p)
	}
	// Cancellation never yields a partially recognized preview.
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	p = Parse(context.Background(), "a.docx", data)
	if e := (&Service{Vision: &fakeVision{err: context.Canceled}}).resolveImages(ctx, &p); e == nil {
		t.Fatal("cancelled recognition accepted")
	}
}

func TestModelVisionTransport(t *testing.T) {
	var got map[string]any
	status := http.StatusOK
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(raw, &got)
		if r.URL.Path != "/v1/chat/completions" || r.Header.Get("Authorization") != "Bearer k" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		w.WriteHeader(status)
		_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"截图文字"},"finish_reason":"stop"}]}`))
	}))
	defer srv.Close()
	v, err := NewModelVision(srv.URL+"/v1/", "k", "qwen3.7-plus")
	if err != nil {
		t.Fatal(err)
	}
	text, truncated, err := v.DescribeImage(context.Background(), "image/png", []byte("img"))
	if err != nil || text != "截图文字" || truncated {
		t.Fatal(text, err)
	}
	content := got["messages"].([]any)[0].(map[string]any)["content"].([]any)
	if url := content[0].(map[string]any)["image_url"].(map[string]any)["url"]; url != "data:image/png;base64,aW1n" || got["enable_thinking"] != nil {
		t.Fatal(got)
	}
	for code, want := range map[int]error{http.StatusTooManyRequests: ErrUnavailable, http.StatusBadGateway: ErrUnavailable, http.StatusBadRequest: ErrInvalid} {
		status = code
		if _, _, err := v.DescribeImage(context.Background(), "image/png", []byte("img")); !errors.Is(err, want) {
			t.Fatal(code, err)
		}
	}
	bailian, err := NewModelVision("https://dashscope.aliyuncs.com/compatible-mode/v1", "k", "qwen3.7-plus")
	if err != nil || !bailian.disableThinking {
		t.Fatal("Bailian thinking not disabled")
	}
	for _, base := range []string{"", "ftp://x", "https://u:p@x"} {
		if _, err := NewModelVision(base, "k", "m"); err == nil {
			t.Fatal(base)
		}
	}
}
