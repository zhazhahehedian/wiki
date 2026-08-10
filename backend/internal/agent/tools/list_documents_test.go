package tools

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/zenith-wang/it-wiki/backend/internal/domain"
)

type fakeDocLister struct {
	docs      []*domain.Document
	total     int
	gotKBID   string
	gotStatus *string
}

func (f *fakeDocLister) ListByKB(_ context.Context, _, kbID string, statusFilter *string, _, _ int) ([]*domain.Document, int, error) {
	f.gotKBID = kbID
	f.gotStatus = statusFilter
	return f.docs, f.total, nil
}

func TestListDocumentsFormatsDocs(t *testing.T) {
	lister := &fakeDocLister{
		docs: []*domain.Document{{
			Title: "Runbook.md", Status: domain.StatusReady, Bytes: 2048,
			UpdatedAt: time.Date(2026, 7, 8, 10, 0, 0, 0, time.UTC),
		}},
		total: 1,
	}
	tool := NewListDocuments(lister, "00000000-0000-0000-0000-000000000001", "kb-1")

	if tool.Name() != "list_documents" {
		t.Errorf("Name() = %q", tool.Name())
	}
	result, err := tool.Invoke(context.Background(), `{}`)
	if err != nil {
		t.Fatalf("Invoke() error = %v", err)
	}
	if lister.gotKBID != "kb-1" {
		t.Errorf("kbID = %q", lister.gotKBID)
	}
	for _, want := range []string{"Runbook.md", "ready", "2.0 KB"} {
		if !strings.Contains(result, want) {
			t.Errorf("result missing %q\nresult: %s", want, result)
		}
	}
}

func TestListDocumentsPassesStatusFilter(t *testing.T) {
	lister := &fakeDocLister{}
	tool := NewListDocuments(lister, "00000000-0000-0000-0000-000000000001", "kb-1")
	if _, err := tool.Invoke(context.Background(), `{"status":"failed"}`); err != nil {
		t.Fatalf("Invoke() error = %v", err)
	}
	if lister.gotStatus == nil || *lister.gotStatus != "failed" {
		t.Errorf("statusFilter = %v, want failed", lister.gotStatus)
	}
}

func TestListDocumentsRejectsInvalidStatus(t *testing.T) {
	tool := NewListDocuments(&fakeDocLister{}, "00000000-0000-0000-0000-000000000001", "kb-1")
	if _, err := tool.Invoke(context.Background(), `{"status":"bogus"}`); err == nil {
		t.Error("expected error for invalid status")
	}
}

func TestListDocumentsNotesTruncationWhenTotalExceedsLimit(t *testing.T) {
	docs := make([]*domain.Document, 50)
	for i := range docs {
		docs[i] = &domain.Document{Title: "d", Status: domain.StatusReady, UpdatedAt: time.Now()}
	}
	lister := &fakeDocLister{docs: docs, total: 120}
	result, err := NewListDocuments(lister, "00000000-0000-0000-0000-000000000001", "kb-1").Invoke(context.Background(), `{}`)
	if err != nil {
		t.Fatalf("Invoke() error = %v", err)
	}
	if !strings.Contains(result, "120") {
		t.Errorf("result should mention total 120: %s", result)
	}
}
