package registry

import (
	"archive/zip"
	"bytes"
	"io/fs"
	"strings"
	"unicode/utf8"
)

const MaxBundleBytes = 10 << 20
const MaxBundleFiles = 50

// PackSkill never extracts archives or executes attachments. Paths are portable
// relative paths beneath one skill directory, with a required root SKILL.md.
func PackSkill(slug string, files []File) ([]byte, error) {
	if !slugPattern.MatchString(slug) || len(slug) > 80 || len(files) == 0 || len(files) > MaxBundleFiles {
		return nil, ErrInvalid
	}
	seen := map[string]bool{}
	size := 0
	hasSkill := false
	for _, f := range files {
		name := f.Name
		folded := strings.ToLower(name)
		if !fs.ValidPath(name) || name == "." || len(name) > 240 || strings.ContainsAny(name, "\\:\x00\r\n<>\"|?*") || seen[folded] {
			return nil, ErrInvalid
		}
		for _, char := range name {
			if char < 32 || char == 127 {
				return nil, ErrInvalid
			}
		}
		for _, part := range strings.Split(name, "/") {
			stem := strings.ToUpper(strings.SplitN(part, ".", 2)[0])
			if stem == "CON" || stem == "PRN" || stem == "AUX" || stem == "NUL" || (len(stem) == 4 && (strings.HasPrefix(stem, "COM") || strings.HasPrefix(stem, "LPT")) && stem[3] >= '1' && stem[3] <= '9') {
				return nil, ErrInvalid
			}
			if strings.TrimSpace(part) != part || strings.HasSuffix(part, ".") {
				return nil, ErrInvalid
			}
		}
		seen[folded] = true
		size += len(f.Data)
		if size > MaxBundleBytes {
			return nil, ErrInvalid
		}
		if name == "SKILL.md" {
			hasSkill = true
			if len(bytes.TrimSpace(f.Data)) == 0 || len(f.Data) > 256*1024 || !utf8.Valid(f.Data) || bytes.ContainsRune(f.Data, 0) {
				return nil, ErrInvalid
			}
		}
	}
	if !hasSkill {
		return nil, ErrInvalid
	}
	for name := range seen {
		for other := range seen {
			if strings.HasPrefix(other, name+"/") {
				return nil, ErrInvalid
			}
		}
	}
	var out bytes.Buffer
	z := zip.NewWriter(&out)
	for _, f := range files {
		h := &zip.FileHeader{Name: slug + "/" + f.Name, Method: zip.Deflate}
		h.SetMode(0644)
		w, e := z.CreateHeader(h)
		if e != nil {
			return nil, e
		}
		if _, e = w.Write(f.Data); e != nil {
			return nil, e
		}
	}
	if e := z.Close(); e != nil {
		return nil, e
	}
	return out.Bytes(), nil
}
