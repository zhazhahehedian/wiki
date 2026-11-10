package knowledge

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"math"
	"path"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"

	pdf "github.com/ledongthuc/pdf"
)

const (
	wordNS     = "http://schemas.openxmlformats.org/wordprocessingml/2006/main"
	relationNS = "http://schemas.openxmlformats.org/officeDocument/2006/relationships"
	compatNS   = "http://schemas.openxmlformats.org/markup-compatibility/2006"
	// Expanded DOCX parts are bounded to resist archive bombs; images make
	// legitimate packages larger than their text.
	maxDOCXExpanded = 64 << 20
	maxDOCXBodyXML  = 16 << 20
	maxImageBytes   = 10 << 20
	chunkRunes      = 1200
)

var (
	markdownHeading        = regexp.MustCompile(`^ {0,3}(#{1,6})[\t ]+(.+?)\s*#*\s*$`)
	markdownTableSeparator = regexp.MustCompile(`^\s*\|?\s*:?-{3,}:?\s*(\|\s*:?-{3,}:?\s*)*\|?\s*$`)
	markdownMedia          = regexp.MustCompile(`(?i)!\[|<(img|video|audio|iframe|object|svg|canvas)(\s|>)`)
	errPartTooLarge        = errors.New("part too large")
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
	return len(p.Blocks) > 0 && !p.blocked()
}
func (p Preview) blocked() bool {
	for _, d := range p.Diagnostics {
		if d.Blocking {
			return true
		}
	}
	return false
}

