package http

import (
	"bytes"
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/pressly/goose/v3"
	"github.com/zenith-wang/it-wiki/backend/internal/domain"
	"github.com/zenith-wang/it-wiki/backend/internal/knowledge"
	"github.com/zenith-wang/it-wiki/backend/internal/registry"
	"github.com/zenith-wang/it-wiki/backend/internal/repo"
)

// Uses only the UUID disposable database created by the registry harness.
func verifyReviewerIntegration(t *testing.T, db *sql.DB, pool *pgxpool.Pool, s *registry.Service, repository *repo.RegistryRepository, users map[string]string) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()
	admin, owner, outsider := users["admin"], users["owner"], users["outsider"]
	reviewer := uuid.NewString()
	users["reviewer"] = reviewer
	if _, e := pool.Exec(ctx, `INSERT INTO users(id,display_name) VALUES($1,'全平台审核员')`, reviewer); e != nil {
		t.Fatal(e)
	}
	if _, e := pool.Exec(ctx, `INSERT INTO oauth_accounts(user_id,provider,provider_user_id,tenant_key,access_token_encrypted,access_token_expires_at) VALUES($1,'feishu',$2,'fixture',decode('00','hex'),now()+interval '1 hour')`, reviewer, "ou_"+reviewer); e != nil {
		t.Fatal(e)
	}
	call := func(user, method, path string, input any, csrf string) *httptest.ResponseRecorder {
		t.Helper()
		raw, _ := json.Marshal(input)
		hash := sha256.Sum256([]byte("csrf"))
		auth := newTestAuthHandler(t, &fakeAuthFlow{}, &fakeSessionStore{session: domain.Session{UserID: user, CSRFTokenHash: hash[:]}}, fakeUserResolver{user: domain.User{ID: user}})
		req := httptest.NewRequest(method, path, bytes.NewReader(raw))
		req.AddCookie(&http.Cookie{Name: SessionCookieName, Value: "session"})
		req.Header.Set("Origin", "https://app.example.test")
		req.Header.Set(CSRFHeaderName, csrf)
		w := httptest.NewRecorder()
		NewCapabilityHubRouter(Handlers{Auth: auth, Registry: NewRegistryHandler(s)}).ServeHTTP(w, req)
		return w
	}
	status := func(w *httptest.ResponseRecorder, want int) {
		t.Helper()
		if w.Code != want {
			t.Fatalf("want %d got %d: %s", want, w.Code, w.Body.String())
		}
	}
	identity := func(user string) registry.Identity {
		t.Helper()
		i, e := s.Identity(ctx, user)
		if e != nil {
			t.Fatal(e)
		}
		return i
	}
	grant := func(enabled bool) registry.Identity {
		t.Helper()
		i := identity(reviewer)
		out, e := s.SetReviewer(ctx, admin, i.OpenID, registry.ReviewerRequest{IsReviewer: &enabled, Revision: i.Revision, Reason: "审核岗位调整"})
		if e != nil {
			t.Fatal(e)
		}
		return out
	}
	path := "/api/v1/governance/profiles/ou_" + reviewer + "/reviewer"
	body := map[string]any{"is_reviewer": true, "revision": 0, "reason": "试运行审核"}
	status(call(outsider, "PUT", path, body, "csrf"), 403)
	status(call(reviewer, "PUT", path, body, "csrf"), 403)
	status(call(admin, "PUT", path, body, "bad"), 403)
	status(call(admin, "PUT", path, map[string]any{"revision": 0, "reason": "missing flag"}, "csrf"), 400)
	status(call(admin, "PUT", path, map[string]any{"is_reviewer": true, "is_admin": true, "revision": 0, "reason": "forged role"}, "csrf"), 400)
	status(call(admin, "PUT", path, body, "csrf"), 200)
	status(call(admin, "PUT", path, body, "csrf"), 409)
	status(call(admin, "PUT", "/api/v1/governance/profiles/ou_not_logged_in/reviewer", body, "csrf"), 409)
	if i := identity(reviewer); !i.IsReviewer || i.IsAdmin {
		t.Fatal("reviewer grant escalated or missing", i)
	}
	status(call(reviewer, "GET", "/api/v1/governance/reviews", nil, ""), 200)
	for _, p := range []string{"/api/v1/governance/profiles", "/api/v1/governance/audit"} {
		status(call(reviewer, "GET", p, nil, ""), 403)
	}
	status(call(reviewer, "PUT", "/api/v1/governance/profiles/ou_"+owner+"/department", registry.DepartmentRequest{Department: "forged", Reason: "test"}, "csrf"), 403)
	people := call(outsider, "GET", "/api/v1/governance/reviewers?limit=1", nil, "")
	status(people, 200)
	var page registry.ReviewerPage
	if json.Unmarshal(people.Body.Bytes(), &page) != nil || len(page.Items) != 1 || !page.HasMore {
		t.Fatal("reviewer directory pagination")
	}
	if strings.Contains(people.Body.String(), "revision") || strings.Contains(people.Body.String(), "department") {
		t.Fatal("directory leaked profile fields")
	}
	if _, e := pool.Exec(ctx, `ALTER TABLE audit_log ADD CONSTRAINT fixture_reject_role CHECK(action<>'set_reviewer') NOT VALID`); e != nil {
		t.Fatal(e)
	}
	before := identity(reviewer)
	disabled := false
	if _, e := s.SetReviewer(ctx, admin, before.OpenID, registry.ReviewerRequest{IsReviewer: &disabled, Revision: before.Revision, Reason: "rollback"}); e == nil {
		t.Fatal("unaudited role change committed")
	}
	if after := identity(reviewer); !after.IsReviewer || after.Revision != before.Revision {
		t.Fatal("role audit failure did not roll back")
	}
	if _, e := pool.Exec(ctx, `ALTER TABLE audit_log DROP CONSTRAINT fixture_reject_role`); e != nil {
		t.Fatal(e)
	}

	// New migration down/up preserves previous roles, data and audit; it never
	// turns existing users into reviewers when reapplied.
	var caps, audits int
	if e := pool.QueryRow(ctx, `SELECT (SELECT count(*) FROM capabilities),(SELECT count(*) FROM audit_log)`).Scan(&caps, &audits); e != nil {
		t.Fatal(e)
	}
	if e := goose.DownToContext(ctx, db, ".", 19); e != nil {
		t.Fatal(e)
	}
	var exists bool
	if e := pool.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM information_schema.columns WHERE table_name='platform_profiles' AND column_name='is_reviewer')`).Scan(&exists); e != nil || exists {
		t.Fatal("reviewer rollback", e)
	}
	if e := goose.UpContext(ctx, db, "."); e != nil {
		t.Fatal(e)
	}
	var capsAfter, auditsAfter int
	if e := pool.QueryRow(ctx, `SELECT (SELECT count(*) FROM capabilities),(SELECT count(*) FROM audit_log)`).Scan(&capsAfter, &auditsAfter); e != nil {
		t.Fatal(e)
	}
	if caps != capsAfter || audits != auditsAfter || !identity(admin).IsAdmin || identity(reviewer).IsReviewer {
		t.Fatal("reviewer migration changed existing data/roles")
	}
	grant(true)
	makeDraft := func(slug, user string) registry.Detail {
		t.Helper()
		d, e := s.Save(ctx, user, "", registry.SaveRequest{Slug: slug, Type: "mcp", Name: "待审核能力", Description: "审核隔离", Visibility: "allowlist", Allowlist: []string{"ou_" + owner}, Version: "1", MCPEndpoint: "https://mcp.example.test/mcp", MCPTransport: "streamable-http", MCPAuthScheme: "none"}, nil)
		if e != nil {
			t.Fatal(e)
		}
		return d
	}
	knowledgeRepo := repo.NewKnowledgeRepository(pool, nil)
	privateKB, e := knowledgeRepo.SaveKB(ctx, knowledge.Principal{UserID: owner}, "", knowledge.KB{Name: "Reviewer isolation", Visibility: "private"}, "fixture", 1024)
	if e != nil {
		t.Fatal(e)
	}
	viewer := knowledge.Principal{UserID: reviewer}
	visibleKBs, e := knowledgeRepo.ListKB(ctx, viewer)
	if e != nil {
		t.Fatal(e)
	}
	for _, kb := range visibleKBs {
		if kb.ID == privateKB.ID {
			t.Fatal("reviewer discovered private knowledge base")
		}
	}
	if _, e = knowledgeRepo.Documents(ctx, viewer, privateKB.ID); !errors.Is(e, knowledge.ErrForbidden) {
		t.Fatal("reviewer read private knowledge", e)
	}
	if e = knowledgeRepo.AuthorizeManage(ctx, viewer, privateKB.ID); !errors.Is(e, knowledge.ErrForbidden) {
		t.Fatal("reviewer managed private knowledge", e)
	}
	act := func(user, kind string, d registry.Detail) (registry.Detail, error) {
		v := d.DraftVersionID
		if kind == "new-draft" || kind == "offline" || kind == "visibility" {
			v = d.CurrentVersionID
		}
		return s.Act(ctx, user, d.Slug, kind, registry.ActionRequest{Revision: d.Revision, VersionID: v, Reason: "审核验证"})
	}
	mustAct := func(user, kind string, d registry.Detail) registry.Detail {
		t.Helper()
		out, e := act(user, kind, d)
		if e != nil {
			t.Fatalf("%s: %v", kind, e)
		}
		return out
	}
	d := makeDraft("reviewer-private", owner)
	if _, e := s.Get(ctx, reviewer, d.Slug); !errors.Is(e, registry.ErrNotFound) {
		t.Fatal("reviewer read unrelated draft", e)
	}
	d = mustAct(owner, "submit", d)
	rd, e := s.Get(ctx, reviewer, d.Slug)
	if e != nil || !rd.CanReview || !rd.IsReviewView || rd.IsAdmin || len(rd.Versions) != 1 {
		t.Fatal("review projection", rd, e)
	}
	keyCtx := registry.WithCredentialID(ctx, "fixture-publishing-key")
	if _, e = s.Get(keyCtx, reviewer, d.Slug); !errors.Is(e, registry.ErrNotFound) {
		t.Fatal("publishing key inherited reviewer read", e)
	}
	if _, e = s.Reviews(keyCtx, reviewer, registry.Filter{Limit: 24}); !errors.Is(e, registry.ErrForbidden) {
		t.Fatal("publishing key inherited review queue", e)
	}
	for _, kind := range []string{"force-publish", "visibility", "offline", "new-draft"} {
		if _, e := act(reviewer, kind, d); e == nil {
			t.Fatalf("reviewer performed %s", kind)
		}
	}
	d = mustAct(reviewer, "reject", d)
	if len(d.Versions) != 0 || d.Published != nil || len(d.Allowlist) != 0 {
		t.Fatal("review receipt exposed rejected contents")
	}
	if _, e = s.Get(ctx, reviewer, d.Slug); !errors.Is(e, registry.ErrNotFound) {
		t.Fatal("rejected draft remained accessible", e)
	}
	d, e = s.Get(ctx, owner, d.Slug)
	if e != nil {
		t.Fatal(e)
	}
	d = mustAct(owner, "submit", d)
	d = mustAct(reviewer, "approve", d)
	live, e := s.Get(ctx, owner, d.Slug)
	if e != nil {
		t.Fatal(e)
	}
	for _, kind := range []string{"offline", "visibility", "new-draft"} {
		request := registry.ActionRequest{Revision: live.Revision, VersionID: live.CurrentVersionID, Reason: "权限边界验证"}
		if kind == "visibility" {
			request.Visibility = "org"
		}
		status(call(reviewer, "POST", "/api/v1/capabilities/"+live.Slug+"/"+kind, request, "csrf"), 403)
	}
	if _, e = s.Published(ctx, reviewer, d.Slug); !errors.Is(e, registry.ErrNotFound) {
		t.Fatal("reviewer gained private consumer access", e)
	}
	catalog, e := s.Catalog(ctx, reviewer, registry.Filter{Limit: 100})
	if e != nil {
		t.Fatal(e)
	}
	for _, c := range catalog.Items {
		if c.Slug == d.Slug {
			t.Fatal("private capability leaked into catalog")
		}
	}
	self := makeDraft("reviewer-self", reviewer)
	self = mustAct(reviewer, "submit", self)
	self = mustAct(reviewer, "approve", self)
	if self.Status != "published" || !self.IsOwner || self.IsAdmin {
		t.Fatal("reviewer self approval failed")
	}

	// Skill history and file access are projected to pending + live versions.
	in := registry.SaveRequest{Slug: "reviewer-skill", Type: "skill", Name: "审核文件", Description: "文件隔离", Visibility: "allowlist", Allowlist: []string{"ou_" + owner}, Version: "1"}
	files := []registry.File{{Name: "SKILL.md", Data: []byte("---\nname: reviewer-skill\ndescription: Review files\n---\n# Review files\n")}}
	skill, e := s.Save(ctx, owner, "", in, files)
	if e != nil {
		t.Fatal(e)
	}
	first := skill.DraftVersionID
	for _, version := range []string{"2", "3"} {
		skill = mustAct(owner, "submit", skill)
		skill = mustAct(reviewer, "approve", skill)
		skill, e = s.Get(ctx, owner, skill.Slug)
		if e != nil {
			t.Fatal(e)
		}
		skill = mustAct(owner, "new-draft", skill)
		in.Revision = skill.Revision
		in.Version = version
		skill, e = s.Save(ctx, owner, skill.Slug, in, files)
		if e != nil {
			t.Fatal(e)
		}
	}
	skill = mustAct(owner, "submit", skill)
	rd, e = s.Get(ctx, reviewer, skill.Slug)
	if e != nil || len(rd.Versions) != 2 {
		t.Fatal("reviewer saw historical versions", rd, e)
	}
	for _, v := range rd.Versions {
		if _, e = s.Files(ctx, reviewer, skill.Slug, v.ID); e != nil {
			t.Fatal(e)
		}
	}
	if _, e = s.Files(ctx, reviewer, skill.Slug, first); !errors.Is(e, registry.ErrNotFound) {
		t.Fatal("historical file exposed", e)
	}
	if _, e = s.Download(keyCtx, reviewer, skill.Slug, skill.DraftVersionID); !errors.Is(e, registry.ErrNotFound) {
		t.Fatal("publishing key downloaded review files", e)
	}
	grant(false)
	remaining, e := s.Reviewers(ctx, outsider, registry.Filter{Limit: 100})
	if e != nil {
		t.Fatal(e)
	}
	for _, person := range remaining.Items {
		if person.OpenID == "ou_"+reviewer {
			t.Fatal("revoked user remained in reviewer directory")
		}
	}
	if _, e = s.Files(ctx, reviewer, skill.Slug, skill.DraftVersionID); !errors.Is(e, registry.ErrNotFound) {
		t.Fatal("revoked reviewer read files", e)
	}
	if _, e = act(reviewer, "approve", skill); e == nil {
		t.Fatal("stale page approved after revocation")
	}
	grant(true)

	waitBlocked := func(query string) {
		t.Helper()
		deadline := time.Now().Add(5 * time.Second)
		for time.Now().Before(deadline) {
			var waiting bool
			if e := pool.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM pg_stat_activity WHERE datname=current_database() AND pid<>pg_backend_pid() AND wait_event_type='Lock' AND strpos(query,$1)>0)`, query).Scan(&waiting); e != nil {
				t.Fatal(e)
			}
			if waiting {
				return
			}
			time.Sleep(10 * time.Millisecond)
		}
		t.Fatalf("did not block at %s", query)
	}
	// Revoke wins: the approval cannot use the earlier service-layer role read.
	tx, e := pool.Begin(ctx)
	if e != nil {
		t.Fatal(e)
	}
	defer tx.Rollback(context.Background())
	if _, e = tx.Exec(ctx, `UPDATE platform_profiles SET is_reviewer=false,revision=revision+1 WHERE open_id=$1`, "ou_"+reviewer); e != nil {
		t.Fatal(e)
	}
	approval := make(chan error, 1)
	go func() { _, err := act(reviewer, "approve", skill); approval <- err }()
	waitBlocked("-- name: LockGovernanceRole")
	if e = tx.Commit(ctx); e != nil {
		t.Fatal(e)
	}
	if e = <-approval; e == nil {
		t.Fatal("approval raced past committed revocation")
	}
	grant(true)
	// Approval wins: it holds the role lock while waiting for the capability;
	// revocation must wait for the complete transition and audit.
	tx, e = pool.Begin(ctx)
	if e != nil {
		t.Fatal(e)
	}
	defer tx.Rollback(context.Background())
	if _, e = tx.Exec(ctx, `SELECT id FROM capabilities WHERE slug=$1 FOR UPDATE`, skill.Slug); e != nil {
		t.Fatal(e)
	}
	go func() { _, err := act(reviewer, "approve", skill); approval <- err }()
	waitBlocked("-- name: LockCapability")
	i := identity(reviewer)
	revoked := make(chan error, 1)
	go func() {
		_, err := s.SetReviewer(ctx, admin, i.OpenID, registry.ReviewerRequest{IsReviewer: &disabled, Revision: i.Revision, Reason: "并发撤权"})
		revoked <- err
	}()
	waitBlocked("-- name: LockGovernanceProfiles")
	if e = tx.Commit(ctx); e != nil {
		t.Fatal(e)
	}
	if e = <-approval; e != nil {
		t.Fatal("earlier approval failed", e)
	}
	if e = <-revoked; e != nil {
		t.Fatal("revocation failed", e)
	}
	if identity(reviewer).IsReviewer {
		t.Fatal("revocation not visible")
	}
	// Ensure the repository also rejects a direct privileged transition.
	if _, e = repository.Act(ctx, reviewer, skill.Slug, "approve", registry.ActionRequest{Revision: skill.Revision, VersionID: skill.DraftVersionID}); e == nil {
		t.Fatal("repository bypassed revocation")
	}
	grant(true)
	preview := makeDraft("reviewer-preview", owner)
	mustAct(owner, "submit", preview)
	t.Log("reviewers: grant/revoke, CSRF, forged roles, audit rollback, migration down/up, self review, private/version/file/knowledge isolation, API scope boundary and both revoke/approve orderings passed")
}
