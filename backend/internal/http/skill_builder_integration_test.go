package http

import (
	"archive/zip"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
	authstore "github.com/zenith-wang/it-wiki/backend/internal/auth"
	"github.com/zenith-wang/it-wiki/backend/internal/domain"
	"github.com/zenith-wang/it-wiki/backend/internal/playground"
	"github.com/zenith-wang/it-wiki/backend/internal/registry"
	"github.com/zenith-wang/it-wiki/backend/internal/repo"
	"github.com/zenith-wang/it-wiki/backend/internal/skillbuilder"
)

// Invoked inside TestRegistryCloudIntegration's disposable database/object scope.
// The only substitute is the model provider; auth, encrypted connections,
// protocol translation, builder, SQL registry and bundle storage are real code.
func verifySkillBuilderIntegration(t *testing.T, pool *pgxpool.Pool, registryService *registry.Service, alice, bob string) *playground.Service {
	t.Helper()
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		anthropic := r.URL.Path == "/v1/messages"
		if (anthropic && r.Header.Get("X-Api-Key") != "builder-fixture-key") || (!anthropic && r.Header.Get("Authorization") != "Bearer builder-fixture-key") {
			t.Error("wrong fixture credential")
			w.WriteHeader(401)
			return
		}
		if r.URL.Path == "/v1/models" {
			fmt.Fprint(w, `{"data":[{"id":"fixture-model"}]}`)
			return
		}
		var input struct {
			Messages  []playground.Message `json:"messages"`
			System    string               `json:"system"`
			MaxTokens int                  `json:"max_tokens"`
		}
		if json.NewDecoder(r.Body).Decode(&input) != nil || input.MaxTokens != 8192 {
			t.Error("invalid model request")
			w.WriteHeader(400)
			return
		}
		system := input.System
		if !anthropic && len(input.Messages) > 0 {
			system = input.Messages[0].Content
		}
		if !strings.Contains(system, "输出协议") {
			t.Error("missing trusted generation contract")
		}
		last := input.Messages[len(input.Messages)-1].Content
		d := skillbuilder.Draft{Slug: "guided-weekly-report", Name: "技术周报助手", Description: "根据项目进展整理团队周报，用于每周向研发负责人汇报。", Instructions: "# 技术周报\n\n先确认汇报周期，使用[周报模板](references/template.md)整理输入。\n\n1. 提取已完成工作与证据。\n2. 明确风险、负责人和下一步。\n3. 缺少信息时标注待补充，不编造数据。", Files: []skillbuilder.File{{Path: "references/template.md", Content: "# 团队周报\n\n## 本周进展\n## 风险与依赖\n## 下周计划"}}}
		if strings.Contains(system, "女娲") {
			d.Slug = "guided-first-principles"
			d.Name = "第一性原理分析助手"
			d.Description = "评估产品方案时，拆解假设、证据和约束，提出可验证的下一步。"
			d.Instructions = "# 第一性原理分析\n\n明确问题和目标，拆分事实与假设，检查关键约束。不要代替真实人物发言。\n\n输出依据、未知项与下一步验证。"
			d.Files = []skillbuilder.File{}
		}
		reply := skillbuilder.Reply{Message: "已整理成草案，可编辑正文和附件后保存。", Draft: &d}
		if strings.Contains(last, "先提问") {
			reply = skillbuilder.Reply{Message: "周报主要给谁看？需要包含哪些栏目？"}
		}
		raw, _ := json.Marshal(reply)
		if strings.Contains(last, "无效输出") {
			raw = []byte(`{"message":"broken`)
		}
		w.Header().Set("Content-Type", "text/event-stream")
		// Deliberately split JSON across deltas, as real providers do.
		for start := 0; start < len(raw); {
			end := min(start+31, len(raw))
			// Split by rune boundaries so each JSON string delta remains valid UTF-8.
			for end < len(raw) && raw[end]&0xc0 == 0x80 {
				end++
			}
			var delta []byte
			if anthropic {
				delta, _ = json.Marshal(map[string]any{"type": "content_block_delta", "delta": map[string]string{"type": "text_delta", "text": string(raw[start:end])}})
			} else {
				delta, _ = json.Marshal(map[string]any{"choices": []any{map[string]any{"delta": map[string]string{"content": string(raw[start:end])}}}})
			}
			fmt.Fprintf(w, "data: %s\n\n", delta)
			start = end
		}
		if anthropic {
			fmt.Fprint(w, "data: {\"type\":\"message_stop\"}\n\n")
		} else {
			fmt.Fprint(w, "data: [DONE]\n\n")
		}
	}))
	t.Cleanup(upstream.Close)
	client, e := playground.NewHTTPClient(upstream.URL)
	if e != nil {
		t.Fatal(e)
	}
	protector, e := authstore.NewAESGCMProtector([]byte("12345678901234567890123456789012"))
	if e != nil {
		t.Fatal(e)
	}
	model := playground.New(repo.NewPlaygroundRepository(pool), protector, client)
	builder := skillbuilder.New(model, registryService)
	call := func(user, method, path string, input any) *httptest.ResponseRecorder {
		t.Helper()
		raw, _ := json.Marshal(input)
		req := httptest.NewRequest(method, path, bytes.NewReader(raw))
		req.AddCookie(&http.Cookie{Name: SessionCookieName, Value: "fixture"})
		req.Header.Set("Origin", "https://app.example.test")
		req.Header.Set(CSRFHeaderName, "csrf")
		req.Header.Set("Content-Type", "application/json")
		hash := sha256.Sum256([]byte("csrf"))
		auth := newTestAuthHandler(t, &fakeAuthFlow{}, &fakeSessionStore{session: domain.Session{UserID: user, CSRFTokenHash: hash[:]}}, fakeUserResolver{user: domain.User{ID: user}})
		router := NewCapabilityHubRouter(Handlers{Auth: auth, Playground: NewPlaygroundHandler(model), Registry: NewRegistryHandler(registryService), SkillBuilder: NewSkillBuilderHandler(builder)})
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)
		return w
	}
	check := func(w *httptest.ResponseRecorder, status int) {
		t.Helper()
		if w.Code != status {
			t.Fatalf("builder integration: want %d got %d: %s", status, w.Code, w.Body.String())
		}
	}
	input := skillbuilder.Request{Mode: "task", Model: "fixture-model", Messages: []playground.Message{{Role: "user", Content: "先提问，帮我创建周报 Skill"}}}
	check(call(bob, "POST", "/api/v1/skill-builder/turn", input), 404)
	check(call(alice, "PUT", "/api/v1/playground/connection", playground.SaveRequest{BaseURL: upstream.URL + "/v1", APIKey: "builder-fixture-key", Models: []string{"fixture-model"}, DefaultModel: "fixture-model"}), 200)
	w := call(alice, "POST", "/api/v1/skill-builder/turn", input)
	check(w, 200)
	var reply skillbuilder.Reply
	if json.Unmarshal(w.Body.Bytes(), &reply) != nil || reply.Draft != nil || reply.Message == "" {
		t.Fatal("missing clarification")
	}
	input.Messages = append(input.Messages, playground.Message{Role: "assistant", Content: reply.Message}, playground.Message{Role: "user", Content: "给研发负责人看，包括进展、风险、计划"})
	w = call(alice, "POST", "/api/v1/skill-builder/turn", input)
	check(w, 200)
	if json.Unmarshal(w.Body.Bytes(), &reply) != nil || reply.Draft == nil {
		t.Fatal("missing generated draft")
	}
	check(call(alice, "GET", "/api/v1/capabilities/"+reply.Draft.Slug, nil), 404) // generation never auto-saves
	reply.Draft.Name = "Owner 编辑后的周报"
	reply.Draft.Slug = "saved-generated-report"
	reply.Draft.Files[0].Content += "\n## 待确认事项"
	w = call(alice, "POST", "/api/v1/skill-builder/drafts", skillbuilder.SaveRequest{Draft: *reply.Draft})
	check(w, 201)
	var detail registry.Detail
	if json.Unmarshal(w.Body.Bytes(), &detail) != nil || detail.Name != reply.Draft.Name || detail.Status != "draft" || detail.OwnerOpenID != "ou_"+alice {
		t.Fatal("saved draft differs or wrong owner")
	}
	check(call(alice, "POST", "/api/v1/skill-builder/drafts", skillbuilder.SaveRequest{Draft: *reply.Draft}), 409)
	check(call(bob, "GET", "/api/v1/capabilities/"+detail.Slug, nil), 404)
	bundle := "/api/v1/capabilities/" + detail.Slug + "/versions/" + detail.DraftVersionID + "/bundle"
	check(call(bob, "GET", bundle, nil), 404)
	w = call(alice, "GET", bundle, nil)
	check(w, 200)
	z, e := zip.NewReader(bytes.NewReader(w.Body.Bytes()), int64(w.Body.Len()))
	if e != nil {
		t.Fatal(e)
	}
	want, e := skillbuilder.Files(*reply.Draft)
	if e != nil {
		t.Fatal(e)
	}
	if len(z.File) != len(want) {
		t.Fatal("missing generated attachments")
	}
	for _, file := range want {
		r, e := z.Open(detail.Slug + "/" + file.Name)
		if e != nil {
			t.Fatal(e)
		}
		b, e := io.ReadAll(r)
		_ = r.Close()
		if e != nil || !bytes.Equal(b, file.Data) {
			t.Fatal("generated file differs after download", file.Name)
		}
	}
	check(call(bob, "PUT", "/api/v1/playground/connection", playground.SaveRequest{Protocol: "anthropic", BaseURL: upstream.URL, APIKey: "builder-fixture-key", Models: []string{"fixture-model"}, DefaultModel: "fixture-model"}), 200)
	input.Mode = "perspective"
	input.Messages = []playground.Message{{Role: "user", Content: "用第一性原理评估产品方案"}}
	w = call(bob, "POST", "/api/v1/skill-builder/turn", input)
	check(w, 200)
	if json.Unmarshal(w.Body.Bytes(), &reply) != nil || reply.Draft == nil || reply.Draft.Slug != "guided-first-principles" {
		t.Fatal("Anthropic perspective flow failed")
	}
	input.Messages[0].Content = "无效输出"
	check(call(alice, "POST", "/api/v1/skill-builder/turn", input), 502)
	var count int
	if e = pool.QueryRow(context.Background(), `SELECT count(*) FROM capabilities WHERE slug='saved-generated-report'`).Scan(&count); e != nil || count != 1 {
		t.Fatal("duplicate or partial persistence", e)
	}
	t.Log("skill builder: encrypted per-user OpenAI/Anthropic mock protocols, clarification, no auto-save, edited draft, Owner isolation, conflict and exact zip roundtrip passed")
	return model
}
