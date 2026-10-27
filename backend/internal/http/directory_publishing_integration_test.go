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
	"sync/atomic"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/pressly/goose/v3"
	authstore "github.com/zenith-wang/it-wiki/backend/internal/auth"
	"github.com/zenith-wang/it-wiki/backend/internal/directory"
	"github.com/zenith-wang/it-wiki/backend/internal/domain"
	"github.com/zenith-wang/it-wiki/backend/internal/infra/feishu"
	k "github.com/zenith-wang/it-wiki/backend/internal/knowledge"
	"github.com/zenith-wang/it-wiki/backend/internal/publishing"
	"github.com/zenith-wang/it-wiki/backend/internal/registry"
	"github.com/zenith-wang/it-wiki/backend/internal/repo"
	"github.com/zenith-wang/it-wiki/backend/internal/repo/migrations"
	authservice "github.com/zenith-wang/it-wiki/backend/internal/service"
)

type directoryOAuthFixture struct{ refreshes atomic.Int32 }

func (*directoryOAuthFixture) ExchangeCode(context.Context, string) (domain.OAuthToken, error) {
	panic("unused")
}
func (*directoryOAuthFixture) UserInfo(context.Context, string) (domain.FeishuIdentity, error) {
	panic("unused")
}
func (f *directoryOAuthFixture) RefreshToken(_ context.Context, refresh string) (domain.OAuthToken, error) {
	f.refreshes.Add(1)
	return domain.OAuthToken{AccessToken: "token-" + strings.TrimPrefix(refresh, "refresh-"), AccessTokenExpiresAt: time.Now().Add(time.Hour), Scopes: directory.RequiredScopes}, nil
}

