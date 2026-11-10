package registry

import (
	"archive/zip"
	"bytes"
	"context"
	"crypto/md5"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"slices"
	"sync"
	"testing"
	"time"

	"github.com/zenith-wang/it-wiki/backend/internal/domain/ports"
)

// memoryUploadStorage is an in-memory multipart object store.
type memoryUploadStorage struct {
	mu       sync.Mutex
	objects  map[string][]byte
	uploads  map[string]map[int32][]byte
	rangeErr error
	aborts   int
}

func newMemoryUploadStorage() *memoryUploadStorage {
	return &memoryUploadStorage{objects: map[string][]byte{}, uploads: map[string]map[int32][]byte{}}
}

func (s *memoryUploadStorage) Put(_ context.Context, key string, r io.Reader, _ int64, _ string) error {
	data, err := io.ReadAll(r)
	s.mu.Lock()
	defer s.mu.Unlock()
	s.objects[key] = data
	return err
}
func (s *memoryUploadStorage) Get(_ context.Context, key string) (io.ReadCloser, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	data, ok := s.objects[key]
	if !ok {
		return nil, ErrNotFound
	}
	return io.NopCloser(bytes.NewReader(data)), nil
}
func (s *memoryUploadStorage) Delete(_ context.Context, key string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.objects, key)
	return nil
}
func (s *memoryUploadStorage) GetRange(_ context.Context, key string, offset, length int64) (io.ReadCloser, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.rangeErr != nil {
		return nil, s.rangeErr
	}
	data, ok := s.objects[key]
	if !ok || offset+length > int64(len(data)) {
		return nil, ErrNotFound
	}
	return io.NopCloser(bytes.NewReader(data[offset : offset+length])), nil
}
func (s *memoryUploadStorage) ObjectSize(_ context.Context, key string) (int64, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	data, ok := s.objects[key]
	if !ok {
		return 0, ErrNotFound
	}
	return int64(len(data)), nil
}
func (s *memoryUploadStorage) CreateMultipart(_ context.Context, key, _ string) (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	id := "mp-" + key
	s.uploads[id] = map[int32][]byte{}
	return id, nil
}
func (s *memoryUploadStorage) UploadPart(_ context.Context, _, id string, n int32, body io.ReadSeeker, _ int64) error {
	data, err := io.ReadAll(body)
	if err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	parts, ok := s.uploads[id]
	if !ok {
		return ErrNotFound
	}
	parts[n] = data
	return nil
}
func etag(data []byte) string { sum := md5.Sum(data); return hex.EncodeToString(sum[:]) }
func (s *memoryUploadStorage) ListParts(_ context.Context, _, id string) ([]ports.StoragePart, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	parts, ok := s.uploads[id]
	if !ok {
		return nil, ErrNotFound
	}
	out := []ports.StoragePart{}
	for n, data := range parts {
		out = append(out, ports.StoragePart{Number: n, Size: int64(len(data)), ETag: etag(data)})
	}
	slices.SortFunc(out, func(a, b ports.StoragePart) int { return int(a.Number - b.Number) })
	return out, nil
}
func (s *memoryUploadStorage) CompleteMultipart(_ context.Context, key, id string, parts []ports.StoragePart) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	stored, ok := s.uploads[id]
	if !ok {
		return ErrNotFound
	}
	var object []byte
	for _, p := range parts {
		data := stored[p.Number]
		if etag(data) != p.ETag {
			return fmt.Errorf("etag mismatch for part %d", p.Number)
		}
		object = append(object, data...)
	}
	s.objects[key] = object
	delete(s.uploads, id)
	return nil
}
func (s *memoryUploadStorage) AbortMultipart(_ context.Context, _, id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.aborts++
	delete(s.uploads, id)
	return nil
}

type memoryUploadRepo struct {
	mu   sync.Mutex
	rows map[string]memoryUploadRow
}
type memoryUploadRow struct {
	user string
	u    BundleUpload
}

