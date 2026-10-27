package http

import (
	"archive/zip"
	"bufio"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/pressly/goose/v3"
	"github.com/riverqueue/river"
	"github.com/riverqueue/river/riverdriver/riverpgxv5"
	"github.com/riverqueue/river/rivermigrate"
	"github.com/zenith-wang/it-wiki/backend/internal/domain"
	"github.com/zenith-wang/it-wiki/backend/internal/domain/ports"
	"github.com/zenith-wang/it-wiki/backend/internal/infra/storage"
	k "github.com/zenith-wang/it-wiki/backend/internal/knowledge"
	"github.com/zenith-wang/it-wiki/backend/internal/registry"
	"github.com/zenith-wang/it-wiki/backend/internal/repo"
	"github.com/zenith-wang/it-wiki/backend/internal/repo/migrations"
)

type knowledgeFixtureEmbedder struct {
	fail   atomic.Bool
	before func()
}

func (*knowledgeFixtureEmbedder) Dim() int      { return 4 }
func (*knowledgeFixtureEmbedder) Model() string { return "fixture-lexical-4" }
func (e *knowledgeFixtureEmbedder) Embed(ctx context.Context, texts []string) ([][]float32, error) {
	if e.before != nil {
		fn := e.before
		e.before = nil
		fn()
	}
	if e.fail.Load() {
		return nil, errors.New("fixture provider failure with SECRET must never escape")
	}
	out := [][]float32{}
	for _, s := range texts {
		v := []float32{0, 0, 0, 0.01}
		for i, term := range []string{"报销", "住宿", "请假"} {
			if strings.Contains(s, term) {
				v[i] = 1
			}
		}
		out = append(out, v)
	}
	return out, nil
}