// This test refuses external hosts and creates/drops only its own UUID database.
func TestDirectoryPublishingIntegration(t *testing.T) {
	raw := os.Getenv("DIRECTORY_TEST_DATABASE_URL")
	if raw == "" {
		t.Skip("DIRECTORY_TEST_DATABASE_URL not set")
	}
	u, e := url.Parse(raw)
	if e != nil || (u.Hostname() != "127.0.0.1" && u.Hostname() != "localhost") {
		t.Fatal("directory test requires a local disposable PostgreSQL")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()
	u.Path = "/postgres"
	admin, e := repo.OpenDatabase(u.String(), "")
	if e != nil {
		t.Fatal(e)
	}
	defer admin.Close()
	dbname := "hub_directory_api_it_" + strings.ReplaceAll(uuid.NewString(), "-", "")
	if _, e = admin.ExecContext(ctx, `CREATE DATABASE "`+dbname+`"`); e != nil {
		t.Fatal(e)
	}
	defer func() {
		if _, e := admin.ExecContext(context.Background(), `DROP DATABASE "`+dbname+`" WITH (FORCE)`); e != nil {
			t.Error(e)
		}
	}()
	u.Path = "/" + dbname
	db, e := repo.OpenDatabase(u.String(), "")
	if e != nil {
		t.Fatal(e)
	}
	defer db.Close()
	goose.SetBaseFS(migrations.EmbedMigrations)
	if e = goose.SetDialect("postgres"); e != nil {
		t.Fatal(e)
	}
	t.Setenv("EMBEDDING_DIM", "1024")
	if e = goose.UpToContext(ctx, db, ".", 17); e != nil {
		t.Fatal(e)
	}
	if _, e = db.ExecContext(ctx, `INSERT INTO capabilities(slug,type,name,description,owner_open_id,department,visibility) VALUES('legacy-fixture','mcp','Legacy','Preserve','ou-legacy','Original department','department')`); e != nil {
		t.Fatal(e)
	}
	if e = goose.UpContext(ctx, db, "."); e != nil {
		t.Fatal(e)
	}
	if e = goose.DownToContext(ctx, db, ".", 17); e != nil {
		t.Fatal(e)
	}
	var legacy string
	if e = db.QueryRowContext(ctx, `SELECT department FROM capabilities WHERE slug='legacy-fixture'`).Scan(&legacy); e != nil || legacy != "Original department" {
		t.Fatal("rollback changed old metadata", e)
	}
	if e = goose.UpContext(ctx, db, "."); e != nil {
		t.Fatal(e)
	}
	pool, e := repo.NewPool(ctx, u.String(), "")
	if e != nil {
		t.Fatal(e)
	}
	defer pool.Close()
	protector, e := authstore.NewAESGCMProtector([]byte("0123456789abcdef0123456789abcdef"))
	if e != nil {
		t.Fatal(e)
	}
	accounts := repo.NewAuthRepository(pool)
	users := map[string]string{}
	for _, name := range []string{"alice", "bob", "admin"} {
		encrypted, _ := protector.Encrypt("expired-token")
		refresh, _ := protector.Encrypt("refresh-ou_" + name)
		user, _, e := accounts.UpsertOAuthIdentity(ctx, domain.FeishuIdentity{OpenID: "ou_" + name, TenantKey: "fixture", DisplayName: name}, domain.OAuthAccount{Provider: "feishu", TenantKey: "fixture", AccessTokenEncrypted: encrypted, RefreshTokenEncrypted: refresh, AccessTokenExpiresAt: time.Now().Add(-time.Hour), Scopes: directory.RequiredScopes})
		if e != nil {
			t.Fatal(e)
		}
		users[name] = user.ID
	}
	registryRepo := repo.NewRegistryRepository(pool)
	if e = registryRepo.BootstrapAdmin(ctx, "ou_admin"); e != nil {
		t.Fatal(e)
	}
	oauth := &directoryOAuthFixture{}
	auth := authservice.NewAuth(authservice.AuthConfig{}, oauth, protector, nil, nil, accounts)
	var failBob atomic.Bool
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		token := r.Header.Get("Authorization")
		if !strings.HasPrefix(token, "Bearer token-ou_") {
			t.Error("not using refreshed current user token")
		}
		if r.URL.Query().Get("department_id_type") != "open_department_id" {
			t.Error("incorrect department ID type")
		}
		switch r.URL.Path {
		case "/open-apis/contact/v3/departments/0/children":
			if r.URL.Query().Get("page_token") == "next" {
				_, _ = w.Write([]byte(`{"code":0,"data":{"items":[],"has_more":false}}`))
				return
			}
			_, _ = w.Write([]byte(`{"code":0,"data":{"items":[{"open_department_id":"od-engineering","name":"Engineering"}],"has_more":true,"page_token":"next"}}`))
		case "/open-apis/contact/v3/departments/od-engineering":
			_, _ = w.Write([]byte(`{"code":0,"data":{"department":{"open_department_id":"od-engineering","name":"Engineering"}}}`))
		case "/open-apis/contact/v3/users/find_by_department":
			_, _ = w.Write([]byte(`{"code":0,"data":{"items":[{"open_id":"ou_bob","name":"Bob"}]}}`))
		case "/open-apis/contact/v3/users/ou_alice":
			_, _ = w.Write([]byte(`{"code":0,"data":{"user":{"open_id":"ou_alice","name":"Alice","department_ids":["od-sales"]}}}`))
		case "/open-apis/contact/v3/users/ou_admin":
			_, _ = w.Write([]byte(`{"code":0,"data":{"user":{"open_id":"ou_admin","name":"Admin","department_ids":[]}}}`))
		case "/open-apis/contact/v3/users/ou_bob":
			if failBob.Load() {
				w.WriteHeader(403)
				_, _ = w.Write([]byte(`{"code":41050}`))
				return
			}
			_, _ = w.Write([]byte(`{"code":0,"data":{"user":{"open_id":"ou_bob","name":"Bob","department_ids":["od-engineering"]}}}`))
		default:
			w.WriteHeader(400)
			_, _ = w.Write([]byte(`{"code":40004,"msg":"SECRET"}`))
		}
	}))
	defer upstream.Close()
	directoryService := directory.New(accounts, auth, feishu.NewClient(feishu.ClientConfig{BaseURL: upstream.URL, MaxRetries: -1}, upstream.Client()), accounts)
	storage := &registryMemoryStorage{objects: map[string][]byte{}}
	registryService := registry.New(registryRepo, storage).WithDirectory(directoryService)
	credentials := publishing.New(repo.NewPublishingRepository(pool))
	preview := false
	routerFor := func(name string) http.Handler {
		hash := sha256.Sum256([]byte("csrf"))
		user := users[name]
		sessionAuth := newTestAuthHandler(t, &fakeAuthFlow{}, &fakeSessionStore{session: domain.Session{UserID: user, CSRFTokenHash: hash[:]}}, fakeUserResolver{user: domain.User{ID: user}})
		if preview {
			sessionAuth.frontendOrigin = "http://localhost:13000"
		}
		return NewCapabilityHubRouter(Handlers{Auth: sessionAuth, Registry: NewRegistryHandler(registryService), Directory: NewDirectoryHandler(directoryService), Publishing: NewPublishingHandler(credentials)})
	}
	call := func(name, token, method, path string, input any, files []registry.File, csrf bool) *httptest.ResponseRecorder {
		t.Helper()
		var body []byte
		content := "application/json"
		if files != nil {
			body, content = registryMultipart(t, input, files)
		} else if input != nil {
			body, _ = json.Marshal(input)
		}
		r := httptest.NewRequest(method, path, bytes.NewReader(body))
		r.Header.Set("Content-Type", content)
		if token != "" {
			r.Header.Set("Authorization", "Bearer "+token)
		}
		if name != "" {
			r.AddCookie(&http.Cookie{Name: SessionCookieName, Value: "fixture"})
		}
		if csrf {
			r.Header.Set("Origin", "https://app.example.test")
			r.Header.Set(CSRFHeaderName, "csrf")
		}
		w := httptest.NewRecorder()
		routerFor(name).ServeHTTP(w, r)
		return w
	}
	check := func(w *httptest.ResponseRecorder, want int) {
		t.Helper()
		if w.Code != want {
			t.Fatalf("want %d, got %d: %s", want, w.Code, w.Body.String())
		}
	}
	detail := func(w *httptest.ResponseRecorder) registry.Detail {
		t.Helper()
		var d registry.Detail
		if e := json.Unmarshal(w.Body.Bytes(), &d); e != nil {
			t.Fatal(e)
		}
		return d
	}
	issue := func(name string, scopes []string) publishing.Issued {
		t.Helper()
		w := call(name, "", "POST", "/api/v1/publishing/credentials", publishing.CreateRequest{Name: name + " CI", Scopes: scopes, ExpiresAt: time.Now().Add(time.Hour)}, nil, true)
		check(w, 201)
		var c publishing.Issued
		if e := json.Unmarshal(w.Body.Bytes(), &c); e != nil {
			t.Fatal(e)
		}
		return c
	}
	const root = "/api/v1/open/capabilities"
	alice := issue("alice", publishing.Scopes)
	bob := issue("bob", publishing.Scopes)
	readOnly := issue("alice", []string{"capabilities:read"})
	writer := issue("alice", []string{"capabilities:write"})
	check(call("alice", "", "POST", "/api/v1/publishing/credentials", publishing.CreateRequest{}, nil, false), 403)
	check(call("", alice.Token, "GET", "/api/v1/publishing/credentials", nil, nil, false), 401)
	check(call("alice", "", "GET", root, nil, nil, false), 401)      // Cookie never authenticates machine API.
	check(call("alice", "wrong", "GET", root, nil, nil, false), 401) // No cookie fallback.
	w := call("alice", "", "GET", "/api/v1/directory/departments", nil, nil, false)
	check(w, 200)
	if !strings.Contains(w.Body.String(), "od-engineering") {
		t.Fatal("missing visible department")
	}
	check(call("alice", "", "GET", "/api/v1/directory/departments?page_token=next", nil, nil, false), 200)
	if oauth.refreshes.Load() != 1 {
		t.Fatal("directory did not reuse encrypted OAuth refresh")
	}
	a, _ := accounts.FeishuAccount(ctx, users["alice"])
	m, _ := accounts.Membership(ctx, a.ID)
	if len(m.DepartmentIDs) > 0 {
		t.Fatal("directory listing granted department membership")
	}
	input := registry.SaveRequest{Slug: "api-mcp", Type: "mcp", Name: "API MCP", Description: "Fixture", Visibility: "department", DepartmentID: "od-engineering", Department: "Untrusted name", Version: "1", MCPEndpoint: "https://example.test/mcp", MCPTransport: "streamable-http", MCPAuthScheme: "none"}
	check(call("", readOnly.Token, "POST", root, input, nil, false), 403)
	input.DepartmentID = "od-hidden"
	check(call("", alice.Token, "POST", root, input, nil, false), 403)
	input.DepartmentID = "od-engineering"
	w = call("", alice.Token, "POST", root, input, nil, false)
	check(w, 201)
	d := detail(w)
	if d.Department != "Engineering" || d.DepartmentID != "od-engineering" || d.OwnerOpenID != "ou_alice" {
		t.Fatal("client metadata or owner override accepted", d)
	}
	input.Revision = d.Revision
	check(call("", bob.Token, "PUT", root+"/api-mcp", input, nil, false), 404)
	w = call("", alice.Token, "PUT", root+"/api-mcp", input, nil, false)
	check(w, 200)
	d = detail(w)
	check(call("", alice.Token, "PUT", root+"/api-mcp", input, nil, false), 409)
	action := registry.ActionRequest{Revision: d.Revision, VersionID: d.DraftVersionID}
	check(call("", writer.Token, "POST", root+"/api-mcp/submit", action, nil, false), 403)
	w = call("", alice.Token, "POST", root+"/api-mcp/submit", action, nil, false)
	check(w, 200)
	d = detail(w)
	if d.Status != "in_review" || d.IsLive {
		t.Fatal("submit bypassed review")
	}
	check(call("", alice.Token, "POST", root+"/api-mcp/approve", action, nil, false), 404)
	check(call("alice", "", "POST", "/api/v1/capabilities/api-mcp/approve", registry.ActionRequest{Revision: d.Revision, VersionID: d.DraftVersionID}, nil, true), 403)
	w = call("admin", "", "POST", "/api/v1/capabilities/api-mcp/approve", registry.ActionRequest{Revision: d.Revision, VersionID: d.DraftVersionID}, nil, true)
	check(w, 200)
	d = detail(w)
	check(call("bob", "", "GET", "/api/v1/catalog/api-mcp", nil, nil, false), 200)
	who, e := registryService.Identity(ctx, users["alice"])
	if e != nil || len(who.DepartmentIDs) != 1 || who.DepartmentIDs[0] != "od-sales" {
		t.Fatal("visibility became membership", who, e)
	}
	// Revoke actual membership, expire the snapshot, fail its refresh: no stale access.
	if _, e = pool.Exec(ctx, `UPDATE feishu_memberships SET expires_at=now()-interval '1 second' WHERE account_id=(SELECT id FROM oauth_accounts WHERE user_id=$1)`, users["bob"]); e != nil {
		t.Fatal(e)
	}
	failBob.Store(true)
	check(call("bob", "", "GET", "/api/v1/catalog/api-mcp", nil, nil, false), 404)
	w = call("bob", "", "GET", "/api/v1/catalog", nil, nil, false)
	check(w, 200)
	if strings.Contains(w.Body.String(), "api-mcp") {
		t.Fatal("catalog leaked stale membership")
	}
	failBob.Store(false)
	// Knowledge uses the same ID membership, and never a display-name match.
	kbRepo := repo.NewKnowledgeRepository(pool, nil)
	kb, e := kbRepo.SaveKB(ctx, k.Principal{UserID: users["alice"]}, "", k.KB{Name: "Directory fixture KB", Visibility: "department", Department: "Engineering", DepartmentID: "od-engineering"}, "fixture", 1024)
	if e != nil {
		t.Fatal(e)
	}
	kbs, e := kbRepo.ListKB(ctx, k.Principal{UserID: users["bob"]})
	if e != nil || len(kbs) != 0 {
		t.Fatal("knowledge accepted stale membership", kbs, e)
	}
	if e = directoryService.EnsureMembership(ctx, users["bob"]); e != nil {
		t.Fatal(e)
	}
	kbs, e = kbRepo.ListKB(ctx, k.Principal{UserID: users["bob"]})
	if e != nil || len(kbs) != 1 || kbs[0].ID != kb.ID || kbs[0].DepartmentID != "od-engineering" {
		t.Fatal("knowledge ID permission mismatch", kbs, e)
	}
	// Owner can make a new candidate without changing the published snapshot.
	w = call("", alice.Token, "POST", root+"/api-mcp/new-draft", registry.ActionRequest{Revision: d.Revision, VersionID: d.CurrentVersionID}, nil, false)
	check(w, 200)
	d = detail(w)
	input.Revision = d.Revision
	input.Version = "2"
	w = call("", alice.Token, "PUT", root+"/api-mcp", input, nil, false)
	check(w, 200)
	if !detail(w).IsLive || detail(w).CurrentVersionID != d.CurrentVersionID {
		t.Fatal("draft replaced live version")
	}
	skill := registry.SaveRequest{Slug: "api-skill", Type: "skill", Name: "Skill", Description: "Fixture", Visibility: "allowlist", Allowlist: []string{"ou_bob"}, Version: "1"}
	files := []registry.File{{Name: "SKILL.md", Data: []byte("---\nname: api-skill\ndescription: Example\n---\nUse this skill.")}}
	w = call("", alice.Token, "POST", root, skill, files, false)
	check(w, 201)
	sd := detail(w)
	w = call("", alice.Token, "POST", root+"/api-skill/submit", registry.ActionRequest{Revision: sd.Revision, VersionID: sd.DraftVersionID}, nil, false)
	check(w, 200)
	check(call("", bob.Token, "GET", root+"/api-skill/versions/"+sd.DraftVersionID+"/bundle", nil, nil, false), 404)
	check(call("", alice.Token, "GET", root+"/api-skill/versions/"+sd.DraftVersionID+"/bundle", nil, nil, false), 200)
	skill.Slug = "bad-skill"
	files[0].Name = "../SKILL.md"
	check(call("", alice.Token, "POST", root, skill, files, false), 400)
	// List never returns secrets; credentials belong only to their creator.
	w = call("alice", "", "GET", "/api/v1/publishing/credentials", nil, nil, false)
	check(w, 200)
	if strings.Contains(w.Body.String(), alice.Token) || strings.Contains(w.Body.String(), "token_hash") || strings.Contains(w.Body.String(), bob.ID) {
		t.Fatal("credential list leak")
	}
	check(call("bob", "", "DELETE", "/api/v1/publishing/credentials/"+alice.ID, nil, nil, true), 404)
	check(call("alice", "", "DELETE", "/api/v1/publishing/credentials/"+alice.ID, nil, nil, true), 204)
	check(call("", alice.Token, "GET", root, nil, nil, false), 401)
	if _, e = pool.Exec(ctx, `UPDATE publishing_credentials SET created_at=now()-interval '2 hours',expires_at=now()-interval '1 hour' WHERE id=$1`, readOnly.ID); e != nil {
		t.Fatal(e)
	}
	check(call("", readOnly.Token, "GET", root, nil, nil, false), 401)
	var auditCount int
	if e = pool.QueryRow(ctx, `SELECT count(*) FROM audit_log WHERE detail->>'api_credential_id'=$1 AND action IN ('create','edit','submit')`, alice.ID).Scan(&auditCount); e != nil || auditCount < 5 {
		t.Fatal("API audit attribution missing", auditCount, e)
	}
	var hash []byte
	if e = pool.QueryRow(ctx, `SELECT token_hash FROM publishing_credentials WHERE id=$1`, alice.ID).Scan(&hash); e != nil {
		t.Fatal(e)
	}
	want := sha256.Sum256([]byte(alice.Token))
	if !bytes.Equal(hash, want[:]) {
		t.Fatal("credential not stored as digest")
	}
	if os.Getenv("DIRECTORY_BROWSER_CHECK") == "1" {
		preview = true
		runDirectoryBrowserFixture(t, routerFor("alice"))
	}
	t.Log("isolated migrations 17->19->17->19, OAuth refresh, visible directory vs membership, account/scope isolation, MCP/Skill registration/update/upload/submit, review, stale memberships, expiry/revocation and audit passed")
}

func runDirectoryBrowserFixture(t *testing.T, router http.Handler) {
	t.Helper()
	mux := http.NewServeMux()
	mux.Handle("/", router)
	mux.HandleFunc("GET /__test/session", func(w http.ResponseWriter, r *http.Request) {
		http.SetCookie(w, &http.Cookie{Name: SessionCookieName, Value: "fixture", Path: "/", HttpOnly: true})
		http.SetCookie(w, &http.Cookie{Name: CSRFCookieName, Value: "csrf", Path: "/"})
		http.Redirect(w, r, "http://localhost:13000/hub/api-docs", 302)
	})
	server := &http.Server{Addr: "127.0.0.1:18089", Handler: mux, ReadHeaderTimeout: 5 * time.Second}
	done := make(chan struct{}, 1)
	mux.HandleFunc("POST /__test/finish", func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(204); done <- struct{}{} })
	errs := make(chan error, 1)
	go func() { errs <- server.ListenAndServe() }()
	fmt.Println("directory browser fixture ready at 127.0.0.1:18089")
	select {
	case <-done:
	case e := <-errs:
		t.Error(e)
	case <-time.After(10 * time.Minute):
		t.Error("browser fixture timeout")
	}
	_ = server.Close()
}
