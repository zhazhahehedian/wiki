package http

import (
	"archive/zip"
	"bytes"
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/json"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/pressly/goose/v3"
	"github.com/zenith-wang/it-wiki/backend/internal/domain"
	"github.com/zenith-wang/it-wiki/backend/internal/domain/ports"
	"github.com/zenith-wang/it-wiki/backend/internal/infra/storage"
	"github.com/zenith-wang/it-wiki/backend/internal/playground"
	"github.com/zenith-wang/it-wiki/backend/internal/registry"
	"github.com/zenith-wang/it-wiki/backend/internal/repo"
	"github.com/zenith-wang/it-wiki/backend/internal/repo/migrations"
	"github.com/zenith-wang/it-wiki/backend/internal/skillbuilder"
)

type registryMemoryStorage struct{ objects map[string][]byte }

func (s *registryMemoryStorage) Put(_ context.Context, key string, r io.Reader, _ int64, _ string) error {
	data, e := io.ReadAll(r)
	s.objects[key] = data
	return e
}
func (s *registryMemoryStorage) Get(_ context.Context, key string) (io.ReadCloser, error) {
	b, ok := s.objects[key]
	if !ok {
		return nil, registry.ErrNotFound
	}
	return io.NopCloser(bytes.NewReader(b)), nil
}
func (s *registryMemoryStorage) Delete(_ context.Context, key string) error {
	delete(s.objects, key)
	return nil
}
func registryMultipart(t *testing.T, input any, files []registry.File) ([]byte, string) {
	t.Helper()
	var b bytes.Buffer
	w := multipart.NewWriter(&b)
	p, e := w.CreateFormField("metadata")
	if e != nil {
		t.Fatal(e)
	}
	if e = json.NewEncoder(p).Encode(input); e != nil {
		t.Fatal(e)
	}
	for _, file := range files {
		p, e = w.CreateFormFile("files", file.Name)
		if e != nil {
			t.Fatal(e)
		}
		_, _ = p.Write(file.Data)
	}
	_ = w.Close()
	return b.Bytes(), w.FormDataContentType()
}
func TestRegistryCloudIntegration(t *testing.T)   { runRegistryCloudIntegration(t, false) }
func TestGovernanceCloudIntegration(t *testing.T) { runRegistryCloudIntegration(t, true) }
func runRegistryCloudIntegration(t *testing.T, governanceOnly bool) {
	raw := os.Getenv("REGISTRY_TEST_DATABASE_URL")
	if raw == "" {
		t.Skip("REGISTRY_TEST_DATABASE_URL not set")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Minute)
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
	name := "capability_hub_registry_it_" + strings.ReplaceAll(uuid.NewString(), "-", "")
	if _, e = admin.ExecContext(ctx, `CREATE DATABASE "`+name+`"`); e != nil {
		t.Fatal("create disposable database:", e)
	}
	defer func() {
		c, done := context.WithTimeout(context.Background(), 20*time.Second)
		defer done()
		if _, e := admin.ExecContext(c, `DROP DATABASE "`+name+`" WITH (FORCE)`); e != nil {
			t.Error("cleanup disposable database:", e)
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
	if e = goose.UpToContext(ctx, db, ".", 14); e != nil {
		t.Fatal(e)
	}
	if e = goose.DownContext(ctx, db, "."); e != nil {
		t.Fatal(e)
	}
	var absent bool
	if e = db.QueryRowContext(ctx, `SELECT to_regclass('public.capabilities') IS NULL AND to_regclass('public.capability_versions') IS NULL AND to_regclass('public.capability_allowlist') IS NULL AND to_regclass('public.user_llm_keys') IS NOT NULL`).Scan(&absent); e != nil || !absent {
		t.Fatal("registry rollback failed or affected playground")
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
	for _, id := range []string{alice, bob} {
		if _, e = pool.Exec(ctx, `INSERT INTO users(id,display_name) VALUES ($1,'Registry test owner')`, id); e != nil {
			t.Fatal(e)
		}
		if _, e = pool.Exec(ctx, `INSERT INTO oauth_accounts(user_id,provider,provider_user_id,tenant_key,access_token_encrypted,access_token_expires_at) VALUES ($1,'feishu',$2,'fixture',decode('00','hex'),now()+interval '1 hour')`, id, "ou_"+id); e != nil {
			t.Fatal(e)
		}
	}
	var objectStorage ports.ObjectStorage = &registryMemoryStorage{objects: map[string][]byte{}}
	if os.Getenv("REGISTRY_TEST_S3") == "1" {
		mc, err := storage.NewMinioClient(ctx, storage.MinioConfig{Endpoint: os.Getenv("S3_ENDPOINT"), AccessKey: os.Getenv("S3_ACCESS_KEY"), SecretKey: os.Getenv("S3_SECRET_KEY"), Bucket: os.Getenv("S3_BUCKET"), Region: os.Getenv("S3_REGION"), UsePathStyle: true})
		if err != nil {
			t.Fatal(err)
		}
		objectStorage = mc
	}
	repository := repo.NewRegistryRepository(pool)
	service := registry.New(repository, objectStorage)
	// Objects created by this test have random keys. Track surviving references
	// before dropping the database; never delete shared prefixes or existing data.
	defer func() {
		c, done := context.WithTimeout(context.Background(), 20*time.Second)
		defer done()
		rows, err := pool.Query(c, `SELECT skill_bundle_key FROM capability_versions WHERE skill_bundle_key IS NOT NULL`)
		if err != nil {
			t.Error(err)
			return
		}
		defer rows.Close()
		for rows.Next() {
			var key string
			if err = rows.Scan(&key); err != nil {
				t.Error(err)
				continue
			}
			if err = objectStorage.Delete(c, key); err != nil {
				t.Error("cleanup fixture object:", err)
			}
		}
	}()
	if governanceOnly {
		_, e := service.Save(ctx, alice, "", registry.SaveRequest{Slug: "migration-preserved", Type: "mcp", Name: "Migration fixture", Description: "Retain existing draft", Visibility: "org", Version: "1", MCPEndpoint: "https://mcp.example.test/mcp", MCPTransport: "streamable-http", MCPAuthScheme: "none"}, nil)
		if e != nil {
			t.Fatal(e)
		}
		verifyGovernanceMigration(t, ctx, db)
		users := verifyGovernanceIntegration(t, pool, service, repository, alice, bob)
		if os.Getenv("REGISTRY_BROWSER_CHECK") == "1" {
			model := verifySkillBuilderIntegration(t, pool, service, alice, bob)
			runRegistryBrowserFixture(t, service, model, users)
		}
		return
	}
	routerFor := func(user string) http.Handler {
		hash := sha256.Sum256([]byte("csrf"))
		auth := newTestAuthHandler(t, &fakeAuthFlow{}, &fakeSessionStore{session: domain.Session{UserID: user, CSRFTokenHash: hash[:]}}, fakeUserResolver{user: domain.User{ID: user}})
		return NewCapabilityHubRouter(Handlers{Auth: auth, Registry: NewRegistryHandler(service)})
	}
	call := func(user, method, path string, input any, files []registry.File, csrf string) *httptest.ResponseRecorder {
		var b []byte
		contentType := ""
		if input != nil {
			b, contentType = registryMultipart(t, input, files)
		}
		req := httptest.NewRequest(method, path, bytes.NewReader(b))
		if user != "" {
			req.AddCookie(&http.Cookie{Name: SessionCookieName, Value: "session"})
		}
		req.Header.Set("Origin", "https://app.example.test")
		req.Header.Set(CSRFHeaderName, csrf)
		req.Header.Set("Content-Type", contentType)
		w := httptest.NewRecorder()
		routerFor(user).ServeHTTP(w, req)
		return w
	}
	requireStatus := func(w *httptest.ResponseRecorder, status int) {
		t.Helper()
		if w.Code != status {
			t.Fatalf("expected %d, got %d: %s", status, w.Code, w.Body.String())
		}
	}
	decode := func(w *httptest.ResponseRecorder) registry.Detail {
		t.Helper()
		var d registry.Detail
		if e := json.Unmarshal(w.Body.Bytes(), &d); e != nil {
			t.Fatal(e)
		}
		return d
	}
	path := "/api/v1/capabilities"
	input := registry.SaveRequest{Slug: "team-search", Type: "mcp", Name: "Team search", Description: "Internal search", Department: "Engineering", Visibility: "allowlist", Allowlist: []string{"ou_" + bob}, Version: "1.0.0", MCPEndpoint: "https://mcp.example.test/mcp", MCPTransport: "streamable-http", MCPAuthScheme: "bearer", Tools: []registry.Tool{{Name: "search", Description: "Search", InputSchema: []byte(`{"type":"object","properties":{"q":{"type":"string"}}}`)}}}
	requireStatus(call("", "GET", path, nil, nil, ""), 401)
	requireStatus(call(alice, "POST", path, input, nil, "bad"), 403)
	spoof := map[string]any{"owner_open_id": "ou_" + bob}
	requireStatus(call(alice, "POST", path, spoof, nil, "csrf"), 400)
	w := call(alice, "POST", path, input, nil, "csrf")
	requireStatus(w, 201)
	first := decode(w)
	if first.OwnerOpenID != "ou_"+alice || first.Status != "draft" || first.CurrentVersionID != "" || first.DraftVersionID == "" || first.Revision != 1 || len(first.Versions) != 1 {
		t.Fatal("invalid initial draft", first)
	}
	requireStatus(call(alice, "POST", path, input, nil, "csrf"), 409)
	requireStatus(call(bob, "GET", path+"/team-search", nil, nil, ""), 404)
	w = call(bob, "GET", path, nil, nil, "")
	requireStatus(w, 200)
	if strings.Contains(w.Body.String(), "team-search") {
		t.Fatal("draft leaked to allowlisted consumer")
	}
	input.Revision = 1
	input.Name = "Revised search"
	requireStatus(call(bob, "PUT", path+"/team-search", input, nil, "csrf"), 404)
	w = call(alice, "PUT", path+"/team-search", input, nil, "csrf")
	requireStatus(w, 200)
	updated := decode(w)
	if updated.Revision != 2 || len(updated.Versions) != 1 || updated.DraftVersionID != first.DraftVersionID {
		t.Fatal("draft edit created duplicate version")
	}
	requireStatus(call(alice, "PUT", path+"/team-search", input, nil, "csrf"), 409)
	input.Revision = 2
	input.Version = "2.0.0"
	input.MCPEndpoint = "https://mcp.example.test/v2"
	// The same expected revision can only win once, even with concurrent requests.
	var wg sync.WaitGroup
	codes := make(chan int, 2)
	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func() { defer wg.Done(); codes <- call(alice, "PUT", path+"/team-search", input, nil, "csrf").Code }()
	}
	wg.Wait()
	close(codes)
	counts := map[int]int{}
	for code := range codes {
		counts[code]++
	}
	if counts[200] != 1 || counts[409] != 1 {
		t.Fatal("concurrent revision conflict failed", counts)
	}
	latest := decode(call(alice, "GET", path+"/team-search", nil, nil, ""))
	if latest.Revision != 3 || len(latest.Versions) != 2 || latest.DraftVersionID == first.DraftVersionID {
		t.Fatal("new version failed")
	}
	input.Revision = 3
	input.Version = "1.0.0"
	requireStatus(call(alice, "PUT", path+"/team-search", input, nil, "csrf"), 409)
	input.Version = "3.0.0"
	input.Visibility = "department"
	input.Allowlist = nil
	// A late SQL failure must roll back metadata, version and allowlist together.
	if _, e = pool.Exec(ctx, `ALTER TABLE capability_versions ADD CONSTRAINT fixture_reject_version CHECK(version <> '3.0.0')`); e != nil {
		t.Fatal(e)
	}
	requireStatus(call(alice, "PUT", path+"/team-search", input, nil, "csrf"), 500)
	after := decode(call(alice, "GET", path+"/team-search", nil, nil, ""))
	if after.Revision != 3 || after.Visibility != "allowlist" || len(after.Allowlist) != 1 || len(after.Versions) != 2 {
		t.Fatal("transaction partially committed")
	}
	if _, e = pool.Exec(ctx, `ALTER TABLE capability_versions DROP CONSTRAINT fixture_reject_version`); e != nil {
		t.Fatal(e)
	}
	requireStatus(call(alice, "GET", path+"?limit=2147483648", nil, nil, ""), 400)
	requireStatus(call(alice, "GET", path+"?status=invalid", nil, nil, ""), 400)
	w = call(alice, "GET", path+"?q=Revised&type=mcp&department=Engineering&status=draft&limit=1", nil, nil, "")
	requireStatus(w, 200)
	if !strings.Contains(w.Body.String(), "team-search") {
		t.Fatal("filter lost owned row")
	}
	w = call(alice, "GET", path+"?q=%25", nil, nil, "")
	requireStatus(w, 200)
	if strings.Contains(w.Body.String(), "team-search") {
		t.Fatal("wildcard search was not literal")
	}
	skill := registry.SaveRequest{Slug: "report-skill", Type: "skill", Name: "Report skill", Description: "Prepare a report", Visibility: "org", Version: "1"}
	files := []registry.File{{Name: "SKILL.md", Data: []byte("---\nname: report-skill\ndescription: Prepare a report\n---\n# Instructions\nWrite a report.")}, {Name: "references/template.md", Data: []byte("# Report")}}
	requireStatus(call(alice, "POST", path, skill, nil, "csrf"), 400)
	requireStatus(call(alice, "POST", path, skill, []registry.File{{Name: "SKILL.md", Data: []byte("hi")}, {Name: "../bad", Data: []byte("x")}}, "csrf"), 400)
	w = call(alice, "POST", path, skill, files, "csrf")
	requireStatus(w, 201)
	sd := decode(w)
	if strings.Contains(w.Body.String(), "registry/") || !sd.Versions[0].HasBundle {
		t.Fatal("storage keys exposed or missing bundle")
	}
	bundlePath := path + "/report-skill/versions/" + sd.DraftVersionID + "/bundle"
	requireStatus(call(bob, "GET", bundlePath, nil, nil, ""), 404)
	w = call(alice, "GET", bundlePath, nil, nil, "")
	requireStatus(w, 200)
	z, e := zip.NewReader(bytes.NewReader(w.Body.Bytes()), int64(w.Body.Len()))
	if e != nil || len(z.File) != 2 || z.File[1].Name != "report-skill/references/template.md" {
		t.Fatal("skill zip roundtrip failed", e)
	}
	skill.Revision = 1
	skill.Name = "Updated report"
	requireStatus(call(alice, "PUT", path+"/report-skill", skill, nil, "csrf"), 200)
	skill.Revision = 2
	skill.Version = "2"
	requireStatus(call(alice, "PUT", path+"/report-skill", skill, nil, "csrf"), 400)
	requireStatus(call(alice, "PUT", path+"/report-skill", skill, files, "csrf"), 200)
	requireStatus(call(alice, "GET", bundlePath, nil, nil, ""), 200)
	w = call(alice, "GET", path+"?limit=1", nil, nil, "")
	requireStatus(w, 200)
	if !strings.Contains(w.Body.String(), `"has_more":true`) {
		t.Fatal("pagination boundary failed")
	}
	if _, e = pool.Exec(ctx, `UPDATE capabilities SET status='in_review' WHERE slug='team-search'`); e != nil {
		t.Fatal(e)
	}
	requireStatus(call(alice, "PUT", path+"/team-search", input, nil, "csrf"), 409)
	// A current pointer may never reference another capability's version.
	if _, e = pool.Exec(ctx, `UPDATE capabilities SET current_version_id=$1 WHERE slug='team-search'`, sd.DraftVersionID); e == nil {
		t.Fatal("cross-capability current pointer accepted")
	}
	t.Log("registry migration up/down/up, Owner isolation, CSRF, atomic save/rollback, concurrent conflict, history, filters and Skill zip roundtrip passed")
	verifyGovernanceMigration(t, ctx, db)
	governanceUsers := verifyGovernanceIntegration(t, pool, service, repository, alice, bob)
	model := verifySkillBuilderIntegration(t, pool, service, alice, bob)
	if os.Getenv("REGISTRY_BROWSER_CHECK") == "1" {
		runRegistryBrowserFixture(t, service, model, governanceUsers)
	}
}
func runRegistryBrowserFixture(t *testing.T, service *registry.Service, model *playground.Service, users map[string]string) {
	// Opt-in multi-user fixture, loopback and disposable database only.
	hash := sha256.Sum256([]byte("csrf"))
	routers := map[string]http.Handler{}
	for role, id := range users {
		auth, err := NewAuthHandler(AuthHandlerConfig{AppID: "test", RedirectURL: "http://localhost:18089/api/v1/auth/feishu/callback", FrontendOrigin: "http://localhost:13000", FrontendPath: "/hub/registry"}, &fakeAuthFlow{}, &fakeSessionStore{session: domain.Session{UserID: id, CSRFTokenHash: hash[:]}}, fakeUserResolver{user: domain.User{ID: id, DisplayName: map[string]string{"owner": "能力维护者", "admin": "平台管理员", "consumer": "研发使用者", "outsider": "其他部门用户"}[role]}})
		if err != nil {
			t.Fatal(err)
		}
		routers[role] = NewCapabilityHubRouter(Handlers{Auth: auth, Registry: NewRegistryHandler(service), Playground: NewPlaygroundHandler(model), SkillBuilder: NewSkillBuilderHandler(skillbuilder.New(model, service))})
	}
	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		actor := "owner"
		if c, e := r.Cookie("fixture_actor"); e == nil {
			actor = c.Value
		}
		router, ok := routers[actor]
		if !ok {
			http.Error(w, "invalid fixture actor", 400)
			return
		}
		router.ServeHTTP(w, r)
	})
	mux.HandleFunc("GET /__test/session", func(w http.ResponseWriter, r *http.Request) {
		actor := r.URL.Query().Get("as")
		if actor == "" {
			actor = "owner"
		}
		if _, ok := users[actor]; !ok {
			http.Error(w, "unknown fixture actor", 400)
			return
		}
		http.SetCookie(w, &http.Cookie{Name: SessionCookieName, Value: "fixture", Path: "/", HttpOnly: true})
		http.SetCookie(w, &http.Cookie{Name: "fixture_actor", Value: actor, Path: "/", HttpOnly: true})
		http.SetCookie(w, &http.Cookie{Name: CSRFCookieName, Value: "csrf", Path: "/"})
		http.Redirect(w, r, "http://localhost:13000/hub/registry", 302)
	})
	done := make(chan struct{}, 1)
	mux.HandleFunc("POST /__test/finish", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(204)
		select {
		case done <- struct{}{}:
		default:
		}
	})
	server := &http.Server{Addr: "127.0.0.1:18089", Handler: mux, ReadHeaderTimeout: 5 * time.Second}
	errs := make(chan error, 1)
	go func() { errs <- server.ListenAndServe() }()
	t.Log("registry browser fixture ready at 127.0.0.1:18089")
	select {
	case <-done:
	case err := <-errs:
		t.Error(err)
	case <-time.After(12 * time.Minute):
		t.Error("browser fixture timed out")
	}
	_ = server.Close()
}

