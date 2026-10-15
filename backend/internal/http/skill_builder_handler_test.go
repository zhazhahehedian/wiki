package http

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
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
	calls  int
	user   string
	output string
	delay  time.Duration
	err    error
	tokens int
	ctxErr error
}

func (m *builderModelFixture) Stream(ctx context.Context, user string, input playground.ChatRequest, emit func(string) error) error {
	m.calls++
	m.user = user
	m.ctxErr = ctx.Err()
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

const builderTurnBody = `{"mode":"task","model":"model","messages":[{"role":"user","content":"创建周报"}],"material":""}`

func builderRouter(t *testing.T, model *builderModelFixture, store skillbuilder.Registry) (http.Handler, string) {
	t.Helper()
	id := uuid.NewString()
	hash := sha256.Sum256([]byte("csrf"))
	auth := newTestAuthHandler(t, &fakeAuthFlow{}, &fakeSessionStore{session: domain.Session{UserID: id, CSRFTokenHash: hash[:]}}, fakeUserResolver{user: domain.User{ID: id}})
	return NewCapabilityHubRouter(Handlers{Auth: auth, SkillBuilder: NewSkillBuilderHandler(skillbuilder.New(model, store))}), id
}

func builderRequest(ctx context.Context, path, body string) *http.Request {
	r := httptest.NewRequestWithContext(ctx, "POST", path, strings.NewReader(body))
	r.Header.Set("Origin", "https://app.example.test")
	r.Header.Set(CSRFHeaderName, "csrf")
	r.AddCookie(&http.Cookie{Name: SessionCookieName, Value: "session"})
	return r
}

func TestSkillBuilderHTTPBoundary(t *testing.T) {
	model := &builderModelFixture{output: `{"message":"期望的输出格式是什么？","draft":null}`}
	store := &builderRegistryFixture{}
	router, id := builderRouter(t, model, store)
	call := func(path, body, csrf string, session bool) *httptest.ResponseRecorder {
		r := builderRequest(context.Background(), path, body)
		r.Header.Set(CSRFHeaderName, csrf)
		if !session {
			r.Header.Del("Cookie")
		}
		w := httptest.NewRecorder()
		router.ServeHTTP(w, r)
		return w
	}
	path := "/api/v1/skill-builder/turn"
	body := builderTurnBody
	if w := call(path, body, "csrf", false); w.Code != 401 {
		t.Fatal(w.Code)
	}
	if w := call(path, body, "bad", true); w.Code != 403 || model.calls != 0 {
		t.Fatal("CSRF not enforced")
	}
	if w := call(path, body, "csrf", true); w.Code != 200 || model.user != id || w.Header().Get("Cache-Control") != "no-store" {
		t.Fatal("identity/cache", w.Code)
	}
	if model.tokens != playground.DefaultOutputTokens {
		t.Fatal("default output budget was not applied")
	}
	withBudget := strings.Replace(body, `"material":""`, `"material":"","max_tokens":128000`, 1)
	if w := call(path, withBudget, "csrf", true); w.Code != 200 || model.tokens != 128000 {
		t.Fatal("custom output budget was not applied", w.Code)
	}
	calls := model.calls
	if w := call(path, strings.Replace(withBudget, "128000", "1.5", 1), "csrf", true); w.Code != 400 || model.calls != calls {
		t.Fatal("fractional budget reached the model", w.Code)
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
		{playground.ErrOutputLimit, 502, "model_output_limit"},
		{&playground.UpstreamHTTPError{StatusCode: 429}, 502, "model_upstream_rejected"},
		{&skillbuilder.OutputError{Reason: "invalid_output", Message: "invalid"}, 502, "invalid_skill_generation"},
		{playground.ErrUpstream, 502, "model_service_failed"},
	} {
		t.Run(tc.code, func(t *testing.T) {
			r := httptest.NewRequest("POST", "/api/v1/skill-builder/turn", nil)
			w := httptest.NewRecorder()
			skillBuilderError(w, r, tc.err)
			var payload errorEnvelope
			if err := json.Unmarshal(w.Body.Bytes(), &payload); err != nil {
				t.Fatal(err)
			}
			if w.Code != tc.status || payload.Error.Code != tc.code {
				t.Fatalf("wrong failure category: %d %s", w.Code, w.Body.String())
			}
			var rejected *playground.UpstreamHTTPError
			if errors.As(tc.err, &rejected) && payload.Error.Details["upstream_status"] != float64(rejected.StatusCode) {
				t.Fatal("missing provider status")
			}
		})
	}
}

func TestSkillGenerationFailuresDoNotLeakContent(t *testing.T) {
	var logs bytes.Buffer
	previous := log.Writer()
	log.SetOutput(&logs)
	t.Cleanup(func() { log.SetOutput(previous) })
	const secret = "private-user-material"
	draft := skillbuilder.Draft{Slug: "weekly-report", Name: "周报", Description: "编写周报", Instructions: "[模板](references/" + secret + ".md)", Files: []skillbuilder.File{}}
	missingFile, _ := json.Marshal(skillbuilder.Reply{Message: "完成", Draft: &draft})
	for _, tc := range []struct {
		name, raw, code, reason string
		modelError              error
	}{
		{"unfinished JSON", `{"message":"` + secret, "invalid_skill_generation", "invalid_output", nil},
		{"unexpected field", `{"message":"ok","` + secret + `":true}`, "invalid_skill_generation", "invalid_output", nil},
		{"invalid draft", string(missingFile), "invalid_skill_generation", "invalid_draft", nil},
		{"size limit", strings.Repeat("x", skillbuilder.MaxResponseBytes+1), "invalid_skill_generation", "response_too_large", nil},
		{"token limit", `{"message":"` + secret, "invalid_skill_generation", "token_limit", playground.ErrOutputLimit},
		{"provider rejection", secret, "model_upstream_rejected", "", &playground.UpstreamHTTPError{StatusCode: 503}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			logs.Reset()
			router, _ := builderRouter(t, &builderModelFixture{output: tc.raw, err: tc.modelError}, nil)
			w := httptest.NewRecorder()
			router.ServeHTTP(w, builderRequest(context.Background(), "/api/v1/skill-builder/turn", builderTurnBody))
			var payload errorEnvelope
			if err := json.Unmarshal(w.Body.Bytes(), &payload); err != nil {
				t.Fatal(err)
			}
			if w.Code != 502 || payload.Error.Code != tc.code || payload.Error.RequestID == "" {
				t.Fatalf("wrong failure: %d %s", w.Code, w.Body.String())
			}
			if tc.reason != "" && payload.Error.Details["reason"] != tc.reason {
				t.Fatalf("wrong reason: %s", w.Body.String())
			}
			if tc.reason == "invalid_draft" && !strings.Contains(payload.Error.Message, "未通过校验：") {
				t.Fatalf("validation detail lost: %s", payload.Error.Message)
			}
			if strings.Contains(w.Body.String(), secret) || strings.Contains(logs.String(), secret) {
				t.Fatal("model content leaked through diagnostics")
			}
		})
	}
}

func TestLongModelRoutesOutliveTheDefaultTimeoutAndKeepCancellation(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		model := &builderModelFixture{delay: 61 * time.Second, output: `{"message":"已整理需求，请确认交付格式","draft":null}`}
		router, _ := builderRouter(t, model, nil)
		w := httptest.NewRecorder()
		start := time.Now()
		router.ServeHTTP(w, builderRequest(context.Background(), "/api/v1/skill-builder/turn", builderTurnBody))
		if time.Since(start) < 61*time.Second || w.Code != http.StatusOK || !strings.Contains(w.Body.String(), "已整理需求") {
			t.Fatalf("long generation was cut off by the router: %d %s", w.Code, w.Body.String())
		}
	})
	model := &builderModelFixture{output: `{"message":"ok","draft":null}`}
	router, _ := builderRouter(t, model, nil)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	router.ServeHTTP(httptest.NewRecorder(), builderRequest(ctx, "/api/v1/skill-builder/turn", builderTurnBody))
	if model.calls != 1 || !errors.Is(model.ctxErr, context.Canceled) {
		t.Fatalf("caller cancellation did not reach the model: calls=%d err=%v", model.calls, model.ctxErr)
	}
}