func (r *memoryUploadRepo) CreateUpload(_ context.Context, user string, u BundleUpload, maxActive int) (BundleUpload, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	active := 0
	for _, row := range r.rows {
		if row.user == user && !row.u.Completed && row.u.ExpiresAt.After(time.Now()) {
			active++
		}
	}
	if active >= maxActive {
		return BundleUpload{}, ErrUploadQuota
	}
	u.CreatedAt = time.Now()
	r.rows[u.ID] = memoryUploadRow{user, u}
	return u, nil
}
func (r *memoryUploadRepo) GetUpload(_ context.Context, user, id string) (BundleUpload, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	row, ok := r.rows[id]
	if !ok || row.user != user {
		return BundleUpload{}, ErrNotFound
	}
	return row.u, nil
}
func (r *memoryUploadRepo) CompleteUpload(_ context.Context, user, id, sha string, findings []BundleFinding) (BundleUpload, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	row, ok := r.rows[id]
	if !ok || row.user != user || row.u.Completed {
		return BundleUpload{}, ErrConflict
	}
	row.u.Completed, row.u.BundleSHA256, row.u.BundleFindings = true, sha, findings
	r.rows[id] = row
	return row.u, nil
}
func (r *memoryUploadRepo) DeleteUpload(_ context.Context, user, id string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if row, ok := r.rows[id]; ok && row.user == user {
		delete(r.rows, id)
	}
	return nil
}
func (r *memoryUploadRepo) ExpiredUploads(_ context.Context, now time.Time) ([]BundleUpload, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	var out []BundleUpload
	for _, row := range r.rows {
		if row.u.ExpiresAt.Before(now) {
			out = append(out, row.u)
		}
	}
	return out, nil
}
func (r *memoryUploadRepo) DeleteExpiredUpload(_ context.Context, id string, now time.Time) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if row, ok := r.rows[id]; ok && row.u.ExpiresAt.Before(now) {
		delete(r.rows, id)
	}
	return nil
}

func uploadService(repo Repository) (*Service, *memoryUploadRepo, *memoryUploadStorage) {
	uploads := &memoryUploadRepo{rows: map[string]memoryUploadRow{}}
	store := newMemoryUploadStorage()
	return New(repo, store).WithUploads(uploads, store), uploads, store
}

