package skillbuilder

import (
	"strings"
	"testing"

	"github.com/zenith-wang/it-wiki/backend/internal/registry"
)

func TestTemplatePreservesTextAndAttachmentsAndCreatesNewIdentity(t *testing.T) {
	original := validDraft()
	files, err := Files(original)
	if err != nil {
		t.Fatal(err)
	}
	detail := registry.Detail{Capability: registry.Capability{Type: "skill", Slug: original.Slug, Name: original.Name}}
	copy, err := Template(detail, files)
	if err != nil {
		t.Fatal(err)
	}
	if copy.Slug == original.Slug || !strings.HasPrefix(copy.Slug, original.Slug+"-copy-") || strings.TrimSpace(copy.Instructions) != strings.TrimSpace(original.Instructions) || copy.Description != original.Description || len(copy.Files) != len(original.Files) {
		t.Fatalf("incomplete copy: %+v", copy)
	}
	for i, file := range original.Files {
		if copy.Files[i] != file {
			t.Fatal("attachment changed")
		}
	}
	if _, err := Files(copy); err != nil {
		t.Fatal("copied draft cannot be saved", err)
	}
	copy.Files[0].Content = "edited"
	if string(files[1].Data) == "edited" {
		t.Fatal("source mutated")
	}
	second, err := Template(detail, files)
	if err != nil || copy.Slug == second.Slug {
		t.Fatal("copies reused identity")
	}
}

func TestTemplateRejectsLossyConversions(t *testing.T) {
	for _, scenario := range []string{"script", "binary", "extra metadata", "oversize", "missing header", "duplicate", "mcp"} {
		t.Run(scenario, func(t *testing.T) {
			d := validDraft()
			files, _ := Files(d)
			detail := registry.Detail{Capability: registry.Capability{Type: "skill", Slug: d.Slug, Name: d.Name}}
			switch scenario {
			case "script":
				files = append(files, registry.File{Name: "scripts/run.py", Data: []byte("print('hello')")})
			case "binary":
				files = append(files, registry.File{Name: "assets/image.txt", Data: []byte{0, 255}})
			case "extra metadata":
				files[0].Data = []byte(strings.Replace(string(files[0].Data), "---\n", "---\nallowed-tools: Read\n", 1))
			case "oversize":
				files = append(files, registry.File{Name: "references/large.txt", Data: []byte(strings.Repeat("x", 32*1024+1))})
			case "missing header":
				files[0].Data = []byte("# Instructions")
			case "duplicate":
				files = append(files, files[1])
			case "mcp":
				detail.Type = "mcp"
			}
			if _, err := Template(detail, files); err != ErrTemplateUnsupported {
				t.Fatalf("lossy conversion accepted: %v", err)
			}
		})
	}
}
