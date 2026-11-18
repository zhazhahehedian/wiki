// Package notify turns committed governance events into Feishu messages.
// An event job resolves recipients at send time; each recipient then gets an
// independent send job so one failure never repeats messages to others.
package notify

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"log/slog"
	"net/url"
	"strings"
	"time"

	"github.com/riverqueue/river"
	"github.com/zenith-wang/it-wiki/backend/internal/registry"
)

const Queue = "notify"

type EventJob struct {
	Event     registry.Event `json:"event"`
	RequestID string         `json:"request_id,omitempty"`
}

func (EventJob) Kind() string { return "registry_event_v1" }
func (EventJob) InsertOpts() river.InsertOpts {
	return river.InsertOpts{Queue: Queue, MaxAttempts: 10}
}

type SendJob struct {
	OpenID string `json:"open_id"`
	Text   string `json:"text"`
	// Key lets Feishu drop duplicates if a send is retried after success.
	Key string `json:"key"`
}

func (SendJob) Kind() string { return "feishu_message_v1" }
func (SendJob) InsertOpts() river.InsertOpts {
	// Identical sends (same recipient, text, and idempotency key) collapse while
	// an earlier copy is still queued or already completed, so an event retry
	// does not deliver the message twice.
	return river.InsertOpts{Queue: Queue, MaxAttempts: 8, UniqueOpts: river.UniqueOpts{ByArgs: true}}
}

type Recipients interface {
	NotifyReviewers(ctx context.Context) ([]string, error)
	NotifyDownloaders(ctx context.Context, capabilityID string) ([]registry.Account, error)
	CanSeePublished(ctx context.Context, userID, slug string) (bool, error)
}

// Sender delivers one text message. Permanent reports failures that retrying
// cannot fix, such as a user outside the app's availability scope.
type Sender interface {
	Send(ctx context.Context, openID, text, key string) error
	Permanent(err error) bool
}

type Message struct {
	OpenID string
	Text   string
}

// Plan builds the messages for one event. The actor is never notified of
// their own action; downloaders only hear about releases they can still see.
func Plan(ctx context.Context, r Recipients, origin string, ev registry.Event) ([]Message, error) {
	var out []Message
	seen := map[string]bool{ev.ActorOpenID: true}
	add := func(openID, text string) {
		if openID != "" && !seen[openID] {
			seen[openID] = true
			out = append(out, Message{OpenID: openID, Text: text})
		}
	}
	label := fmt.Sprintf("「%s」（%s v%s）", ev.Name, strings.ToUpper(ev.Type), ev.Version)
	manage := link(origin, "/hub/registry/"+url.PathEscape(ev.Slug))
	market := link(origin, "/hub/registry/"+url.PathEscape(ev.Slug)+"/market")
	reason := ""
	if ev.Reason != "" {
		reason = "\n原因：" + ev.Reason
	}
	switch ev.Action {
	case "submit":
		reviewers, err := r.NotifyReviewers(ctx)
		if err != nil {
			return nil, err
		}
		for _, id := range reviewers {
			add(id, "【能力中心】待审核："+label+"已提交审核。"+manage)
		}
	case "approve", "force-publish":
		add(ev.OwnerOpenID, "【能力中心】已上线："+label+"已通过审核并上线。"+manage)
		if ev.PreviousVersionID == "" {
			break
		}
		text := "【能力中心】版本更新：" + label + "已发布，请下载新版本替换。" + market
		if ev.PreviousVersionID == ev.VersionID {
			text = "【能力中心】重新上线：" + label + "已恢复上线。" + market
		}
		if err := addDownloaders(ctx, r, ev, true, func(id string) { add(id, text) }); err != nil {
			return nil, err
		}
	case "reject":
		add(ev.OwnerOpenID, "【能力中心】未通过审核："+label+reason+"\n修改后可重新提交。"+manage)
	case "offline":
		add(ev.OwnerOpenID, "【能力中心】已下线："+label+reason+manage)
		text := "【能力中心】已下线：" + label + "已停止分发，请停用已下载的版本。" + reason
		if err := addDownloaders(ctx, r, ev, false, func(id string) { add(id, text) }); err != nil {
			return nil, err
		}
	}
	return out, nil
}

// Offline notices go to every past downloader: they already hold the package.
// Release notices require current visibility so access changes are respected.
func addDownloaders(ctx context.Context, r Recipients, ev registry.Event, requireAccess bool, add func(string)) error {
	accounts, err := r.NotifyDownloaders(ctx, ev.CapabilityID)
	if err != nil {
		return err
	}
	for _, a := range accounts {
		if requireAccess {
			ok, err := r.CanSeePublished(ctx, a.UserID, ev.Slug)
			if err != nil {
				return err
			}
			if !ok {
				continue
			}
		}
		add(a.OpenID)
	}
	return nil
}

func link(origin, path string) string {
	if origin == "" {
		return ""
	}
	return "\n" + strings.TrimRight(origin, "/") + path
}

// messageKey is stable per event job and recipient, within Feishu's 50-char limit.
func messageKey(jobID int64, openID string) string {
	sum := sha256.Sum256([]byte(fmt.Sprintf("%d\x00%s", jobID, openID)))
	return hex.EncodeToString(sum[:])[:40]
}

type EventWorker struct {
	river.WorkerDefaults[EventJob]
	Recipients Recipients
	Origin     string
	Enqueue    func(context.Context, []SendJob) error
}

func (w *EventWorker) Work(ctx context.Context, job *river.Job[EventJob]) error {
	slog.InfoContext(ctx, "registry notification planned", "request_id", job.Args.RequestID, "action", job.Args.Event.Action, "slug", job.Args.Event.Slug)
	messages, err := Plan(ctx, w.Recipients, w.Origin, job.Args.Event)
	if err != nil || len(messages) == 0 {
		return err
	}
	sends := make([]SendJob, 0, len(messages))
	for _, m := range messages {
		sends = append(sends, SendJob{OpenID: m.OpenID, Text: m.Text, Key: messageKey(job.ID, m.OpenID)})
	}
	return w.Enqueue(ctx, sends)
}

type SendWorker struct {
	river.WorkerDefaults[SendJob]
	Sender Sender
}

func (w *SendWorker) Work(ctx context.Context, job *river.Job[SendJob]) error {
	err := w.Sender.Send(ctx, job.Args.OpenID, job.Args.Text, job.Args.Key)
	if err != nil && w.Sender.Permanent(err) {
		return river.JobCancel(err)
	}
	return err
}

func (w *SendWorker) Timeout(*river.Job[SendJob]) time.Duration { return 30 * time.Second }
