package http

import (
	"archive/zip"
	"bytes"
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/pressly/goose/v3"
	"github.com/zenith-wang/it-wiki/backend/internal/domain"
	"github.com/zenith-wang/it-wiki/backend/internal/registry"
)

func mcpPackageFixture(t *testing.T, content []byte) []byte {
	t.Helper()
	var b bytes.Buffer
	z := zip.NewWriter(&b)
	w, err := z.CreateHeader(&zip.FileHeader{Name: "README.md", Method: zip.Store})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = w.Write(content); err != nil {
		t.Fatal(err)
	}
	if err = z.Close(); err != nil {
		t.Fatal(err)
	}
	return b.Bytes()
}

// The caller owns a UUID disposable database; no business data or real accounts.
func verifyMCPPackageIntegration(t *testing.T, db *sql.DB, s *registry.Service, users map[string]string) {
	t.Helper()
	ctx := context.Background()
	var before, after int
	if err := db.QueryRow(`SELECT count(*) FROM capability_versions`).Scan(&before); err != nil {
		t.Fatal(err)
	}
	if err := goose.DownToContext(ctx, db, ".", 20); err != nil {
		t.Fatal(err)
	}
	if err := goose.UpContext(ctx, db, "."); err != nil {
		t.Fatal(err)
	}
	if err := db.QueryRow(`SELECT count(*) FROM capability_versions`).Scan(&after); err != nil || before != after {
		t.Fatal("migration lost existing versions", err)
	}
	call := func(role, method, path string, input any, files []registry.File, csrf bool) *httptest.ResponseRecorder {
		t.Helper()
		var raw []byte
		contentType := "application/json"
		if files != nil {
			raw, contentType = registryMultipart(t, input, files)
		} else {
			raw, _ = json.Marshal(input)
		}
		user := users[role]
		hash := sha256.Sum256([]byte("csrf"))
		auth := newTestAuthHandler(t, &fakeAuthFlow{}, &fakeSessionStore{session: domain.Session{UserID: user, CSRFTokenHash: hash[:]}}, fakeUserResolver{user: domain.User{ID: user}})
		req := httptest.NewRequest(method, path, bytes.NewReader(raw))
		if role != "" {
			req.AddCookie(&http.Cookie{Name: SessionCookieName, Value: "session"})
		}
		req.Header.Set("Origin", "https://app.example.test")
		req.Header.Set("Content-Type", contentType)
		if csrf {
			req.Header.Set(CSRFHeaderName, "csrf")
		}
		w := httptest.NewRecorder()
		NewCapabilityHubRouter(Handlers{Auth: auth, Registry: NewRegistryHandler(s)}).ServeHTTP(w, req)
		return w
	}
	check := func(w *httptest.ResponseRecorder, status int) {
		t.Helper()
		if w.Code != status {
			t.Fatalf("want %d got %d: %.1000s", status, w.Code, w.Body.String())
		}
	}
	decode := func(w *httptest.ResponseRecorder) registry.Detail {
		t.Helper()
		var d registry.Detail
		if err := json.Unmarshal(w.Body.Bytes(), &d); err != nil {
			t.Fatal(err)
		}
		return d
	}
	const root = "/api/v1/capabilities"
	const path = root + "/downloadable-mcp"
	get := func() registry.Detail {
		w := call("owner", "GET", path, nil, nil, false)
		check(w, 200)
		return decode(w)
	}
	act := func(role, kind string, d registry.Detail) registry.Detail {
		v := d.DraftVersionID
		if v == "" {
			v = d.CurrentVersionID
		}
		w := call(role, "POST", path+"/"+kind, registry.ActionRequest{Revision: d.Revision, VersionID: v, Reason: "package fixture"}, nil, true)
		check(w, 200)
		return get()
	}
	bundlePath := func(id string) string { return path + "/versions/" + id + "/bundle" }
	download := func(role, id string, want []byte) {
		t.Helper()
		w := call(role, "GET", bundlePath(id), nil, nil, false)
		check(w, 200)
		if !bytes.Equal(w.Body.Bytes(), want) || w.Header().Get("Content-Type") != "application/zip" || w.Header().Get("Content-Disposition") != "attachment; filename=downloadable-mcp.zip" || w.Header().Get("Cache-Control") != "no-store" {
			t.Fatal("download changed bytes or headers")
		}
	}
	input := registry.SaveRequest{Slug: "downloadable-mcp", Type: "mcp", Name: "可下载的 MCP", Description: "下载 ZIP 后按 README 运行。", Visibility: "allowlist", Allowlist: []string{"ou_" + users["consumer"]}, Version: "1.0.0", MCPTransport: "stdio", MCPAuthScheme: "none"}
	// Exercise HTTP upload above the old Skill 10 MiB ceiling.
	large := mcpPackageFixture(t, bytes.Repeat([]byte("x"), 11<<20))
	files := []registry.File{{Name: "release.zip", Data: large}}
	check(call("", "POST", root, input, files, true), 401)
	check(call("owner", "POST", root, input, files, false), 403)
	check(call("owner", "POST", root, input, nil, true), 400)
	w := call("owner", "POST", root, input, files, true)
	check(w, 201)
	d := decode(w)
	v1 := d.DraftVersionID
	if !d.Versions[0].HasBundle || d.Versions[0].MCPEndpoint != "" {
		t.Fatal("missing package-only version")
	}
	download("owner", v1, large)
	for _, role := range []string{"consumer", "outsider", "reviewer"} {
		check(call(role, "GET", bundlePath(v1), nil, nil, false), 404)
	}
	check(call("owner", "GET", path+"/versions/"+v1+"/files", nil, nil, false), 404)
	input.Revision = d.Revision
	check(call("consumer", "PUT", path, input, files, true), 404)
	// Keep a package for metadata-only edits, replace it only on explicit upload.
	w = call("owner", "PUT", path, input, nil, true)
	check(w, 200)
	d = decode(w)
	download("owner", v1, large)
	input.Revision = d.Revision
	small := mcpPackageFixture(t, []byte("# MCP example\nInstall dependencies and run the entrypoint described here.\n"))
	files[0].Data = small
	for _, endpoint := range []string{"", "https://mcp.example.test/mcp"} {
		httpInput := input
		httpInput.Slug = "http-package-only"
		if endpoint != "" {
			httpInput.Slug = "remote-with-package"
		}
		httpInput.MCPTransport = "streamable-http"
		httpInput.MCPEndpoint = endpoint
		httpInput.Revision = 0
		check(call("owner", "POST", root, httpInput, files, true), 201)
	}
	w = call("owner", "PUT", path, input, files, true)
	check(w, 200)
	d = decode(w)
	download("owner", v1, small)
	check(call("owner", "PUT", path, input, files, true), 409)
	d = act("owner", "submit", d)
	download("reviewer", v1, small)
	check(call("consumer", "GET", bundlePath(v1), nil, nil, false), 404)
	if body, err := s.Download(registry.WithCredentialID(ctx, "fixture"), users["reviewer"], input.Slug, v1); !errors.Is(err, registry.ErrNotFound) {
		if body != nil {
			body.Close()
		}
		t.Fatal("API credential inherited reviewer access", err)
	}
	d = act("reviewer", "approve", d)
	download("consumer", v1, small)
	check(call("outsider", "GET", bundlePath(v1), nil, nil, false), 404)
	check(call("reviewer", "GET", bundlePath(v1), nil, nil, false), 404)
	input.Revision = d.Revision
	check(call("owner", "PUT", path, input, files, true), 409)
	d = act("owner", "new-draft", d)
	input.Revision = d.Revision
	input.Version = "2.0.0"
	check(call("owner", "PUT", path, input, nil, true), 400)
	newBytes := mcpPackageFixture(t, []byte("# MCP v2\nNew package.\n"))
	files[0].Data = newBytes
	w = call("owner", "PUT", path, input, files, true)
	check(w, 200)
	d = decode(w)
	v2 := d.DraftVersionID
	download("consumer", v1, small)
	check(call("consumer", "GET", bundlePath(v2), nil, nil, false), 404)
	d = act("owner", "submit", d)
	d = act("reviewer", "approve", d)
	download("consumer", v2, newBytes)
	check(call("consumer", "GET", bundlePath(v1), nil, nil, false), 404)
	download("owner", v1, small)
	// Visibility changes and offline revoke future downloads, including pinned URLs.
	w = call("admin", "POST", path+"/visibility", registry.ActionRequest{Revision: d.Revision, VersionID: v2, Reason: "revoke consumer", Visibility: "allowlist", Allowlist: []string{"ou_" + users["outsider"]}}, nil, true)
	check(w, 200)
	d = get()
	check(call("consumer", "GET", bundlePath(v2), nil, nil, false), 404)
	download("outsider", v2, newBytes)
	d = act("owner", "offline", d)
	check(call("outsider", "GET", bundlePath(v2), nil, nil, false), 404)
	if err := goose.DownToContext(ctx, db, ".", 20); err == nil {
		t.Fatal("destructive MCP package rollback allowed")
	}
	if version, err := goose.GetDBVersionContext(ctx, db); err != nil || version != 21 {
		t.Fatal("failed rollback changed migration version", version, err)
	}
	download("owner", v2, newBytes)
	// Leave a published package fixture for the opt-in browser acceptance.
	d = act("owner", "submit", d)
	d = act("reviewer", "approve", d)
	_ = d
	// Verify storage reads also work through the service without public bucket URLs.
	r, err := s.Download(ctx, users["owner"], input.Slug, v2)
	if err != nil {
		t.Fatal(err)
	}
	defer r.Close()
	b, err := io.ReadAll(r)
	if err != nil || !bytes.Equal(b, newBytes) {
		t.Fatal("stored package changed", err)
	}
	t.Log("MCP package: migration up/down, guarded rollback, >10MiB HTTP upload, exact download, draft/reviewer/consumer/API isolation, replacement/version publication, visibility and offline passed")
}
