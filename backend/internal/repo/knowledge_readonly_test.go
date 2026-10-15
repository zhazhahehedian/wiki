package repo

import (
	"bufio"
	"context"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/jackc/pgx/v5"
	"github.com/zenith-wang/it-wiki/backend/internal/infra/storage"
)

// Explicit opt-in read-only inventory. Never starts the server or runs migrations.
func TestKnowledgeReadOnlyInventory(t *testing.T) {
	if os.Getenv("KNOWLEDGE_READONLY_CHECK") != "1" {
		t.Skip("read-only external inventory not requested")
	}
	f, e := os.Open("../../../.env")
	if e != nil {
		t.Fatal("configuration file unavailable")
	}
	defer f.Close()
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
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	t.Setenv("HTTP_PROXY", "")
	t.Setenv("HTTPS_PROXY", "")
	t.Setenv("NO_PROXY", "*")
	defer cancel()
	p, e := NewPool(ctx, os.Getenv("DATABASE_URL"), "")
	if e != nil {
		t.Fatal("database connection failed (details omitted)")
	}
	defer p.Close()
	tx, e := p.BeginTx(ctx, pgx.TxOptions{AccessMode: pgx.ReadOnly})
	if e != nil {
		t.Fatal("read-only transaction failed")
	}
	defer tx.Rollback(ctx)
	var migration int
	var bases, owned, documents, chunks int64
	var dim string
	if e = tx.QueryRow(ctx, "SELECT COALESCE(max(version_id),0) FROM goose_db_version WHERE is_applied").Scan(&migration); e != nil {
		t.Fatal("migration inventory failed")
	}
	if e = tx.QueryRow(ctx, "SELECT count(*),count(owner_user_id) FROM knowledge_bases").Scan(&bases, &owned); e != nil {
		t.Fatal("ownership inventory failed")
	}
	if e = tx.QueryRow(ctx, "SELECT count(*) FROM documents").Scan(&documents); e != nil {
		t.Fatal("document inventory failed")
	}
	if e = tx.QueryRow(ctx, "SELECT count(*) FROM chunks").Scan(&chunks); e != nil {
		t.Fatal("chunk inventory failed")
	}
	if e = tx.QueryRow(ctx, "SELECT format_type(atttypid,atttypmod) FROM pg_attribute WHERE attrelid='chunks'::regclass AND attname='embedding'").Scan(&dim); e != nil {
		t.Fatal("dimension inventory failed")
	}
	t.Logf("READ ONLY: migration=%d bases=%d with_owner=%d documents=%d legacy_chunks=%d embedding_type=%s", migration, bases, owned, documents, chunks, dim)
	var temporaryDatabases int
	if e = tx.QueryRow(ctx, `SELECT count(*) FROM pg_database WHERE datname LIKE 'capability_hub_k1_it_%'`).Scan(&temporaryDatabases); e != nil {
		t.Fatal("test database inventory failed")
	}
	t.Logf("remaining K1 disposable databases=%d", temporaryDatabases)
	t.Logf("embedding config present: base=%t key=%t model=%t dim=%t", os.Getenv("EMBEDDING_BASE_URL") != "", os.Getenv("EMBEDDING_API_KEY") != "", os.Getenv("EMBEDDING_MODEL") != "", os.Getenv("EMBEDDING_DIM") != "")
	mc, e := storage.NewMinioClient(ctx, storage.MinioConfig{Endpoint: os.Getenv("S3_ENDPOINT"), AccessKey: os.Getenv("S3_ACCESS_KEY"), SecretKey: os.Getenv("S3_SECRET_KEY"), Bucket: os.Getenv("S3_BUCKET"), Region: os.Getenv("S3_REGION"), UsePathStyle: true})
	if e != nil {
		t.Fatal("MinIO client unavailable")
	}
	if _, e = mc.Client().HeadBucket(ctx, &s3.HeadBucketInput{Bucket: aws.String(os.Getenv("S3_BUCKET"))}); e != nil {
		t.Fatal("MinIO read-only HeadBucket failed (details omitted)")
	}
	t.Log("MinIO read-only HeadBucket passed")
}
