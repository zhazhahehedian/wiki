package skillbuilder

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/zenith-wang/it-wiki/backend/internal/playground"
	"github.com/zenith-wang/it-wiki/backend/internal/registry"
	"gopkg.in/yaml.v3"
)

func validDraft() Draft {
	return Draft{Slug: "weekly-report", Name: "技术周报", Description: "根据项目进展编写周报", Instructions: "# 周报\n\n按[模板](references/template.md)整理输入，列出进展、风险和下一步。", Files: []File{{Path: "references/template.md", Content: "# 本周进展\n# 风险\n# 下周计划"}}}
}
func TestDraftFrontmatterAndRelativeLinks(t *testing.T) {
	draft := validDraft()
	draft.Description = "场景：每周汇报\n包括进展与风险"
	files, e := Files(draft)
	if e != nil {
		t.Fatal(e)
	}
	parts := strings.SplitN(string(files[0].Data), "---\n", 3)
	var header map[string]string
	if len(parts) != 3 || yaml.Unmarshal([]byte(parts[1]), &header) != nil || header["name"] != draft.Slug || header["description"] != draft.Description {
		t.Fatal("frontmatter corrupted")
	}
	if files[1].Name != "references/template.md" {
		t.Fatal("attachment missing")
	}
	draft.Files[0].Content = "[正文](../SKILL.md#周报)"
	if _, e = Files(draft); e != nil {
		t.Fatal("valid parent link inside bundle rejected", e)
	}
}
func TestInvalidArtifacts(t *testing.T) {
	cases := map[string]func(*Draft){
		"reserved slug":        func(d *Draft) { d.Slug = "create-skill" },
		"empty instructions":   func(d *Draft) { d.Instructions = " " },
		"frontmatter in body":  func(d *Draft) { d.Instructions = "---\nname: spoof\n---\nbody" },
		"traversal":            func(d *Draft) { d.Files[0].Path = "references/../../outside.md" },
		"duplicate":            func(d *Draft) { d.Files = append(d.Files, d.Files[0]) },
		"script":               func(d *Draft) { d.Files[0].Path = "references/run.sh" },
		"missing linked file":  func(d *Draft) { d.Files = nil },
		"encoded missing file": func(d *Draft) { d.Instructions = "[bad](%2e%2e/secret.md)" },
		"absolute path":        func(d *Draft) { d.Instructions = "[bad](/etc/passwd)" },
		"external image":       func(d *Draft) { d.Instructions = "![bad](https://example.test/pixel)" },
		"network image":        func(d *Draft) { d.Instructions = "![bad](//example.test/pixel)" },
		"executable link":      func(d *Draft) { d.Instructions = "[bad](javascript:alert)" },
		"oversize file":        func(d *Draft) { d.Files[0].Content = strings.Repeat("a", 32*1024+1) },
		"bad text":             func(d *Draft) { d.Instructions = "body\x00" },
		"too many":             func(d *Draft) { d.Files = make([]File, 11) },
	}
	for name, change := range cases {
		t.Run(name, func(t *testing.T) {
			d := validDraft()
			change(&d)
			if _, e := Files(d); !errors.Is(e, ErrInvalid) {
				t.Fatalf("accepted invalid artifact: %v", e)
			}
		})
	}
}

type fakeModel struct {
	output   string
	err      error
	calls    int
	user     string
	input    playground.ChatRequest
	onStream func(context.Context, func(string) error) error
}

func (m *fakeModel) Stream(ctx context.Context, user string, r playground.ChatRequest, emit func(string) error) error {
	m.calls++
	m.user = user
	m.input = r
	if m.onStream != nil {
		return m.onStream(ctx, emit)
	}
	if m.err != nil {
		return m.err
	}
	return emit(m.output)
}

type fakeRegistry struct {
	calls int
	user  string
	input registry.SaveRequest
	files []registry.File
}

