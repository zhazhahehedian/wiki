package worker

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/riverqueue/river"

	"github.com/zenith-wang/it-wiki/backend/internal/domain"
	"github.com/zenith-wang/it-wiki/backend/internal/domain/ports"
	"github.com/zenith-wang/it-wiki/backend/internal/repo/generated"
)

type FeishuTokenProvider interface {
	AccessToken(ctx context.Context, accountID string) (string, error)
}

type StagedIngestionEnqueuer interface {
	EnqueueStagedIngestion(ctx context.Context, snapshot PendingFeishuSnapshot) error
}

type StagedIngestionEnqueuerFunc func(context.Context, PendingFeishuSnapshot) error

func (f StagedIngestionEnqueuerFunc) EnqueueStagedIngestion(ctx context.Context, snapshot PendingFeishuSnapshot) error {
	return f(ctx, snapshot)
}

type PendingFeishuSnapshot struct {
	DocumentID     uuid.UUID
	ContentRef     string
	Checksum       string
	RemoteRevision string
	Title          string
	Bytes          int64
	Metadata       json.RawMessage
}

type FeishuSyncExpectation struct {
	DocumentID     uuid.UUID
	RemoteRevision *string
	Checksum       string
}

type StageFeishuSnapshotInput struct {
	Expected FeishuSyncExpectation
	Snapshot PendingFeishuSnapshot
}

type FailFeishuSyncInput struct {
	DocumentID uuid.UUID
	Pending    *PendingFeishuSnapshot
	SafeError  string
}

type FeishuSyncRepository interface {
	ClaimFeishuSync(ctx context.Context, documentID uuid.UUID) (generated.Document, error)
	CompleteUnchangedFeishuSync(ctx context.Context, input FeishuSyncExpectation) (bool, error)
	StageFeishuSnapshot(ctx context.Context, input StageFeishuSnapshotInput) (bool, error)
	PromoteFeishuSnapshot(ctx context.Context, snapshot PendingFeishuSnapshot) (bool, error)
	FailFeishuSync(ctx context.Context, input FailFeishuSyncInput) (bool, error)
}

type FeishuSyncWorkerDeps struct {
	Repository FeishuSyncRepository
	Resolver   ports.SourceResolver
	Loaders    map[domain.ResourceType]ports.SourceLoader
	Tokens     FeishuTokenProvider
	Storage    ports.ObjectStorage
	Ingestion  StagedIngestionEnqueuer
}

type FeishuSyncWorker struct {
	river.WorkerDefaults[FeishuSyncJobArgs]
	repo      FeishuSyncRepository
	resolver  ports.SourceResolver
	loaders   map[domain.ResourceType]ports.SourceLoader
	tokens    FeishuTokenProvider
	storage   ports.ObjectStorage
	ingestion StagedIngestionEnqueuer
}

func NewFeishuSyncWorker(deps FeishuSyncWorkerDeps) *FeishuSyncWorker {
	return &FeishuSyncWorker{repo: deps.Repository, resolver: deps.Resolver, loaders: deps.Loaders, tokens: deps.Tokens, storage: deps.Storage, ingestion: deps.Ingestion}
}

func (w *FeishuSyncWorker) Work(ctx context.Context, job *river.Job[FeishuSyncJobArgs]) error {
	documentID, err := uuid.Parse(job.Args.DocumentID)
	if err != nil {
		return errors.New("invalid document ID")
	}
	doc, err := w.repo.ClaimFeishuSync(ctx, documentID)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil
	}
	if err != nil {
		return errors.New("claim sync failed")
	}
	expected := FeishuSyncExpectation{DocumentID: documentID, RemoteRevision: cloneString(doc.RemoteRevision), Checksum: doc.Checksum}
	fail := func(pending *PendingFeishuSnapshot, safe string) error {
		_, _ = w.repo.FailFeishuSync(ctx, FailFeishuSyncInput{DocumentID: documentID, Pending: pending, SafeError: safe})
		return errors.New(safe)
	}
	if !doc.OauthAccountID.Valid {
		return fail(nil, "OAuth account unavailable")
	}
	token, err := w.tokens.AccessToken(ctx, uuid.UUID(doc.OauthAccountID.Bytes).String())
	if err != nil {
		return fail(nil, "access token unavailable")
	}
	if doc.SourceUrl == nil {
		return fail(nil, "source reference unavailable")
	}
	ref, err := w.resolver.Resolve(*doc.SourceUrl)
	if err != nil || ref.Identity != doc.SourceRef || "feishu-"+string(ref.Type) != doc.SourceType {
		return fail(nil, "source reference unavailable")
	}
	loader := w.loaders[ref.Type]
	if loader == nil {
		return fail(nil, "source type unsupported")
	}
	canonical, err := loader.Load(ctx, ref, token)
	if err != nil {
		return fail(nil, "source load failed")
	}
	if doc.RemoteRevision != nil && *doc.RemoteRevision == canonical.RemoteRevision {
		ok, err := w.repo.CompleteUnchangedFeishuSync(ctx, expected)
		if err != nil {
			return fail(nil, "sync completion failed")
		}
		if !ok {
			return nil
		}
		return nil
	}

	body := []byte(canonical.Markdown)
	checksum := sha256Hex(body)
	key := snapshotKey(documentID, canonical.RemoteRevision, checksum)
	if err := w.storage.Put(ctx, key, bytes.NewReader(body), int64(len(body)), "text/markdown"); err != nil {
		return fail(nil, "snapshot write failed")
	}
	metadata, err := canonicalMetadata(canonical)
	if err != nil {
		return fail(nil, "source metadata invalid")
	}
	pending := PendingFeishuSnapshot{
		DocumentID: documentID, ContentRef: key, Checksum: checksum, RemoteRevision: canonical.RemoteRevision,
		Title: canonical.Title, Bytes: int64(len(body)), Metadata: metadata,
	}
	staged, err := w.repo.StageFeishuSnapshot(ctx, StageFeishuSnapshotInput{Expected: expected, Snapshot: pending})
	if err != nil {
		return fail(nil, "snapshot stage failed")
	}
	if !staged {
		return nil
	}
	if checksum == doc.Checksum {
		promoted, err := w.repo.PromoteFeishuSnapshot(ctx, pending)
		if err != nil {
			return fail(&pending, "snapshot promotion failed")
		}
		if !promoted {
			return nil
		}
		return nil
	}
	if err := w.ingestion.EnqueueStagedIngestion(ctx, pending); err != nil {
		return fail(&pending, "ingestion enqueue failed")
	}
	return nil
}

