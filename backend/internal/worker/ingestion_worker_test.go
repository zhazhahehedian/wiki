package worker

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/riverqueue/river"

	"github.com/zenith-wang/it-wiki/backend/internal/domain"
	"github.com/zenith-wang/it-wiki/backend/internal/domain/ports"
	"github.com/zenith-wang/it-wiki/backend/internal/repo/generated"
)

func TestIngestionRemoteReadsPendingMarkdownAndAtomicallyPromotes(t *testing.T) {
	docID := uuid.New()
	pendingRef, pendingSum, pendingRevision := "feishu/doc/snapshot.md", "new-sum", "rev-2"
	activeRef, activeRevision := "feishu/doc/old.md", "rev-1"
	pendingTitle, pendingBytes := "New title", int64(18)
	pendingMetadata := json.RawMessage(`{"source_type":"feishu-docx","source_url":"https://acme.feishu.cn/docx/DocToken_123","remote_revision":"rev-2","image_url":null,"source_locator":"https://acme.feishu.cn/docx/DocToken_123"}`)
	repo := &fakeIngestionRepository{doc: generated.Document{
		ID: docID, KbID: uuid.New(), SourceType: "feishu-docx", SourceRef: "feishu://feishu.cn/docx/DocToken_123",
		ContentRef: &activeRef, RemoteRevision: &activeRevision, Checksum: "old-sum", Status: "ready", SyncStatus: "syncing",
		PendingContentRef: &pendingRef, PendingChecksum: &pendingSum, PendingRemoteRevision: &pendingRevision,
		PendingTitle: &pendingTitle, PendingBytes: &pendingBytes, PendingMetadata: pendingMetadata,
		Metadata: json.RawMessage(`{"source_type":"docx","source_url":"https://acme.feishu.cn/docx/DocToken_123","remote_revision":"rev-2"}`),
	}}
	storage := &recordingIngestionStorage{objects: map[string]string{pendingRef: "# New\n\nRemote body"}}
	parser := &recordingParser{}
	staged := &fakeStagedVectorStore{beforeReplace: func() {
		if len(storage.deleted) != 0 {
			t.Fatalf("active snapshot deleted before promotion: %v", storage.deleted)
		}
	}}
	worker := newIngestionWorkerForTest(repo, storage, parser, &fakeIngestionVectorStore{}, staged, fakeIngestionEmbedder{})

	err := worker.Work(context.Background(), &river.Job[IngestionJobArgs]{Args: IngestionJobArgs{
		DocumentID: docID.String(), PendingContentRef: pendingRef, PendingChecksum: pendingSum, PendingRemoteRevision: pendingRevision,
		Title: "stale job title", Bytes: 999, Metadata: json.RawMessage(`{"unknown_secret":"must-not-pass"}`),
	}})
	if err != nil {
		t.Fatalf("Work() error = %v", err)
	}
	if storage.gotKey != pendingRef || parser.mime != "text/markdown" || !strings.Contains(parser.body, "Remote body") {
		t.Fatalf("remote read = key:%q mime:%q body:%q", storage.gotKey, parser.mime, parser.body)
	}
	if staged.calls != 1 || staged.promotion.ContentRef != pendingRef || staged.promotion.Checksum != pendingSum || staged.promotion.RemoteRevision != pendingRevision {
		t.Fatalf("staged promotion = %+v calls=%d", staged.promotion, staged.calls)
	}
	if staged.promotion.Title != pendingTitle || staged.promotion.Bytes != pendingBytes || string(staged.promotion.Metadata) != string(pendingMetadata) {
		t.Fatalf("staged active metadata = %+v", staged.promotion)
	}
	if len(staged.items) != 1 || staged.items[0].Metadata["source_type"] != "feishu-docx" || staged.items[0].Metadata["remote_revision"] != "rev-2" {
		t.Fatalf("remote chunk metadata = %+v", staged.items)
	}
	if repo.statusCalls != 0 {
		t.Fatalf("remote ingestion used non-atomic status updates: %d", repo.statusCalls)
	}
	if len(storage.deleted) != 1 || storage.deleted[0] != activeRef {
		t.Fatalf("deleted snapshots=%v, want superseded active %q", storage.deleted, activeRef)
	}
}