func (r *fakeRegistry) Save(_ context.Context, user, slug string, input registry.SaveRequest, files []registry.File) (registry.Detail, error) {
	r.calls++
	r.user = user
	r.input = input
	r.files = files
	return registry.Detail{Capability: registry.Capability{Slug: input.Slug, Status: "draft"}}, nil
}
func request() Request {
	return Request{Mode: "task", Model: "test-model", Messages: []playground.Message{{Role: "user", Content: "想创建周报 Skill"}}, Material: "输入：项目进度，风险，下周计划"}
}
func TestClarificationAndDraftGeneration(t *testing.T) {
	model := &fakeModel{output: `{"message":"周报主要给谁看？","draft":null}`}
	service := New(model, nil)
	reply, e := service.Turn(context.Background(), "alice", request())
	if e != nil || reply.Draft != nil || reply.Message == "" {
		t.Fatal(reply, e)
	}
	r := request()
	r.Mode = "perspective"
	r.Messages = append(r.Messages, playground.Message{Role: "assistant", Content: reply.Message}, playground.Message{Role: "user", Content: "给研发负责人看"})
	d := validDraft()
	r.Draft = &d
	raw, _ := json.Marshal(Reply{Message: "已整理草案", Draft: &d})
	model.output = string(raw)
	reply, e = service.Turn(context.Background(), "alice", r)
	if e != nil || reply.Draft.Slug != d.Slug {
		t.Fatal(reply, e)
	}
	if model.user != "alice" || model.input.Model != "test-model" || model.input.Messages[0].Role != "system" || !strings.Contains(model.input.Messages[0].Content, "女娲") || !strings.Contains(model.input.Messages[1].Content, "reference_material") {
		t.Fatal("wrong owner or trusted prompt composition")
	}
	if model.input.MaxTokens == nil || *model.input.MaxTokens != 8192 {
		t.Fatal("unbounded generation")
	}
}
func TestInvalidRequestNeverCallsModel(t *testing.T) {
	for _, mutate := range []func(*Request){func(r *Request) { r.Mode = "custom" }, func(r *Request) { r.Messages[0].Role = "system" }, func(r *Request) { r.Messages = nil }, func(r *Request) { r.Material = strings.Repeat("x", MaxMaterialBytes+1) }, func(r *Request) { r.Messages = append(r.Messages, playground.Message{Role: "assistant", Content: "x"}) }} {
		model := &fakeModel{}
		r := request()
		mutate(&r)
		if _, e := New(model, nil).Turn(context.Background(), "alice", r); !errors.Is(e, ErrInvalid) || model.calls != 0 {
			t.Fatal("invalid request reached model", e)
		}
	}
}
func TestRejectsInvalidOrPartialModelOutput(t *testing.T) {
	for _, output := range []string{`{"message":"ok","draft":`, `{"message":"ok"} {}`, `{"message":"ok","owner":"bob"}`, `{"message":"ok","draft":{"slug":"../bad"}}`, strings.Repeat("x", MaxResponseBytes+1)} {
		repo := &fakeRegistry{}
		_, e := New(&fakeModel{output: output}, repo).Turn(context.Background(), "alice", request())
		if !errors.Is(e, ErrOutput) || repo.calls != 0 {
			t.Fatal("invalid generation accepted or auto-saved", e)
		}
	}
}
func TestCancellationAndUpstreamFailureNeverReturnDraft(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	model := &fakeModel{onStream: func(ctx context.Context, emit func(string) error) error {
		_ = emit(`{"message":"partial`)
		cancel()
		return emit("data")
	}}
	if result, e := New(model, nil).Turn(ctx, "alice", request()); !errors.Is(e, context.Canceled) || result.Draft != nil {
		t.Fatal("cancelled output returned", e)
	}
	if _, e := New(&fakeModel{err: playground.ErrUpstream}, nil).Turn(context.Background(), "alice", request()); !errors.Is(e, playground.ErrUpstream) {
		t.Fatal("upstream failure hidden", e)
	}
}
func TestSaveRevalidatesEditedDraftAndUsesOwnerRegistry(t *testing.T) {
	repo := &fakeRegistry{}
	s := New(nil, repo)
	d := validDraft()
	d.Instructions = "Use [missing](references/absent.md)"
	if _, e := s.Save(context.Background(), "alice", SaveRequest{Draft: d}); !errors.Is(e, ErrInvalid) || repo.calls != 0 {
		t.Fatal("invalid user edit saved")
	}
	d = validDraft()
	result, e := s.Save(context.Background(), "alice", SaveRequest{Draft: d})
	if e != nil || result.Status != "draft" || repo.user != "alice" || repo.input.Revision != 0 || repo.input.Type != "skill" || len(repo.files) != 2 {
		t.Fatal("wrong registry save", e)
	}
}

func TestIncompleteEditsCanBeRepairedButNotSaved(t *testing.T) {
	d := validDraft()
	d.Files = nil // user removed a linked file and asks the assistant to repair it
	r := request()
	r.Draft = &d
	model := &fakeModel{output: `{"message":"请补充模板内容","draft":null}`}
	repository := &fakeRegistry{}
	service := New(model, repository)
	if _, e := service.Turn(context.Background(), "alice", r); e != nil || model.calls != 1 {
		t.Fatal("cannot repair incomplete edit", e)
	}
	if _, e := service.Save(context.Background(), "alice", SaveRequest{Draft: d}); !errors.Is(e, ErrInvalid) || repository.calls != 0 {
		t.Fatal("invalid edit persisted", e)
	}
}
