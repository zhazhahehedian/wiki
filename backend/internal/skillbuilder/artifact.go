package skillbuilder

import (
	"bytes"
	"net/url"
	"path"
	"regexp"
	"strings"
	"unicode/utf8"

	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/text"
	"github.com/zenith-wang/it-wiki/backend/internal/registry"
	"gopkg.in/yaml.v3"
)

var slugPattern = regexp.MustCompile(`^[a-z0-9]+(-[a-z0-9]+)*$`)

// Files derives the frontmatter from validated metadata. Generated output and
// user edits share this validation at both preview and save boundaries.
func Files(d Draft) ([]registry.File, error) {
	if !slugPattern.MatchString(d.Slug) || len(d.Slug) > 64 || d.Slug == "new" || d.Slug == "create-skill" {
		return nil, invalid("标识需为 1–64 个小写字母、数字和单连字符，不能使用保留名称")
	}
	if strings.TrimSpace(d.Name) == "" || utf8.RuneCountInString(d.Name) > 120 || strings.TrimSpace(d.Description) == "" || utf8.RuneCountInString(d.Description) > 1024 {
		return nil, invalid("请填写名称与用途描述；名称最多 120 字，描述最多 1024 字")
	}
	if !plainText(d.Name) || !plainText(d.Description) || !plainText(d.Instructions) || strings.TrimSpace(d.Instructions) == "" || len(d.Instructions) > 64*1024 || len(d.Files) > 10 {
		return nil, invalid("正文需为非空 UTF-8 文本，最多 64 KiB，附件最多 10 个")
	}
	if strings.HasPrefix(strings.TrimSpace(d.Instructions), "---") {
		return nil, invalid("正文不应包含 YAML frontmatter；名称和描述将由平台自动生成")
	}
	meta, e := yaml.Marshal(struct {
		Name        string `yaml:"name"`
		Description string `yaml:"description"`
	}{d.Slug, d.Description})
	if e != nil {
		return nil, e
	}
	skill := append([]byte("---\n"), meta...)
	skill = append(skill, []byte("---\n\n"+d.Instructions+"\n")...)
	files := []registry.File{{Name: "SKILL.md", Data: skill}}
	known := map[string]bool{"SKILL.md": true}
	total := len(skill)
	for _, f := range d.Files {
		if !(strings.HasPrefix(f.Path, "references/") || strings.HasPrefix(f.Path, "assets/")) || !plainText(f.Content) || len(f.Content) > 32*1024 {
			return nil, invalid("附件需放在 references/ 或 assets/ 下，每个最多 32 KiB")
		}
		switch strings.ToLower(path.Ext(f.Path)) {
		case ".md", ".txt", ".json", ".yaml", ".yml":
		default:
			return nil, invalid("附件仅支持 Markdown、TXT、JSON 或 YAML 文本")
		}
		total += len(f.Content)
		known[f.Path] = true
		files = append(files, registry.File{Name: f.Path, Data: []byte(f.Content)})
	}
	if total > MaxArtifactBytes {
		return nil, invalid("Skill 文件合计不能超过 128 KiB")
	}
	// Reuse the registry's portable-path and duplicate/conflict checks.
	if _, e = registry.PackSkill(d.Slug, files); e != nil {
		return nil, invalid("附件路径无效、重复或存在文件与目录冲突")
	}
	for _, f := range files {
		if strings.EqualFold(path.Ext(f.Name), ".md") {
			if e = checkLinks(f, known); e != nil {
				return nil, e
			}
		}
	}
	return files, nil
}
func plainText(s string) bool {
	if !utf8.ValidString(s) {
		return false
	}
	for _, c := range s {
		if c < 32 && c != '\n' && c != '\r' && c != '\t' {
			return false
		}
		if c == 127 {
			return false
		}
	}
	return true
}
func checkLinks(f registry.File, known map[string]bool) error {
	doc := goldmark.DefaultParser().Parse(text.NewReader(f.Data))
	return ast.Walk(doc, func(n ast.Node, entering bool) (ast.WalkStatus, error) {
		if !entering {
			return ast.WalkContinue, nil
		}
		var dest []byte
		isImage := false
		switch link := n.(type) {
		case *ast.Link:
			dest = link.Destination
		case *ast.Image:
			dest = link.Destination
			isImage = true
		default:
			return ast.WalkContinue, nil
		}
		raw := string(bytes.TrimSpace(dest))
		u, e := url.Parse(raw)
		if e != nil || strings.Contains(raw, "\\") {
			return ast.WalkStop, invalid("Markdown 文件链接格式无效")
		}
		if u.Scheme != "" {
			if (u.Scheme == "https" || u.Scheme == "http" || u.Scheme == "mailto") && !isImage {
				return ast.WalkContinue, nil
			}
			return ast.WalkStop, invalid("Markdown 仅支持网页/邮箱链接及包内文件，不支持外部图片或可执行链接")
		}
		if u.Host != "" || strings.HasPrefix(u.Path, "/") {
			return ast.WalkStop, invalid("附件链接必须使用包内相对路径")
		}
		if u.Path == "" {
			return ast.WalkContinue, nil
		}
		target := path.Clean(path.Join(path.Dir(f.Name), u.Path))
		if !known[target] {
			return ast.WalkStop, invalid("Markdown 引用了不存在的包内文件，请补充附件或修正链接")
		}
		return ast.WalkContinue, nil
	})
}
