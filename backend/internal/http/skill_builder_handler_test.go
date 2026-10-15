package http

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"testing/synctest"
	"time"

	"github.com/google/uuid"
	"github.com/zenith-wang/it-wiki/backend/internal/domain"
	"github.com/zenith-wang/it-wiki/backend/internal/playground"
	"github.com/zenith-wang/it-wiki/backend/internal/registry"
	"github.com/zenith-wang/it-wiki/backend/internal/skillbuilder"
)

type builderModelFixture struct {
	calls    int
	user     string
	output   string
	deadline time.Time
	delay    time.Duration
	err      error
	tokens   int
}

func (m *builderModelFixture) Stream(ctx context.Context, user string, input playground.ChatRequest, emit func(string) error) error {
	m.calls++
	m.user = user
	m.deadline, _ = ctx.Deadline()
	if input.MaxTokens != nil {
		m.tokens = *input.MaxTokens
	}
	if m.delay > 0 {
		time.Sleep(m.delay)
	}
	if err := emit(m.output); err != nil {
		return err
	}
	return m.err
}

type builderRegistryFixture struct {
	calls int
	user  string
}

func (r *builderRegistryFixture) Save(_ context.Context, user, _ string, input registry.SaveRequest, _ []registry.File) (registry.Detail, error) {
	r.calls++
	r.user = user
	return registry.Detail{Capability: registry.Capability{Slug: input.Slug, Status: "draft"}}, nil
}
func TestSkillBuilderHTTPBoundary(t *testing.T) {
	id := uuid.NewString()
	hash := sha256.Sum256([]byte("csrf"))
	auth := newTestAuthHandler(t, &fakeAuthFlow{}, &fakeSessionStore{session: domain.Session{UserID: id, CSRFTokenHash: hash[:]}}, fakeUserResolver{user: domain.User{ID: id}})
	model := &builderModelFixture{output: `{"message":"期望的输出格式是什么？","draft":null}`}
	store := &builderRegistryFixture{}
	router := NewCapabilityHubRouter(Handlers{Auth: auth, SkillBuilder: NewSkillBuilderHandler(skillbuilder.New(model, store))})
	call := func(path, body, csrf string, session bool) *httptest.ResponseRecorder {
		r := httptest.NewRequest("POST", path, strings.NewReader(body))
		r.Header.Set("Origin", "https://app.example.test")
		r.Header.Set(CSRFHeaderName, csrf)
		if session {
			r.AddCookie(&http.Cookie{Name: SessionCookieName, Value: "session"})
		}
		w := httptest.NewRecorder()
		router.ServeHTTP(w, r)
		return w
	}
	path := "/api/v1/skill-builder/turn"
	body := `{"mode":"task","model":"model","messages":[{"role":"user","content":"创建周报"}],"material":""}`
	if w := call(path, body, "csrf", false); w.Code != 401 {
		t.Fatal(w.Code)
	}
	if w := call(path, body, "bad", true); w.Code != 403 || model.calls != 0 {
		t.Fatal("CSRF not enforced")
	}
	if w := call(path, body, "csrf", true); w.Code != 200 || model.user != id || w.Header().Get("Cache-Control") != "no-store" {
		t.Fatal("identity/cache", w.Code)
	}
	if remaining := time.Until(model.deadline); remaining <= playground.StreamTimeout || remaining > playground.StreamTimeout+time.Minute {
		t.Fatalf("router shortened the model generation budget: %v", remaining)
	}
	if model.tokens != skillbuilder.DefaultOutputTokens {
		t.Fatal("default output budget was not applied")
	}
	withBudget := strings.Replace(body, `"material":""`, `"material":"","max_tokens":128000`, 1)
	if w := call(path, withBudget, "csrf", true); w.Code != 200 || model.tokens != 128000 {
		t.Fatal("custom output budget was not applied", w.Code)
	}
	for _, value := range []string{"0", "1048577", "1.5"} {
		calls := model.calls
		if w := call(path, strings.Replace(withBudget, "128000", value, 1), "csrf", true); w.Code != 400 || model.calls != calls {
			t.Fatal("invalid budget reached the model", value, w.Code)
		}
	}
	if w := call(path, strings.Replace(body, `"user"`, `"system"`, 1), "csrf", true); w.Code != 400 {
		t.Fatal("system injection accepted")
	}
	if w := call(path, body+" {}", "csrf", true); w.Code != 400 {
		t.Fatal("trailing JSON accepted")
	}
	model.output = "upstream-private-body-and-key"
	if w := call(path, body, "csrf", true); w.Code != 502 || strings.Contains(w.Body.String(), model.output) {
		t.Fatal("bad upstream response leaked")
	}
	savePath := "/api/v1/skill-builder/drafts"
	draft := skillbuilder.Draft{Slug: "test-skill", Name: "测试", Description: "测试技能", Instructions: "# 步骤\n处理用户提供的输入", Files: []skillbuilder.File{}}
	payload, _ := json.Marshal(skillbuilder.SaveRequest{Draft: draft})
	if w := call(savePath, string(payload), "bad", true); w.Code != 403 || store.calls != 0 {
		t.Fatal("save CSRF bypass")
	}
	if w := call(savePath, string(payload), "csrf", true); w.Code != 201 || store.user != id {
		t.Fatal("save ownership", w.Code)
	}
	if w := call(savePath, `{"draft":{},"owner_open_id":"bob"}`, "csrf", true); w.Code != 400 {
		t.Fatal("forged owner accepted")
	}
}