func TestIngestionRemoteMetadataOnlyAtomicallyPatchesAndPromotesWithoutReembedding(t *testing.T) {
	docID := uuid.New()
	pendingRef, pendingSum, pendingRevision := "feishu/doc/snapshot.md", "same-sum", "rev-2"
	activeRef, activeRevision := "feishu/doc/old.md", "rev-1"
	pendingTitle, pendingBytes := "Updated title", int64(18)
	pendingMetadata := json.RawMessage(`{"source_type":"feishu-docx","source_url":"https://acme.feishu.cn/docx/DocToken_123","remote_revision":"rev-2","image_url":null,"source_locator":"https://acme.feishu.cn/docx/DocToken_123"}`)
	claimToken := time.Date(2026, 8, 9, 12, 0, 0, 0, time.UTC)
	repo := &fakeIngestionRepository{doc: generated.Document{
		ID: docID, KbID: uuid.New(), SourceType: "feishu-docx", SourceRef: "feishu://feishu.cn/docx/DocToken_123",
		ContentRef: &activeRef, RemoteRevision: &activeRevision, Checksum: pendingSum, Status: "ready", SyncStatus: "syncing", UpdatedAt: claimToken,
		PendingContentRef: &pendingRef, PendingChecksum: &pendingSum, PendingRemoteRevision: &pendingRevision,
		PendingTitle: &pendingTitle, PendingBytes: &pendingBytes, PendingMetadata: pendingMetadata,
	}}
	storage := &recordingIngestionStorage{objects: map[string]string{}}
	parser := &recordingParser{}
	regular := &fakeIngestionVectorStore{}
	staged := &fakeStagedVectorStore{}
	embedCalls, splitCalls := 0, 0
	worker := newIngestionWorkerForTest(repo, storage, parser, regular, staged, fakeIngestionEmbedder{calls: &embedCalls})
	worker.splitter = fakeIngestionSplitter{calls: &splitCalls}

	err := worker.Work(context.Background(), &river.Job[IngestionJobArgs]{Args: IngestionJobArgs{
		DocumentID: docID.String(), PendingContentRef: pendingRef, PendingChecksum: pendingSum, PendingRemoteRevision: pendingRevision,
		Title: "stale job title", Bytes: 999, Metadata: json.RawMessage(`{"unknown_secret":"must-not-pass"}`),
		ClaimToken: claimToken, MetadataOnly: true,
	}})
	if err != nil {
		t.Fatalf("Work() error = %v", err)
	}
	if storage.gotKey != "" || parser.body != "" || parser.mime != "" || splitCalls != 0 || embedCalls != 0 || regular.replaceCalls != 0 || staged.calls != 0 {
		t.Fatalf("metadata-only performed content pipeline: key=%q body=%q mime=%q split=%d embed=%d regular=%d staged=%d", storage.gotKey, parser.body, parser.mime, splitCalls, embedCalls, regular.replaceCalls, staged.calls)
	}
	if staged.patchCalls != 1 || staged.patchPromotion.Title != pendingTitle || staged.patchPromotion.Bytes != pendingBytes ||
		string(staged.patchPromotion.Metadata) != string(pendingMetadata) || staged.patchPromotion.ClaimToken != claimToken {
		t.Fatalf("metadata-only promotion = %+v calls=%d", staged.patchPromotion, staged.patchCalls)
	}
	patched := staged.patcher(map[string]any{"section_path": "Heading", "custom": "keep"})
	if patched["source_type"] != "feishu-docx" || patched["remote_revision"] != pendingRevision || patched["custom"] != "keep" {
		t.Fatalf("patched citation metadata = %+v", patched)
	}
	if len(storage.deleted) != 1 || storage.deleted[0] != activeRef {
		t.Fatalf("deleted snapshots=%v, want superseded active %q", storage.deleted, activeRef)
	}
}

func TestIngestionRemoteMetadataOnlyStalePromotionIsNoOp(t *testing.T) {
	docID := uuid.New()
	pendingRef, checksum, pendingRevision := "pending.md", "same-sum", "rev-2"
	activeRef := "active.md"
	pendingTitle, pendingBytes := "Updated title", int64(18)
	claimToken := time.Date(2026, 8, 9, 12, 0, 0, 0, time.UTC)
	repo := &fakeIngestionRepository{doc: generated.Document{
		ID: docID, KbID: uuid.New(), SourceType: "feishu-docx", ContentRef: &activeRef, Checksum: checksum, SyncStatus: "syncing", UpdatedAt: claimToken,
		PendingContentRef: &pendingRef, PendingChecksum: &checksum, PendingRemoteRevision: &pendingRevision,
		PendingTitle: &pendingTitle, PendingBytes: &pendingBytes,
		PendingMetadata: json.RawMessage(`{"source_type":"feishu-docx","source_url":"https://acme.feishu.cn/docx/DocToken_123","remote_revision":"rev-2","image_url":null,"source_locator":"https://acme.feishu.cn/docx/DocToken_123"}`),
	}}
	storage := &recordingIngestionStorage{}
	staged := &fakeStagedVectorStore{patchErr: ports.ErrStaleDocumentPromotion}
	worker := newIngestionWorkerForTest(repo, storage, &recordingParser{}, &fakeIngestionVectorStore{}, staged, fakeIngestionEmbedder{})

	err := worker.Work(context.Background(), &river.Job[IngestionJobArgs]{Args: IngestionJobArgs{
		DocumentID: docID.String(), PendingContentRef: pendingRef, PendingChecksum: checksum,
		PendingRemoteRevision: pendingRevision, ClaimToken: claimToken, MetadataOnly: true,
	}})
	if err != nil {
		t.Fatalf("Work() error = %v", err)
	}
	if staged.patchCalls != 1 || storage.gotKey != "" || repo.failed != nil {
		t.Fatalf("stale metadata-only result = patches:%d key:%q failure:%+v", staged.patchCalls, storage.gotKey, repo.failed)
	}
	if len(storage.deleted) != 0 {
		t.Fatalf("stale metadata-only promotion deleted active snapshot: %v", storage.deleted)
	}
}

