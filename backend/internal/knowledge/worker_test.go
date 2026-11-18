package knowledge

import (
	"bytes"
	"context"
	"log/slog"
	"strings"
	"testing"

	"github.com/riverqueue/river/rivertype"
)

type indexFailRepo struct {
	Repository
	code string
}

func (r *indexFailRepo) LoadVersion(context.Context, string) (Version, error) {
	return Version{ID: "v", Model: "other", Dim: 4, Status: "pending"}, nil
}
func (r *indexFailRepo) Fail(_ context.Context, id, code string) error {
	if id != "v" {
		return ErrInvalid
	}
	r.code = code
	return nil
}

type fixedEmbed struct{}

func (fixedEmbed) Model() string { return "m" }
func (fixedEmbed) Dim() int      { return 4 }
func (fixedEmbed) Embed(context.Context, []string) ([][]float32, error) {
	return nil, nil
}

func TestProcessLogsFailureCodeWithoutDocumentContent(t *testing.T) {
	var buf bytes.Buffer
	previous := slog.Default()
	slog.SetDefault(slog.New(slog.NewTextHandler(&buf, nil)))
	t.Cleanup(func() { slog.SetDefault(previous) })
	repo := &indexFailRepo{}
	if err := New(repo, nil, fixedEmbed{}, 0.5).Process(WithIndexRequestID(context.Background(), "req-1"), "v"); err != nil {
		t.Fatal(err)
	}
	if repo.code != "embedding_configuration_changed" {
		t.Fatal(repo.code)
	}
	log := buf.String()
	if !strings.Contains(log, "version_id=v") || !strings.Contains(log, "failure_code=embedding_configuration_changed") || !strings.Contains(log, "request_id=req-1") {
		t.Fatalf("log=%q", log)
	}
	if strings.Contains(log, "发票") || strings.Contains(log, "content=") {
		t.Fatalf("document content logged: %q", log)
	}
}

func TestIndexJobUniqueKeyIsVersion(t *testing.T) {
	opts := (Job{}).InsertOpts()
	if !opts.UniqueOpts.ByArgs || opts.Queue != Queue || opts.MaxAttempts != 3 {
		t.Fatalf("%+v", opts)
	}
	for _, state := range opts.UniqueOpts.ByState {
		if state == rivertype.JobStateCompleted || state == rivertype.JobStateDiscarded {
			t.Fatalf("finished state %s would block a later retry", state)
		}
	}
}