func TestSkillBuilderDistinguishesModelFailures(t *testing.T) {
	for _, tc := range []struct {
		err    error
		status int
		code   string
	}{
		{playground.ErrTimeout, 504, "model_response_timeout"},
		{playground.ErrInterrupted, 502, "model_stream_interrupted"},
		{playground.ErrOutputLimit, 502, "model_output_limit"},
		{&playground.UpstreamHTTPError{StatusCode: 429}, 502, "model_upstream_rejected"},
		{&playground.UpstreamHTTPError{StatusCode: 400}, 502, "model_upstream_rejected"},
		{&playground.UpstreamHTTPError{StatusCode: 422}, 502, "model_upstream_rejected"},
		{skillbuilder.ErrOutput, 502, "invalid_skill_generation"},
		{playground.ErrUpstream, 502, "model_service_failed"},
	} {
		t.Run(tc.code, func(t *testing.T) {
			r := httptest.NewRequest("POST", "/api/v1/skill-builder/turn", nil)
			w := httptest.NewRecorder()
			skillBuilderError(w, r, tc.err)
			var payload struct {
				Error struct {
					Code    string         `json:"code"`
					Details map[string]any `json:"details"`
				} `json:"error"`
			}
			if err := json.Unmarshal(w.Body.Bytes(), &payload); err != nil {
				t.Fatal(err)
			}
			if w.Code != tc.status || payload.Error.Code != tc.code {
				t.Fatalf("wrong failure category: %d %s", w.Code, w.Body.String())
			}
			if rejected, ok := tc.err.(*playground.UpstreamHTTPError); ok {
				if payload.Error.Details["upstream_status"] != float64(rejected.StatusCode) {
					t.Fatal("missing provider status")
				}
				if (rejected.StatusCode == 400 || rejected.StatusCode == 422) && !strings.Contains(w.Body.String(), "最大输出 Tokens") {
					t.Fatal("parameter rejection has no actionable hint")
				}
			}
		})
	}
}