func TestIngestionRemoteMetadataOnlyChecksumMismatchIsNoOp(t *testing.T) {
	docID := uuid.New()
	pendingRef, pendingChecksum, pendingRevision := "pending.md", "pending-sum", "rev-2"
	pendingTitle, pendingBytes := "Updated title", int64(18)
	claimToken := time.Date(2026, 8, 9, 12, 0, 0, 0, time.UTC)
	repo := &fakeIngestionRepository{doc: generated.Document{
		ID: docID, KbID: uuid.New(), SourceType: "feishu-docx", Checksum: "active-sum", SyncStatus: "syncing", UpdatedAt: claimToken,
		PendingContentRef: &pendingRef, PendingChecksum: &pendingChecksum, PendingRemoteRevision: &pendingRevision,
		PendingTitle: &pendingTitle, PendingBytes: &pendingBytes,
		PendingMetadata: json.RawMessage(`{"source_type":"feishu-docx","source_url":"https://acme.feishu.cn/docx/DocToken_123","remote_revision":"rev-2","image_url":null,"source_locator":"https://acme.feishu.cn/docx/DocToken_123"}`),
	}}
	storage := &recordingIngestionStorage{}
	staged := &fakeStagedVectorStore{}
	worker := newIngestionWorkerForTest(repo, storage, &recordingParser{}, &fakeIngestionVectorStore{}, staged, fakeIngestionEmbedder{})

	err := worker.Work(context.Background(), &river.Job[IngestionJobArgs]{Args: IngestionJobArgs{
		DocumentID: docID.String(), PendingContentRef: pendingRef, PendingChecksum: pendingChecksum,
		PendingRemoteRevision: pendingRevision, ClaimToken: claimToken, MetadataOnly: true,
	}})
	if err != nil {
		t.Fatalf("Work() error = %v", err)
	}
	if staged.patchCalls != 0 || storage.gotKey != "" || repo.failed != nil {
		t.Fatalf("mismatched metadata-only job was processed = patches:%d key:%q failure:%+v", staged.patchCalls, storage.gotKey, repo.failed)
	}
}

func TestIngestionRemoteMetadataOnlyPromotionFailureFailsExactPendingWithRedactedError(t *testing.T) {
	docID := uuid.New()
	pendingRef, checksum, pendingRevision := "pending.md", "same-sum", "rev-2"
	activeRef := "active.md"
	pendingTitle, pendingBytes := "Updated title", int64(18)
	claimToken := time.Date(2026, 8, 9, 12, 0, 0, 0, time.UTC)
	repo := &fakeIngestionRepository{doc: generated.Document{
		ID: docID, KbID: uuid.New(), SourceType: "feishu-docx", ContentRef: &activeRef, Checksum: checksum, SyncStatus: "syncing", UpdatedAt: claimToken,
		PendingContentRef: &pendingRef, PendingChecksum: &checksum, PendingRemoteRevision: &pendingRevision,
		PendingTitle: &pendingTitle, PendingBytes: &pendingBytes,
		PendingMetadata: json.RawMessage(`{"source_type":"feishu-docx","source_url":"https://acme.feishu.cn/docx/DocToken_123","remote_revision":"rev-2","image_url":null,"source_locator":"https://acme.feishu.cn/docx/DocToken_123"}`),
	}}
	storage := &recordingIngestionStorage{}
	staged := &fakeStagedVectorStore{patchErr: errors.New("database SECRET detail")}
	worker := newIngestionWorkerForTest(repo, storage, &recordingParser{}, &fakeIngestionVectorStore{}, staged, fakeIngestionEmbedder{})

	err := worker.Work(context.Background(), &river.Job[IngestionJobArgs]{Args: IngestionJobArgs{
		DocumentID: docID.String(), PendingContentRef: pendingRef, PendingChecksum: checksum,
		PendingRemoteRevision: pendingRevision, ClaimToken: claimToken, MetadataOnly: true,
	}})
	if err == nil || err.Error() != "snapshot promotion failed" || strings.Contains(err.Error(), "SECRET") {
		t.Fatalf("Work() error = %v", err)
	}
	if repo.failed == nil || repo.failed.PendingContentRef == nil || *repo.failed.PendingContentRef != pendingRef ||
		repo.failed.SafeError == nil || *repo.failed.SafeError != "snapshot promotion failed" || repo.failed.ClaimToken != claimToken {
		t.Fatalf("conditional metadata-only failure = %+v", repo.failed)
	}
	if storage.gotKey != "" {
		t.Fatalf("metadata-only failure read storage key %q", storage.gotKey)
	}
	if len(storage.deleted) != 0 {
		t.Fatalf("metadata-only promotion failure deleted active snapshot: %v", storage.deleted)
	}
}

