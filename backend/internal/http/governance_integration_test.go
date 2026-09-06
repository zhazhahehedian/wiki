package http

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/zenith-wang/it-wiki/backend/internal/domain"
	"github.com/zenith-wang/it-wiki/backend/internal/registry"
	"github.com/zenith-wang/it-wiki/backend/internal/repo"
)

// Runs only in the UUID disposable database owned by TestRegistryCloudIntegration.
func verifyGovernanceIntegration(t *testing.T, pool *pgxpool.Pool, s *registry.Service, repos *repo.RegistryRepository, alice, bob string) map[string]string {
	t.Helper()
	ctx := context.Background()
	admin, eve := uuid.NewString(), uuid.NewString()
	for _, id := range []string{admin, eve} {
		if _, e := pool.Exec(ctx, `INSERT INTO users(id,display_name) VALUES($1,'Governance fixture')`, id); e != nil {
			t.Fatal(e)
		}
		if _, e := pool.Exec(ctx, `INSERT INTO oauth_accounts(user_id,provider,provider_user_id,tenant_key,access_token_encrypted,access_token_expires_at) VALUES($1,'feishu',$2,'fixture',decode('00','hex'),now()+interval '1 hour')`, id, "ou_"+id); e != nil {
			t.Fatal(e)
		}
	}
	// Bootstrap reserves an unlogged identity, is idempotent, cannot overwrite or
	// restore an explicitly revoked role, and reads privileges from the database.
	pending := uuid.NewString()
	if e := repos.BootstrapAdmin(ctx, "ou_"+pending); e != nil {
		t.Fatal(e)
	}
	var isAdmin bool
	if e := pool.QueryRow(ctx, `SELECT is_admin FROM platform_profiles WHERE open_id=$1`, "ou_"+pending).Scan(&isAdmin); e != nil || !isAdmin {
		t.Fatal("pending admin not provisioned", e)
	}
	if _, e := pool.Exec(ctx, `INSERT INTO users(id,display_name) VALUES($1,'First login pending admin')`, pending); e != nil {
		t.Fatal(e)
	}
	if _, e := pool.Exec(ctx, `INSERT INTO oauth_accounts(user_id,provider,provider_user_id,tenant_key,access_token_encrypted,access_token_expires_at) VALUES($1,'feishu',$2,'fixture',decode('00','hex'),now()+interval '1 hour')`, pending, "ou_"+pending); e != nil {
		t.Fatal(e)
	}
	bound, e := s.Identity(ctx, pending)
	if e != nil || !bound.IsAdmin {
		t.Fatal("first login did not bind reserved admin", e)
	}
	if e := repos.BootstrapAdmin(ctx, "ou_"+admin); e != nil {
		t.Fatal(e)
	}
	identity, e := s.Identity(ctx, admin)
	if e != nil || identity.IsAdmin {
		t.Fatal("bootstrap overrode first choice")
	}
	if _, e = pool.Exec(ctx, `UPDATE platform_profiles SET is_admin=false WHERE open_id=$1`, "ou_"+pending); e != nil {
		t.Fatal(e)
	}
	if e = repos.BootstrapAdmin(ctx, "ou_"+pending); e != nil {
		t.Fatal(e)
	}
	if e = pool.QueryRow(ctx, `SELECT is_admin FROM platform_profiles WHERE open_id=$1`, "ou_"+pending).Scan(&isAdmin); e != nil || isAdmin {
		t.Fatal("bootstrap restored revoked admin")
	}
	if _, e = pool.Exec(ctx, `INSERT INTO platform_profiles(open_id,is_admin) VALUES($1,true)`, "ou_"+admin); e != nil {
		t.Fatal(e)
	}
	routerFor := func(user string) http.Handler {
		hash := sha256.Sum256([]byte("csrf"))
		auth := newTestAuthHandler(t, &fakeAuthFlow{}, &fakeSessionStore{session: domain.Session{UserID: user, CSRFTokenHash: hash[:]}}, fakeUserResolver{user: domain.User{ID: user}})
		return NewCapabilityHubRouter(Handlers{Auth: auth, Registry: NewRegistryHandler(s)})
	}
	call := func(user, method, path string, input any, csrf string) *httptest.ResponseRecorder {
		var raw []byte
		if input != nil {
			raw, _ = json.Marshal(input)
		}
		req := httptest.NewRequest(method, path, bytes.NewReader(raw))
		if user != "" {
			req.AddCookie(&http.Cookie{Name: SessionCookieName, Value: "session"})
		}
		req.Header.Set("Origin", "https://app.example.test")
		req.Header.Set(CSRFHeaderName, csrf)
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		routerFor(user).ServeHTTP(w, req)
		return w
	}
	status := func(w *httptest.ResponseRecorder, want int) {
		t.Helper()
		if w.Code != want {
			t.Fatalf("want %d got %d: %s", want, w.Code, w.Body.String())
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
	get := func(user, slug string) registry.Detail {
		t.Helper()
		w := call(user, "GET", "/api/v1/capabilities/"+slug, nil, "")
		status(w, 200)
		return decode(w)
	}
	action := func(user, slug, kind string, d registry.Detail, reason string) *httptest.ResponseRecorder {
		v := d.DraftVersionID
		if v == "" && d.Status == "offline" {
			v = d.CurrentVersionID
		}
		if kind == "new-draft" || kind == "offline" {
			v = d.CurrentVersionID
		}
		return call(user, "POST", "/api/v1/capabilities/"+slug+"/"+kind, registry.ActionRequest{Revision: d.Revision, VersionID: v, Reason: reason}, "csrf")
	}
	for _, path := range []string{"/api/v1/governance/reviews", "/api/v1/governance/audit", "/api/v1/governance/profiles"} {
		status(call("", "GET", path, nil, ""), 401)
		status(call(bob, "GET", path, nil, ""), 403)
		status(call(admin, "GET", path, nil, ""), 200)
	}
	input := registry.SaveRequest{Slug: "governed-search", Type: "mcp", Name: "Original visible name", Description: "Original description", Visibility: "org", Version: "1", MCPEndpoint: "https://mcp.example.test/v1", MCPTransport: "streamable-http", MCPAuthScheme: "bearer"}
	d, e := s.Save(registry.WithRequestID(ctx, "fixture-create"), alice, "", input, nil)
	if e != nil {
		t.Fatal(e)
	}
	status(call(bob, "GET", "/api/v1/capabilities/"+d.Slug, nil, ""), 404)
	request := registry.ActionRequest{Revision: d.Revision, VersionID: d.DraftVersionID}
	status(call(alice, "POST", "/api/v1/capabilities/"+d.Slug+"/submit", request, "bad"), 403)
	status(action(bob, d.Slug, "submit", d, ""), 404)
	status(action(alice, d.Slug, "approve", d, ""), 403)
	w := action(alice, d.Slug, "submit", d, "")
	status(w, 200)
	review := decode(w)
	status(action(alice, d.Slug, "submit", d, ""), 409)
	if _, e = s.Save(ctx, alice, d.Slug, input, nil); e != registry.ErrConflict {
		t.Fatalf("review edited: %v", e)
	}
	status(action(admin, d.Slug, "reject", review, ""), 400)
	status(action(admin, d.Slug, "reject", review, "补充工具说明"), 200)
	d = get(alice, d.Slug)
	if d.ReviewReason != "补充工具说明" || d.Status != "draft" {
		t.Fatal("rejection reason/state missing")
	}
	status(action(alice, d.Slug, "submit", d, ""), 200)
	d = get(admin, d.Slug)
	// Failure at the final audit insert rolls back pointers, published flags and status.
	if _, e = pool.Exec(ctx, `ALTER TABLE audit_log ADD CONSTRAINT fixture_reject_approve CHECK(action<>'approve')`); e != nil {
		t.Fatal(e)
	}
	status(action(admin, d.Slug, "approve", d, ""), 500)
	after := get(alice, d.Slug)
	if after.Revision != d.Revision || after.CurrentVersionID != "" || after.IsLive || after.Status != "in_review" {
		t.Fatal("publication partially committed after audit failure")
	}
	if _, e = pool.Exec(ctx, `ALTER TABLE audit_log DROP CONSTRAINT fixture_reject_approve`); e != nil {
		t.Fatal(e)
	}
	// Two reviewers of the same revision: one success, one conflict.
	var wg sync.WaitGroup
	codes := make(chan int, 2)
	for range 2 {
		wg.Add(1)
		go func() { defer wg.Done(); codes <- action(admin, d.Slug, "approve", d, "").Code }()
	}
	wg.Wait()
	close(codes)
	counts := map[int]int{}
	for code := range codes {
		counts[code]++
	}
	if counts[200] != 1 || counts[409] != 1 {
		t.Fatal("double approval", counts)
	}
	live := get(alice, d.Slug)
	originalVersion := live.CurrentVersionID
	consumer := get(bob, d.Slug)
	if consumer.Name != input.Name || len(consumer.Versions) != 1 || consumer.DraftVersionID != "" || consumer.Published != nil || consumer.Allowlist != nil || consumer.IsOwner || consumer.IsAdmin {
		t.Fatal("consumer projection leaked management data", consumer)
	}
	status(action(alice, live.Slug, "new-draft", live, ""), 200)
	d = get(alice, live.Slug)
	input.Revision = d.Revision
	if _, e = s.Save(ctx, alice, d.Slug, input, nil); e != registry.ErrConflict {
		t.Fatal("published version overwritten", e)
	}
	input.Version = "2"
	input.Name = "Pending restricted name"
	input.MCPEndpoint = "https://mcp.example.test/v2"
	input.Visibility = "allowlist"
	input.Allowlist = []string{"ou_" + bob}
	d, e = s.Save(ctx, alice, d.Slug, input, nil)
	if e != nil {
		t.Fatal(e)
	}
	if got := get(eve, d.Slug); got.Name != "Original visible name" || got.Versions[0].MCPEndpoint != "https://mcp.example.test/v1" {
		t.Fatal("unapproved content visible")
	}
	catalog := call(eve, "GET", "/api/v1/catalog?q=Original", nil, "")
	status(catalog, 200)
	if !strings.Contains(catalog.Body.String(), d.Slug) || strings.Contains(catalog.Body.String(), "Pending restricted name") {
		t.Fatal("catalog leaked draft snapshot")
	}
	noDraft := call(eve, "GET", "/api/v1/catalog?q=Pending", nil, "")
	status(noDraft, 200)
	if strings.Contains(noDraft.Body.String(), d.Slug) {
		t.Fatal("catalog searched unapproved content")
	}
	status(action(alice, d.Slug, "submit", d, ""), 200)
	d = get(admin, d.Slug)
	if get(eve, d.Slug).CurrentVersionID != originalVersion {
		t.Fatal("old publication vanished in review")
	}
	status(action(admin, d.Slug, "approve", d, ""), 200)
	d = get(alice, d.Slug)
	status(call(eve, "GET", "/api/v1/capabilities/"+d.Slug, nil, ""), 404)
	if got := get(bob, d.Slug); got.Name != input.Name || got.CurrentVersionID == originalVersion || len(got.Versions) != 1 {
		t.Fatal("promotion mismatch")
	}
	// Department permission is from trusted profiles only, with atomic audit + CAS.
	override := registry.ActionRequest{Revision: d.Revision, VersionID: d.CurrentVersionID, Reason: "收紧为部门范围", Visibility: "department", Department: "Engineering"}
	status(call(alice, "POST", "/api/v1/capabilities/"+d.Slug+"/visibility", override, "csrf"), 403)
	status(call(admin, "POST", "/api/v1/capabilities/"+d.Slug+"/visibility", override, "csrf"), 200)
	status(call(bob, "GET", "/api/v1/capabilities/"+d.Slug+"?department=Engineering", nil, ""), 404)
	dept := registry.DepartmentRequest{Department: "Engineering", Revision: 0, Reason: "已核对部门"}
	deptPath := "/api/v1/governance/profiles/ou_" + bob + "/department"
	status(call(bob, "PUT", deptPath, dept, "csrf"), 403)
	status(call(admin, "PUT", deptPath, dept, "bad"), 403)
	status(call(admin, "PUT", deptPath, dept, "csrf"), 200)
	status(call(admin, "PUT", deptPath, dept, "csrf"), 409)
	status(call(bob, "GET", "/api/v1/capabilities/"+d.Slug, nil, ""), 200)
	status(call(eve, "GET", "/api/v1/catalog/"+d.Slug, nil, ""), 404)
	dept.Revision = 1
	dept.Department = ""
	status(call(admin, "PUT", deptPath, dept, "csrf"), 200)
	status(call(bob, "GET", "/api/v1/capabilities/"+d.Slug, nil, ""), 404)
	d = get(alice, d.Slug)
	status(action(alice, d.Slug, "offline", d, "临时维护"), 200)
	d = get(alice, d.Slug)
	status(call(bob, "GET", "/api/v1/capabilities/"+d.Slug, nil, ""), 404)
	status(action(alice, d.Slug, "offline", d, "again"), 409)
	status(action(alice, d.Slug, "submit", d, "重新上线"), 200)
	d = get(admin, d.Slug)
	status(action(admin, d.Slug, "approve", d, ""), 200)
	// Re-submission of the same version must preserve an Admin's live ACL override.
	status(call(bob, "GET", "/api/v1/capabilities/"+d.Slug, nil, ""), 404)
	if got := get(alice, d.Slug); got.Published == nil || got.Published.Visibility != "department" {
		t.Fatal("offline resubmission silently undid admin visibility override")
	}
	// An edit and submit of the same revision cannot both win.
	racing := input
	racing.Slug = "concurrent-submit"
	racing.Revision = 0
	racing.Version = "1"
	raceDraft, e := s.Save(ctx, alice, "", racing, nil)
	if e != nil {
		t.Fatal(e)
	}
	racing.Revision = raceDraft.Revision
	racing.Name = "Concurrent edit"
	results := make(chan int, 2)
	wg.Add(2)
	go func() {
		defer wg.Done()
		_, err := s.Save(ctx, alice, raceDraft.Slug, racing, nil)
		code := 200
		if err == registry.ErrConflict {
			code = 409
		} else if err != nil {
			code = 500
		}
		results <- code
	}()
	go func() { defer wg.Done(); results <- action(alice, raceDraft.Slug, "submit", raceDraft, "").Code }()
	wg.Wait()
	close(results)
	counts = map[int]int{}
	for code := range results {
		counts[code]++
	}
	if counts[200] != 1 || counts[409] != 1 {
		t.Fatal("edit/submit race", counts)
	}
	finalRace := get(alice, raceDraft.Slug)
	if finalRace.Status == "in_review" && finalRace.Name == "Concurrent edit" {
		t.Fatal("submission silently switched to edited content")
	}
	// A failure in create/edit audit cannot leave an unlogged draft or edit.
	broken := input
	broken.Slug = "audit-rollback"
	broken.Revision = 0
	broken.Version = "1"
	if _, e = pool.Exec(ctx, `ALTER TABLE audit_log ADD CONSTRAINT fixture_reject_create CHECK(action<>'create') NOT VALID`); e != nil {
		t.Fatal(e)
	}
	if _, e = s.Save(ctx, alice, "", broken, nil); e == nil {
		t.Fatal("create audit failure accepted")
	}
	status(call(alice, "GET", "/api/v1/capabilities/"+broken.Slug, nil, ""), 404)
	if _, e = pool.Exec(ctx, `ALTER TABLE audit_log DROP CONSTRAINT fixture_reject_create`); e != nil {
		t.Fatal(e)
	}
	bd, e := s.Save(ctx, alice, "", broken, nil)
	if e != nil {
		t.Fatal(e)
	}
	broken.Revision = bd.Revision
	broken.Name = "Uncommitted edit"
	if _, e = pool.Exec(ctx, `ALTER TABLE audit_log ADD CONSTRAINT fixture_reject_edit CHECK(action<>'edit') NOT VALID`); e != nil {
		t.Fatal(e)
	}
	if _, e = s.Save(ctx, alice, bd.Slug, broken, nil); e == nil {
		t.Fatal("edit audit failure accepted")
	}
	unchanged := get(alice, bd.Slug)
	if unchanged.Name != bd.Name || unchanged.Revision != bd.Revision {
		t.Fatal("edit survived audit rollback")
	}
	if _, e = pool.Exec(ctx, `ALTER TABLE audit_log DROP CONSTRAINT fixture_reject_edit`); e != nil {
		t.Fatal(e)
	}
	// Skill drafts are repairable but publication checks real stored files.
	skill := registry.SaveRequest{Slug: "governed-skill", Type: "skill", Name: "Skill", Description: "Use a template", Visibility: "org", Version: "1"}
	files := []registry.File{{Name: "SKILL.md", Data: []byte("# Legacy draft")}}
	sd, e := s.Save(ctx, alice, "", skill, files)
	if e != nil {
		t.Fatal(e)
	}
	status(action(alice, sd.Slug, "submit", sd, ""), 400)
	status(action(admin, sd.Slug, "force-publish", sd, "紧急上线"), 400)
	skill.Revision = sd.Revision
	files[0].Data = []byte("---\nname: governed-skill\ndescription: Use a template\n---\n# Instructions\n[Template](references/template.md)")
	files = append(files, registry.File{Name: "references/template.md", Data: []byte("# Template")})
	sd, e = s.Save(ctx, alice, sd.Slug, skill, files)
	if e != nil {
		t.Fatal(e)
	}
	status(action(alice, sd.Slug, "submit", sd, ""), 200)
	sd = get(admin, sd.Slug)
	filePath := "/api/v1/capabilities/" + sd.Slug + "/versions/" + sd.DraftVersionID + "/files"
	status(call(admin, "GET", filePath, nil, ""), 200)
	status(call(bob, "GET", filePath, nil, ""), 404)
	status(action(admin, sd.Slug, "approve", sd, ""), 200)
	sd = get(alice, sd.Slug)
	oldSkill := sd.CurrentVersionID
	bundlePath := "/api/v1/capabilities/" + sd.Slug + "/versions/" + oldSkill + "/bundle"
	status(call(bob, "GET", bundlePath, nil, ""), 200)
	status(action(alice, sd.Slug, "new-draft", sd, ""), 200)
	sd = get(alice, sd.Slug)
	skill.Revision = sd.Revision
	skill.Version = "2"
	sd, e = s.Save(ctx, alice, sd.Slug, skill, files)
	if e != nil {
		t.Fatal(e)
	}
	status(call(bob, "GET", "/api/v1/capabilities/"+sd.Slug+"/versions/"+sd.DraftVersionID+"/bundle", nil, ""), 404)
	status(action(alice, sd.Slug, "submit", sd, ""), 200)
	sd = get(admin, sd.Slug)
	status(action(admin, sd.Slug, "approve", sd, ""), 200)
	status(call(bob, "GET", bundlePath, nil, ""), 404)
	status(call(alice, "GET", bundlePath, nil, ""), 200)
	sd = get(alice, sd.Slug)
	status(action(admin, sd.Slug, "offline", sd, "管理员下线"), 200)
	sd = get(admin, sd.Slug)
	status(action(admin, sd.Slug, "force-publish", sd, "恢复已校验版本"), 200)
	// An administrator's own publication uses the same state machine.
	own := input
	own.Slug = "admin-own"
	own.Revision = 0
	own.Version = "1"
	own.Visibility = "org"
	own.Allowlist = nil
	ad, e := s.Save(ctx, admin, "", own, nil)
	if e != nil {
		t.Fatal(e)
	}
	status(action(admin, ad.Slug, "submit", ad, ""), 200)
	ad = get(admin, ad.Slug)
	status(action(admin, ad.Slug, "approve", ad, ""), 200)
	logs := call(admin, "GET", "/api/v1/governance/audit?target="+d.ID+"&limit=100", nil, "")
	status(logs, 200)
	var page registry.AuditPage
	if e = json.Unmarshal(logs.Body.Bytes(), &page); e != nil {
		t.Fatal(e)
	}
	var approvals int
	for _, entry := range page.Items {
		if entry.Action == "approve" {
			approvals++
		}
		if entry.Action != "create" && entry.Action != "edit" && entry.RequestID == "" {
			t.Fatal("missing audit request id")
		}
	}
	if approvals != 3 {
		t.Fatalf("expected 3 committed approvals, got %d", approvals)
	}
	for _, secret := range []string{"access_token", "key_ciphertext", "https://mcp.example.test", "Original description", "Pending restricted name"} {
		if strings.Contains(logs.Body.String(), secret) {
			t.Fatal("audit copied sensitive or verbose content")
		}
	}
	status(call(admin, "GET", "/api/v1/governance/audit?offset=-1", nil, ""), 400)
	firstPage := call(admin, "GET", "/api/v1/governance/audit?action=approve&limit=1", nil, "")
	status(firstPage, 200)
	nextPage := call(admin, "GET", "/api/v1/governance/audit?action=approve&limit=1&offset=1", nil, "")
	status(nextPage, 200)
	var firstAudit, nextAudit registry.AuditPage
	if json.Unmarshal(firstPage.Body.Bytes(), &firstAudit) != nil || json.Unmarshal(nextPage.Body.Bytes(), &nextAudit) != nil || !firstAudit.HasMore || len(firstAudit.Items) != 1 || len(nextAudit.Items) != 1 || firstAudit.Items[0].ID == nextAudit.Items[0].ID || firstAudit.Items[0].Action != "approve" {
		t.Fatal("audit action filtering/pagination failed")
	}
	t.Log("governance: bootstrap, review/reject, atomic audit rollback, concurrent approval, published snapshots, org/allowlist/trusted department, Skill validation/files/downloads, forced operations, self review and audit permission passed")
	return map[string]string{"owner": alice, "admin": admin, "consumer": bob, "outsider": eve}
}
