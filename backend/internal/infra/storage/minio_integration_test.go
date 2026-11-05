package storage

import (
	"bytes"
	"context"
	"io"
	"os"
	"slices"
	"testing"
	"time"

	"github.com/google/uuid"
)

// Opt-in: uses only random keys under a random prefix and deletes them.
func TestMinioListOlderIntegration(t *testing.T) {
	if os.Getenv("STORAGE_TEST_S3") != "1" {
		t.Skip("STORAGE_TEST_S3 not set")
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	m, err := NewMinioClient(ctx, MinioConfig{Endpoint: os.Getenv("S3_ENDPOINT"), AccessKey: os.Getenv("S3_ACCESS_KEY"), SecretKey: os.Getenv("S3_SECRET_KEY"), Bucket: os.Getenv("S3_BUCKET"), Region: os.Getenv("S3_REGION"), UsePathStyle: true})
	if err != nil {
		t.Fatal(err)
	}
	if err = m.EnsureBucket(ctx); err != nil {
		t.Fatal(err)
	}
	prefix := "storage-it-" + uuid.NewString() + "/"
	var keys []string
	for i := range 3 {
		key := prefix + uuid.NewString() + ".zip"
		data := bytes.Repeat([]byte{byte(i)}, 1024)
		if err = m.Put(ctx, key, bytes.NewReader(data), int64(len(data)), "application/zip"); err != nil {
			t.Fatal(err)
		}
		keys = append(keys, key)
	}
	defer func() {
		for _, key := range keys {
			_ = m.Delete(context.Background(), key)
		}
	}()
	future, err := m.ListOlder(ctx, prefix, time.Now().Add(time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	slices.Sort(future)
	slices.Sort(keys)
	if !slices.Equal(future, keys) {
		t.Fatal("ListOlder missed objects", future, keys)
	}
	if past, err := m.ListOlder(ctx, prefix, time.Now().Add(-time.Hour)); err != nil || len(past) != 0 {
		t.Fatal("ListOlder returned fresh objects", past, err)
	}
	r, err := m.Get(ctx, keys[0])
	if err != nil {
		t.Fatal(err)
	}
	got, _ := io.ReadAll(r)
	_ = r.Close()
	if len(got) != 1024 {
		t.Fatal("round trip changed object")
	}
	if err = m.Delete(ctx, keys[0]); err != nil {
		t.Fatal(err)
	}
	if left, _ := m.ListOlder(ctx, prefix, time.Now().Add(time.Hour)); len(left) != 2 {
		t.Fatal("delete not reflected", left)
	}
	t.Log("MinIO put/get/delete and age-filtered listing passed")
}