// largeMCPZIP stores incompressible content so the archive spans several parts.
func largeMCPZIP(t *testing.T, payload int) []byte {
	t.Helper()
	var b bytes.Buffer
	z := zip.NewWriter(&b)
	w, err := z.Create("pkg/README.md")
	if err != nil {
		t.Fatal(err)
	}
	_, _ = w.Write([]byte("# install\n"))
	w, err = z.CreateHeader(&zip.FileHeader{Name: "pkg/model.bin", Method: zip.Store})
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

func sendParts(t *testing.T, s *Service, user string, u BundleUpload, data []byte, numbers ...int) {
	t.Helper()
	for _, n := range numbers {
		start := int64(n-1) * u.PartSize
		end := min(start+u.PartSize, int64(len(data)))
		if err := s.UploadPart(context.Background(), user, u.ID, n, bytes.NewReader(data[start:end])); err != nil {
			t.Fatalf("part %d: %v", n, err)
		}
	}
}

func TestPartUploadResumesCompletesAndSaves(t *testing.T) {
	ctx := context.Background()
	repo := &savingRepo{}
	s, uploads, store := uploadService(repo)
	data := largeMCPZIP(t, 2*UploadPartSize+1234)
	u, err := s.CreateUpload(ctx, "user", "release.zip", int64(len(data)))
	if err != nil || u.PartCount != 3 || u.PartSize != UploadPartSize {
		t.Fatal("create", u, err)
	}
	if _, err = s.CreateUpload(ctx, "user", "release.zip", MaxMCPBundleBytes+1); !errors.Is(err, ErrMCPPackage) {
		t.Fatal("oversized upload accepted", err)
	}
	// Out of order, with a retried part; then the client asks what arrived.
	sendParts(t, s, "user", u, data, 3, 1, 1)
	status, err := s.Upload(ctx, "user", u.ID)
	if err != nil || !slices.Equal(status.ReceivedParts, []int{1, 3}) || status.Completed {
		t.Fatal("status", status.ReceivedParts, err)
	}
	if _, err = s.CompleteUpload(ctx, "user", u.ID); !errors.Is(err, ErrUploadNotReady) {
		t.Fatal("completed with a missing part", err)
	}
	if err = s.UploadPart(ctx, "user", u.ID, 2, bytes.NewReader(data[:10])); !errors.Is(err, ErrInvalid) {
		t.Fatal("short part accepted", err)
	}
	if err = s.UploadPart(ctx, "user", u.ID, 4, bytes.NewReader(data[:10])); !errors.Is(err, ErrInvalid) {
		t.Fatal("part beyond the end accepted", err)
	}
	if _, err = s.Upload(ctx, "other", u.ID); !errors.Is(err, ErrNotFound) {
		t.Fatal("another user saw the upload", err)
	}
	sendParts(t, s, "user", u, data, 2)
	done, err := s.CompleteUpload(ctx, "user", u.ID)
	sum := sha256.Sum256(data)
	if err != nil || !done.Completed || done.BundleSHA256 != hex.EncodeToString(sum[:]) {
		t.Fatal("complete", done, err)
	}
	// Completion is idempotent.
	if again, err := s.CompleteUpload(ctx, "user", u.ID); err != nil || again.BundleSHA256 != done.BundleSHA256 {
		t.Fatal("repeat complete", err)
	}
	if err = s.UploadPart(ctx, "user", u.ID, 1, bytes.NewReader(data[:UploadPartSize])); !errors.Is(err, ErrConflict) {
		t.Fatal("part accepted after completion", err)
	}

	input := validMCP()
	input.MCPEndpoint, input.MCPTransport, input.MCPAuthScheme = "", "stdio", "none"
	input.BundleUploadID = u.ID
	other := input
	other.Type = "skill"
	if _, err = s.Save(ctx, "user", "", other, nil); !errors.Is(err, ErrInvalid) {
		t.Fatal("skill accepted an MCP upload", err)
	}
	if _, err = s.Save(ctx, "user", "", input, []File{{"a.zip", data}}); !errors.Is(err, ErrInvalid) {
		t.Fatal("upload and file accepted together", err)
	}
	if _, err = s.Save(ctx, "other", "", input, nil); !errors.Is(err, ErrUploadNotReady) {
		t.Fatal("another user saved the upload", err)
	}
	if _, err = s.Save(ctx, "user", "", input, nil); err != nil {
		t.Fatal(err)
	}
	if repo.saved.BundleKey != u.ObjectKey || repo.saved.BundleSize != int64(len(data)) || repo.saved.BundleSHA256 != done.BundleSHA256 {
		t.Fatalf("version does not reference the upload: %+v", repo.saved)
	}
	if !bytes.Equal(store.objects[u.ObjectKey], data) || len(uploads.rows) != 0 {
		t.Fatal("object changed or upload record kept after save")
	}
}

func TestFailedSaveKeepsCompletedUpload(t *testing.T) {
	ctx := context.Background()
	s, uploads, store := uploadService(&failingRepo{})
	data := mcpZIP(t, "README.md")
	u, _ := s.CreateUpload(ctx, "user", "a.zip", int64(len(data)))
	sendParts(t, s, "user", u, data, 1)
	if _, err := s.CompleteUpload(ctx, "user", u.ID); err != nil {
		t.Fatal(err)
	}
	input := validMCP()
	input.BundleUploadID = u.ID
	if _, err := s.Save(ctx, "user", "", input, nil); !errors.Is(err, ErrConflict) {
		t.Fatal(err)
	}
	if len(uploads.rows) != 1 || store.objects[u.ObjectKey] == nil {
		t.Fatal("failed save discarded the upload; a retry would need a re-upload")
	}
}

func TestInvalidUploadIsDiscardedButStorageFailureIsRetryable(t *testing.T) {
	ctx := context.Background()
	s, uploads, store := uploadService(&failingRepo{})
	bad := bytes.Repeat([]byte("not a zip"), 100)
	u, _ := s.CreateUpload(ctx, "user", "bad.zip", int64(len(bad)))
	sendParts(t, s, "user", u, bad, 1)
	if _, err := s.CompleteUpload(ctx, "user", u.ID); !errors.Is(err, ErrMCPPackage) {
		t.Fatal("invalid package accepted", err)
	}
	if len(uploads.rows) != 0 || store.objects[u.ObjectKey] != nil {
		t.Fatal("invalid package kept")
	}

	good := mcpZIP(t, "README.md")
	u, _ = s.CreateUpload(ctx, "user", "good.zip", int64(len(good)))
	sendParts(t, s, "user", u, good, 1)
	store.rangeErr = errors.New("storage unavailable")
	if _, err := s.CompleteUpload(ctx, "user", u.ID); err == nil || errors.Is(err, ErrMCPPackage) {
		t.Fatal("storage failure reported as a bad package", err)
	}
	if len(uploads.rows) != 1 || store.objects[u.ObjectKey] == nil {
		t.Fatal("transient failure discarded the assembled package")
	}
	store.rangeErr = nil
	if done, err := s.CompleteUpload(ctx, "user", u.ID); err != nil || !done.Completed {
		t.Fatal("retry after assembly failed", err)
	}
}

func TestExpiredUploadsAndQuota(t *testing.T) {
	ctx := context.Background()
	s, uploads, store := uploadService(&failingRepo{})
	var ids []string
	for range MaxActiveUploads {
		u, err := s.CreateUpload(ctx, "user", "a.zip", 100)
		if err != nil {
			t.Fatal(err)
		}
		ids = append(ids, u.ID)
	}
	if _, err := s.CreateUpload(ctx, "user", "a.zip", 100); !errors.Is(err, ErrUploadQuota) || store.aborts != 1 {
		t.Fatal("quota not enforced or storage upload leaked", err, store.aborts)
	}
	if err := s.CancelUpload(ctx, "user", ids[0]); err != nil || len(uploads.rows) != MaxActiveUploads-1 {
		t.Fatal("cancel", err)
	}
	later := time.Now().Add(UploadTTL + time.Minute)
	s.uploads.now = func() time.Time { return later }
	if _, err := s.Upload(ctx, "user", ids[1]); !errors.Is(err, ErrNotFound) {
		t.Fatal("expired upload still usable", err)
	}
	input := validMCP()
	input.BundleUploadID = ids[1]
	if _, err := s.Save(ctx, "user", "", input, nil); !errors.Is(err, ErrUploadNotReady) {
		t.Fatal("expired upload saved", err)
	}
	n, err := s.SweepExpiredUploads(ctx, later)
	if err != nil || n != MaxActiveUploads-1 || len(uploads.rows) != 0 || len(store.uploads) != 0 {
		t.Fatal("sweep", n, err, len(uploads.rows), len(store.uploads))
	}
}

func TestWindowReaderMatchesBytes(t *testing.T) {
	data := make([]byte, 10_000)
	_, _ = rand.Read(data)
	w := &windowReader{ctx: context.Background(), size: int64(len(data)), window: 777, read: func(_ context.Context, off, n int64) (io.ReadCloser, error) {
		return io.NopCloser(bytes.NewReader(data[off : off+n])), nil
	}}
	for _, tc := range []struct{ off, n int }{{0, 10}, {770, 20}, {5000, 3000}, {9990, 10}, {100, 1}} {
		got := make([]byte, tc.n)
		if n, err := w.ReadAt(got, int64(tc.off)); err != nil || n != tc.n || !bytes.Equal(got, data[tc.off:tc.off+tc.n]) {
			t.Fatal("ReadAt", tc, n, err)
		}
	}
	got := make([]byte, 20)
	if n, err := w.ReadAt(got, 9990); n != 10 || err != io.EOF {
		t.Fatal("read past end", n, err)
	}
}