func TestSkillGenerationExplainsInvalidOutputWithoutLeakingContent(t *testing.T) {
	var logs bytes.Buffer
	previous := log.Writer()
	log.SetOutput(&logs)
	t.Cleanup(func() { log.SetOutput(previous) })
	const secret = "private-user-material"
	draft := skillbuilder.Draft{Slug: "weekly-report", Name: "周报", Description: "编写周报", Instructions: "[模板](references/" + secret + ".md)", Files: []skillbuilder.File{}}
	missingFile, _ := json.Marshal(skillbuilder.Reply{Message: "完成", Draft: &draft})
	draft.Instructions = "---\nname: " + secret + "\n---\n正文"
	frontmatter, _ := json.Marshal(skillbuilder.Reply{Message: "完成", Draft: &draft})
	for _, tc := range []struct {
		name, raw, reason, message string
		modelError                 error
	}{
		{"empty", " ", "empty_response", "未返回可用", nil},
		{"unfinished JSON", `{"message":"` + secret, "incomplete_json", "尚不能确认是否达到输出上限", nil},
		{"invalid JSON", "```json\n" + secret, "invalid_json", "不是有效的 JSON", nil},
		{"unexpected field", `{"message":"ok","` + secret + `":true}`, "invalid_response", "结构或字段", nil},
		{"wrong field type", `{"message":{"private":"` + secret + `"},"draft":null}`, "invalid_response", "结构或字段", nil},
		{"trailing object", `{"message":"ok","draft":null} {}`, "invalid_response", "结构或字段", nil},
		{"missing message", `{"draft":null}`, "invalid_response", "结果说明为空或过长", nil},
		{"missing attachment", string(missingFile), "invalid_draft", "引用了不存在的包内文件", nil},
		{"frontmatter", string(frontmatter), "invalid_draft", "正文不应包含 YAML frontmatter", nil},
		{"size limit", strings.Repeat("x", skillbuilder.MaxResponseBytes+1), "response_too_large", "192 KiB", nil},
		{"explicit token limit", `{"message":"` + secret, "token_limit", "内容被截断", playground.ErrOutputLimit},
		{"token limit despite valid JSON", `{"message":"done","draft":null}`, "token_limit", "内容被截断", playground.ErrOutputLimit},
	} {
		t.Run(tc.name, func(t *testing.T) {
			logs.Reset()
			id := uuid.NewString()
			hash := sha256.Sum256([]byte("csrf"))
			auth := newTestAuthHandler(t, &fakeAuthFlow{}, &fakeSessionStore{session: domain.Session{UserID: id, CSRFTokenHash: hash[:]}}, fakeUserResolver{user: domain.User{ID: id}})
			model := &builderModelFixture{output: tc.raw, err: tc.modelError}
			router := NewCapabilityHubRouter(Handlers{Auth: auth, SkillBuilder: NewSkillBuilderHandler(skillbuilder.New(model, nil))})
			r := httptest.NewRequest("POST", "/api/v1/skill-builder/turn", strings.NewReader(`{"mode":"task","model":"model","messages":[{"role":"user","content":"创建周报"}],"material":""}`))
			r.Header.Set("Origin", "https://app.example.test")
			r.Header.Set(CSRFHeaderName, "csrf")
			r.AddCookie(&http.Cookie{Name: SessionCookieName, Value: "session"})
			w := httptest.NewRecorder()
			router.ServeHTTP(w, r)
			var payload errorEnvelope
			if err := json.Unmarshal(w.Body.Bytes(), &payload); err != nil {
				t.Fatal(err)
			}
			if w.Code != 502 || payload.Error.Code != "invalid_skill_generation" || payload.Error.Details["reason"] != tc.reason || !strings.Contains(payload.Error.Message, tc.message) {
				t.Fatalf("missing actionable error: %d %s", w.Code, w.Body.String())
			}
			if payload.Error.Details["response_bytes"] != float64(len(tc.raw)) || payload.Error.RequestID == "" || !strings.Contains(logs.String(), "request_id="+payload.Error.RequestID+" reason="+tc.reason) {
				t.Fatalf("missing safe diagnostic metadata: %s %s", w.Body.String(), logs.String())
			}
			if strings.Contains(w.Body.String(), secret) || strings.Contains(logs.String(), secret) {
				t.Fatal("model content leaked through diagnostics")
			}
		})
	}
}