func sha256Hex(body []byte) string {
	sum := sha256.Sum256(body)
	return hex.EncodeToString(sum[:])
}

func snapshotKey(documentID uuid.UUID, revision, checksum string) string {
	revisionHash := sha256.Sum256([]byte(revision))
	return fmt.Sprintf("feishu/%s/%s-%s.md", documentID.String(), hex.EncodeToString(revisionHash[:8]), checksum)
}

func canonicalMetadata(document domain.CanonicalDocument) (json.RawMessage, error) {
	encoded, err := json.Marshal(document.SourceMetadata)
	if err != nil {
		return nil, err
	}
	metadata := map[string]any{}
	if err := json.Unmarshal(encoded, &metadata); err != nil {
		return nil, err
	}
	metadata["source_type"] = "feishu-" + string(document.SourceMetadata.Values().SourceType)
	metadata["source_url"] = document.SafeSourceURL.String()
	return json.Marshal(metadata)
}

func cloneString(value *string) *string {
	if value == nil {
		return nil
	}
	copy := *value
	return &copy
}

type SQLFeishuSyncRepository struct{ queries *generated.Queries }

func NewSQLFeishuSyncRepository(queries *generated.Queries) *SQLFeishuSyncRepository {
	return &SQLFeishuSyncRepository{queries: queries}
}

func (r *SQLFeishuSyncRepository) ClaimFeishuSync(ctx context.Context, documentID uuid.UUID) (generated.Document, error) {
	return r.queries.ClaimFeishuSync(ctx, documentID)
}

func (r *SQLFeishuSyncRepository) CompleteUnchangedFeishuSync(ctx context.Context, input FeishuSyncExpectation) (bool, error) {
	rows, err := r.queries.CompleteUnchangedFeishuSync(ctx, generated.CompleteUnchangedFeishuSyncParams{
		ID: input.DocumentID, RemoteRevision: input.RemoteRevision, Checksum: input.Checksum,
	})
	return rows == 1, err
}

func (r *SQLFeishuSyncRepository) StageFeishuSnapshot(ctx context.Context, input StageFeishuSnapshotInput) (bool, error) {
	contentRef, checksum, revision := input.Snapshot.ContentRef, input.Snapshot.Checksum, input.Snapshot.RemoteRevision
	rows, err := r.queries.StageFeishuSnapshot(ctx, generated.StageFeishuSnapshotParams{
		PendingContentRef: &contentRef, PendingChecksum: &checksum, PendingRemoteRevision: &revision,
		ID: input.Expected.DocumentID, ExpectedRemoteRevision: input.Expected.RemoteRevision, ExpectedChecksum: input.Expected.Checksum,
	})
	return rows == 1, err
}

func (r *SQLFeishuSyncRepository) PromoteFeishuSnapshot(ctx context.Context, snapshot PendingFeishuSnapshot) (bool, error) {
	contentRef, checksum, revision := snapshot.ContentRef, snapshot.Checksum, snapshot.RemoteRevision
	rows, err := r.queries.PromoteFeishuSnapshot(ctx, generated.PromoteFeishuSnapshotParams{
		ID: snapshot.DocumentID, PendingContentRef: &contentRef, PendingChecksum: &checksum, PendingRemoteRevision: &revision,
		Title: snapshot.Title, Bytes: snapshot.Bytes, Metadata: snapshot.Metadata,
	})
	return rows == 1, err
}

func (r *SQLFeishuSyncRepository) FailFeishuSync(ctx context.Context, input FailFeishuSyncInput) (bool, error) {
	var contentRef, checksum, revision *string
	if input.Pending != nil {
		contentRef, checksum, revision = &input.Pending.ContentRef, &input.Pending.Checksum, &input.Pending.RemoteRevision
	}
	rows, err := r.queries.FailFeishuSync(ctx, generated.FailFeishuSyncParams{
		SafeError: &input.SafeError, ID: input.DocumentID,
		PendingContentRef: contentRef, PendingChecksum: checksum, PendingRemoteRevision: revision,
	})
	return rows == 1, err
}
