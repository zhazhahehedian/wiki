package http

import (
	"context"
	"errors"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/zenith-wang/it-wiki/backend/internal/registry"
	"github.com/zenith-wang/it-wiki/backend/internal/repo"
)

// verifyGovernanceEvents checks that notifications are enqueued inside the
// transition transaction: a failing sink rolls the transition back.
func verifyGovernanceEvents(t *testing.T, s *registry.Service, repository *repo.RegistryRepository, users map[string]string) {
	t.Helper()
	ctx := context.Background()
	var events []registry.Event
	var sinkErr error
	repository.WithEvents(func(_ context.Context, _ pgx.Tx, e registry.Event) error {
		if sinkErr != nil {
			return sinkErr
		}
		events = append(events, e)
		return nil
	})
	defer repository.WithEvents(nil)
	d, err := s.Save(ctx, users["owner"], "", registry.SaveRequest{Slug: "event-fixture", Type: "mcp", Name: "事件", Description: "notify", Visibility: "org", Version: "1.0.0", MCPEndpoint: "https://mcp.example.test/mcp", MCPTransport: "streamable-http", MCPAuthScheme: "none"}, nil)
	if err != nil {
		t.Fatal(err)
	}
	act := func(role, action string, d registry.Detail) (registry.Detail, error) {
		v := d.DraftVersionID
		if v == "" {
			v = d.CurrentVersionID
		}
		return s.Act(ctx, users[role], d.Slug, action, registry.ActionRequest{Revision: d.Revision, VersionID: v, Reason: "event fixture"})
	}
	if d, err = act("owner", "submit", d); err != nil {
		t.Fatal(err)
	}
	sinkErr = errors.New("queue unavailable")
	if _, err = act("admin", "approve", d); err == nil {
		t.Fatal("approval committed without its notification")
	}
	if current, _ := s.Get(ctx, users["owner"], d.Slug); current.Status != "in_review" {
		t.Fatal("failed enqueue did not roll back", current.Status)
	}
	sinkErr = nil
	if _, err = act("admin", "approve", d); err != nil {
		t.Fatal(err)
	}
	if len(events) != 2 || events[0].Action != "submit" || events[1].Action != "approve" || events[1].Version != "1.0.0" || events[1].ActorOpenID == events[1].OwnerOpenID || events[1].PreviousVersionID != "" {
		t.Fatalf("unexpected events %+v", events)
	}
	t.Log("governance events: transactional enqueue and rollback passed")
}