func TestSkillGenerationReportsProviderRejectionAndSafeInputMetadata(t *testing.T) {
	var logs bytes.Buffer
	previous := log.Writer()
	log.SetOutput(&logs)
	t.Cleanup(func() { log.SetOutput(previous) })
	const private = "private-user-content-保密"
	budget := 131072
	input := skillbuilder.Request{
		Mode: "task", Model: "model", MaxTokens: &budget,
		Messages: []playground.Message{{Role: "user", Content: private}, {Role: "assistant", Content: private}, {Role: "user", Content: private}},
		Material: private,
		Draft:    &skillbuilder.Draft{Instructions: private},
	}
	body, _ := json.Marshal(input)
	draft, _ := json.Marshal(input.Draft)
	for _, tc := range []struct {
		name, reason, message string
	}{
		{"success", "", ""},
		{"unspecified 503", "", "HTTP 503"},
		{"context", "context_limit", "上游报告上下文超限"},
		{"model", "model_unavailable", "线路不可用"},
		{"overload", "overloaded", "负载过高"},
		{"rate", "rate_limited", "请求限流"},
		{"quota", "quota_exhausted", "额度不足"},
		{"parameters", "invalid_parameters", "请求参数不受支持"},
		{"request", "invalid_request", "输入格式与参数"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			logs.Reset()
			id := uuid.NewString()
			hash := sha256.Sum256([]byte("csrf"))
			auth := newTestAuthHandler(t, &fakeAuthFlow{}, &fakeSessionStore{session: domain.Session{UserID: id, CSRFTokenHash: hash[:]}}, fakeUserResolver{user: domain.User{ID: id}})
			model := &builderModelFixture{output: `{"message":"请补充验收条件","draft":null}`}
			if tc.name != "success" {
				model.output = ""
				model.err = &playground.UpstreamHTTPError{StatusCode: 503, Reason: tc.reason}
			}
			router := NewCapabilityHubRouter(Handlers{Auth: auth, SkillBuilder: NewSkillBuilderHandler(skillbuilder.New(model, nil))})
			r := httptest.NewRequest("POST", "/api/v1/skill-builder/turn", bytes.NewReader(body))
			r.Header.Set("Origin", "https://app.example.test")
			r.Header.Set(CSRFHeaderName, "csrf")
			r.AddCookie(&http.Cookie{Name: SessionCookieName, Value: "session"})
			w := httptest.NewRecorder()
			router.ServeHTTP(w, r)
			if tc.name == "success" {
				if w.Code != 200 || !strings.Contains(logs.String(), "success=true") {
					t.Fatal("successful generation metadata missing")
				}
			} else {
				var payload errorEnvelope
				if err := json.Unmarshal(w.Body.Bytes(), &payload); err != nil {
					t.Fatal(err)
				}
				if w.Code != 502 || payload.Error.Code != "model_upstream_rejected" || payload.Error.Details["upstream_status"] != float64(503) || !strings.Contains(payload.Error.Message, tc.message) {
					t.Fatalf("incorrect provider rejection: %d %s", w.Code, w.Body.String())
				}
				reason, _ := payload.Error.Details["upstream_reason"].(string)
				if reason != tc.reason || payload.Error.RequestID == "" || !strings.Contains(logs.String(), "request_id="+payload.Error.RequestID) || !strings.Contains(logs.String(), "upstream_status=503") || !strings.Contains(logs.String(), "success=false") {
					t.Fatal("provider reason or request correlation lost")
				}
			}
			for _, expected := range []string{`model="model"`, "history_messages=3", fmt.Sprintf("history_bytes=%d", 3*len(private)), fmt.Sprintf("reference_bytes=%d", len(private)), fmt.Sprintf("draft_bytes=%d", len(draft)), "max_tokens=131072", "duration="} {
				if !strings.Contains(logs.String(), expected) {
					t.Errorf("missing diagnostic metadata %q", expected)
				}
			}
			if strings.Contains(w.Body.String(), private) || strings.Contains(logs.String(), private) {
				t.Fatal("user content leaked through diagnostics")
			}
		})
	}
}

func TestSkillGenerationSurvivesTheFormerRouterTimeout(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		id := uuid.NewString()
		hash := sha256.Sum256([]byte("csrf"))
		auth := newTestAuthHandler(t, &fakeAuthFlow{}, &fakeSessionStore{session: domain.Session{UserID: id, CSRFTokenHash: hash[:]}}, fakeUserResolver{user: domain.User{ID: id}})
		model := &builderModelFixture{delay: 61 * time.Second, output: `{"message":"已整理需求，请确认交付格式","draft":null}`}
		router := NewCapabilityHubRouter(Handlers{Auth: auth, SkillBuilder: NewSkillBuilderHandler(skillbuilder.New(model, nil))})
		r := httptest.NewRequest("POST", "/api/v1/skill-builder/turn", strings.NewReader(`{"mode":"task","model":"model","messages":[{"role":"user","content":"创建周报"}],"material":""}`))
		r.Header.Set("Origin", "https://app.example.test")
		r.Header.Set(CSRFHeaderName, "csrf")
		r.AddCookie(&http.Cookie{Name: SessionCookieName, Value: "session"})
		w := httptest.NewRecorder()
		start := time.Now()
		router.ServeHTTP(w, r)
		if time.Since(start) < 61*time.Second || w.Code != http.StatusOK || !strings.Contains(w.Body.String(), "已整理需求") {
			t.Fatalf("long generation was cut off by the router: %d %s", w.Code, w.Body.String())
		}
	})
}
