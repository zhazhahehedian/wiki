package registry

import (
	"archive/zip"
	"bytes"
	"context"
	"errors"
	"io"
	"strings"
	"testing"
)

func validMCP() SaveRequest {
	return SaveRequest{Slug: "search", Type: "mcp", Name: "Search", Description: "Team search", Visibility: "org", Version: "1.0.0", MCPEndpoint: "https://mcp.example.test/mcp", MCPTransport: "streamable-http", MCPAuthScheme: "bearer", Tools: []Tool{{Name: "search", Description: "search", InputSchema: []byte(`{"type":"object"}`)}}}
}
func TestValidateMCPAndVisibility(t *testing.T) {
	cases := map[string]func(*SaveRequest){
		"reserved route":        func(r *SaveRequest) { r.Slug = "new" },
		"builder route":         func(r *SaveRequest) { r.Slug = "create-skill" },
		"credential URL":        func(r *SaveRequest) { r.MCPEndpoint = "https://user:password@example.test/mcp" },
		"credential query":      func(r *SaveRequest) { r.MCPEndpoint += "?token=secret" },
		"fragment":              func(r *SaveRequest) { r.MCPEndpoint += "#token" },
		"local file":            func(r *SaveRequest) { r.MCPEndpoint = "file:///etc/passwd" },
		"unsupported transport": func(r *SaveRequest) { r.MCPTransport = "stdio" },
		"duplicate tools":       func(r *SaveRequest) { r.Tools = append(r.Tools, r.Tools[0]) },
		"invalid schema":        func(r *SaveRequest) { r.Tools[0].InputSchema = []byte(`[]`) },
		"missing department":    func(r *SaveRequest) { r.Visibility = "department" },
		"empty allowlist":       func(r *SaveRequest) { r.Visibility = "allowlist" },
		"duplicate allowlist":   func(r *SaveRequest) { r.Visibility = "allowlist"; r.Allowlist = []string{"ou_a", "ou_a"} },
		"extraneous allowlist":  func(r *SaveRequest) { r.Allowlist = []string{"ou_a"} },
		"mixed types":           func(r *SaveRequest) { r.Type = "skill" },
	}
	for name, mutate := range cases {
		t.Run(name, func(t *testing.T) {
			r := validMCP()
			mutate(&r)
			if !errors.Is(validate(&r), ErrInvalid) {
				t.Fatal("accepted invalid input")
			}
		})
	}
	r := validMCP()
	if e := validate(&r); e != nil {
		t.Fatal(e)
	}
	r.Visibility = "allowlist"
	r.Allowlist = []string{"ou_alice", "ou_bob"}
	if e := validate(&r); e != nil {
		t.Fatal(e)
	}
}
func TestSkillBundlePathsAndLimits(t *testing.T) {
	for _, name := range []string{"../secret", "/secret", "nested/../secret", `a\b`, "a:b", "a\x00b", "a/./b", "", "a/ ", "a.", "CON.txt", "com1", "x?y", "x\ty"} {
		t.Run(name, func(t *testing.T) {
			_, e := PackSkill("skill", []File{{"SKILL.md", []byte("# Skill")}, {name, []byte("x")}})
			if !errors.Is(e, ErrInvalid) {
				t.Fatal("accepted unsafe filename")
			}
		})
	}
	cases := [][]File{
		{{"file.txt", []byte("x")}},
		{{"SKILL.md", nil}},
		{{"SKILL.md", []byte{255}}},
		{{"SKILL.md", []byte("# Skill")}, {"skill.md", []byte("duplicate")}},
		{{"SKILL.md", []byte("# Skill")}, {"scripts", []byte("file")}, {"scripts/run.py", []byte("x")}},
		{{"SKILL.md", []byte("# Skill")}, {"large", make([]byte, MaxBundleBytes)}},
	}
	for i, files := range cases {
		if _, e := PackSkill("skill", files); !errors.Is(e, ErrInvalid) {
			t.Fatalf("accepted invalid bundle %d", i)
		}
	}
	packed, e := PackSkill("my-skill", []File{{"SKILL.md", []byte("# Hello")}, {"scripts/run.py", []byte("print('hello')")}})
	if e != nil {
		t.Fatal(e)
	}
	z, e := zip.NewReader(bytes.NewReader(packed), int64(len(packed)))
	if e != nil {
		t.Fatal(e)
	}
	if len(z.File) != 2 || z.File[0].Name != "my-skill/SKILL.md" || z.File[1].Name != "my-skill/scripts/run.py" {
		t.Fatal("incorrect portable bundle structure")
	}
	reader, e := z.File[0].Open()
	if e != nil {
		t.Fatal(e)
	}
	defer reader.Close()
	raw, _ := io.ReadAll(reader)
	if string(raw) != "# Hello" {
		t.Fatal("content changed")
	}
}

