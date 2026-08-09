package worker

import (
	"context"
	"encoding/json"
	"errors"
	"io"
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
	repo := &fakeIngestionRepository{doc: generated.Document{
		ID: docID, KbID: uuid.New(), SourceType: "feishu-docx", SourceRef: "feishu://feishu.cn/docx/DocToken_123",
		ContentRef: &activeRef, RemoteRevision: &activeRevision, Checksum: "old-sum", Status: "ready", SyncStatus: "syncing",
		PendingContentRef: &pendingRef, PendingChecksum: &pendingSum, PendingRemoteRevision: &pendingRevision,
		Metadata: json.RawMessage(`{"source_type":"docx","source_url":"https://acme.feishu.cn/docx/DocToken_123","remote_revision":"rev-2"}`),
	}}
	storage := &recordingIngestionStorage{objects: map[string]string{pendingRef: "# New\n\nRemote body"}}
	parser := &recordingParser{}
	staged := &fakeStagedVectorStore{}
	worker := newIngestionWorkerForTest(repo, storage, parser, &fakeIngestionVectorStore{}, staged, fakeIngestionEmbedder{})

	err := worker.Work(context.Background(), &river.Job[IngestionJobArgs]{Args: IngestionJobArgs{
		DocumentID: docID.String(), PendingContentRef: pendingRef, PendingChecksum: pendingSum, PendingRemoteRevision: pendingRevision,
		Title: "New title", Bytes: 18,
		Metadata: json.RawMessage(`{"source_type":"feishu-docx","source_url":"https://acme.feishu.cn/docx/DocToken_123","remote_revision":"rev-2"}`),
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
	if staged.promotion.Title != "New title" || staged.promotion.Bytes != 18 || !strings.Contains(string(staged.promotion.Metadata), "source_url") {
		t.Fatalf("staged active metadata = %+v", staged.promotion)
	}
	if len(staged.items) != 1 || staged.items[0].Metadata["source_type"] != "feishu-docx" || staged.items[0].Metadata["remote_revision"] != "rev-2" {
		t.Fatalf("remote chunk metadata = %+v", staged.items)
	}
	if repo.statusCalls != 0 {
		t.Fatalf("remote ingestion used non-atomic status updates: %d", repo.statusCalls)
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
	repo := &fakeIngestionRepository{doc: generated.Document{
		ID: docID, KbID: uuid.New(), SourceType: "feishu-docx", ContentRef: &activeRef, RemoteRevision: &activeRevision,
		Checksum: "old-sum", Status: "ready", SyncStatus: "syncing", PendingContentRef: &pendingRef,
		PendingChecksum: &pendingSum, PendingRemoteRevision: &pendingRevision, Metadata: json.RawMessage(`{}`),
	}}
	worker := newIngestionWorkerForTest(repo, &recordingIngestionStorage{objects: map[string]string{pendingRef: "remote body"}}, &recordingParser{}, &fakeIngestionVectorStore{}, &fakeStagedVectorStore{}, fakeIngestionEmbedder{err: errors.New("provider SECRET body")})

	claimToken := time.Date(2026, 8, 9, 12, 0, 0, 0, time.UTC)
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
	objects map[string]string
	gotKey  string
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
func (s *recordingIngestionStorage) Delete(context.Context, string) error { return nil }

type recordingParser struct{ body, mime string }

func (*recordingParser) Supports(string) bool { return true }
func (p *recordingParser) Parse(_ context.Context, body io.Reader, mime string) (*ports.ParseResult, error) {
	b, _ := io.ReadAll(body)
	p.body, p.mime = string(b), mime
	return &ports.ParseResult{Text: string(b), Metadata: map[string]any{"format": "text"}}, nil
}

type fakeIngestionSplitter struct{}

func (fakeIngestionSplitter) Split(_ context.Context, text string, _ ports.SplitOptions) ([]ports.SplitChunk, error) {
	return []ports.SplitChunk{{Content: text, TokenCount: 2}}, nil
}

type fakeIngestionEmbedder struct{ err error }

func (f fakeIngestionEmbedder) Embed(_ context.Context, texts []string) ([][]float32, error) {
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
	calls      int
	promotion  ports.PendingDocumentPromotion
	items      []domain.ChunkWithEmbedding
	patchCalls int
	state      *memorySyncRepository
}

func (f *fakeStagedVectorStore) ReplaceChunksAndPromote(_ context.Context, promotion ports.PendingDocumentPromotion, items []domain.ChunkWithEmbedding) error {
	f.calls++
	f.promotion = promotion
	f.items = items
	return nil
}

func (f *fakeStagedVectorStore) PatchChunkMetadataAndPromote(_ context.Context, promotion ports.PendingDocumentPromotion, _ ports.ChunkMetadataPatcher) error {
	f.patchCalls++
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
