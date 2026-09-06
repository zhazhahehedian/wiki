package http

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/pressly/goose/v3"
	authstore "github.com/zenith-wang/it-wiki/backend/internal/auth"
	"github.com/zenith-wang/it-wiki/backend/internal/domain"
	"github.com/zenith-wang/it-wiki/backend/internal/playground"
	"github.com/zenith-wang/it-wiki/backend/internal/repo"
	"github.com/zenith-wang/it-wiki/backend/internal/repo/migrations"
)

// The integration test owns a freshly created database and drops only that
// database. The supplied URL provides administrative connectivity, never a
// schema to reset. It uses no real OAuth account or model credentials.
func TestPlaygroundCloudIntegration(t *testing.T) {
	raw := os.Getenv("PLAYGROUND_TEST_DATABASE_URL")
	if raw == "" {
		t.Skip("PLAYGROUND_TEST_DATABASE_URL not set")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	u, e := url.Parse(raw)
	if e != nil {
		t.Fatal("invalid integration database URL")
	}
	u.Path = "/postgres"
	proxy := os.Getenv("DATABASE_PROXY_URL")
	admin, e := repo.OpenDatabase(u.String(), proxy)
	if e != nil {
		t.Fatal(e)
	}
	defer admin.Close()
	name := "capability_hub_it_" + strings.ReplaceAll(uuid.NewString(), "-", "")
	if _, e = admin.ExecContext(ctx, `CREATE DATABASE "`+name+`"`); e != nil {
		t.Fatal("create disposable database:", e)
	}
	defer func() {
		cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cleanupCancel()
		// Only the UUID database created above is owned by this test. Force
		// disconnect any session still closing through a network proxy.
		if _, e := admin.ExecContext(cleanupCtx, `DROP DATABASE "`+name+`" WITH (FORCE)`); e != nil {
			t.Error("drop disposable database:", e)
		}
	}()
	u.Path = "/" + name
	db, e := repo.OpenDatabase(u.String(), proxy)
	if e != nil {
		t.Fatal(e)
	}
	defer db.Close()
	goose.SetBaseFS(migrations.EmbedMigrations)
	_ = goose.SetDialect("postgres")
	t.Setenv("EMBEDDING_DIM", "1024")
	if e = goose.UpToContext(ctx, db, ".", 13); e != nil {
		t.Fatal(e)
	}
	if e = goose.DownContext(ctx, db, "."); e != nil {
		t.Fatal(e)
	}
	var protocolAbsent bool
	if e = db.QueryRowContext(ctx, "SELECT NOT EXISTS (SELECT 1 FROM information_schema.columns WHERE table_name='user_llm_keys' AND column_name='protocol')").Scan(&protocolAbsent); e != nil || !protocolAbsent {
		t.Fatal("protocol rollback failed")
	}
	if e = goose.DownContext(ctx, db, "."); e != nil {
		t.Fatal(e)
	}
	var absent bool
	if e = db.QueryRowContext(ctx, "SELECT to_regclass('public.user_llm_keys') IS NULL").Scan(&absent); e != nil || !absent {
		t.Fatal("rollback did not remove new table")
	}
	if e = goose.UpContext(ctx, db, "."); e != nil {
		t.Fatal(e)
	}
	pool, e := repo.NewPool(ctx, u.String(), proxy)
	if e != nil {
		t.Fatal(e)
	}
	defer pool.Close()
	alice, bob := uuid.NewString(), uuid.NewString()
	if _, e = pool.Exec(ctx, "INSERT INTO users(id,display_name) VALUES ($1,'Alice'),($2,'Bob')", alice, bob); e != nil {
		t.Fatal(e)
	}
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer integration-only-key" {
			t.Error("wrong upstream credential")
		}
		if r.URL.Path == "/v1/models" {
			w.Header().Set("Content-Type", "application/json")
			fmt.Fprint(w, `{"data":[{"id":"model"}]}`)
			return
		}
		w.Header().Set("Content-Type", "text/event-stream")
		fmt.Fprint(w, "data: {\"choices\":[{\"delta\":{\"content\":\"cloud roundtrip ok\"}}]}\n\ndata: [DONE]\n\n")
	}))
	defer upstream.Close()
	client, _ := playground.NewHTTPClient(upstream.URL)
	protector, _ := authstore.NewAESGCMProtector([]byte("12345678901234567890123456789012"))
	service := playground.New(repo.NewPlaygroundRepository(pool), protector, client)
	routerFor := func(user string) http.Handler {
		hash := sha256.Sum256([]byte("csrf"))
		auth := newTestAuthHandler(t, &fakeAuthFlow{}, &fakeSessionStore{session: domain.Session{UserID: user, CSRFTokenHash: hash[:]}}, fakeUserResolver{user: domain.User{ID: user}})
		return NewPlatformRouter(auth, NewPlaygroundHandler(service))
	}
	call := func(user, method, path, body, csrf string) *httptest.ResponseRecorder {
		r := httptest.NewRequest(method, path, strings.NewReader(body))
		r.AddCookie(&http.Cookie{Name: SessionCookieName, Value: "session"})
		r.Header.Set("Origin", "https://app.example.test")
		r.Header.Set(CSRFHeaderName, csrf)
		w := httptest.NewRecorder()
		routerFor(user).ServeHTTP(w, r)
		return w
	}
	payload, _ := json.Marshal(playground.SaveRequest{BaseURL: upstream.URL + "/v1", APIKey: "integration-only-key", Models: []string{"model"}, DefaultModel: "model"})
	path := "/api/v1/playground/connection"
	if w := call(alice, "PUT", path, string(payload), "bad"); w.Code != 403 {
		t.Fatal("CSRF not enforced", w.Code)
	}
	w := call(alice, "PUT", path, string(payload), "csrf")
	if w.Code != 200 || strings.Contains(w.Body.String(), "integration-only-key") {
		t.Fatal("save/redaction failed", w.Code)
	}
	if w := call(bob, "GET", path, "", ""); strings.TrimSpace(w.Body.String()) != "null" {
		t.Fatal("cross-user config visible")
	}
	var cipher []byte
	if e = pool.QueryRow(ctx, "SELECT key_ciphertext FROM user_llm_keys WHERE user_id=$1", alice).Scan(&cipher); e != nil || bytes.Contains(cipher, []byte("integration-only-key")) {
		t.Fatal("key not encrypted")
	}
	if w := call(alice, "PUT", path, string(payload), "csrf"); w.Code != 409 {
		t.Fatal("stale configuration overwrote data", w.Code)
	}
	discoveryPayload, _ := json.Marshal(playground.ModelsRequest{BaseURL: upstream.URL + "/v1"})
	if w := call(alice, "POST", "/api/v1/playground/models", string(discoveryPayload), "bad"); w.Code != 403 {
		t.Fatal("discovery CSRF not enforced")
	}
	if w := call(alice, "POST", "/api/v1/playground/models", string(discoveryPayload), "csrf"); w.Code != 200 || !strings.Contains(w.Body.String(), `"models":["model"]`) {
		t.Fatal("discovery with owned key failed", w.Code)
	}
	if w := call(bob, "POST", "/api/v1/playground/models", string(discoveryPayload), "csrf"); w.Code != 400 {
		t.Fatal("discovery reused another user's key")
	}
	w = call(alice, "POST", "/api/v1/playground/chat/stream", `{"model":"model","messages":[{"role":"user","content":"hello"}]}`, "csrf")
	if w.Code != 200 || !strings.Contains(w.Body.String(), "cloud roundtrip ok") || !strings.Contains(w.Body.String(), "event: done") {
		t.Fatal("stream failed", w.Code, w.Body.String())
	}
	if w := call(bob, "DELETE", path, "", "csrf"); w.Code != 204 {
		t.Fatal(w.Code)
	}
	if w := call(alice, "GET", path, "", ""); strings.TrimSpace(w.Body.String()) == "null" {
		t.Fatal("cross-user delete")
	}
	if w := call(alice, "DELETE", path, "", "csrf"); w.Code != 204 {
		t.Fatal(w.Code)
	}
	t.Log("cloud migrations up/down/up, encrypted config, owner isolation, CSRF, conflict and model stream passed")
	if os.Getenv("PLAYGROUND_BROWSER_CHECK") == "1" {
		// Opt-in test fixture only: loopback, disposable users/database and a
		// bounded lifetime. This route is never linked into cmd/server.
		repository := repo.NewAuthRepository(pool)
		sessions := authstore.NewPersistentSessionStore(repository, nil)
		_, token, csrf, err := sessions.Create(ctx, alice, time.Now().Add(10*time.Minute))
		if err != nil {
			t.Fatal(err)
		}
		auth, err := NewAuthHandler(AuthHandlerConfig{AppID: "test", RedirectURL: "http://localhost:8080/api/v1/auth/feishu/callback", FrontendOrigin: "http://localhost:3000", FrontendPath: "/hub/playground"}, &fakeAuthFlow{}, sessions, repository)
		if err != nil {
			t.Fatal(err)
		}
		mux := http.NewServeMux()
		mux.Handle("/", NewPlatformRouter(auth, NewPlaygroundHandler(service)))
		mux.HandleFunc("GET /__test/session", func(w http.ResponseWriter, r *http.Request) {
			http.SetCookie(w, &http.Cookie{Name: SessionCookieName, Value: token, Path: "/", HttpOnly: true, SameSite: http.SameSiteLaxMode})
			http.SetCookie(w, &http.Cookie{Name: CSRFCookieName, Value: csrf, Path: "/", SameSite: http.SameSiteLaxMode})
			http.Redirect(w, r, "http://localhost:3000/hub/playground", 302)
		})
		finished := make(chan struct{}, 1)
		mux.HandleFunc("POST /__test/finish", func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(204)
			select {
			case finished <- struct{}{}:
			default:
			}
		})
		server := &http.Server{Addr: "127.0.0.1:8080", Handler: mux, ReadHeaderTimeout: 5 * time.Second}
		errors := make(chan error, 1)
		go func() { errors <- server.ListenAndServe() }()
		t.Logf("Browser fixture ready; session=http://localhost:8080/__test/session model_base=%s/v1 model=model", upstream.URL)
		select {
		case <-finished:
		case err := <-errors:
			t.Error(err)
		case <-time.After(8 * time.Minute):
			t.Error("browser fixture timed out")
		}
		_ = server.Close()
	}
}
