package main

import (
	"context"
	"net/url"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/riverqueue/river/riverdriver/riverpgxv5"
	"github.com/riverqueue/river/rivermigrate"
	"github.com/zenith-wang/it-wiki/backend/internal/registry"
	"github.com/zenith-wang/it-wiki/backend/internal/repo"
)

type recordingSender struct {
	mu   sync.Mutex
	sent map[string][]string
	keys map[string]bool
}

func (s *recordingSender) Send(_ context.Context, openID, text, key string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.sent[openID] = append(s.sent[openID], text)
	s.keys[key] = true
	return nil
}
func (s *recordingSender) Permanent(error) bool { return false }
func (s *recordingSender) messages(openID string) []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]string(nil), s.sent[openID]...)
}

// Opt-in: real PostgreSQL + River, local disposable database only; Feishu is a recorder.
func TestNotifierEndToEndIntegration(t *testing.T) {
	raw := os.Getenv("REGISTRY_TEST_DATABASE_URL")
	if raw == "" {
		t.Skip("REGISTRY_TEST_DATABASE_URL not set")
	}
	u, err := url.Parse(raw)
	if err != nil || (u.Hostname() != "127.0.0.1" && u.Hostname() != "localhost") {
		t.Fatal("notifier test requires a local disposable PostgreSQL")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	u.Path = "/postgres"
	admin, err := repo.OpenDatabase(u.String(), "")
	if err != nil {
		t.Fatal(err)
	}
	defer admin.Close()
	name := "capability_hub_notify_it_" + strings.ReplaceAll(uuid.NewString(), "-", "")
	if _, err = admin.ExecContext(ctx, `CREATE DATABASE "`+name+`"`); err != nil {
		t.Fatal(err)
	}
	defer func() { _, _ = admin.ExecContext(context.Background(), `DROP DATABASE "`+name+`" WITH (FORCE)`) }()
	u.Path = "/" + name
	t.Setenv("EMBEDDING_DIM", "1024")
	if err = runGooseMigrations(ctx, u.String()); err != nil {
		t.Fatal(err)
	}
	pool, err := repo.NewPool(ctx, u.String(), "")
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	migrator, err := rivermigrate.New(riverpgxv5.New(pool), nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = migrator.Migrate(ctx, rivermigrate.DirectionUp, nil); err != nil {
		t.Fatal(err)
	}
	owner, reviewer := uuid.NewString(), uuid.NewString()
	for _, id := range []string{owner, reviewer} {
		if _, err = pool.Exec(ctx, `INSERT INTO users(id,display_name) VALUES ($1,'Notify fixture')`, id); err != nil {
			t.Fatal(err)
		}
		if _, err = pool.Exec(ctx, `INSERT INTO oauth_accounts(user_id,provider,provider_user_id,tenant_key,access_token_encrypted,access_token_expires_at) VALUES ($1,'feishu',$2,'fixture',decode('00','hex'),now()+interval '1 hour')`, id, "ou_"+id); err != nil {
			t.Fatal(err)
		}
	}
	if _, err = pool.Exec(ctx, `INSERT INTO platform_profiles(open_id,is_reviewer) VALUES ($1,true)`, "ou_"+reviewer); err != nil {
		t.Fatal(err)
	}

	registryRepo := repo.NewRegistryRepository(pool)
	sender := &recordingSender{sent: map[string][]string{}, keys: map[string]bool{}}
	client, err := newNotifier(pool, registryRepo, "https://hub.example.test", sender)
	if err != nil {
		t.Fatal(err)
	}
	if err = client.Start(ctx); err != nil {
		t.Fatal(err)
	}
	defer func() {
		c, done := context.WithTimeout(context.Background(), 15*time.Second)
		defer done()
		_ = client.Stop(c)
	}()
	service := registry.New(registryRepo, nil)
	d, err := service.Save(ctx, owner, "", registry.SaveRequest{Slug: "notify-fixture", Type: "mcp", Name: "通知演示", Description: "notify", Visibility: "org", Version: "1.0.0", MCPEndpoint: "https://mcp.example.test/mcp", MCPTransport: "streamable-http", MCPAuthScheme: "none"}, nil)
	if err != nil {
		t.Fatal(err)
	}
	act := func(user, action string) {
		t.Helper()
		v := d.DraftVersionID
		if v == "" {
			v = d.CurrentVersionID
		}
		if d, err = service.Act(ctx, user, d.Slug, action, registry.ActionRequest{Revision: d.Revision, VersionID: v, Reason: "缺少 README"}); err != nil {
			t.Fatal(action, err)
		}
		if d, err = service.Get(ctx, owner, d.Slug); err != nil {
			t.Fatal(err)
		}
	}
	wait := func(openID, want string) {
		t.Helper()
		deadline := time.Now().Add(30 * time.Second)
		for time.Now().Before(deadline) {
			for _, text := range sender.messages(openID) {
				if strings.Contains(text, want) && strings.Contains(text, "https://hub.example.test/hub/registry/notify-fixture") {
					return
				}
			}
			time.Sleep(100 * time.Millisecond)
		}
		t.Fatalf("%s never received %q; got %v", openID, want, sender.messages(openID))
	}
	act(owner, "submit")
	wait("ou_"+reviewer, "待审核")
	act(reviewer, "reject")
	wait("ou_"+owner, "缺少 README")
	act(owner, "submit")
	act(reviewer, "approve")
	wait("ou_"+owner, "已上线")
	if got := sender.messages("ou_" + owner); len(got) != 2 {
		t.Fatal("owner got messages for own actions", got)
	}
	if got := sender.messages("ou_" + reviewer); len(got) != 2 {
		t.Fatal("reviewer messages", got)
	}
	t.Log("notifications: River enqueue in transition transaction, fan-out and delivery for submit/reject/approve passed")
}
