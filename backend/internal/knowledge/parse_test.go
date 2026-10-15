package knowledge

import (
	"archive/zip"
	"bytes"
	"context"
	"fmt"
	"strings"
	"testing"
)

func docxFixture(t *testing.T, body string) []byte {
	t.Helper()
	var b bytes.Buffer
	z := zip.NewWriter(&b)
	manifest, _ := z.Create("[Content_Types].xml")
	_, _ = manifest.Write([]byte(`<Types xmlns="http://schemas.openxmlformats.org/package/2006/content-types"><Override PartName="/word/document.xml" ContentType="application/vnd.openxmlformats-officedocument.wordprocessingml.document.main+xml"/></Types>`))
	f, e := z.Create("word/document.xml")
	if e != nil {
		t.Fatal(e)
	}
	_, _ = f.Write([]byte(`<w:document xmlns:w="http://schemas.openxmlformats.org/wordprocessingml/2006/main"><w:body>` + body + `</w:body></w:document>`))
	if e = z.Close(); e != nil {
		t.Fatal(e)
	}
	return b.Bytes()
}
func TestStrictFormatsAndQuality(t *testing.T) {
	for _, tc := range []struct {
		name string
		body []byte
	}{{"a.csv", []byte("a,b")}, {"a.xlsx", []byte("PK")}, {"a.doc", []byte("legacy")}, {"a.txt", []byte{0xff, 0x00}}, {"a.txt", nil}, {"a.pdf", []byte("not pdf")}, {"a.docx", []byte("not zip")}, {"a.md", []byte("正文\n![制度](image.png)")}} {
		t.Run(tc.name+fmt.Sprint(len(tc.body)), func(t *testing.T) {
			p := Parse(context.Background(), tc.name, tc.body)
			if p.Valid() {
				t.Fatal("invalid content passed", p)
			}
		})
	}
}
func TestLocationsAndTableContext(t *testing.T) {
	p := Parse(context.Background(), "policy.md", []byte("# 报销\n\n需提供发票和审批单。\n\n|项目|限额（元）|\n|---|---|\n|住宿|500|"))
	if !p.Valid() || len(p.Blocks) != 3 || p.Blocks[1].Locator.Line != 3 || p.Blocks[2].Locator.Line != 5 {
		t.Fatal(p)
	}
	blocks, e := Split(p)
	if e != nil || !strings.Contains(blocks[0].Content, "限额（元）") || blocks[0].Locator.LineEnd != 7 {
		t.Fatal(blocks, e)
	}
	data := docxFixture(t, `<w:p><w:pPr><w:pStyle w:val="Heading1"/></w:pPr><w:r><w:t>差旅制度</w:t></w:r></w:p><w:p><w:r><w:t>提交审批单</w:t></w:r></w:p><w:tbl><w:tr><w:tc><w:p><w:r><w:t>项目</w:t></w:r></w:p></w:tc><w:tc><w:p><w:r><w:t>限额（元）</w:t></w:r></w:p></w:tc></w:tr><w:tr><w:tc><w:p><w:r><w:t>住宿</w:t></w:r></w:p></w:tc><w:tc><w:p><w:r><w:t>500</w:t></w:r></w:p></w:tc></w:tr></w:tbl>`)
	p = Parse(context.Background(), "policy.docx", data)
	if !p.Valid() || p.Blocks[1].Locator.Paragraph != 2 || p.Blocks[1].Locator.Page != 0 || p.Blocks[1].Locator.Heading != "差旅制度" || !strings.Contains(p.Blocks[2].Content, "项目 | 限额（元）\n住宿 | 500") {
		t.Fatal(p)
	}
}
func TestDOCXCorruptionAndEmbeddedContent(t *testing.T) {
	for _, body := range []string{`<w:p><w:t>unterminated`, `<w:p><w:r><w:t>文字</w:t><w:drawing/></w:r></w:p>`} {
		if p := Parse(context.Background(), "a.docx", docxFixture(t, body)); p.Valid() {
			t.Fatal(p)
		}
	}
}
func TestSplitPreservesAllSourceText(t *testing.T) {
	text := strings.Repeat("测试文本。", 700)
	p := Parse(context.Background(), "a.txt", []byte(text))
	blocks, e := Split(p)
	if e != nil || len(blocks) < 2 {
		t.Fatal(e)
	}
	var rebuilt []rune
	for i, b := range blocks {
		if len([]rune(b.Content)) > 1200 || b.Locator.Line != 1 || b.Locator.Part != i+1 {
			t.Fatal(b)
		}
		r := []rune(b.Content)
		if i > 0 {
			r = r[120:]
		}
		rebuilt = append(rebuilt, r...)
	}
	if string(rebuilt) != text {
		t.Fatal("source dropped")
	}
}
func TestSearchInputAndVectors(t *testing.T) {
	for _, in := range []SearchRequest{{Query: "a"}, {KBIDs: []string{"invalid"}, Query: "a"}, {KBIDs: []string{"00000000-0000-0000-0000-000000000001"}, Query: strings.Repeat("中", 4001)}} {
		if ValidateSearch(&in) == nil {
			t.Fatal(in)
		}
	}
	in := SearchRequest{KBIDs: []string{"00000000-0000-0000-0000-000000000001", "00000000-0000-0000-0000-000000000001"}, Query: " a "}
	if ValidateSearch(&in) != nil || len(in.KBIDs) != 1 || in.TopK != 5 {
		t.Fatal(in)
	}
	if validVector([]float32{0, 0}, 2) || validVector([]float32{1}, 2) {
		t.Fatal("invalid vectors accepted")
	}
}

