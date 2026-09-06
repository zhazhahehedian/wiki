package http

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

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
}

func (m *builderModelFixture) Stream(_ context.Context, user string, _ playground.ChatRequest, emit func(string) error) error {
	m.calls++
	m.user = user
	return emit(m.output)
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
