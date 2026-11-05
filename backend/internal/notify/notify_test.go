package notify

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/riverqueue/river"
	"github.com/riverqueue/river/rivertype"
	"github.com/zenith-wang/it-wiki/backend/internal/registry"
)

type fakeRecipients struct {
	reviewers   []string
	downloaders []registry.Account
	visible     map[string]bool
}

func (f fakeRecipients) NotifyReviewers(context.Context) ([]string, error) { return f.reviewers, nil }
func (f fakeRecipients) NotifyDownloaders(context.Context, string) ([]registry.Account, error) {
	return f.downloaders, nil
}
func (f fakeRecipients) CanSeePublished(_ context.Context, userID, _ string) (bool, error) {
	return f.visible[userID], nil
}

func recipients(messages []Message) map[string]string {
	out := map[string]string{}
	for _, m := range messages {
		out[m.OpenID] = m.Text
	}
	return out
}

func TestPlanRecipients(t *testing.T) {
	r := fakeRecipients{
		reviewers:   []string{"ou_owner", "ou_rev", "ou_admin"},
		downloaders: []registry.Account{{UserID: "u1", OpenID: "ou_d1"}, {UserID: "u2", OpenID: "ou_d2"}, {UserID: "u3", OpenID: "ou_owner"}},
		visible:     map[string]bool{"u1": true},
	}
	base := registry.Event{Slug: "demo", Name: "演示", Type: "mcp", Version: "2.0.0", VersionID: "v2", OwnerOpenID: "ou_owner"}
	for _, tc := range []struct {
		name   string
		mutate func(*registry.Event)
		want   []string
		text   string
	}{
		{"submit goes to reviewers but not the submitter", func(e *registry.Event) { e.Action, e.ActorOpenID = "submit", "ou_owner" }, []string{"ou_rev", "ou_admin"}, "待审核"},
		{"first approval only tells the owner", func(e *registry.Event) { e.Action, e.ActorOpenID = "approve", "ou_rev" }, []string{"ou_owner"}, "已上线"},
		{"new version reaches downloaders who still have access", func(e *registry.Event) {
			e.Action, e.ActorOpenID, e.PreviousVersionID = "approve", "ou_rev", "v1"
		}, []string{"ou_owner", "ou_d1"}, ""},
		{"self-approval skips the owner", func(e *registry.Event) { e.Action, e.ActorOpenID = "approve", "ou_owner" }, nil, ""},
		{"rejection carries the reason", func(e *registry.Event) { e.Action, e.ActorOpenID, e.Reason = "reject", "ou_rev", "缺少 README" }, []string{"ou_owner"}, "缺少 README"},
		{"offline tells every past downloader", func(e *registry.Event) { e.Action, e.ActorOpenID = "offline", "ou_admin" }, []string{"ou_owner", "ou_d1", "ou_d2"}, "已下线"},
		{"visibility changes are silent", func(e *registry.Event) { e.Action, e.ActorOpenID = "visibility", "ou_admin" }, nil, ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ev := base
			tc.mutate(&ev)
			messages, err := Plan(context.Background(), r, "https://hub.example.test/", ev)
			if err != nil {
				t.Fatal(err)
			}
			got := recipients(messages)
			if len(got) != len(tc.want) || len(messages) != len(tc.want) {
				t.Fatalf("recipients = %v, want %v", got, tc.want)
			}
			for _, id := range tc.want {
				text, ok := got[id]
				// Offline downloaders get no link: the market page is gone.
				linked := ev.Action != "offline" || id == ev.OwnerOpenID
				if !ok || !strings.Contains(text, tc.text) || strings.Contains(text, "https://hub.example.test/hub/registry/demo") != linked {
					t.Fatalf("%s: %q", id, text)
				}
			}
		})
	}
}

type fakeSender struct {
	err       error
	permanent bool
	sent      []string
}

func (s *fakeSender) Send(_ context.Context, openID, _, _ string) error {
	s.sent = append(s.sent, openID)
	return s.err
}
func (s *fakeSender) Permanent(error) bool { return s.permanent }

func TestWorkersFanOutAndCancelPermanentFailures(t *testing.T) {
	var queued []SendJob
	w := &EventWorker{Recipients: fakeRecipients{reviewers: []string{"ou_a", "ou_b"}}, Enqueue: func(_ context.Context, jobs []SendJob) error {
		queued = jobs
		return nil
	}}
	job := &river.Job[EventJob]{JobRow: &rivertype.JobRow{ID: 7}, Args: EventJob{Event: registry.Event{Action: "submit", ActorOpenID: "ou_x"}}}
	if err := w.Work(context.Background(), job); err != nil || len(queued) != 2 || queued[0].Key == queued[1].Key || len(queued[0].Key) > 50 {
		t.Fatal("fan-out failed", err, queued)
	}
	sender := &fakeSender{err: errors.New("outside scope"), permanent: true}
	err := (&SendWorker{Sender: sender}).Work(context.Background(), &river.Job[SendJob]{Args: queued[0]})
	var cancel *rivertype.JobCancelError
	if !errors.As(err, &cancel) {
		t.Fatal("permanent failure retried", err)
	}
	sender.permanent = false
	err = (&SendWorker{Sender: sender}).Work(context.Background(), &river.Job[SendJob]{Args: queued[1]})
	if err == nil || errors.As(err, &cancel) {
		t.Fatal("transient failure not retried", err)
	}
}
