package registry

import (
	"archive/zip"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io"
	"strings"
	"testing"
	"time"
)

type savingRepo struct {
	failingRepo
	saved Version
}

func (r *savingRepo) Save(_ context.Context, _, _ string, _ SaveRequest, v Version) (Detail, error) {
	r.saved = v
	return Detail{}, nil
}

func TestSaveRecordsBundleDigest(t *testing.T) {
	input := validMCP()
	data := mcpZIP(t, "README.md")
	r := &savingRepo{}
	if _, err := New(r, &recordingStorage{}).Save(context.Background(), "user", "", input, []File{{"mcp.zip", data}}); err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256(data)
	if r.saved.BundleSHA256 != hex.EncodeToString(sum[:]) || r.saved.BundleSize != int64(len(data)) || !strings.HasPrefix(r.saved.BundleKey, bundlePrefix) {
		t.Fatalf("digest not recorded: %+v", r.saved)
	}
}

type downloadRepo struct {
	failingRepo
	auditErr error
	audits   int
}

func (r *downloadRepo) Read(context.Context, string, string, bool) (Detail, error) {
	return Detail{Capability: Capability{ID: "cap"}, Versions: []Version{{ID: "v1", Version: "1.0.0", BundleKey: "registry/a.zip"}}}, nil
}
func (r *downloadRepo) RecordDownload(_ context.Context, _, capabilityID, versionID string) error {
	if capabilityID != "cap" || versionID != "v1" {
		return errors.New("wrong audit target")
	}
	r.audits++
	return r.auditErr
}

type bodyStorage struct {
	recordingStorage
	closed bool
}

type closeRecorder struct {
	io.Reader
	closed *bool
}

func (c closeRecorder) Close() error { *c.closed = true; return nil }

func (s *bodyStorage) Get(context.Context, string) (io.ReadCloser, error) {
	return closeRecorder{bytes.NewReader([]byte("zip")), &s.closed}, nil
}

func TestDownloadIsAuditedAndFailsClosed(t *testing.T) {
	r, storage := &downloadRepo{}, &bodyStorage{}
	v, body, err := New(r, storage).Download(context.Background(), "user", "slug", "v1")
	if err != nil || v.Version != "1.0.0" || r.audits != 1 {
		t.Fatal("audited download failed", err)
	}
	body.Close()
	if _, _, err = New(r, storage).Download(context.Background(), "user", "slug", "missing"); !errors.Is(err, ErrNotFound) || r.audits != 1 {
		t.Fatal("unknown version audited or served", err)
	}
	r.auditErr, storage.closed = errors.New("audit offline"), false
	if _, body, err = New(r, storage).Download(context.Background(), "user", "slug", "v1"); err == nil || body != nil || !storage.closed {
		t.Fatal("bundle served without audit")
	}
}

type sweepRepo struct {
	failingRepo
	used map[string]bool
}

func (r *sweepRepo) BundleReferenced(_ context.Context, key string) (bool, error) {
	return r.used[key], nil
}

type staticLister struct {
	keys   []string
	cutoff time.Time
}

func (l *staticLister) ListOlder(_ context.Context, prefix string, cutoff time.Time) ([]string, error) {
	if prefix != bundlePrefix {
		return nil, errors.New("unexpected prefix")
	}
	l.cutoff = cutoff
	return l.keys, nil
}

func TestSweepDeletesOnlyOldUnreferencedBundles(t *testing.T) {
	now := time.Date(2026, 11, 5, 12, 0, 0, 0, time.UTC)
	storage := &recordingStorage{}
	lister := &staticLister{keys: []string{"registry/used.zip", "registry/orphan.zip"}}
	n, err := New(&sweepRepo{used: map[string]bool{"registry/used.zip": true}}, storage).SweepOrphanBundles(context.Background(), lister, now)
	if err != nil || n != 1 || storage.deletes != 1 || !lister.cutoff.Equal(now.Add(-OrphanMinAge)) {
		t.Fatal("sweep removed wrong objects", n, err, storage.deletes, lister.cutoff)
	}
}

type zipStorage struct {
	recordingStorage
	data []byte
}

func (s *zipStorage) Get(context.Context, string) (io.ReadCloser, error) {
	return io.NopCloser(bytes.NewReader(s.data)), nil
}

type mcpReadRepo struct{ failingRepo }

func (r *mcpReadRepo) Read(context.Context, string, string, bool) (Detail, error) {
	return Detail{Capability: Capability{Type: "mcp"}, Versions: []Version{{ID: "v1", BundleKey: "registry/a.zip", HasBundle: true}}}, nil
}

func TestMCPFilesInlineSmallTextOnly(t *testing.T) {
	var b bytes.Buffer
	z := zip.NewWriter(&b)
	for name, content := range map[string][]byte{
		"pkg/README.md":  []byte("# 安装\n导入客户端"),
		"pkg/bin/tool":   {0, 1, 2},
		"pkg/large.json": bytes.Repeat([]byte("a"), maxPreviewFileBytes+1),
	} {
		w, _ := z.Create(name)
		_, _ = w.Write(content)
	}
	_, _ = z.Create("pkg/")
	_ = z.Close()
	files, err := New(&mcpReadRepo{}, &zipStorage{data: b.Bytes()}).Files(context.Background(), "user", "slug", "v1")
	if err != nil || len(files) != 3 {
		t.Fatal("unexpected listing", files, err)
	}
	for _, f := range files {
		inlined := f.Content != nil
		if inlined != (f.Name == "pkg/README.md") {
			t.Errorf("%s inlined=%v", f.Name, inlined)
		}
	}
}