// The explicit switch authorizes only a UUID disposable database. The configured
// business database is never migrated, written, reset, or dropped by this test.
func TestKnowledgeCloudIntegration(t *testing.T) {
	if os.Getenv("KNOWLEDGE_TEST_DISPOSABLE") != "1" {
		t.Skip("isolated database writes not requested")
	}
	rawDatabaseURL := os.Getenv("KNOWLEDGE_TEST_DATABASE_URL")
	if rawDatabaseURL == "" {
		f, e := os.Open("../../../.env")
		if e != nil {
			t.Fatal("configuration unavailable")
		}
		scan := bufio.NewScanner(f)
		for scan.Scan() {
			s := strings.TrimSpace(scan.Text())
			if s == "" || strings.HasPrefix(s, "#") {
				continue
			}
			key, value, ok := strings.Cut(s, "=")
			if ok {
				t.Setenv(strings.TrimSpace(key), strings.Trim(strings.TrimSpace(value), "\"'"))
			}
		}
		f.Close()
		rawDatabaseURL = os.Getenv("DATABASE_URL")
	}
	t.Setenv("HTTP_PROXY", "")
	t.Setenv("HTTPS_PROXY", "")
	t.Setenv("NO_PROXY", "*")
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Minute)
	defer cancel()
	u, e := url.Parse(rawDatabaseURL)
	if e != nil {
		t.Fatal("invalid database configuration")
	}
	u.Path = "/postgres"
	adminDB, e := repo.OpenDatabase(u.String(), "")
	if e != nil {
		t.Fatal("database unavailable")
	}
	defer adminDB.Close()
	name := "capability_hub_k1_it_" + strings.ReplaceAll(uuid.NewString(), "-", "")
	if _, e = adminDB.ExecContext(ctx, `CREATE DATABASE "`+name+`"`); e != nil {
		t.Fatal("cannot create isolated database", e)
	}
	defer func() {
		c, done := context.WithTimeout(context.Background(), 20*time.Second)
		defer done()
		if _, e := adminDB.ExecContext(c, `DROP DATABASE "`+name+`" WITH (FORCE)`); e != nil {
			t.Error("isolated database cleanup failed", e)
		}
	}()
	u.Path = "/" + name
	db, e := repo.OpenDatabase(u.String(), "")
	if e != nil {
		t.Fatal(e)
	}
	defer db.Close()
	goose.SetBaseFS(migrations.EmbedMigrations)
	_ = goose.SetDialect("postgres")
	t.Setenv("EMBEDDING_DIM", "4")
	if e = goose.UpToContext(ctx, db, ".", 15); e != nil {
		t.Fatal(e)
	}
	legacy := uuid.NewString()
	if _, e = db.ExecContext(ctx, `INSERT INTO knowledge_bases(id,name,embed_model,embed_dim) VALUES($1,'legacy preserved','legacy',4)`, legacy); e != nil {
		t.Fatal(e)
	}
	if e = goose.UpContext(ctx, db, "."); e != nil {
		t.Fatal(e)
	}
	var private bool
	if e = db.QueryRowContext(ctx, `SELECT visibility='private' AND NOT application_shared FROM knowledge_bases WHERE id=$1`, legacy).Scan(&private); e != nil || !private {
		t.Fatal("legacy data not private", e)
	}
	if e = goose.DownToContext(ctx, db, ".", 15); e != nil {
		t.Fatal(e)
	}
	if e = goose.UpContext(ctx, db, "."); e != nil {
		t.Fatal(e)
	}
	pool, e := repo.NewPool(ctx, u.String(), "")
	if e != nil {
		t.Fatal(e)
	}
	defer pool.Close()
	rm, e := rivermigrate.New(riverpgxv5.New(pool), nil)
	if e != nil {
		t.Fatal(e)
	}
	if _, e = rm.Migrate(ctx, rivermigrate.DirectionUp, nil); e != nil {
		t.Fatal(e)
	}
	alice, bob, admin := uuid.NewString(), uuid.NewString(), uuid.NewString()
	for _, id := range []string{alice, bob, admin} {
		if _, e = pool.Exec(ctx, `INSERT INTO users(id)VALUES($1)`, id); e != nil {
			t.Fatal(e)
		}
		if _, e = pool.Exec(ctx, `INSERT INTO oauth_accounts(user_id,provider,provider_user_id,tenant_key,access_token_encrypted,access_token_expires_at) VALUES($1,'feishu',$2,'fixture','\x00',now()+interval '1 hour')`, id, "ou_"+id); e != nil {
			t.Fatal(e)
		}
	}
	if _, e = pool.Exec(ctx, `INSERT INTO platform_profiles(open_id,is_admin,department) VALUES($1,true,'Finance'),($2,false,'Finance'),($3,false,'Other')`, "ou_"+admin, "ou_"+alice, "ou_"+bob); e != nil {
		t.Fatal(e)
	}
	var objects ports.ObjectStorage = &registryMemoryStorage{objects: map[string][]byte{}}
	if os.Getenv("KNOWLEDGE_TEST_S3") == "1" {
		mc, e := storage.NewMinioClient(ctx, storage.MinioConfig{Endpoint: os.Getenv("S3_ENDPOINT"), AccessKey: os.Getenv("S3_ACCESS_KEY"), SecretKey: os.Getenv("S3_SECRET_KEY"), Bucket: os.Getenv("S3_BUCKET"), Region: os.Getenv("S3_REGION"), UsePathStyle: true})
		if e != nil {
			t.Fatal("MinIO configuration failed")
		}
		objects = mc
	}
	var rc *river.Client[pgx.Tx]
	repository := repo.NewKnowledgeRepository(pool, func(ctx context.Context, tx pgx.Tx, id string) error {
		_, e := rc.InsertTx(ctx, tx, k.Job{VersionID: id}, nil)
		return e
	})
	embedding := &knowledgeFixtureEmbedder{}
	service := k.New(repository, objects, embedding, 0.5)
	defer func() {
		rows, e := pool.Query(context.Background(), "SELECT object_key FROM knowledge_versions")
		if e != nil {
			t.Error(e)
			return
		}
		defer rows.Close()
		for rows.Next() {
			var key string
			if e = rows.Scan(&key); e != nil {
				t.Error(e)
				continue
			}
			c, done := context.WithTimeout(context.Background(), 10*time.Second)
			if e = objects.Delete(c, key); e != nil {
				t.Error("test object cleanup failed")
			}
			done()
		}
	}()
	workers := river.NewWorkers()
	river.AddWorker(workers, &k.Worker{Service: service})
	rc, e = river.NewClient(riverpgxv5.New(pool), &river.Config{Workers: workers, Queues: map[string]river.QueueConfig{k.Queue: {MaxWorkers: 1}}})
	if e != nil {
		t.Fatal(e)
	}
	owner := k.Principal{UserID: alice}
	administrator := k.Principal{UserID: admin}
	kb, e := service.SaveKB(ctx, owner, "", k.KB{Name: "财务制度", Visibility: "department", Department: "Finance", ApplicationShared: true})
	if e != nil {
		t.Fatal(e)
	}
	if _, e = repository.Documents(ctx, k.Principal{UserID: bob}, kb.ID); !errors.Is(e, k.ErrForbidden) {
		t.Fatal("trusted department isolation", e)
	}
	doc, e := service.Upload(ctx, owner, kb.ID, "", "policy.md", []byte("# 报销制度\n\n报销需要发票与审批单。\n\n住宿限额为每晚 500 元，仅限国内出差。"))
	if e != nil {
		t.Fatal("upload failed", e)
	}
	if _, _, e = service.Search(ctx, owner, k.SearchRequest{KBIDs: []string{kb.ID}, Query: "报销"}); !errors.Is(e, k.ErrNotReady) {
		t.Fatal("pending must not be empty success", e)
	}
	if e = rc.Start(ctx); e != nil {
		t.Fatal(e)
	}
	ready := false
	for i := 0; i < 100; i++ {
		docs, e := repository.Documents(ctx, owner, kb.ID)
		if e != nil {
			t.Fatal(e)
		}
		if len(docs) > 0 && docs[0].ActiveVersion != "" {
			doc = docs[0]
			ready = true
			break
		}
		time.Sleep(100 * time.Millisecond)
	}
	stopCtx, stop := context.WithTimeout(context.Background(), 10*time.Second)
	e = rc.Stop(stopCtx)
	stop()
	if e != nil {
		t.Fatal(e)
	}
	if !ready {
		t.Fatal("River did not publish version")
	}
	t.Log("real PostgreSQL + River pipeline passed; embedding is a fixture")
	matches, _, e := service.Search(ctx, owner, k.SearchRequest{KBIDs: []string{kb.ID}, Query: "报销材料"})
	if e != nil || len(matches) == 0 || matches[0].VersionID != doc.ActiveVersion || matches[0].Locator.Line == 0 {
		t.Fatal("retrieval/reference", matches, e)
	}
	old := doc.ActiveVersion
	// The fixture embedding has no dimension for 发票, so only the lexical
	// channel finds this evidence (real scoped PostgreSQL SQL). Without a model
	// relevance judgment, keyword-only candidates are not returned.
	lexicalMatches, info, err := service.Search(ctx, owner, k.SearchRequest{KBIDs: []string{kb.ID}, Query: "发票"})
	if err != nil || len(lexicalMatches) != 0 || info.Strategy != "hybrid_rrf" {
		t.Fatal("keyword-only candidate returned without relevance judgment", err, info)
	}
	if direct, err := repository.LexicalSearch(ctx, []string{kb.ID}, k.SearchTerms("发票"), 20); err != nil || len(direct) == 0 || direct[0].VersionID != old {
		t.Fatal("lexical recall failed", err)
	}
	outside, err := repository.LexicalSearch(ctx, []string{legacy}, k.SearchTerms("发票"), 20)
	if err != nil || len(outside) != 0 {
		t.Fatal("lexical candidate scope leaked", err)
	}
	missing, _, err := service.Search(ctx, owner, k.SearchRequest{KBIDs: []string{kb.ID}, Query: "火星移民签证"})
	if err != nil || len(missing) != 0 {
		t.Fatal("unrelated query unexpectedly recalled", err)
	}
	// Model wire is deliberately simulated; PG/MinIO/River remain real.
	optimizerCalls := 0
	optimizerServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		optimizerCalls++
		var body struct {
			Messages []struct {
				Content string `json:"content"`
			} `json:"messages"`
		}
		if json.NewDecoder(r.Body).Decode(&body) != nil || len(body.Messages) != 2 {
			t.Error("optimizer input")
			w.WriteHeader(400)
			return
		}
		var input struct {
			Query     string            `json:"query"`
			Documents []json.RawMessage `json:"documents"`
		}
		if json.Unmarshal([]byte(body.Messages[1].Content), &input) != nil {
			t.Error("optimizer data")
			return
		}
		var response any = map[string]string{"query": input.Query}
		if input.Documents != nil {
			results := []map[string]any{}
			for i := range input.Documents {
				results = append(results, map[string]any{"index": i, "score": 0.9})
			}
			response = map[string]any{"results": results}
		}
		content, _ := json.Marshal(response)
		_ = json.NewEncoder(w).Encode(map[string]any{"choices": []any{map[string]any{"finish_reason": "stop", "message": map[string]string{"content": string(content)}}}})
	}))
	defer optimizerServer.Close()
	service.Optimizer, err = k.NewModelOptimizer(optimizerServer.URL, "fixture-only", "fixture-only")
	if err != nil {
		t.Fatal(err)
	}
	optimized, info, err := service.Search(ctx, owner, k.SearchRequest{KBIDs: []string{kb.ID}, Query: "发票"})
	if err != nil || len(optimized) == 0 || optimized[0].VersionID != old || info.Rerank != "model" || optimizerCalls != 2 {
		t.Fatal("optimizer integration", info, err)
	}
	service.Optimizer = nil
	t.Log("hybrid lexical-only recall, scope isolation and simulated model optimization passed")
	var docx bytes.Buffer
	zw := zip.NewWriter(&docx)
	manifest, _ := zw.Create("[Content_Types].xml")
	_, _ = manifest.Write([]byte(`<Types xmlns="http://schemas.openxmlformats.org/package/2006/content-types"><Override PartName="/word/document.xml" ContentType="application/vnd.openxmlformats-officedocument.wordprocessingml.document.main+xml"/></Types>`))
	part, _ := zw.Create("word/document.xml")
	_, _ = part.Write([]byte(`<w:document xmlns:w="http://schemas.openxmlformats.org/wordprocessingml/2006/main"><w:body><w:p><w:pPr><w:pStyle w:val="Heading1"/></w:pPr><w:r><w:t>请假制度</w:t></w:r></w:p><w:p><w:r><w:t>请假需要直属主管审批。</w:t></w:r></w:p><w:tbl><w:tr><w:tc><w:p><w:r><w:t>类别</w:t></w:r></w:p></w:tc><w:tc><w:p><w:r><w:t>天数</w:t></w:r></w:p></w:tc></w:tr><w:tr><w:tc><w:p><w:r><w:t>年假</w:t></w:r></w:p></w:tc><w:tc><w:p><w:r><w:t>5</w:t></w:r></w:p></w:tc></w:tr></w:tbl></w:body></w:document>`))
	_ = zw.Close()
	complex, e := service.Upload(ctx, owner, kb.ID, "", "请假制度.docx", docx.Bytes())
	if e != nil {
		t.Fatal(e)
	}
	if e = service.Process(ctx, complex.PendingVersion); e != nil {
		t.Fatal(e)
	}
	v, e := repository.Preview(ctx, owner, complex.PendingVersion)
	if e != nil || v.Status != "ready" || len(v.Preview.Blocks) < 3 || v.Preview.Blocks[1].Locator.Paragraph != 2 || v.Preview.Blocks[1].Locator.Page != 0 {
		t.Fatal("DOCX ingestion/preview failed", e)
	}
	// Failed replacement keeps old content and safe error codes.
	bad, e := service.Upload(ctx, owner, kb.ID, doc.ID, "broken.pdf", []byte("not a PDF"))
	if e != nil {
		t.Fatal(e)
	}
	if e = service.Process(ctx, bad.PendingVersion); e != nil {
		t.Fatal(e)
	}
	docs, e := repository.Documents(ctx, owner, kb.ID)
	var updated k.Document
	for _, d := range docs {
		if d.ID == doc.ID {
			updated = d
		}
	}
	if e != nil || updated.ActiveVersion != old || updated.Status != "failed" {
		t.Fatal("failed replacement lost old version", e)
	}
	// Two queued replacements: the older job must not supersede the latest.
	first, e := service.Upload(ctx, owner, kb.ID, doc.ID, "first.txt", []byte("报销第一更新"))
	if e != nil {
		t.Fatal(e)
	}
	second, e := service.Upload(ctx, owner, kb.ID, doc.ID, "second.txt", []byte("报销最新更新"))
	if e != nil {
		t.Fatal(e)
	}
	if e = service.Process(ctx, second.PendingVersion); e != nil {
		t.Fatal(e)
	}
	if e = service.Process(ctx, first.PendingVersion); e != nil {
		t.Fatal(e)
	}
	docs, e = repository.Documents(ctx, owner, kb.ID)
	for _, d := range docs {
		if d.ID == doc.ID {
			updated = d
		}
	}
	if e != nil || updated.ActiveVersion != second.PendingVersion {
		t.Fatal("stale job overwrote latest")
	}
	app, e := service.SaveApplication(ctx, administrator, "", k.ApplicationInput{Name: "应用 A", Enabled: true, KBIDs: []string{kb.ID}})
	if e != nil {
		t.Fatal(e)
	}
	credential, e := service.Rotate(ctx, administrator, app.ID)
	if e != nil {
		t.Fatal(e)
	}
	principal, e := service.Authenticate(ctx, credential.Token)
	if e != nil {
		t.Fatal(e)
	}
	// A real HTTP request exercises the machine route without a cookie/CSRF token.
	hash := sha256.Sum256([]byte("csrf"))
	auth := newTestAuthHandler(t, &fakeAuthFlow{}, &fakeSessionStore{session: domain.Session{UserID: alice, CSRFTokenHash: hash[:]}}, fakeUserResolver{user: domain.User{ID: alice}})
	browserHarness := os.Getenv("KNOWLEDGE_BROWSER_HARNESS") == "1"
	if browserHarness {
		auth.frontendOrigin = "http://127.0.0.1:3091"
	}
	router := NewCapabilityHubRouter(Handlers{Auth: auth, Knowledge: NewKnowledgeHandler(service), Registry: NewRegistryHandler(registry.New(repo.NewRegistryRepository(pool), objects))})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Test-only fixture session: never compiled into cmd/server.
		if browserHarness {
			r.AddCookie(&http.Cookie{Name: SessionCookieName, Value: "fixture-session"})
			http.SetCookie(w, &http.Cookie{Name: CSRFCookieName, Value: "csrf", Path: "/", SameSite: http.SameSiteLaxMode})
		}
		router.ServeHTTP(w, r)
	}))
	defer server.Close()
	call := func(token, body string, want int) {
		t.Helper()
		req, _ := http.NewRequestWithContext(ctx, "POST", server.URL+"/api/v1/knowledge/search", strings.NewReader(body))
		req.Header.Set("Authorization", "Bearer "+token)
		res, e := server.Client().Do(req)
		if e != nil {
			t.Fatal(e)
		}
		defer res.Body.Close()
		raw, _ := io.ReadAll(res.Body)
		if res.StatusCode != want {
			t.Fatalf("HTTP want %d got %d: %s", want, res.StatusCode, raw)
		}
		if bytes.Contains(raw, []byte("SECRET")) {
			t.Fatal("provider details leaked")
		}
	}
	input, _ := json.Marshal(k.SearchRequest{KBIDs: []string{kb.ID}, Query: "报销", TopK: 5})
	call(credential.Token, string(input), 200)
	audits, e := repository.AuditEntries(ctx, administrator)
	if e != nil || len(audits) == 0 || len(audits[0].KBIDs) != 1 || audits[0].KBIDs[0] != kb.ID {
		t.Fatal("audit omitted authorized resources", e)
	}
	call("invalid", string(input), 401)
	call(credential.Token, strings.TrimSuffix(string(input), "}")+`,"user_id":"forged"}`, 400)
	mixed, _ := json.Marshal(k.SearchRequest{KBIDs: []string{kb.ID, legacy}, Query: "报销"})
	call(credential.Token, string(mixed), 403)
	appB, e := service.SaveApplication(ctx, administrator, "", k.ApplicationInput{Name: "应用 B", Enabled: true, KBIDs: []string{}})
	if e != nil {
		t.Fatal(e)
	}
	tokenB, e := service.Rotate(ctx, administrator, appB.ID)
	if e != nil {
		t.Fatal(e)
	}
	call(tokenB.Token, string(input), 403)
	if _, e = pool.Exec(ctx, `UPDATE knowledge_credentials SET scopes='{}' WHERE application_id=$1`, appB.ID); e != nil {
		t.Fatal(e)
	}
	call(tokenB.Token, string(input), 403)
	if _, e = pool.Exec(ctx, `UPDATE knowledge_credentials SET expires_at=now()-interval '1 minute' WHERE application_id=$1`, appB.ID); e != nil {
		t.Fatal(e)
	}
	call(tokenB.Token, string(input), 401)
	embedding.fail.Store(true)
	call(credential.Token, string(input), 503)
	embedding.fail.Store(false)
	if browserHarness {
		if _, e = pool.Exec(ctx, `UPDATE platform_profiles SET is_admin=true WHERE open_id=$1`, "ou_"+alice); e != nil {
			t.Fatal(e)
		}
		browserWorker, e := river.NewClient(riverpgxv5.New(pool), &river.Config{Workers: workers, Queues: map[string]river.QueueConfig{"knowledge": {MaxWorkers: 1}}})
		if e != nil {
			t.Fatal(e)
		}
		if e = browserWorker.Start(ctx); e != nil {
			t.Fatal(e)
		}
		defer func() {
			c, done := context.WithTimeout(context.Background(), 10*time.Second)
			defer done()
			_ = browserWorker.Stop(c)
		}()
		for _, file := range []string{"expense.md", "travel.md", "leave.txt"} {
			raw, _ := k.EvaluationData.ReadFile("testdata/" + file)
			if _, e = service.Upload(ctx, owner, kb.ID, "", file, raw); e != nil {
				t.Fatal(e)
			}
		}
		if _, e = service.Upload(ctx, owner, kb.ID, "", "扫描件.pdf", []byte("invalid scanned fixture")); e != nil {
			t.Fatal(e)
		}
		if e = os.WriteFile("../../../.cache/k1-browser-api.txt", []byte(server.URL), 0600); e != nil {
			t.Fatal(e)
		}
		t.Log("isolated browser harness ready; fixture session/embedding, real PostgreSQL/MinIO/River")
		for {
			if _, e = os.Stat("../../../.cache/k1-browser-stop"); e == nil {
				break
			}
			select {
			case <-ctx.Done():
				t.Fatal("browser harness timed out")
			case <-time.After(time.Second):
			}
		}
		return
	}
	// Revoke while the embedding provider is processing; return-time authorization wins.
	embedding.before = func() {
		if e := repository.Revoke(ctx, administrator, app.ID); e != nil {
			t.Fatal(e)
		}
	}
	if _, _, e = service.Search(ctx, principal, k.SearchRequest{KBIDs: []string{kb.ID}, Query: "报销"}); !errors.Is(e, k.ErrUnauthenticated) {
		t.Fatal("in-flight revocation leaked", e)
	}
	call(credential.Token, string(input), 401)
	credential, e = service.Rotate(ctx, administrator, app.ID)
	if e != nil {
		t.Fatal(e)
	}
	if _, e = service.SaveApplication(ctx, administrator, app.ID, k.ApplicationInput{Name: app.Name, Enabled: false, KBIDs: app.KBIDs}); e != nil {
		t.Fatal(e)
	}
	call(credential.Token, string(input), 401)
	if _, e = service.SaveApplication(ctx, owner, "", k.ApplicationInput{Name: "unauthorized", Enabled: true}); !errors.Is(e, k.ErrForbidden) {
		t.Fatal("owner issued app", e)
	}
	// Delete during retrieval, then run a late replacement job: nothing can revive it.
	pending, e := service.Upload(ctx, owner, kb.ID, doc.ID, "late.txt", []byte("报销已删除"))
	if e != nil {
		t.Fatal(e)
	}
	embedding.before = func() {
		if e := repository.DeleteDocument(ctx, owner, doc.ID); e != nil {
			t.Fatal(e)
		}
	}
	matches, _, e = service.Search(ctx, owner, k.SearchRequest{KBIDs: []string{kb.ID}, Query: "报销"})
	if e != nil || len(matches) != 0 {
		t.Fatal("deleted content leaked", e)
	}
	if e = service.Process(ctx, pending.PendingVersion); e != nil {
		t.Fatal(e)
	}
	docs, e = repository.Documents(ctx, owner, kb.ID)
	if e != nil || len(docs) != 1 || docs[0].ID != complex.ID {
		t.Fatal("deleted document revived", e)
	}
	// Removing the sharing boundary revokes grants permanently; toggling it back
	// must not silently resurrect an administrator's old application grants.
	kb.ApplicationShared = false
	if _, e = service.SaveKB(ctx, owner, kb.ID, kb); e != nil {
		t.Fatal(e)
	}
	kb.ApplicationShared = true
	if _, e = service.SaveKB(ctx, owner, kb.ID, kb); e != nil {
		t.Fatal(e)
	}
	apps, e := repository.Applications(ctx, administrator)
	if e != nil {
		t.Fatal(e)
	}
	for _, a := range apps {
		if a.ID == app.ID && len(a.KBIDs) != 0 {
			t.Fatal("unsharing resurrected grants")
		}
	}
	if _, e = service.SaveApplication(ctx, administrator, app.ID, k.ApplicationInput{Name: app.Name, Enabled: true, KBIDs: []string{kb.ID}}); e != nil {
		t.Fatal(e)
	}
	if e = repository.DeleteKB(ctx, owner, kb.ID); e != nil {
		t.Fatal(e)
	}
	apps, e = repository.Applications(ctx, administrator)
	if e != nil {
		t.Fatal(e)
	}
	for _, a := range apps {
		if a.ID == app.ID && len(a.KBIDs) != 0 {
			t.Fatal("deleted base left unusable stale grants")
		}
	}
	t.Log("migration up/down, legacy privacy, session identities, API scopes/grants, atomic versions, deletion/revocation races passed")
}
