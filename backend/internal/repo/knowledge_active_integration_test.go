package repo

import (
	"bufio"
	"context"
	"net/url"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/zenith-wang/it-wiki/backend/internal/repo/generated"
)

// Focused real PostgreSQL query regression in a UUID disposable database. This
// complements the full migration/auth/River test; it is not a migration test.
func TestKnowledgeActiveDocumentsCloud(t *testing.T) {
	if os.Getenv("KNOWLEDGE_TEST_DISPOSABLE") != "1" {
		t.Skip("isolated writes not requested")
	}
	f, e := os.Open("../../../.env")
	if e != nil {
		t.Fatal("configuration unavailable")
	}
	defer f.Close()
	scanner := bufio.NewScanner(f)
	dsn := ""
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if strings.HasPrefix(line, "DATABASE_URL=") {
			dsn = strings.Trim(strings.TrimPrefix(line, "DATABASE_URL="), "\"'")
		}
	}
	if scanner.Err() != nil || dsn == "" {
		t.Fatal("database config unavailable")
	}
	u, e := url.Parse(dsn)
	if e != nil {
		t.Fatal("invalid database config")
	}
	u.Path = "/postgres"
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	admin, e := OpenDatabase(u.String(), "")
	if e != nil {
		t.Fatal("database unavailable")
	}
	defer admin.Close()
	name := "capability_hub_k1_it_" + strings.ReplaceAll(uuid.NewString(), "-", "")
	if _, e = admin.ExecContext(ctx, `CREATE DATABASE "`+name+`"`); e != nil {
		t.Fatal("isolated create failed")
	}
	defer func() {
		c, done := context.WithTimeout(context.Background(), 20*time.Second)
		defer done()
		if _, e := admin.ExecContext(c, `DROP DATABASE "`+name+`" WITH (FORCE)`); e != nil {
			t.Error("isolated cleanup failed")
		}
	}()
	u.Path = "/" + name
	pool, e := NewPool(ctx, u.String(), "")
	if e != nil {
		t.Fatal("isolated pool failed")
	}
	defer pool.Close()
	if _, e = pool.Exec(ctx, `CREATE TABLE knowledge_bases(id uuid PRIMARY KEY,deleted_at timestamptz);
 CREATE TABLE documents(id uuid PRIMARY KEY,kb_id uuid,active_version_id uuid,deleted_at timestamptz)`); e != nil {
		t.Fatal(e)
	}
	live, hidden, other := uuid.New(), uuid.New(), uuid.New()
	for _, id := range []uuid.UUID{live, hidden, other} {
		if _, e = pool.Exec(ctx, `INSERT INTO knowledge_bases(id) VALUES($1)`, id); e != nil {
			t.Fatal(e)
		}
	}
	if _, e = pool.Exec(ctx, `UPDATE knowledge_bases SET deleted_at=now() WHERE id=$1`, hidden); e != nil {
		t.Fatal(e)
	}
	ids := []uuid.UUID{uuid.New(), uuid.New(), uuid.New(), uuid.New()}
	versions := []uuid.UUID{uuid.New(), uuid.New(), uuid.New(), uuid.New()}
	for i, kb := range []uuid.UUID{live, live, hidden, other} {
		if _, e = pool.Exec(ctx, `INSERT INTO documents(id,kb_id,active_version_id) VALUES($1,$2,$3)`, ids[i], kb, versions[i]); e != nil {
			t.Fatal(e)
		}
	}
	if _, e = pool.Exec(ctx, `UPDATE documents SET deleted_at=now() WHERE id=$1`, ids[1]); e != nil {
		t.Fatal(e)
	}
	q := generated.New(pool)
	check := func(want uuid.UUID) {
		t.Helper()
		rows, e := q.KnowledgeActiveDocuments(ctx, generated.KnowledgeActiveDocumentsParams{DocumentIds: ids, KbIds: []uuid.UUID{live, hidden}})
		if e != nil || len(rows) != 1 || rows[0].ID != ids[0] || ks(rows[0].ActiveVersionID) != want.String() {
			t.Fatal("batch visibility/version mismatch", rows, e)
		}
	}
	check(versions[0])
	replacement := uuid.New()
	if _, e = pool.Exec(ctx, `UPDATE documents SET active_version_id=$2 WHERE id=$1`, ids[0], replacement); e != nil {
		t.Fatal(e)
	}
	check(replacement)
	t.Log("real batch query: active replacement observed; deleted documents/bases and out-of-scope KB excluded")
}
