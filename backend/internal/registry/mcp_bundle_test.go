package registry

import (
	"archive/zip"
	"bytes"
	"context"
	"errors"
	"fmt"
	"io/fs"
	"testing"
)

func mcpZIP(t *testing.T, names ...string) []byte {
	t.Helper()
	var b bytes.Buffer
	z := zip.NewWriter(&b)
	for _, name := range names {
		w, err := z.Create(name)
		if err != nil {
			t.Fatal(err)
		}
		if name[len(name)-1] != '/' {
			_, _ = w.Write([]byte("example package content"))
		}
	}
	if err := z.Close(); err != nil {
		t.Fatal(err)
	}
	return b.Bytes()
}

func TestMCPBundleValidation(t *testing.T) {
	valid := mcpZIP(t, "project/", "project/README.md", "project/server.py")
	out, err := validateMCPBundle(context.Background(), []File{{"release.ZIP", valid}})
	if err != nil || !bytes.Equal(out, valid) {
		t.Fatal("ZIP bytes changed", err)
	}
	for _, names := range [][]string{
		{"../secret"}, {"/absolute"}, {`dir\file`}, {"CON.txt"}, {"a."}, {"a:b"},
		{"a", "a/b"}, {"a/b", "A"}, {"README.md", "readme.md"}, {"only-dir/"},
		{"dup/", "dup/"}, {"a//b"}, {"a/../b"},
	} {
		t.Run(fmt.Sprint(names), func(t *testing.T) {
			if _, err := validateMCPBundle(context.Background(), []File{{"release.zip", mcpZIP(t, names...)}}); !errors.Is(err, ErrMCPPackage) {
				t.Fatal("unsafe archive accepted", err)
			}
		})
	}
	for _, files := range [][]File{nil, {{"not.zip", []byte("not a zip")}}, {{"a.tar", valid}}, {{"../a.zip", valid}}, {{"a.zip", valid}, {"b.zip", valid}}, {{"large.zip", make([]byte, MaxMCPBundleBytes+1)}}} {
		if _, err := validateMCPBundle(context.Background(), files); !errors.Is(err, ErrMCPPackage) {
			t.Fatal("invalid files accepted", err)
		}
	}
	for _, mode := range []fs.FileMode{fs.ModeSymlink | 0777, fs.ModeNamedPipe | 0600} {
		var b bytes.Buffer
		z := zip.NewWriter(&b)
		h := &zip.FileHeader{Name: "link"}
		h.SetMode(mode)
		w, _ := z.CreateHeader(h)
		_, _ = w.Write([]byte("target"))
		_ = z.Close()
		if _, err := validateMCPBundle(context.Background(), []File{{"a.zip", b.Bytes()}}); !errors.Is(err, ErrMCPPackage) {
			t.Fatal("special file accepted", err)
		}
	}
	for _, h := range []*zip.FileHeader{
		{Name: "large", Method: zip.Store, UncompressedSize64: MaxMCPExpandedBytes + 1},
		{Name: "encrypted", Method: zip.Store, Flags: 1},
		{Name: "unsupported", Method: 99},
	} {
		var b bytes.Buffer
		z := zip.NewWriter(&b)
		_, _ = z.CreateRaw(h)
		_ = z.Close()
		if _, err := validateMCPBundle(context.Background(), []File{{"a.zip", b.Bytes()}}); !errors.Is(err, ErrMCPPackage) {
			t.Fatal("invalid ZIP metadata accepted", err)
		}
	}
	names := make([]string, MaxMCPBundleEntries+1)
	for i := range names {
		names[i] = fmt.Sprintf("file-%d", i)
	}
	if _, err := validateMCPBundle(context.Background(), []File{{"a.zip", mcpZIP(t, names...)}}); !errors.Is(err, ErrMCPPackage) {
		t.Fatal("entry limit ignored")
	}
	corrupt := bytes.Clone(valid)
	// Corrupt a central-directory checksum while leaving structure readable.
	i := bytes.LastIndex(corrupt, []byte{'P', 'K', 1, 2})
	corrupt[i+16] ^= 1
	if _, err := validateMCPBundle(context.Background(), []File{{"a.zip", corrupt}}); !errors.Is(err, ErrMCPPackage) {
		t.Fatal("CRC corruption accepted", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := validateMCPBundle(ctx, []File{{"a.zip", valid}}); !errors.Is(err, context.Canceled) {
		t.Fatal("ignored cancellation", err)
	}
}

func TestMCPPackageSaveFailures(t *testing.T) {
	input := validMCP()
	input.MCPEndpoint = ""
	input.MCPTransport = "stdio"
	input.MCPAuthScheme = "none"
	if err := validate(&input); err != nil {
		t.Fatal("package-only metadata rejected", err)
	}
	r := &failingRepo{}
	storage := &recordingStorage{}
	if _, err := New(r, storage).Save(context.Background(), "user", "", input, nil); !errors.Is(err, ErrMCPPackage) || r.saves != 0 {
		t.Fatal("package-only MCP without package accepted", err)
	}
	files := []File{{"mcp.zip", mcpZIP(t, "README.md")}}
	for _, tc := range []struct {
		name           string
		upload, save   error
		referenced     bool
		deletes, saves int
	}{
		{"upload failure", errors.New("storage offline"), nil, false, 1, 0},
		{"conflict", nil, ErrConflict, false, 1, 1},
		{"referenced", nil, ErrConflict, true, 0, 1},
		{"ambiguous", nil, ErrCommitUnknown, false, 0, 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			r := &failingRepo{saveErr: tc.save, referenced: tc.referenced}
			storage := &recordingStorage{putErr: tc.upload}
			_, err := New(r, storage).Save(context.Background(), "user", "", input, files)
			if err == nil || storage.puts != 1 || storage.deletes != tc.deletes || r.saves != tc.saves {
				t.Fatalf("unsafe package save: %v %+v", err, storage)
			}
		})
	}
}