type failingRepo struct {
	Repository
	referenced   bool
	referenceErr error
	saveErr      error
	saves        int
}

func (r *failingRepo) Owner(context.Context, string) (string, error) { return "owner", nil }
func (r *failingRepo) Save(context.Context, string, string, SaveRequest, Version) (Detail, error) {
	r.saves++
	if r.saveErr != nil {
		return Detail{}, r.saveErr
	}
	return Detail{}, ErrConflict
}
func (r *failingRepo) BundleReferenced(context.Context, string) (bool, error) {
	return r.referenced, r.referenceErr
}

type recordingStorage struct {
	puts, deletes int
	putErr        error
}

func (s *recordingStorage) Put(context.Context, string, io.Reader, int64, string) error {
	s.puts++
	return s.putErr
}
func (s *recordingStorage) Get(context.Context, string) (io.ReadCloser, error) {
	return nil, ErrNotFound
}
func (s *recordingStorage) Delete(context.Context, string) error { s.deletes++; return nil }
func TestFailedSaveCleansOnlyProvenUnreferencedObjects(t *testing.T) {
	for _, tc := range []struct {
		name    string
		ref     bool
		err     error
		deleted int
	}{{"rolled back", false, nil, 1}, {"ambiguous committed", true, nil, 0}, {"unavailable database", false, errors.New("offline"), 0}} {
		t.Run(tc.name, func(t *testing.T) {
			r := &failingRepo{referenced: tc.ref, referenceErr: tc.err}
			storage := &recordingStorage{}
			s := New(r, storage)
			input := validMCP()
			input.Type = "skill"
			input.MCPEndpoint = ""
			input.MCPTransport = ""
			input.MCPAuthScheme = ""
			input.Tools = nil
			_, e := s.Save(context.Background(), "user", "", input, []File{{"SKILL.md", []byte("# Hello")}})
			if !errors.Is(e, ErrConflict) || storage.puts != 1 || storage.deletes != tc.deleted {
				t.Fatalf("unsafe cleanup: err=%v storage=%+v", e, storage)
			}
		})
	}
}
func TestFailedUploadDoesNotCreateVersion(t *testing.T) {
	r := &failingRepo{}
	storage := &recordingStorage{putErr: errors.New("storage unavailable")}
	s := New(r, storage)
	input := validMCP()
	input.Type = "skill"
	input.MCPEndpoint = ""
	input.MCPTransport = ""
	input.MCPAuthScheme = ""
	input.Tools = nil
	if _, e := s.Save(context.Background(), "user", "", input, []File{{"SKILL.md", []byte("# Hello")}}); e == nil || r.saves != 0 {
		t.Fatal("failed upload saved a version")
	}
}

func TestAmbiguousCommitNeverDeletesUploadedBundle(t *testing.T) {
	r := &failingRepo{saveErr: ErrCommitUnknown}
	storage := &recordingStorage{}
	input := SaveRequest{Slug: "skill", Type: "skill", Name: "Skill", Description: "Describe", Visibility: "org", Version: "1"}
	_, e := New(r, storage).Save(context.Background(), "user", "", input, []File{{"SKILL.md", []byte("# Hello")}})
	if !errors.Is(e, ErrCommitUnknown) || storage.deletes != 0 {
		t.Fatal("ambiguous commit caused premature deletion", e)
	}
}

func TestHumanTextLimitsCountUnicodeCharacters(t *testing.T) {
	input := validMCP()
	input.Name = strings.Repeat("文", 120)
	input.Department = strings.Repeat("部", 120)
	if e := validate(&input); e != nil {
		t.Fatal("valid Chinese text rejected", e)
	}
	input.Name += "字"
	if e := validate(&input); !errors.Is(e, ErrInvalid) {
		t.Fatal("name character limit not enforced")
	}
}