func TestIngestionRemoteInvalidDurablePayloadFailsExpectedPendingWithoutReadingSnapshot(t *testing.T) {
	docID := uuid.New()
	pendingRef, pendingSum, pendingRevision := "pending.md", "new-sum", "rev-2"
	pendingTitle, pendingBytes := "New title", int64(11)
	claimToken := time.Date(2026, 8, 9, 12, 0, 0, 0, time.UTC)
	repo := &fakeIngestionRepository{doc: generated.Document{
		ID: docID, KbID: uuid.New(), SourceType: "feishu-docx", SyncStatus: "syncing", UpdatedAt: claimToken,
		PendingContentRef: &pendingRef, PendingChecksum: &pendingSum, PendingRemoteRevision: &pendingRevision,
		PendingTitle: &pendingTitle, PendingBytes: &pendingBytes,
		PendingMetadata: json.RawMessage(`{"unknown_secret":"must-not-pass"}`),
	}}
	storage := &recordingIngestionStorage{objects: map[string]string{pendingRef: "remote body"}}
	staged := &fakeStagedVectorStore{}
	worker := newIngestionWorkerForTest(repo, storage, &recordingParser{}, &fakeIngestionVectorStore{}, staged, fakeIngestionEmbedder{})

	err := worker.Work(context.Background(), &river.Job[IngestionJobArgs]{Args: IngestionJobArgs{
		DocumentID: docID.String(), PendingContentRef: pendingRef, PendingChecksum: pendingSum, PendingRemoteRevision: pendingRevision,
		Title: "apparently valid", Bytes: 11,
		Metadata:   json.RawMessage(`{"source_type":"feishu-docx","source_url":"https://acme.feishu.cn/docx/DocToken_123","remote_revision":"rev-2","image_url":null,"source_locator":"https://acme.feishu.cn/docx/DocToken_123"}`),
		ClaimToken: claimToken,
	}})
	if err == nil || err.Error() != "pending snapshot invalid" {
		t.Fatalf("Work() error = %v", err)
	}
	if repo.failed == nil || repo.failed.SafeError == nil || *repo.failed.SafeError != "pending snapshot invalid" || repo.failed.ClaimToken != claimToken {
		t.Fatalf("conditional remote failure = %+v", repo.failed)
	}
	if storage.gotKey != "" || staged.calls != 0 {
		t.Fatalf("invalid pending payload reached snapshot processing: key=%q promotions=%d", storage.gotKey, staged.calls)
	}
}

func TestIngestionRemoteStaleClaimTokenIsNoOp(t *testing.T) {
	docID := uuid.New()
	pendingRef, pendingSum, pendingRevision := "pending.md", "new-sum", "rev-2"
	pendingTitle, pendingBytes := "New title", int64(11)
	currentClaim := time.Date(2026, 8, 9, 12, 5, 0, 0, time.UTC)
	repo := &fakeIngestionRepository{doc: generated.Document{
		ID: docID, KbID: uuid.New(), SourceType: "feishu-docx", SyncStatus: "syncing", UpdatedAt: currentClaim,
		PendingContentRef: &pendingRef, PendingChecksum: &pendingSum, PendingRemoteRevision: &pendingRevision,
		PendingTitle: &pendingTitle, PendingBytes: &pendingBytes,
		PendingMetadata: json.RawMessage(`{"source_type":"feishu-docx","source_url":"https://acme.feishu.cn/docx/DocToken_123","remote_revision":"rev-2","image_url":null,"source_locator":"https://acme.feishu.cn/docx/DocToken_123"}`),
	}}
	storage := &recordingIngestionStorage{objects: map[string]string{pendingRef: "remote body"}}
	staged := &fakeStagedVectorStore{}
	worker := newIngestionWorkerForTest(repo, storage, &recordingParser{}, &fakeIngestionVectorStore{}, staged, fakeIngestionEmbedder{})

	err := worker.Work(context.Background(), &river.Job[IngestionJobArgs]{Args: IngestionJobArgs{
		DocumentID: docID.String(), PendingContentRef: pendingRef, PendingChecksum: pendingSum, PendingRemoteRevision: pendingRevision,
		ClaimToken: currentClaim.Add(-time.Minute),
	}})
	if err != nil {
		t.Fatalf("Work() error = %v", err)
	}
	if storage.gotKey != "" || staged.calls != 0 || repo.failed != nil {
		t.Fatalf("stale claim was processed: key=%q promotions=%d failure=%+v", storage.gotKey, staged.calls, repo.failed)
	}
}

