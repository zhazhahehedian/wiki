package skillbuilder

import (
	"bytes"
	"errors"
	"strings"

	"github.com/google/uuid"
	"github.com/zenith-wang/it-wiki/backend/internal/registry"
	"gopkg.in/yaml.v3"
)

var ErrTemplateUnsupported = errors.New("skill cannot be represented by the text builder without losing content")

// Template copies all files or fails. It must never silently strip scripts,
// binary files, extra frontmatter or oversized attachments from a publication.
func Template(detail registry.Detail, files []registry.File) (Draft, error) {
	if detail.Type != "skill" || registry.ValidateSkillPublication(detail.Slug, files) != nil {
		return Draft{}, ErrTemplateUnsupported
	}
	base := strings.TrimRight(detail.Slug[:min(len(detail.Slug), 50)], "-")
	d := Draft{Slug: base + "-copy-" + uuid.NewString()[:8], Name: detail.Name, Files: []File{}}
	for _, f := range files {
		if f.Name != "SKILL.md" {
			d.Files = append(d.Files, File{Path: f.Name, Content: string(f.Data)})
			continue
		}
		raw := bytes.ReplaceAll(f.Data, []byte("\r\n"), []byte("\n"))
		end := bytes.Index(raw[4:], []byte("\n---\n")) + 4
		var meta map[string]any
		if yaml.Unmarshal(raw[4:end], &meta) != nil || len(meta) != 2 {
			return Draft{}, ErrTemplateUnsupported
		}
		d.Description, _ = meta["description"].(string)
		d.Instructions = string(raw[end+5:])
	}
	if _, err := Files(d); err != nil {
		return Draft{}, ErrTemplateUnsupported
	}
	return d, nil
}