// Parse retains verified source locations. A failed page is never skipped as success.
// DOCX images become placeholder blocks that resolveImages fills or removes.
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
		var paragraph []string
		var fenced []bool
		start := 1
		flush := func() {
			if len(paragraph) > 0 {
				p.Blocks = append(p.Blocks, textBlocks(paragraph, fenced, start, heading, p.Format == "txt")...)
				paragraph, fenced = nil, nil
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
			marker := strings.HasPrefix(trimmed, "```") || strings.HasPrefix(trimmed, "~~~")
			if marker {
				if fence == "" {
					fence = trimmed[:3]
				} else if fence == trimmed[:3] {
					fence = ""
				}
			}
			if h := markdownHeading.FindStringSubmatch(line); len(h) > 0 && p.Format != "txt" && fence == "" {
				flush()
				headingPath, heading = appendHeading(headingPath, len(h[1]), h[2])
			}
			if len(paragraph) == 0 {
				start = i + 1
			}
			paragraph = append(paragraph, line)
			fenced = append(fenced, marker || fence != "")
		}
		flush()
		if p.Format != "txt" && markdownMedia.Match(data) {
			diagnostic(&p, "images_not_extracted", "Markdown 引用的图片未下载识别，图片内容未纳入检索；关键信息请写入正文", 0, false)
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

// textBlocks turns one blank-line separated paragraph into blocks, separating
// Markdown pipe tables (header line plus delimiter row) from surrounding text.
func textBlocks(lines []string, fenced []bool, start int, heading string, plain bool) []Block {
	out := []Block{}
	text := func(from, to int) {
		if from < to {
			out = append(out, Block{Content: strings.Join(lines[from:to], "\n"), Locator: Locator{Line: start + from, LineEnd: start + to - 1, Heading: heading}})
		}
	}
	from := 0
	for i := 0; !plain && i+1 < len(lines); {
		if fenced[i] || fenced[i+1] || !strings.Contains(lines[i], "|") || !strings.Contains(lines[i+1], "|") || !markdownTableSeparator.MatchString(lines[i+1]) {
			i++
			continue
		}
		text(from, i)
		end := i + 2
		for end < len(lines) && !fenced[end] && strings.Contains(lines[end], "|") {
			end++
		}
		b := Block{Content: strings.Join(lines[i:end], "\n"), Locator: Locator{Line: start + i, LineEnd: start + end - 1, Heading: heading}}
		if end > i+2 {
			b.table = &table{header: lines[i : i+2]}
			for j := i + 2; j < end; j++ {
				b.table.rows = append(b.table.rows, tableRow{lines[j], start + j, start + j})
			}
		}
		out = append(out, b)
		from, i = end, end
	}
	text(from, len(lines))
	return out
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
		p.Blocks = append(p.Blocks, Block{Content: s, Locator: Locator{Page: i}})
		if !page.V.Key("Resources").Key("XObject").IsNull() {
			diagnostic(p, "pdf_objects", "该页含图片或嵌入对象，无法确认完整正文", i, true)
		}
	}
	diagnostic(p, "pdf_reading_order", "PDF 按页抽取；多栏、表格阅读顺序须在预览中核对", 0, false)
}

func readZipFile(f *zip.File, limit int64) ([]byte, error) {
	r, e := f.Open()
	if e != nil {
		return nil, e
	}
	defer r.Close()
	b, e := io.ReadAll(io.LimitReader(r, limit+1))
	if e != nil {
		return nil, e
	}
	if int64(len(b)) > limit {
		return nil, errPartTooLarge
	}
	return b, nil
}

type docxRelation struct {
	target   string
	external bool
}

func docxRelations(raw []byte) (map[string]docxRelation, error) {
	out := map[string]docxRelation{}
	dec := xml.NewDecoder(bytes.NewReader(raw))
	for len(raw) > 0 {
		token, e := dec.Token()
		if e == io.EOF {
			break
		}
		if e != nil {
			return nil, e
		}
		start, ok := token.(xml.StartElement)
		if !ok || start.Name.Local != "Relationship" {
			continue
		}
		id, rel := "", docxRelation{}
		for _, a := range start.Attr {
			switch a.Name.Local {
			case "Id":
				id = a.Value
			case "Target":
				rel.target = a.Value
			case "TargetMode":
				rel.external = strings.EqualFold(a.Value, "External")
			}
		}
		if !rel.external {
			if strings.HasPrefix(rel.target, "/") {
				rel.target = path.Clean(strings.TrimPrefix(rel.target, "/"))
			} else {
				rel.target = path.Join("word", rel.target)
			}
		}
		out[id] = rel
	}
	return out, nil
}

func attr(start xml.StartElement, space, local string) (string, bool) {
	for _, a := range start.Attr {
		if a.Name.Local == local && (space == "" || a.Name.Space == space) {
			return a.Value, true
		}
	}
	return "", false
}

func joinIndexes(values []int) string {
	parts := []string{}
	for i, v := range values {
		if i == 10 {
			return strings.Join(parts, "、") + fmt.Sprintf(" 等 %d 处", len(values))
		}
		parts = append(parts, strconv.Itoa(v))
	}
	return strings.Join(parts, "、")
}

func parseDOCX(ctx context.Context, data []byte, p *Preview) {
	z, e := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if e != nil {
		diagnostic(p, "invalid_docx", "实际内容不是有效 DOCX", 0, true)
		return
	}
	var body, rawRelations []byte
	files := map[string]*zip.File{}
	packageType, headerFooter := false, false
	var expanded uint64
	for _, f := range z.File {
		expanded += f.UncompressedSize64
		if expanded > maxDOCXExpanded {
			diagnostic(p, "archive_limit", "DOCX 解压内容超过上限", 0, true)
			return
		}
		files[f.Name] = f
		if f.Name == "[Content_Types].xml" {
			manifest, e := readZipFile(f, 64<<10)
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
					part, _ := attr(start, "", "PartName")
					kind, _ := attr(start, "", "ContentType")
					if part == "/word/document.xml" && kind == "application/vnd.openxmlformats-officedocument.wordprocessingml.document.main+xml" {
						packageType = true
					}
				}
			}
		}
		if !headerFooter && (strings.HasPrefix(f.Name, "word/header") || strings.HasPrefix(f.Name, "word/footer")) {
			headerFooter = true
			diagnostic(p, "header_footer_omitted", "页眉/页脚不纳入正文检索；正文段落定位不包含页眉/页脚", 0, false)
		}
		if f.Name == "word/document.xml" {
			if body, e = readZipFile(f, maxDOCXBodyXML); e != nil {
				diagnostic(p, "invalid_docx", "DOCX 正文损坏或过大", 0, true)
				return
			}
		}
		if f.Name == "word/_rels/document.xml.rels" {
			if rawRelations, e = readZipFile(f, 1<<20); e != nil {
				diagnostic(p, "invalid_docx", "DOCX 关系清单损坏或过大", 0, true)
				return
			}
		}
	}
	if len(body) == 0 || !packageType {
		diagnostic(p, "invalid_docx", "DOCX 缺少正文", 0, true)
		return
	}
	relations, e := docxRelations(rawRelations)
	if e != nil {
		diagnostic(p, "invalid_docx", "DOCX 关系清单 XML 损坏", 0, true)
		return
	}
	type media struct {
		data    []byte
		problem string
	}
	loaded := map[string]media{}
	dec := xml.NewDecoder(bytes.NewReader(body))
	var paragraph strings.Builder
	heading := ""
	headingPath := []string{}
	headingLevel := 1
	n := 0
	inP, inText, rootSeen, isHeading := false, false, false, false
	// Table state. Merged cells are expanded so each row keeps its columns.
	tableDepth, tableStart, rowStart, headerRows, column, span := 0, 0, 0, 0, 0, 1
	rowIsHeader := false
	merge := ""
	above := map[int]string{}
	var cell, row []string
	var rows []tableRow
	// Drawing state. Images wait for their paragraph (or table) to be emitted.
	drawDepth, textboxDepth := 0, 0
	var drawRelations []string
	var textbox strings.Builder
	drawAlt, drawChart, drawDiagram := "", false, false
	var pending []Block
	var charts, diagrams []int
	flushPending := func() {
		for _, b := range pending {
			b.Locator.Heading = heading
			p.Blocks = append(p.Blocks, b)
		}
		pending = nil
	}
	addImage := func(id string) {
		img := docImage{index: len(p.images) + 1, paragraph: n, alt: strings.TrimSpace(drawAlt)}
		rel, ok := relations[id]
		switch {
		case !ok:
			img.problem = "图片引用缺失"
		case rel.external:
			img.problem = "外部链接图片未下载"
		default:
			m, seen := loaded[rel.target]
			if !seen {
				if f := files[rel.target]; f == nil {
					m.problem = "图片文件缺失"
				} else if m.data, e = readZipFile(f, maxImageBytes); e != nil {
					m.data, m.problem = nil, "图片超过 10 MiB 或无法读取"
				}
				loaded[rel.target] = m
			}
			img.key, img.data, img.problem = rel.target, m.data, m.problem
		}
		p.images = append(p.images, img)
		pending = append(pending, Block{Content: fmt.Sprintf("[图片 %d]", img.index), Locator: Locator{Paragraph: n, ParagraphEnd: n}, image: img.index})
		if tableDepth > 0 && inP {
			fmt.Fprintf(&paragraph, " [图片 %d]", img.index)
		}
	}
	finishDrawing := func() {
		if s := strings.TrimSpace(textbox.String()); s != "" && inP {
			if paragraph.Len() > 0 {
				paragraph.WriteString("\n")
			}
			paragraph.WriteString(s)
		}
		for _, id := range drawRelations {
			addImage(id)
		}
		if len(drawRelations) == 0 && strings.TrimSpace(textbox.String()) == "" {
			if drawChart {
				charts = append(charts, n)
			} else if drawDiagram {
				diagrams = append(diagrams, n)
			}
		}
	}
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
				if t.Name.Local != "document" || t.Name.Space != wordNS {
					diagnostic(p, "invalid_docx", "DOCX 正文根节点或命名空间无效", 0, true)
					return
				}
			}
			// Word writes legacy duplicates of modern drawings as fallbacks.
			if t.Name.Local == "Fallback" && t.Name.Space == compatNS {
				if e := dec.Skip(); e != nil {
					diagnostic(p, "invalid_xml", "DOCX 正文 XML 损坏", 0, true)
					return
				}
				continue
			}
			if drawDepth > 0 {
				drawDepth++
				switch t.Name.Local {
				case "blip", "imagedata":
					local := "embed"
					if t.Name.Local == "imagedata" {
						local = "id"
					}
					if id, ok := attr(t, relationNS, local); ok && id != "" {
						drawRelations = append(drawRelations, id)
					}
				case "docPr":
					if v, _ := attr(t, "", "descr"); v != "" {
						drawAlt = v
					} else if v, _ := attr(t, "", "title"); v != "" && drawAlt == "" {
						drawAlt = v
					}
				case "chart":
					drawChart = true
				case "relIds":
					drawDiagram = true
				case "txbxContent":
					textboxDepth = drawDepth
				case "t":
					inText = true
				case "tab":
					if textboxDepth > 0 {
						textbox.WriteString("\t")
					}
				case "br":
					if textboxDepth > 0 {
						textbox.WriteString("\n")
					}
				case "del", "ins", "moveFrom", "moveTo":
					diagnostic(p, "unresolved_revisions", "DOCX 含未接受的修订，请先接受或拒绝修订后上传", 0, true)
				}
				continue
			}
			if t.Name.Space == wordNS && (t.Name.Local == "drawing" || t.Name.Local == "pict" || t.Name.Local == "object") {
				drawDepth, textboxDepth = 1, 0
				drawRelations, drawAlt, drawChart, drawDiagram = nil, "", false, false
				textbox.Reset()
				continue
			}
			switch t.Name.Local {
			case "tbl":
				if tableDepth > 0 {
					diagnostic(p, "nested_table", "嵌套表格暂不支持可靠还原", 0, true)
				} else {
					tableStart, headerRows, rows = n+1, 0, nil
					above = map[int]string{}
				}
				tableDepth++
			case "tr":
				if tableDepth == 1 {
					row, column, rowStart, rowIsHeader = nil, 0, n+1, false
				}
			case "tblHeader":
				v, _ := attr(t, wordNS, "val")
				rowIsHeader = v != "false" && v != "0" && v != "off"
			case "gridBefore":
				v, _ := attr(t, wordNS, "val")
				skip, _ := strconv.Atoi(v)
				for i := 0; tableDepth == 1 && i < min(skip, 64); i++ {
					row = append(row, "")
					column++
				}
			case "tc":
				if tableDepth == 1 {
					cell, span, merge = nil, 1, ""
				}
			case "gridSpan":
				if tableDepth == 1 {
					v, _ := attr(t, wordNS, "val")
					if s, e := strconv.Atoi(v); e == nil && s > 1 {
						span = min(s, 64)
					}
				}
			case "vMerge":
				if tableDepth == 1 {
					merge, _ = attr(t, wordNS, "val")
					if merge != "restart" {
						merge = "continue"
					}
				}
			case "p":
				inP = true
				n++
				paragraph.Reset()
				isHeading = false
			case "pStyle":
				if v, ok := attr(t, "", "val"); ok && strings.HasPrefix(strings.ToLower(v), "heading") {
					isHeading = true
					headingLevel, _ = strconv.Atoi(strings.TrimSpace(strings.TrimPrefix(strings.ToLower(v), "heading")))
				}
			case "t":
				inText = true
			case "tab":
				paragraph.WriteString("\t")
			case "br":
				paragraph.WriteString("\n")
			case "altChunk", "footnoteReference", "endnoteReference":
				diagnostic(p, "unsupported_docx_content", "DOCX 含未支持的嵌入正文或注释引用", 0, true)
			case "del", "ins", "moveFrom", "moveTo":
				diagnostic(p, "unresolved_revisions", "DOCX 含未接受的修订，请先接受或拒绝修订后上传", 0, true)
			}
		case xml.CharData:
			if drawDepth > 0 {
				if textboxDepth > 0 && inText {
					textbox.Write(t)
				}
			} else if inP && inText {
				paragraph.Write([]byte(t))
			}
		case xml.EndElement:
			if drawDepth > 0 {
				drawDepth--
				switch t.Name.Local {
				case "t":
					inText = false
				case "p":
					if textboxDepth > 0 {
						textbox.WriteString("\n")
					}
				case "txbxContent":
					if drawDepth < textboxDepth {
						textboxDepth = 0
					}
				}
				if drawDepth == 0 {
					finishDrawing()
				}
				continue
			}
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
					if tableDepth > 0 {
						cell = append(cell, s)
					} else {
						p.Blocks = append(p.Blocks, Block{Content: s, Locator: Locator{Paragraph: n, ParagraphEnd: n, Heading: heading}})
					}
				}
				if tableDepth == 0 {
					flushPending()
				}
			case "tc":
				if tableDepth == 1 {
					text := strings.Join(cell, " / ")
					if merge == "continue" {
						text = above[column]
					}
					for i := 0; i < span; i++ {
						row = append(row, text)
						if merge == "" {
							delete(above, column+i)
						} else {
							above[column+i] = text
						}
					}
					column += span
				}
			case "tr":
				if tableDepth == 1 {
					rows = append(rows, tableRow{strings.Join(row, " | "), rowStart, n})
					if rowIsHeader && headerRows == len(rows)-1 {
						headerRows++
					}
				}
			case "tbl":
				tableDepth--
				if tableDepth == 0 {
					if len(rows) > 0 {
						texts := make([]string, len(rows))
						for i, r := range rows {
							texts[i] = r.text
						}
						b := Block{Content: strings.Join(texts, "\n"), Locator: Locator{Paragraph: tableStart, ParagraphEnd: n, Heading: heading}}
						if h := max(headerRows, 1); len(rows) > h {
							b.table = &table{header: texts[:h], rows: rows[h:]}
						}
						p.Blocks = append(p.Blocks, b)
					}
					flushPending()
				}
			}
		}
	}
	flushPending()
	if len(charts) > 0 {
		diagnostic(p, "chart_not_extracted", "第 "+joinIndexes(charts)+" 段含 Word 图表，图表数据未提取，未纳入检索", 0, false)
	}
	if len(diagrams) > 0 {
		diagnostic(p, "diagram_not_extracted", "第 "+joinIndexes(diagrams)+" 段含 SmartArt 图形，内容未提取，未纳入检索", 0, false)
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
// never merge. Oversized paragraphs use overlap and retain a part ordinal;
// oversized tables split between rows and repeat their header in every part.
func Split(p Preview) ([]Block, error) {
	return splitWithInputLimit(p, 0)
}

func splitWithInputLimit(p Preview, maxInputBytes int) ([]Block, error) {
	if !p.Valid() {
		return nil, ErrInvalid
	}
	out := []Block{}
	groups := []Block{}
	for _, b := range p.Blocks {
		n := len(groups)
		fitsBytes := n > 0 && (maxInputBytes <= 0 || len(b.Locator.Heading)+1+len(groups[n-1].Content)+2+len(b.Content) <= maxInputBytes)
		if fitsBytes && b.Locator.Page == 0 && groups[n-1].Locator.Page == 0 && b.Locator.Heading == groups[n-1].Locator.Heading && len([]rune(groups[n-1].Content))+len([]rune(b.Content))+2 <= chunkRunes {
			groups[n-1].Content += "\n\n" + b.Content
			groups[n-1].Locator.LineEnd = b.Locator.LineEnd
			groups[n-1].Locator.ParagraphEnd = b.Locator.ParagraphEnd
			groups[n-1].table = nil
		} else {
			groups = append(groups, b)
		}
	}
	for _, b := range groups {
		budget := 0
		if maxInputBytes > 0 {
			if budget = maxInputBytes - len(b.Locator.Heading) - 1; budget < utf8.UTFMax {
				return nil, fmt.Errorf("heading_too_large")
			}
		}
		if b.table != nil && !fits(b.Content, chunkRunes, budget) {
			parts, e := splitTable(b, budget)
			if e != nil {
				return nil, e
			}
			out = append(out, parts...)
			continue
		}
		for i, piece := range splitText(b.Content, chunkRunes, budget) {
			loc := b.Locator
			loc.Part = i + 1
			out = append(out, Block{Content: piece, Locator: loc})
		}
	}
	if len(out) > 3000 {
		return nil, ErrInvalid
	}
	return out, nil
}

func fits(s string, maxRunes, maxBytes int) bool {
	return utf8.RuneCountInString(s) <= maxRunes && (maxBytes <= 0 || len(s) <= maxBytes)
}

// splitText cuts text within rune and byte limits; consecutive pieces overlap.
// A cut prefers the last line break in the second half of the piece, and its
// overlap then repeats only whole lines, so pieces never start mid-line.
func splitText(s string, maxRunes, maxBytes int) []string {
	r := []rune(s)
	out := []string{}
	for len(r) > 0 {
		end := min(len(r), maxRunes)
		if maxBytes > 0 {
			used := 0
			for i, char := range r[:end] {
				if used += utf8.RuneLen(char); used > maxBytes {
					end = i
					break
				}
			}
		}
		if end == len(r) {
			out = append(out, string(r))
			break
		}
		overlap := min(120, end/10)
		for i := end - 1; i >= end/2 && i > 0; i-- {
			if r[i] == '\n' {
				end = i + 1
				overlap = lineOverlap(r[:end], min(120, end/10))
				break
			}
		}
		out = append(out, string(r[:end]))
		r = r[end-overlap:]
	}
	return out
}

// lineOverlap returns the length of the longest run of trailing whole lines of
// piece (which ends with a line break) that fits within limit runes.
func lineOverlap(piece []rune, limit int) int {
	end := len(piece)
	for j := end - limit; j < end; j++ {
		if j > 0 && piece[j-1] == '\n' {
			return end - j
		}
	}
	return 0
}

// splitTable packs whole rows under the repeated header. A single row that
// still does not fit is cut like text, each piece keeping the header.
func splitTable(b Block, maxBytes int) ([]Block, error) {
	header := strings.Join(b.table.header, "\n") + "\n"
	rowRunes, rowBytes := chunkRunes-utf8.RuneCountInString(header), 0
	if maxBytes > 0 {
		rowBytes = maxBytes - len(header)
	}
	if rowRunes < 16 || (maxBytes > 0 && rowBytes < 16*utf8.UTFMax) {
		return nil, fmt.Errorf("table_too_large")
	}
	out := []Block{}
	emit := func(content string, first, last tableRow) {
		loc := b.Locator
		if loc.Line > 0 {
			loc.Line, loc.LineEnd = first.start, last.end
		} else {
			loc.Paragraph, loc.ParagraphEnd = first.start, last.end
		}
		loc.Part = len(out) + 1
		out = append(out, Block{Content: header + content, Locator: loc})
	}
	var current []tableRow
	text := ""
	flush := func() {
		if len(current) > 0 {
			emit(text, current[0], current[len(current)-1])
			current, text = nil, ""
		}
	}
	for _, r := range b.table.rows {
		if len(current) > 0 && fits(text+"\n"+r.text, rowRunes, rowBytes) {
			current, text = append(current, r), text+"\n"+r.text
			continue
		}
		flush()
		if fits(r.text, rowRunes, rowBytes) {
			current, text = []tableRow{r}, r.text
			continue
		}
		for _, piece := range splitText(r.text, rowRunes, rowBytes) {
			emit(piece, r, r)
		}
	}
	flush()
	return out, nil
}