func TestIngestionLocalUploadStillUsesSourceRefAndNormalReplacement(t *testing.T) {
	docID := uuid.New()
	repo := &fakeIngestionRepository{doc: generated.Document{
		ID: docID, KbID: uuid.New(), SourceType: "local-upload", SourceRef: "kb/doc/manual.txt", Title: "manual.txt",
		MimeType: "text/plain", Status: "pending", Metadata: json.RawMessage(`{}`),
	}}
	storage := &recordingIngestionStorage{objects: map[string]string{"kb/doc/manual.txt": "local body"}}
	parser := &recordingParser{}
	regular := &fakeIngestionVectorStore{}
	staged := &fakeStagedVectorStore{}
	worker := newIngestionWorkerForTest(repo, storage, parser, regular, staged, fakeIngestionEmbedder{})

	if err := worker.Work(context.Background(), &river.Job[IngestionJobArgs]{Args: IngestionJobArgs{DocumentID: docID.String()}}); err != nil {
		t.Fatalf("Work() error = %v", err)
	}
	if storage.gotKey != repo.doc.SourceRef || parser.mime != "text/plain" {
		t.Fatalf("local read = key:%q mime:%q", storage.gotKey, parser.mime)
	}
	if regular.replaceCalls != 1 || staged.calls != 0 || repo.lastStatus != "ready" {
		t.Fatalf("local replacement/status = regular:%d staged:%d status:%q", regular.replaceCalls, staged.calls, repo.lastStatus)
	}
}

func TestIngestionRemoteEmbeddingFailurePreservesActiveAndFailsOnlyExpectedPending(t *testing.T) {
	docID := uuid.New()
	pendingRef, pendingSum, pendingRevision := "pending.md", "new-sum", "rev-2"
	activeRef, activeRevision := "active.md", "rev-1"
	pendingTitle, pendingBytes := "New title", int64(11)
	pendingMetadata := json.RawMessage(`{"source_type":"feishu-docx","source_url":"https://acme.feishu.cn/docx/DocToken_123","remote_revision":"rev-2","image_url":null,"source_locator":"https://acme.feishu.cn/docx/DocToken_123"}`)
	claimToken := time.Date(2026, 8, 9, 12, 0, 0, 0, time.UTC)
	repo := &fakeIngestionRepository{doc: generated.Document{
		ID: docID, KbID: uuid.New(), SourceType: "feishu-docx", ContentRef: &activeRef, RemoteRevision: &activeRevision,
		Checksum: "old-sum", Status: "ready", SyncStatus: "syncing", UpdatedAt: claimToken, PendingContentRef: &pendingRef,
		PendingChecksum: &pendingSum, PendingRemoteRevision: &pendingRevision, PendingTitle: &pendingTitle,
		PendingBytes: &pendingBytes, PendingMetadata: pendingMetadata, Metadata: json.RawMessage(`{}`),
	}}
	storage := &recordingIngestionStorage{objects: map[string]string{pendingRef: "remote body"}}
	worker := newIngestionWorkerForTest(repo, storage, &recordingParser{}, &fakeIngestionVectorStore{}, &fakeStagedVectorStore{}, fakeIngestionEmbedder{err: errors.New("provider SECRET body")})

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	err := worker.Work(ctx, &river.Job[IngestionJobArgs]{Args: IngestionJobArgs{
		DocumentID: docID.String(), PendingContentRef: pendingRef, PendingChecksum: pendingSum, PendingRemoteRevision: pendingRevision,
		Title: "New title", Bytes: 11, Metadata: json.RawMessage(`{}`), ClaimToken: claimToken,
	}})
	if err == nil || strings.Contains(err.Error(), "SECRET") {
		t.Fatalf("Work() error = %v", err)
	}
	if repo.failed == nil || repo.failed.PendingContentRef == nil || *repo.failed.PendingContentRef != pendingRef || repo.failed.SafeError == nil || *repo.failed.SafeError != "embedding failed" {
		t.Fatalf("conditional remote failure = %+v", repo.failed)
	}
	if repo.failed.ClaimToken != claimToken {
		t.Fatalf("failure claim token = %v, want %v", repo.failed.ClaimToken, claimToken)
	}
	if repo.failContextErr != nil {
		t.Fatalf("failure cleanup context = %v, want detached context", repo.failContextErr)
	}
	if repo.doc.ContentRef == nil || *repo.doc.ContentRef != activeRef || repo.doc.Checksum != "old-sum" || repo.doc.RemoteRevision == nil || *repo.doc.RemoteRevision != activeRevision {
		t.Fatalf("active document changed on failure: %+v", repo.doc)
	}
	if len(storage.deleted) != 0 {
		t.Fatalf("embedding failure deleted active snapshot: %v", storage.deleted)
	}
}