func verifyGovernanceMigration(t *testing.T, ctx context.Context, db *sql.DB) {
	t.Helper()
	// C rollback must retain B capabilities, users and playground foundations.
	var absent bool
	var beforeCaps, beforeVersions, beforeUsers int
	if e := db.QueryRowContext(ctx, `SELECT (SELECT count(*) FROM capabilities),(SELECT count(*) FROM capability_versions),(SELECT count(*) FROM users)`).Scan(&beforeCaps, &beforeVersions, &beforeUsers); e != nil {
		t.Fatal(e)
	}
	if e := goose.DownContext(ctx, db, "."); e != nil {
		t.Fatal(e)
	}
	var afterCaps, afterVersions, afterUsers int
	if e := db.QueryRowContext(ctx, `SELECT (SELECT count(*) FROM capabilities),(SELECT count(*) FROM capability_versions),(SELECT count(*) FROM users)`).Scan(&afterCaps, &afterVersions, &afterUsers); e != nil {
		t.Fatal(e)
	}
	if beforeCaps != afterCaps || beforeVersions != afterVersions || beforeUsers != afterUsers {
		t.Fatal("governance rollback changed preexisting records")
	}
	if e := db.QueryRowContext(ctx, `SELECT to_regclass('public.audit_log') IS NULL AND to_regclass('public.platform_profiles') IS NULL AND to_regclass('public.user_llm_keys') IS NOT NULL`).Scan(&absent); e != nil || !absent {
		t.Fatal("governance rollback did not isolate its tables", e)
	}
	if e := goose.UpContext(ctx, db, "."); e != nil {
		t.Fatal(e)
	}

}
