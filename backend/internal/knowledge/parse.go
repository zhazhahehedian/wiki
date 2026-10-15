package knowledge

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/xml"
	"fmt"
	"io"
	"math"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"

	pdf "github.com/ledongthuc/pdf"
)

func Supported(name string) bool {
	switch strings.ToLower(filepath.Ext(name)) {
	case ".txt", ".md", ".markdown", ".docx", ".pdf":
		return true
	}
	return false
}
func diagnostic(p *Preview, code, message string, page int, blocking bool) {
	p.Diagnostics = append(p.Diagnostics, Diagnostic{code, message, page, blocking})
}
func (p Preview) Valid() bool {
	if len(p.Blocks) == 0 {
		return false
	}
	for _, d := range p.Diagnostics {
		if d.Blocking {
			return false
		}
	}
	return true
}

// Parse retains verified source locations. A failed page is never skipped as success.
func Parse(ctx context.Context, name string, data []byte) (p Preview) {
	p.Blocks = []Block{}
	p.Diagnostics = []Diagnostic{}
	defer func() {
		if recover() != nil {
			diagnostic(&p, "corrupt_document", "文档结构损坏，解析未完成", 0, true)
		}
	}()
	if !Supported(name) || len(data) > MaxUpload {
		diagnostic(&p, "unsupported_format", "仅支持 TXT、Markdown、DOCX 和文字型 PDF（最大 20 MiB）", 0, true)
		return
	}
	p.Format = strings.TrimPrefix(strings.ToLower(filepath.Ext(name)), ".")
	switch p.Format {
	case "pdf":
		parsePDF(ctx, data, &p)
	case "docx":
		parseDOCX(ctx, data, &p)
	default:
		data = bytes.TrimPrefix(data, []byte{0xef, 0xbb, 0xbf})
		if bytes.HasPrefix(data, []byte("%PDF-")) || bytes.HasPrefix(data, []byte("PK\x03\x04")) {
			diagnostic(&p, "format_mismatch", "实际文件格式与扩展名不一致", 0, true)
			return
		}
		if !utf8.Valid(data) || badText(string(data)) {
			diagnostic(&p, "damaged_text", "正文编码无效或包含损坏字符；请导出为 UTF-8", 0, true)
			return
		}
		lines := strings.Split(strings.ReplaceAll(string(data), "\r\n", "\n"), "\n")
		heading := ""
		headingPath := []string{}
		fence := ""
		headingPattern := regexp.MustCompile(`^ {0,3}(#{1,6})[\t ]+(.+?)\s*#*\s*$`)
		var paragraph []string
		start := 1
		flush := func() {
			if len(paragraph) > 0 {
				p.Blocks = append(p.Blocks, Block{strings.Join(paragraph, "\n"), Locator{Line: start, LineEnd: start + len(paragraph) - 1, Heading: heading}})
				paragraph = nil
			}
		}
		for i, line := range lines {
			if ctx.Err() != nil {
				diagnostic(&p, "cancelled", "解析已取消", 0, true)
				return
			}
			if strings.TrimSpace(line) == "" {
				flush()
				continue
			}
			trimmed := strings.TrimSpace(line)
			if strings.HasPrefix(trimmed, "```") || strings.HasPrefix(trimmed, "~~~") {
				marker := trimmed[:3]
				if fence == "" {
					fence = marker
				} else if fence == marker {
					fence = ""
				}
			}
			if h := headingPattern.FindStringSubmatch(line); len(h) > 0 && p.Format != "txt" && fence == "" {
				flush()
				headingPath, heading = appendHeading(headingPath, len(h[1]), h[2])
			}
			if len(paragraph) == 0 {
				start = i + 1
			}
			paragraph = append(paragraph, line)
		}
		flush()
		if p.Format != "txt" && regexp.MustCompile(`(?i)!\[|<(img|video|audio|iframe|object|svg|canvas)(\s|>)`).Match(data) {
			diagnostic(&p, "images_not_extracted", "Markdown 图片内容未提取，请确认图片不包含必要正文", 0, true)
		}
	}
	total := 0
	for _, b := range p.Blocks {
		total += len(b.Content)
		if badText(b.Content) {
			diagnostic(&p, "damaged_text", "提取正文包含损坏字符", b.Locator.Page, true)
		}
	}
	if total > MaxText {
		diagnostic(&p, "text_too_large", "提取文本超过 2 MiB 上限", 0, true)
	}
	if total == 0 {
		diagnostic(&p, "empty_text", "没有可可靠提取的正文；扫描件 OCR 暂不支持", 0, true)
	}
	return
}
func badText(s string) bool {
	for _, r := range s {
		if r == utf8.RuneError || r == 0 || (unicode.IsControl(r) && r != '\n' && r != '\r' && r != '\t') {
			return true
		}
	}
	return false
}
func parsePDF(ctx context.Context, data []byte, p *Preview) {
	if !bytes.HasPrefix(data, []byte("%PDF-")) {
		diagnostic(p, "invalid_pdf", "实际内容不是 PDF", 0, true)
		return
	}
	d, e := pdf.NewReader(bytes.NewReader(data), int64(len(data)))
	if e != nil {
		diagnostic(p, "invalid_pdf", "PDF 损坏、加密或无法解析", 0, true)
		return
	}
	if d.NumPage() > 500 {
		diagnostic(p, "too_many_pages", "PDF 超过 500 页上限", 0, true)
		return
	}
	for i := 1; i <= d.NumPage(); i++ {
		if ctx.Err() != nil {
			diagnostic(p, "cancelled", "解析已取消", i, true)
			return
		}
		page := d.Page(i)
		if page.V.IsNull() {
			diagnostic(p, "missing_page", "页面缺失", i, true)
			continue
		}
		s, e := page.GetPlainText(nil)
		if e != nil {
			diagnostic(p, "page_failed", "页面文字提取失败", i, true)
			continue
		}
		s = strings.TrimSpace(s)
		if s == "" {
			diagnostic(p, "page_without_text", "该页无可靠文字，可能是扫描件或空白页；需提供可提取文字的版本", i, true)
			continue
		}
		// PlainText follows drawing operator order, which is not reading order.
		// Reconstruct simple text rows from glyph coordinates, rejecting layouts
		// where column/table gaps cannot be distinguished reliably.
		glyphs := page.Content().Text
		sort.SliceStable(glyphs, func(a, b int) bool {
			ya, yb := math.Round(glyphs[a].Y/2), math.Round(glyphs[b].Y/2)
			if ya == yb {
				return glyphs[a].X < glyphs[b].X
			}
			return ya > yb
		})
		var ordered strings.Builder
		for j, g := range glyphs {
			if j > 0 {
				prev := glyphs[j-1]
				if math.Round(g.Y/2) != math.Round(prev.Y/2) {
					ordered.WriteString("\n")
				} else if gap := g.X - (prev.X + prev.W); gap > math.Max(3*g.FontSize, 24) {
					diagnostic(p, "complex_pdf_layout", "检测到多栏或表格间隔，无法可靠保留阅读顺序，请提供 DOCX 或 Markdown", i, true)
					ordered.WriteString(" | ")
				} else if gap > g.FontSize*0.2 {
					ordered.WriteString(" ")
				}
			}
			ordered.WriteString(g.S)
		}
		if ordered.Len() == 0 {
			diagnostic(p, "missing_glyph_mapping", "无法校验页面文字位置", i, true)
			continue
		}
		s = ordered.String()
		p.Blocks = append(p.Blocks, Block{s, Locator{Page: i}})
		if !page.V.Key("Resources").Key("XObject").IsNull() {
			diagnostic(p, "pdf_objects", "该页含图片或嵌入对象，无法确认完整正文", i, true)
		}
	}
	diagnostic(p, "pdf_reading_order", "PDF 按页抽取；多栏、表格阅读顺序须在预览中核对", 0, false)
}
func parseDOCX(ctx context.Context, data []byte, p *Preview) {
	z, e := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if e != nil {
		diagnostic(p, "invalid_docx", "实际内容不是有效 DOCX", 0, true)
		return
	}
	var body []byte
	packageType := false
	var expanded uint64
	for _, f := range z.File {
		expanded += f.UncompressedSize64
		if expanded > 20<<20 {
			diagnostic(p, "archive_limit", "DOCX 解压内容超过上限", 0, true)
			return
		}
		if f.Name == "[Content_Types].xml" {
			r, e := f.Open()
			if e != nil {
				diagnostic(p, "invalid_docx", "DOCX 包清单损坏", 0, true)
				return
			}
			manifest, e := io.ReadAll(io.LimitReader(r, 64<<10))
			r.Close()
			if e != nil {
				diagnostic(p, "invalid_docx", "DOCX 包清单无法读取", 0, true)
				return
			}
			decoder := xml.NewDecoder(bytes.NewReader(manifest))
			for {
				token, e := decoder.Token()
				if e == io.EOF {
					break
				}
				if e != nil {
					diagnostic(p, "invalid_docx", "DOCX 包清单 XML 损坏", 0, true)
					return
				}
				if start, ok := token.(xml.StartElement); ok && start.Name.Local == "Override" {
					part, kind := "", ""
					for _, a := range start.Attr {
						if a.Name.Local == "PartName" {
							part = a.Value
						}
						if a.Name.Local == "ContentType" {
							kind = a.Value
						}
					}
					if part == "/word/document.xml" && kind == "application/vnd.openxmlformats-officedocument.wordprocessingml.document.main+xml" {
						packageType = true
					}
				}
			}
		}
		if strings.HasPrefix(f.Name, "word/media/") || strings.HasPrefix(f.Name, "word/embeddings/") {
			diagnostic(p, "embedded_content", "DOCX 含图片或嵌入内容，无法确认完整正文", 0, true)
		}
		if strings.HasPrefix(f.Name, "word/header") || strings.HasPrefix(f.Name, "word/footer") {
			diagnostic(p, "header_footer_omitted", "页眉/页脚不纳入正文检索；正文段落定位不包含页眉/页脚", 0, false)
		}
		if f.Name == "word/document.xml" {
			r, e := f.Open()
			if e != nil {
				diagnostic(p, "invalid_docx", "无法读取 DOCX 正文", 0, true)
				return
			}
			body, e = io.ReadAll(io.LimitReader(r, MaxText+1))
			r.Close()
			if e != nil || len(body) > MaxText {
				diagnostic(p, "invalid_docx", "DOCX 正文损坏或过大", 0, true)
				return
			}
		}
	}
	if len(body) == 0 || !packageType {
		diagnostic(p, "invalid_docx", "DOCX 缺少正文", 0, true)
		return
	}
	dec := xml.NewDecoder(bytes.NewReader(body))
	var paragraph strings.Builder
	heading := ""
	headingPath := []string{}
	headingLevel := 1
	n := 0
	inP := false
	inText := false
	rootSeen := false
	isHeading := false
	inTable := false
	var cell []string
	tableStart := 0
	var row []string
	var rows []string
	for {
		if ctx.Err() != nil {
			diagnostic(p, "cancelled", "解析已取消", 0, true)
			return
		}
		token, e := dec.Token()
		if e == io.EOF {
			break
		}
		if e != nil {
			diagnostic(p, "invalid_xml", "DOCX 正文 XML 损坏", 0, true)
			return
		}
		switch t := token.(type) {
		case xml.StartElement:
			if !rootSeen {
				rootSeen = true
				if t.Name.Local != "document" || t.Name.Space != "http://schemas.openxmlformats.org/wordprocessingml/2006/main" {
					diagnostic(p, "invalid_docx", "DOCX 正文根节点或命名空间无效", 0, true)
					return
				}
			}
			switch t.Name.Local {
			case "tbl":
				if inTable {
					diagnostic(p, "nested_table", "嵌套表格暂不支持可靠还原", 0, true)
				}
				inTable = true
				tableStart = n + 1
				rows = nil
			case "tc":
				cell = nil
			case "vMerge", "gridSpan":
				diagnostic(p, "merged_table_cells", "合并单元格暂不支持可靠还原，请提供不合并的说明性表格", 0, true)
			case "tr":
				row = nil
			case "p":
				inP = true
				n++
				paragraph.Reset()
				isHeading = false
			case "pStyle":
				for _, a := range t.Attr {
					if a.Name.Local == "val" && strings.HasPrefix(strings.ToLower(a.Value), "heading") {
						isHeading = true
						headingLevel, _ = strconv.Atoi(strings.TrimSpace(strings.TrimPrefix(strings.ToLower(a.Value), "heading")))
					}
				}
			case "t":
				inText = true
			case "tab":
				paragraph.WriteString("\t")
			case "br":
				paragraph.WriteString("\n")
			case "altChunk", "object", "drawing", "footnoteReference", "endnoteReference":
				diagnostic(p, "unsupported_docx_content", "DOCX 含未支持的嵌入正文或注释引用", 0, true)
			case "del", "ins", "moveFrom", "moveTo":
				diagnostic(p, "unresolved_revisions", "DOCX 含未接受的修订，请先接受或拒绝修订后上传", 0, true)
			}
		case xml.CharData:
			if inP && inText {
				paragraph.Write([]byte(t))
			}
		case xml.EndElement:
			switch t.Name.Local {
			case "t":
				inText = false
			case "p":
				inP = false
				s := strings.TrimSpace(paragraph.String())
				if isHeading {
					headingPath, heading = appendHeading(headingPath, headingLevel, s)
				}
				if s != "" {
					if inTable {
						cell = append(cell, s)
					} else {
						p.Blocks = append(p.Blocks, Block{s, Locator{Paragraph: n, ParagraphEnd: n, Heading: heading}})
					}
				}
			case "tr":
				rows = append(rows, strings.Join(row, " | "))
			case "tc":
				row = append(row, strings.Join(cell, " / "))
			case "tbl":
				inTable = false
				if len(rows) > 0 {
					p.Blocks = append(p.Blocks, Block{strings.Join(rows, "\n"), Locator{Paragraph: tableStart, ParagraphEnd: n, Heading: heading}})
				}
			}
		}
	}
}

