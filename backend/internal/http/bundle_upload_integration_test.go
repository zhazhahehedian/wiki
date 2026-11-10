package http

import (
	"archive/zip"
	"bytes"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"slices"
	"testing"
	"time"

	"github.com/zenith-wang/it-wiki/backend/internal/domain"
	"github.com/zenith-wang/it-wiki/backend/internal/registry"
)

// largePackageFixture stores incompressible data so the ZIP has the given size.
func largePackageFixture(t *testing.T, payload int) []byte {
	t.Helper()
	var b bytes.Buffer
	z := zip.NewWriter(&b)
	w, err := z.Create("README.md")
	if err != nil {
		t.Fatal(err)
	}
	_, _ = w.Write([]byte("# Large MCP\nImport the ZIP into your client.\n"))
	w, err = z.CreateHeader(&zip.FileHeader{Name: "runtime/model.bin", Method: zip.Store})
	if err != nil {
		t.Fatal(err)
	}
	data := make([]byte, payload)
	_, _ = rand.Read(data)
	_, _ = w.Write(data)
	if err = z.Close(); err != nil {
		t.Fatal(err)
	}
	return b.Bytes()
}

// verifyBundleUploadIntegration drives the browser part-upload API against
// real PostgreSQL and MinIO, above the 100 MiB single-request limit.
func verifyBundleUploadIntegration(t *testing.T, db *sql.DB, s *registry.Service, users map[string]string) {
	t.Helper()
	ctx := context.Background()
	call := func(role, method, path string, body []byte, contentType string, csrf bool) *httptest.ResponseRecorder {
		t.Helper()
		user := users[role]
		hash := sha256.Sum256([]byte("csrf"))
		auth := newTestAuthHandler(t, &fakeAuthFlow{}, &fakeSessionStore{session: domain.Session{UserID: user, CSRFTokenHash: hash[:]}}, fakeUserResolver{user: domain.User{ID: user}})
		req := httptest.NewRequest(method, path, bytes.NewReader(body))
		req.AddCookie(&http.Cookie{Name: SessionCookieName, Value: "session"})
		req.Header.Set("Origin", "https://app.example.test")
		if contentType != "" {
			req.Header.Set("Content-Type", contentType)
		}
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
	jsonBody := func(v any) []byte {
		raw, _ := json.Marshal(v)
		return raw
	}
	var upload registry.BundleUpload
	decodeUpload := func(w *httptest.ResponseRecorder) registry.BundleUpload {
		t.Helper()
		var u registry.BundleUpload
		if err := json.Unmarshal(w.Body.Bytes(), &u); err != nil {
			t.Fatal(err)
		}
		return u
	}
	sendPart := func(role string, u registry.BundleUpload, data []byte, n int, status int) {
		t.Helper()
		start := int64(n-1) * u.PartSize
		end := min(start+u.PartSize, int64(len(data)))
		check(call(role, "PUT", fmt.Sprintf("/api/v1/bundle-uploads/%s/parts/%d", u.ID, n), data[start:end], "application/octet-stream", true), status)
	}
	start := func(name string, size int) registry.BundleUpload {
		t.Helper()
		w := call("owner", "POST", "/api/v1/bundle-uploads", jsonBody(map[string]any{"file_name": name, "size": size}), "application/json", true)
		check(w, 201)
		return decodeUpload(w)
	}

	data := largePackageFixture(t, 110<<20)
	check(call("owner", "POST", "/api/v1/bundle-uploads", jsonBody(map[string]any{"file_name": "big.zip", "size": len(data)}), "application/json", false), 403)
	check(call("owner", "POST", "/api/v1/bundle-uploads", jsonBody(map[string]any{"file_name": "big.zip", "size": registry.MaxMCPBundleBytes + 1}), "application/json", true), 400)
	upload = start("big.zip", len(data))
	if upload.PartCount != (len(data)+registry.UploadPartSize-1)/registry.UploadPartSize || len(upload.ReceivedParts) != 0 {
		t.Fatal("unexpected upload plan", upload)
	}
	status := "/api/v1/bundle-uploads/" + upload.ID
	// Interrupted transfer: some parts arrive, one is retried, the client resumes.
	sendPart("owner", upload, data, 2, 204)
	sendPart("owner", upload, data, 1, 204)
	sendPart("owner", upload, data, 1, 204)
	check(call("owner", "PUT", status+"/parts/3", data[:100], "application/octet-stream", true), 400)
	check(call("consumer", "PUT", status+"/parts/3", data[:registry.UploadPartSize], "application/octet-stream", true), 404)
	check(call("consumer", "GET", status, nil, "", false), 404)
	check(call("owner", "POST", status+"/complete", nil, "", true), 409)
	w := call("owner", "GET", status, nil, "", false)
	check(w, 200)
	if got := decodeUpload(w).ReceivedParts; !slices.Equal(got, []int{1, 2}) {
		t.Fatal("resume state wrong", got)
	}
	for n := 3; n <= upload.PartCount; n++ {
		sendPart("owner", upload, data, n, 204)
	}
	began := time.Now()
	w = call("owner", "POST", status+"/complete", nil, "", true)
	check(w, 200)
	done := decodeUpload(w)
	sum := sha256.Sum256(data)
	if !done.Completed || done.BundleSHA256 != hex.EncodeToString(sum[:]) || done.Size != int64(len(data)) {
		t.Fatal("completion result wrong", done)
	}
	t.Logf("assembled and inspected %d MiB in %v", len(data)>>20, time.Since(began).Round(time.Millisecond))
	check(call("owner", "POST", status+"/complete", nil, "", true), 200)

	input := registry.SaveRequest{Slug: "large-mcp-package", Type: "mcp", Name: "大体积 MCP", Description: "超过 100 MiB 的程序包。", Visibility: "allowlist", Allowlist: []string{"ou_" + users["consumer"]}, Version: "1.0.0", MCPTransport: "stdio", MCPAuthScheme: "none", BundleUploadID: upload.ID}
	other := input
	other.Slug = "stolen-upload"
	check(call("consumer", "POST", "/api/v1/capabilities", jsonBody(other), "application/json", true), 409)
	w = call("owner", "POST", "/api/v1/capabilities", jsonBody(input), "application/json", true)
	check(w, 201)
	var d registry.Detail
	if err := json.Unmarshal(w.Body.Bytes(), &d); err != nil {
		t.Fatal(err)
	}
	v := d.Versions[0]
	if !v.HasBundle || v.BundleSize != int64(len(data)) || v.BundleSHA256 != done.BundleSHA256 {
		t.Fatal("version does not carry the uploaded package", v)
	}
	// The consumed upload is gone and cannot be attached to another capability.
	check(call("owner", "GET", status, nil, "", false), 404)
	other.Slug = "reused-upload"
	check(call("owner", "POST", "/api/v1/capabilities", jsonBody(other), "application/json", true), 409)

	base := "/api/v1/capabilities/" + input.Slug + "/versions/" + v.ID
	w = call("owner", "GET", base+"/bundle", nil, "", false)
	check(w, 200)
	if !bytes.Equal(w.Body.Bytes(), data) || w.Header().Get("X-Bundle-SHA256") != done.BundleSHA256 {
		t.Fatal("downloaded package differs")
	}
	w = call("owner", "GET", base+"/files", nil, "", false)
	check(w, 200)
	var listing struct {
		Files []registry.ReviewFile `json:"files"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &listing); err != nil || len(listing.Files) != 2 || listing.Files[0].Content == nil || listing.Files[1].Content != nil {
		t.Fatal("ranged preview of a large package failed", err, listing)
	}

	// A malformed archive is rejected at completion and its object discarded.
	bad := bytes.Repeat([]byte("not a zip "), 1000)
	upload = start("bad.zip", len(bad))
	sendPart("owner", upload, bad, 1, 204)
	check(call("owner", "POST", "/api/v1/bundle-uploads/"+upload.ID+"/complete", nil, "", true), 400)
	check(call("owner", "GET", "/api/v1/bundle-uploads/"+upload.ID, nil, "", false), 404)

	// Cancel ends an unfinished upload.
	upload = start("cancel.zip", 1000)
	check(call("consumer", "DELETE", "/api/v1/bundle-uploads/"+upload.ID, nil, "", true), 404)
	check(call("owner", "DELETE", "/api/v1/bundle-uploads/"+upload.ID, nil, "", true), 204)
	check(call("owner", "GET", "/api/v1/bundle-uploads/"+upload.ID, nil, "", false), 404)

	// Expired unfinished uploads are swept.
	upload = start("expire.zip", 1000)
	if _, err := db.ExecContext(ctx, `UPDATE capability_bundle_uploads SET expires_at = now() - interval '1 minute' WHERE id = $1`, upload.ID); err != nil {
		t.Fatal(err)
	}
	check(call("owner", "GET", "/api/v1/bundle-uploads/"+upload.ID, nil, "", false), 404)
	if n, err := s.SweepExpiredUploads(ctx, time.Now()); err != nil || n != 1 {
		t.Fatal("expired upload sweep", n, err)
	}
	var left int
	if err := db.QueryRowContext(ctx, `SELECT count(*) FROM capability_bundle_uploads`).Scan(&left); err != nil || left != 0 {
		t.Fatal("upload records left behind", left, err)
	}
	t.Log("bundle uploads: 110 MiB part upload with resume, CSRF/owner isolation, completion digest, save by upload ID, exact download, ranged preview, invalid package, cancel and expiry passed")
}