func TestIngestionRemotePromotionFailurePreservesActiveSnapshot(t *testing.T) {
	docID := uuid.New()
	pendingRef, pendingSum, pendingRevision := "pending.md", "new-sum", "rev-2"
	activeRef, activeRevision := "active.md", "rev-1"
	pendingTitle, pendingBytes := "New title", int64(11)
	claimToken := time.Date(2026, 8, 9, 12, 0, 0, 0, time.UTC)
	repo := &fakeIngestionRepository{doc: generated.Document{
		ID: docID, KbID: uuid.New(), SourceType: "feishu-docx", ContentRef: &activeRef, RemoteRevision: &activeRevision,
		Checksum: "old-sum", Status: "ready", SyncStatus: "syncing", UpdatedAt: claimToken,
		PendingContentRef: &pendingRef, PendingChecksum: &pendingSum, PendingRemoteRevision: &pendingRevision,
		PendingTitle: &pendingTitle, PendingBytes: &pendingBytes,
		PendingMetadata: json.RawMessage(`{"source_type":"feishu-docx","source_url":"https://acme.feishu.cn/docx/DocToken_123","remote_revision":"rev-2","image_url":null,"source_locator":"https://acme.feishu.cn/docx/DocToken_123"}`),
	}}
	storage := &recordingIngestionStorage{objects: map[string]string{pendingRef: "remote body"}}
	staged := &fakeStagedVectorStore{replaceErr: errors.New("database SECRET body")}
	worker := newIngestionWorkerForTest(repo, storage, &recordingParser{}, &fakeIngestionVectorStore{}, staged, fakeIngestionEmbedder{})

	err := worker.Work(context.Background(), &river.Job[IngestionJobArgs]{Args: IngestionJobArgs{
		DocumentID: docID.String(), PendingContentRef: pendingRef, PendingChecksum: pendingSum,
		PendingRemoteRevision: pendingRevision, ClaimToken: claimToken,
	}})

	if err == nil || err.Error() != "snapshot promotion failed" || strings.Contains(err.Error(), "SECRET") {
		t.Fatalf("Work() error=%v", err)
	}
	if len(storage.deleted) != 0 {
		t.Fatalf("promotion failure deleted active snapshot: %v", storage.deleted)
	}
	if repo.doc.ContentRef == nil || *repo.doc.ContentRef != activeRef {
		t.Fatalf("active snapshot changed on promotion failure: %+v", repo.doc)
	}
}

func TestIngestionRemotePromotionDoesNotDeleteSharedSnapshotReference(t *testing.T) {
	docID := uuid.New()
	sharedRef, pendingSum, pendingRevision := "shared.md", "new-sum", "rev-2"
	pendingTitle, pendingBytes := "New title", int64(11)
	repo := &fakeIngestionRepository{doc: generated.Document{
		ID: docID, KbID: uuid.New(), SourceType: "feishu-docx", ContentRef: &sharedRef, Checksum: "old-sum",
		PendingContentRef: &sharedRef, PendingChecksum: &pendingSum, PendingRemoteRevision: &pendingRevision,
		PendingTitle: &pendingTitle, PendingBytes: &pendingBytes,
		PendingMetadata: json.RawMessage(`{"source_type":"feishu-docx","source_url":"https://acme.feishu.cn/docx/DocToken_123","remote_revision":"rev-2","image_url":null,"source_locator":"https://acme.feishu.cn/docx/DocToken_123"}`),
	}}
	storage := &recordingIngestionStorage{objects: map[string]string{sharedRef: "remote body"}}
	worker := newIngestionWorkerForTest(repo, storage, &recordingParser{}, &fakeIngestionVectorStore{}, &fakeStagedVectorStore{}, fakeIngestionEmbedder{})

	err := worker.Work(context.Background(), &river.Job[IngestionJobArgs]{Args: IngestionJobArgs{
		DocumentID: docID.String(), PendingContentRef: sharedRef, PendingChecksum: pendingSum, PendingRemoteRevision: pendingRevision,
	}})

	if err != nil {
		t.Fatalf("Work() error=%v", err)
	}
	if len(storage.deleted) != 0 {
		t.Fatalf("shared active/pending snapshot was deleted: %v", storage.deleted)
	}
}

