package registry

import (
	"archive/zip"
	"bytes"
	"context"
	"io"
	"net/url"
	"path"
	"strings"
	"unicode/utf8"

	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/text"
	"gopkg.in/yaml.v3"
)

type ReviewFile struct {
	Name    string  `json:"name"`
	Size    int     `json:"size"`
	Content *string `json:"content,omitempty"`
}

func (s *Service) Files(ctx context.Context, user, slug, versionID string) ([]ReviewFile, error) {
	d, e := s.Get(ctx, user, slug)
	if e != nil {
		return nil, e
	}
	for _, v := range d.Versions {
		if v.ID == versionID && v.HasBundle {
			files, e := s.skillFiles(ctx, slug, v, false)
			if e != nil {
				return nil, e
			}
			result := []ReviewFile{}
			for _, f := range files {
				r := ReviewFile{Name: f.Name, Size: len(f.Data)}
				if len(f.Data) <= 256*1024 && utf8.Valid(f.Data) && !bytes.ContainsRune(f.Data, 0) {
					v := string(f.Data)
					r.Content = &v
				}
				result = append(result, r)
			}
			return result, nil
		}
	}
	return nil, ErrNotFound
}
func (s *Service) skillFiles(ctx context.Context, slug string, v Version, validate bool) ([]File, error) {
	if v.BundleKey == "" {
		return nil, ErrInvalid
	}
	reader, e := s.storage.Get(ctx, v.BundleKey)
	if e != nil {
		return nil, e
	}
	defer reader.Close()
	data, e := io.ReadAll(io.LimitReader(reader, MaxBundleBytes+512*1024+1))
	if e != nil {
		return nil, e
	}
	if len(data) > MaxBundleBytes+512*1024 {
		return nil, ErrInvalid
	}
	z, e := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if e != nil || len(z.File) > MaxBundleFiles {
		return nil, ErrInvalid
	}
	files := []File{}
	total := 0
	for _, f := range z.File {
		if !strings.HasPrefix(f.Name, slug+"/") || !f.Mode().IsRegular() {
			return nil, ErrInvalid
		}
		r, e := f.Open()
		if e != nil {
			return nil, e
		}
		b, e := io.ReadAll(io.LimitReader(r, int64(MaxBundleBytes-total+1)))
		_ = r.Close()
		if e != nil {
			return nil, e
		}
		total += len(b)
		if total > MaxBundleBytes {
			return nil, ErrInvalid
		}
		files = append(files, File{Name: strings.TrimPrefix(f.Name, slug+"/"), Data: b})
	}
	if _, e = PackSkill(slug, files); e != nil {
		return nil, e
	}
	if validate {
		if e = ValidateSkillPublication(slug, files); e != nil {
			return nil, e
		}
	}
	return files, nil
}

// Publication checks both manual uploads and builder output, while Save keeps
// older drafts repairable. Markdown is parsed for references, never executed.
func ValidateSkillPublication(slug string, files []File) error {
	known := map[string]bool{}
	var skill []byte
	for _, f := range files {
		known[f.Name] = true
		if f.Name == "SKILL.md" {
			skill = bytes.ReplaceAll(f.Data, []byte("\r\n"), []byte("\n"))
		}
	}
	if !bytes.HasPrefix(skill, []byte("---\n")) {
		return ErrInvalid
	}
	end := bytes.Index(skill[4:], []byte("\n---\n"))
	if end < 0 {
		return ErrInvalid
	}
	end += 4
	var meta map[string]any
	if yaml.Unmarshal(skill[4:end], &meta) != nil {
		return ErrInvalid
	}
	name, ok := meta["name"].(string)
	if !ok || name != slug || len(name) > 64 {
		return ErrInvalid
	}
	desc, ok := meta["description"].(string)
	if !ok || strings.TrimSpace(desc) == "" || utf8.RuneCountInString(desc) > 1024 || len(bytes.TrimSpace(skill[end+5:])) == 0 {
		return ErrInvalid
	}
	for _, f := range files {
		if !strings.EqualFold(path.Ext(f.Name), ".md") {
			continue
		}
		doc := goldmark.DefaultParser().Parse(text.NewReader(f.Data))
		e := ast.Walk(doc, func(n ast.Node, entering bool) (ast.WalkStatus, error) {
			if !entering {
				return ast.WalkContinue, nil
			}
			var dest []byte
			switch link := n.(type) {
			case *ast.Link:
				dest = link.Destination
			case *ast.Image:
				dest = link.Destination
			default:
				return ast.WalkContinue, nil
			}
			raw := string(dest)
			u, e := url.Parse(raw)
			if e != nil || strings.Contains(raw, "\\") {
				return ast.WalkStop, ErrInvalid
			}
			if u.Scheme != "" {
				if u.Scheme == "https" || u.Scheme == "http" || u.Scheme == "mailto" {
					return ast.WalkContinue, nil
				}
				return ast.WalkStop, ErrInvalid
			}
			if u.Host != "" || strings.HasPrefix(u.Path, "/") {
				return ast.WalkStop, ErrInvalid
			}
			if u.Path != "" && !known[path.Clean(path.Join(path.Dir(f.Name), u.Path))] {
				return ast.WalkStop, ErrInvalid
			}
			return ast.WalkContinue, nil
		})
		if e != nil {
			return e
		}
	}
	return nil
}