func appendHeading(path []string, level int, title string) ([]string, string) {
	if level < 1 || level > 9 {
		level = 1
	}
	for len(path) < level {
		path = append(path, "")
	}
	path = path[:level]
	path[level-1] = strings.TrimSpace(title)
	nonempty := []string{}
	for _, s := range path {
		if s != "" {
			nonempty = append(nonempty, s)
		}
	}
	return path, strings.Join(nonempty, " / ")
}

// Split groups adjacent blocks in the same section to retain conditions and
// table context, while preserving their exact line/paragraph range. PDF pages
// never merge. Oversized paragraphs use overlap and retain a part ordinal.
func Split(p Preview) ([]Block, error) {
	if !p.Valid() {
		return nil, ErrInvalid
	}
	out := []Block{}
	groups := []Block{}
	for _, b := range p.Blocks {
		n := len(groups)
		if n > 0 && b.Locator.Page == 0 && groups[n-1].Locator.Page == 0 && b.Locator.Heading == groups[n-1].Locator.Heading && len([]rune(groups[n-1].Content))+len([]rune(b.Content))+2 <= 1200 {
			groups[n-1].Content += "\n\n" + b.Content
			groups[n-1].Locator.LineEnd = b.Locator.LineEnd
			groups[n-1].Locator.ParagraphEnd = b.Locator.ParagraphEnd
		} else {
			groups = append(groups, b)
		}
	}
	for _, b := range groups {
		r := []rune(b.Content)
		if len(r) > 1200 && strings.Contains(b.Content, "|") {
			return nil, fmt.Errorf("table_too_large")
		}
		for part := 1; len(r) > 0; part++ {
			end := len(r)
			if end > 1200 {
				end = 1200
			}
			loc := b.Locator
			loc.Part = part
			out = append(out, Block{string(r[:end]), loc})
			if end == len(r) {
				break
			}
			r = r[end-120:]
		}
	}
	if len(out) > 3000 {
		return nil, ErrInvalid
	}
	return out, nil
}