func TestIngestionRemoteCleanupFailureIsBestEffortAndRedacted(t *testing.T) {
	docID := uuid.MustParse("d046f3f4-f690-4aa3-86a5-10dfd48c59cd")
	pendingRef, pendingSum, pendingRevision := "pending.md", "new-sum", "rev-2"
	activeRef := "tenant/token/https://secret.example/body.md"
	pendingTitle, pendingBytes := "New title", int64(11)
	claimToken := time.Date(2026, 8, 9, 12, 34, 56, 123000000, time.UTC)
	repo := &fakeIngestionRepository{doc: generated.Document{
		ID: docID, KbID: uuid.New(), SourceType: "feishu-docx", ContentRef: &activeRef, Checksum: "old-sum", UpdatedAt: claimToken,
		PendingContentRef: &pendingRef, PendingChecksum: &pendingSum, PendingRemoteRevision: &pendingRevision,
		PendingTitle: &pendingTitle, PendingBytes: &pendingBytes,
		PendingMetadata: json.RawMessage(`{"source_type":"feishu-docx","source_url":"https://acme.feishu.cn/docx/DocToken_123","remote_revision":"rev-2","image_url":null,"source_locator":"https://acme.feishu.cn/docx/DocToken_123"}`),
	}}
	storage := &recordingIngestionStorage{
		objects:   map[string]string{pendingRef: "remote body"},
		deleteErr: errors.New("provider SECRET https://secret.example response body"),
	}
	var output bytes.Buffer
	logger := slog.New(slog.NewJSONHandler(&output, nil))
	worker := NewIngestionWorker(WorkerDeps{
		Queries: repo, Storage: storage, Parser: &recordingParser{}, Splitter: fakeIngestionSplitter{},
		Embedder: fakeIngestionEmbedder{}, VStore: &fakeIngestionVectorStore{}, StagedVStore: &fakeStagedVectorStore{},
		ChunkSize: 200, Overlap: 20, BatchSize: 10, CleanupTimeout: time.Second, Logger: logger,
	})
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	err := worker.Work(ctx, &river.Job[IngestionJobArgs]{Args: IngestionJobArgs{
		DocumentID: docID.String(), PendingContentRef: pendingRef, PendingChecksum: pendingSum,
		PendingRemoteRevision: pendingRevision, ClaimToken: claimToken,
	}})

	if err != nil {
		t.Fatalf("cleanup changed successful promotion result: %v", err)
	}
	if len(storage.deleted) != 1 || storage.deleted[0] != activeRef || storage.deleteContextErr != nil {
		t.Fatalf("cleanup=%v contextErr=%v", storage.deleted, storage.deleteContextErr)
	}
	var record map[string]any
	if err := json.Unmarshal(output.Bytes(), &record); err != nil {
		t.Fatalf("decode log=%v output=%q", err, output.String())
	}
	if record["event"] != "feishu_snapshot_cleanup_failed" || record["error_code"] != "snapshot_delete_failed" ||
		record["document_id"] != docID.String() || record["storage_key_hash"] != shortStorageKeyHash(activeRef) {
		t.Fatalf("cleanup warning=%+v", record)
	}
	for _, secret := range []string{activeRef, "SECRET", "secret.example", "provider", "response body"} {
		if strings.Contains(output.String(), secret) {
			t.Fatalf("cleanup warning leaked %q: %s", secret, output.String())
		}
	}
}

func TestRemoteCitationMetadataFlattensTypedSectionLocations(t *testing.T) {
	tests := []struct {
		name        string
		metadata    string
		sectionPath string
		want        map[string]any
	}{
		{
			name:        "sheet",
			metadata:    `{"source_type":"feishu-sheet","source_url":"https://acme.feishu.cn/sheets/token","remote_revision":"r2","locations":[{"section_path":"Budget","sheet_name":"Budget","sheet_id":"sh_1","row_start":2,"row_end":8}]}`,
			sectionPath: "Budget",
			want:        map[string]any{"sheet_name": "Budget", "sheet_id": "sh_1", "row_start": 2, "row_end": 8},
		},
		{
			name:        "bitable",
			metadata:    `{"source_type":"feishu-bitable","source_url":"https://acme.feishu.cn/base/token","remote_revision":"r3","locations":[{"section_path":"Incidents / Open","table_id":"tbl_1","view_id":"vew_1","row_start":1,"row_end":4}]}`,
			sectionPath: "Incidents / Open",
			want:        map[string]any{"table_id": "tbl_1", "view_id": "vew_1", "row_start": 1, "row_end": 4},
		},
		{
			name:        "wiki delegated sheet",
			metadata:    `{"source_type":"feishu-wiki","source_url":"https://acme.feishu.cn/wiki/token","remote_revision":"r4","locations":[{"section_path":"Handbook / Oncall","sheet_name":"Oncall","sheet_id":"sh_2","row_start":3,"row_end":9}]}`,
			sectionPath: "Handbook / Oncall",
			want:        map[string]any{"sheet_name": "Oncall", "sheet_id": "sh_2", "row_start": 3, "row_end": 9},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := remoteCitationMetadata(json.RawMessage(tt.metadata), tt.sectionPath)
			if got["source_type"] == nil || got["source_url"] == nil || got["remote_revision"] == nil || got["section_path"] != tt.sectionPath {
				t.Fatalf("base citation metadata = %+v", got)
			}
			for key, want := range tt.want {
				if got[key] != want {
					t.Fatalf("metadata[%s] = %#v, want %#v; all=%+v", key, got[key], want, got)
				}
			}
			if _, exists := got["locations"]; exists {
				t.Fatalf("locations were not flattened: %+v", got)
			}
		})
	}
}

func newIngestionWorkerForTest(repo IngestionRepository, storage ports.ObjectStorage, parser ports.Parser, regular ports.VectorStore, staged ports.StagedVectorStore, embedder ports.Embedder) *IngestionWorker {
	return NewIngestionWorker(WorkerDeps{
		Queries: repo, Storage: storage, Parser: parser, Splitter: fakeIngestionSplitter{}, Embedder: embedder,
		VStore: regular, StagedVStore: staged, ChunkSize: 200, Overlap: 20, BatchSize: 10,
	})
}

type fakeIngestionRepository struct {
	doc            generated.Document
	statusCalls    int
	lastStatus     string
	failed         *generated.FailFeishuSyncParams
	failContextErr error
}