func pdfFixture(streams ...string) []byte {
	objects := []string{"<< /Type /Catalog /Pages 2 0 R >>", "", "<< /Type /Font /Subtype /Type1 /BaseFont /Helvetica >>"}
	kids := []string{}
	for _, s := range streams {
		page := len(objects) + 1
		kids = append(kids, fmt.Sprintf("%d 0 R", page))
		objects = append(objects, fmt.Sprintf("<< /Type /Page /Parent 2 0 R /MediaBox [0 0 612 792] /Resources << /Font << /F1 3 0 R >> >> /Contents %d 0 R >>", page+1), fmt.Sprintf("<< /Length %d >>\nstream\n%s\nendstream", len(s), s))
	}
	objects[1] = fmt.Sprintf("<< /Type /Pages /Kids [%s] /Count %d >>", strings.Join(kids, " "), len(streams))
	var b bytes.Buffer
	b.WriteString("%PDF-1.4\n")
	offsets := []int{0}
	for i, s := range objects {
		offsets = append(offsets, b.Len())
		fmt.Fprintf(&b, "%d 0 obj\n%s\nendobj\n", i+1, s)
	}
	start := b.Len()
	fmt.Fprintf(&b, "xref\n0 %d\n0000000000 65535 f \n", len(objects)+1)
	for _, offset := range offsets[1:] {
		fmt.Fprintf(&b, "%010d 00000 n \n", offset)
	}
	fmt.Fprintf(&b, "trailer\n<< /Size %d /Root 1 0 R >>\nstartxref\n%d\n%%%%EOF", len(objects)+1, start)
	return b.Bytes()
}
func TestPDFPageMappingAndFailure(t *testing.T) {
	good := `BT /F1 12 Tf 50 700 Td (Travel reimbursement requires receipt.) Tj ET`
	p := Parse(context.Background(), "policy.pdf", pdfFixture(good, good))
	if !p.Valid() || len(p.Blocks) != 2 || p.Blocks[1].Locator.Page != 2 {
		t.Fatal(p)
	}
	for _, second := range []string{"", `BT /F1 Tf (broken) Tj ET`} {
		p = Parse(context.Background(), "broken.pdf", pdfFixture(good, second))
		if p.Valid() {
			t.Fatal("failed/empty page silently accepted", p)
		}
	}
}