func (f *fakeIngestionRepository) GetDocument(context.Context, uuid.UUID) (generated.Document, error) {
	return f.doc, nil
}
func (f *fakeIngestionRepository) UpdateDocumentStatus(_ context.Context, in generated.UpdateDocumentStatusParams) error {
	f.statusCalls++
	f.lastStatus = in.Status
	return nil
}

func (f *fakeIngestionRepository) FailFeishuSync(ctx context.Context, in generated.FailFeishuSyncParams) (int64, error) {
	f.failed = &in
	f.failContextErr = ctx.Err()
	return 1, nil
}

type recordingIngestionStorage struct {
	objects          map[string]string
	gotKey           string
	deleted          []string
	deleteErr        error
	deleteContextErr error
}

func (s *recordingIngestionStorage) Put(context.Context, string, io.Reader, int64, string) error {
	return nil
}
func (s *recordingIngestionStorage) Get(_ context.Context, key string) (io.ReadCloser, error) {
	s.gotKey = key
	value, ok := s.objects[key]
	if !ok {
		return nil, errors.New("missing")
	}
	return io.NopCloser(strings.NewReader(value)), nil
}
func (s *recordingIngestionStorage) Delete(ctx context.Context, key string) error {
	s.deleted = append(s.deleted, key)
	s.deleteContextErr = ctx.Err()
	return s.deleteErr
}

type recordingParser struct{ body, mime string }

func (*recordingParser) Supports(string) bool { return true }
func (p *recordingParser) Parse(_ context.Context, body io.Reader, mime string) (*ports.ParseResult, error) {
	b, _ := io.ReadAll(body)
	p.body, p.mime = string(b), mime
	return &ports.ParseResult{Text: string(b), Metadata: map[string]any{"format": "text"}}, nil
}

type fakeIngestionSplitter struct{ calls *int }

func (f fakeIngestionSplitter) Split(_ context.Context, text string, _ ports.SplitOptions) ([]ports.SplitChunk, error) {
	if f.calls != nil {
		(*f.calls)++
	}
	return []ports.SplitChunk{{Content: text, TokenCount: 2}}, nil
}

type fakeIngestionEmbedder struct {
	err   error
	calls *int
}

func (f fakeIngestionEmbedder) Embed(_ context.Context, texts []string) ([][]float32, error) {
	if f.calls != nil {
		(*f.calls)++
	}
	if f.err != nil {
		return nil, f.err
	}
	out := make([][]float32, len(texts))
	for i := range out {
		out[i] = []float32{0.1}
	}
	return out, nil
}
func (fakeIngestionEmbedder) Dim() int      { return 1 }
func (fakeIngestionEmbedder) Model() string { return "fake" }

type fakeIngestionVectorStore struct{ replaceCalls int }

func (f *fakeIngestionVectorStore) ReplaceChunks(context.Context, string, []domain.ChunkWithEmbedding) error {
	f.replaceCalls++
	return nil
}
func (*fakeIngestionVectorStore) DeleteByDocument(context.Context, string) error { return nil }
func (*fakeIngestionVectorStore) Search(context.Context, string, []float32, ports.VectorSearchOptions) ([]ports.VectorSearchHit, error) {
	return nil, nil
}

type fakeStagedVectorStore struct {
	calls          int
	promotion      ports.PendingDocumentPromotion
	items          []domain.ChunkWithEmbedding
	patchCalls     int
	patchPromotion ports.PendingDocumentPromotion
	patcher        ports.ChunkMetadataPatcher
	patchErr       error
	replaceErr     error
	beforeReplace  func()
	state          *memorySyncRepository
}

func (f *fakeStagedVectorStore) ReplaceChunksAndPromote(_ context.Context, promotion ports.PendingDocumentPromotion, items []domain.ChunkWithEmbedding) error {
	if f.beforeReplace != nil {
		f.beforeReplace()
	}
	f.calls++
	f.promotion = promotion
	f.items = items
	return f.replaceErr
}

func (f *fakeStagedVectorStore) PatchChunkMetadataAndPromote(_ context.Context, promotion ports.PendingDocumentPromotion, patcher ports.ChunkMetadataPatcher) error {
	f.patchCalls++
	f.patchPromotion = promotion
	f.patcher = patcher
	if f.patchErr != nil {
		return f.patchErr
	}
	if f.state != nil {
		if f.state.promoteErr != nil {
			return f.state.promoteErr
		}
		contentRef, revision := promotion.ContentRef, promotion.RemoteRevision
		f.state.doc.ContentRef, f.state.doc.Checksum, f.state.doc.RemoteRevision = &contentRef, promotion.Checksum, &revision
		f.state.doc.Title, f.state.doc.Bytes, f.state.doc.Metadata = promotion.Title, promotion.Bytes, append([]byte(nil), promotion.Metadata...)
		f.state.doc.PendingContentRef, f.state.doc.PendingChecksum, f.state.doc.PendingRemoteRevision = nil, nil, nil
		f.state.doc.SyncStatus, f.state.doc.Status = "idle", "ready"
	}
	return nil
}
