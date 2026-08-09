package worker

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"time"

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

type StagedIngestionTxEnqueuer interface {
	EnqueueStagedIngestionTx(ctx context.Context, tx pgx.Tx, snapshot PendingFeishuSnapshot) error
}

type StagedIngestionEnqueuerFunc func(context.Context, PendingFeishuSnapshot) error

func (f StagedIngestionEnqueuerFunc) EnqueueStagedIngestion(ctx context.Context, snapshot PendingFeishuSnapshot) error {
	return f(ctx, snapshot)
}

type StagedIngestionEnqueuerFuncs struct {
	EnqueueFunc             func(context.Context, PendingFeishuSnapshot) error
	EnqueueTxFunc           func(context.Context, pgx.Tx, PendingFeishuSnapshot) error
	EnqueueFeishuSyncFunc   func(context.Context, string, string) error
	EnqueueMetadataOnlyFunc func(context.Context, PendingFeishuSnapshot) error
}

func (f StagedIngestionEnqueuerFuncs) EnqueueStagedIngestion(ctx context.Context, snapshot PendingFeishuSnapshot) error {
	return f.EnqueueFunc(ctx, snapshot)
}

func (f StagedIngestionEnqueuerFuncs) EnqueueStagedIngestionTx(ctx context.Context, tx pgx.Tx, snapshot PendingFeishuSnapshot) error {
	return f.EnqueueTxFunc(ctx, tx, snapshot)
}

func (f StagedIngestionEnqueuerFuncs) EnqueueFeishuSync(ctx context.Context, documentID, requestedRevision string) error {
	return f.EnqueueFeishuSyncFunc(ctx, documentID, requestedRevision)
}

func (f StagedIngestionEnqueuerFuncs) EnqueueMetadataOnlyIngestion(ctx context.Context, snapshot PendingFeishuSnapshot) error {
	return f.EnqueueMetadataOnlyFunc(ctx, snapshot)
}

type PendingFeishuSnapshot struct {
	DocumentID     uuid.UUID
	ContentRef     string
	Checksum       string
	RemoteRevision string
	Title          string
	Bytes          int64
	Metadata       json.RawMessage
	ClaimToken     time.Time
}

type FeishuSyncExpectation struct {
	DocumentID     uuid.UUID
	RemoteRevision *string
	Checksum       string
	ClaimToken     time.Time
	Pending        *PendingFeishuSnapshot
}

type StageFeishuSnapshotInput struct {
	Expected FeishuSyncExpectation
	Snapshot PendingFeishuSnapshot
}

var errSnapshotStageRolledBack = errors.New("snapshot stage transaction rolled back")

type FailFeishuSyncInput struct {
	DocumentID uuid.UUID
	Pending    *PendingFeishuSnapshot
	SafeError  string
	ClaimToken time.Time
}

type FeishuSyncRepository interface {
	ClaimFeishuSync(ctx context.Context, documentID uuid.UUID, expectedRemoteRevision *string, lease time.Duration) (generated.Document, error)
	CompleteUnchangedFeishuSync(ctx context.Context, input FeishuSyncExpectation) (bool, error)
	StageFeishuSnapshot(ctx context.Context, input StageFeishuSnapshotInput) (bool, error)
	StageFeishuSnapshotAndEnqueue(ctx context.Context, input StageFeishuSnapshotInput, enqueuer StagedIngestionEnqueuer) (bool, error)
	PromoteFeishuSnapshot(ctx context.Context, snapshot PendingFeishuSnapshot) (bool, error)
	FailFeishuSync(ctx context.Context, input FailFeishuSyncInput) (bool, error)
}

type FeishuSyncWorkerDeps struct {
	Repository     FeishuSyncRepository
	Resolver       ports.SourceResolver
	Loaders        map[domain.ResourceType]ports.SourceLoader
	Tokens         FeishuTokenProvider
	Storage        ports.ObjectStorage
	Ingestion      StagedIngestionEnqueuer
	CleanupTimeout time.Duration
	JobTimeout     time.Duration
	SyncLease      time.Duration
	CitationStore  ports.CitationPromotionStore
	Logger         *slog.Logger
}

type FeishuSyncWorker struct {
	river.WorkerDefaults[FeishuSyncJobArgs]
	repo           FeishuSyncRepository
	resolver       ports.SourceResolver
	loaders        map[domain.ResourceType]ports.SourceLoader
	tokens         FeishuTokenProvider
	storage        ports.ObjectStorage
	ingestion      StagedIngestionEnqueuer
	cleanupTimeout time.Duration
	jobTimeout     time.Duration
	syncLease      time.Duration
	citationStore  ports.CitationPromotionStore
	logger         *slog.Logger
}

func NewFeishuSyncWorker(deps FeishuSyncWorkerDeps) *FeishuSyncWorker {
	if deps.CleanupTimeout <= 0 {
		deps.CleanupTimeout = 5 * time.Second
	}
	if deps.SyncLease <= 0 {
		deps.SyncLease = 15 * time.Minute
	}
	if deps.JobTimeout <= 0 {
		deps.JobTimeout = 10 * time.Minute
	}
	if deps.Logger == nil {
		deps.Logger = slog.Default()
	}
	return &FeishuSyncWorker{repo: deps.Repository, resolver: deps.Resolver, loaders: deps.Loaders, tokens: deps.Tokens, storage: deps.Storage, ingestion: deps.Ingestion, cleanupTimeout: deps.CleanupTimeout, jobTimeout: deps.JobTimeout, syncLease: deps.SyncLease, citationStore: deps.CitationStore, logger: deps.Logger}
}

func (w *FeishuSyncWorker) Timeout(*river.Job[FeishuSyncJobArgs]) time.Duration { return w.jobTimeout }

func (w *FeishuSyncWorker) Work(ctx context.Context, job *river.Job[FeishuSyncJobArgs]) error {
	documentID, err := uuid.Parse(job.Args.DocumentID)
	if err != nil {
		return errors.New("invalid document ID")
	}
	expectedRevision := requestedActiveRevision(job.Args.RequestedRevision)
	doc, err := w.repo.ClaimFeishuSync(ctx, documentID, expectedRevision, w.syncLease)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil
	}
	if err != nil {
		return errors.New("claim sync failed")
	}
	expected := FeishuSyncExpectation{DocumentID: documentID, RemoteRevision: cloneString(doc.RemoteRevision), Checksum: doc.Checksum, ClaimToken: doc.UpdatedAt, Pending: snapshotFromDocument(doc)}
	fail := func(pending *PendingFeishuSnapshot, safe string) error {
		if pending == nil {
			pending = snapshotFromDocument(doc)
		}
		cleanupCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), w.cleanupTimeout)
		defer cancel()
		_, _ = w.repo.FailFeishuSync(cleanupCtx, FailFeishuSyncInput{DocumentID: documentID, Pending: pending, SafeError: safe, ClaimToken: doc.UpdatedAt})
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
		w.deleteSupersededPending(ctx, doc, "")
		return nil
	}

	body := []byte(canonical.Markdown)
	checksum := sha256Hex(body)
	metadata, err := canonicalMetadata(canonical)
	if err != nil {
		return fail(nil, "source metadata invalid")
	}
	key := snapshotKey(documentID, canonical.RemoteRevision, checksum, doc.UpdatedAt)
	if err := w.storage.Put(ctx, key, bytes.NewReader(body), int64(len(body)), "text/markdown"); err != nil {
		w.deleteSnapshot(ctx, documentID, doc.UpdatedAt, key)
		return fail(nil, "snapshot write failed")
	}
	pending := PendingFeishuSnapshot{
		DocumentID: documentID, ContentRef: key, Checksum: checksum, RemoteRevision: canonical.RemoteRevision,
		Title: canonical.Title, Bytes: int64(len(body)), Metadata: metadata,
		ClaimToken: doc.UpdatedAt,
	}
	if checksum == doc.Checksum {
		staged, err := w.repo.StageFeishuSnapshot(ctx, StageFeishuSnapshotInput{Expected: expected, Snapshot: pending})
		if err != nil {
			return fail(snapshotFromDocument(doc), "snapshot stage failed")
		}
		if !staged {
			w.deleteSnapshot(ctx, documentID, doc.UpdatedAt, key)
			return nil
		}
		if w.citationStore == nil {
			return fail(&pending, "snapshot promotion failed")
		}
		err = w.citationStore.PatchChunkMetadataAndPromote(ctx, pendingPromotion(pending), citationMetadataPatcher(pending.Metadata))
		if errors.Is(err, ports.ErrStaleDocumentPromotion) {
			return nil
		}
		if err != nil {
			return fail(&pending, "snapshot promotion failed")
		}
		w.deleteSupersededPending(ctx, doc, pending.ContentRef)
		return nil
	}
	staged, err := w.repo.StageFeishuSnapshotAndEnqueue(ctx, StageFeishuSnapshotInput{Expected: expected, Snapshot: pending}, w.ingestion)
	if err != nil {
		if errors.Is(err, errSnapshotStageRolledBack) {
			w.deleteSnapshot(ctx, documentID, doc.UpdatedAt, key)
		}
		return fail(snapshotFromDocument(doc), "ingestion enqueue failed")
	}
	if !staged {
		w.deleteSnapshot(ctx, documentID, doc.UpdatedAt, key)
		return nil
	}
	w.deleteSupersededPending(ctx, doc, key)
	return nil
}

func pendingPromotion(snapshot PendingFeishuSnapshot) ports.PendingDocumentPromotion {
	return ports.PendingDocumentPromotion{
		DocumentID: snapshot.DocumentID, ContentRef: snapshot.ContentRef, Checksum: snapshot.Checksum,
		RemoteRevision: snapshot.RemoteRevision, Title: snapshot.Title, Bytes: snapshot.Bytes,
		Metadata: snapshot.Metadata, ClaimToken: snapshot.ClaimToken,
	}
}

func citationMetadataPatcher(raw json.RawMessage) ports.ChunkMetadataPatcher {
	return func(existing map[string]any) map[string]any {
		sectionPath, _ := existing["section_path"].(string)
		patched := remoteCitationMetadata(raw, sectionPath)
		for key, value := range existing {
			switch key {
			case "source_type", "source_url", "section_path", "sheet_name", "sheet_id", "table_id", "view_id", "row_start", "row_end", "remote_revision", "locations":
				continue
			default:
				patched[key] = value
			}
		}
		return patched
	}
}

func (w *FeishuSyncWorker) deleteSnapshot(ctx context.Context, documentID uuid.UUID, claimToken time.Time, key string) {
	cleanupCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), w.cleanupTimeout)
	defer cancel()
	if err := w.storage.Delete(cleanupCtx, key); err != nil {
		w.logger.WarnContext(cleanupCtx, "Feishu snapshot cleanup failed",
			"event", "feishu_snapshot_cleanup_failed",
			"error_code", "snapshot_delete_failed",
			"document_id", documentID.String(),
			"claim_timestamp", claimToken.UTC().Format(time.RFC3339Nano),
			"storage_key_hash", shortStorageKeyHash(key),
		)
	}
}

func (w *FeishuSyncWorker) deleteSupersededPending(ctx context.Context, doc generated.Document, newRef string) {
	if doc.PendingContentRef == nil || *doc.PendingContentRef == newRef || (doc.ContentRef != nil && *doc.ContentRef == *doc.PendingContentRef) {
		return
	}
	w.deleteSnapshot(ctx, doc.ID, doc.UpdatedAt, *doc.PendingContentRef)
}

func shortStorageKeyHash(key string) string {
	sum := sha256.Sum256([]byte(key))
	return hex.EncodeToString(sum[:8])
}

func snapshotFromDocument(doc generated.Document) *PendingFeishuSnapshot {
	if doc.PendingContentRef == nil || doc.PendingChecksum == nil || doc.PendingRemoteRevision == nil ||
		doc.PendingTitle == nil || doc.PendingBytes == nil || doc.PendingMetadata == nil {
		return nil
	}
	return &PendingFeishuSnapshot{
		DocumentID: doc.ID, ContentRef: *doc.PendingContentRef, Checksum: *doc.PendingChecksum,
		RemoteRevision: *doc.PendingRemoteRevision, Title: *doc.PendingTitle, Bytes: *doc.PendingBytes,
		Metadata: append(json.RawMessage(nil), doc.PendingMetadata...), ClaimToken: doc.UpdatedAt,
	}
}

func sha256Hex(body []byte) string {
	sum := sha256.Sum256(body)
	return hex.EncodeToString(sum[:])
}

func snapshotKey(documentID uuid.UUID, revision, checksum string, claimToken time.Time) string {
	revisionHash := sha256.Sum256([]byte(revision))
	attemptHash := sha256.Sum256([]byte(claimToken.UTC().Format(time.RFC3339Nano)))
	return fmt.Sprintf("feishu/%s/%s-%s-%s.md", documentID.String(), hex.EncodeToString(revisionHash[:8]), hex.EncodeToString(attemptHash[:8]), checksum)
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

func requestedActiveRevision(requested string) *string {
	if requested == "" || requested == "initial" {
		return nil
	}
	return &requested
}

type SQLFeishuSyncRepository struct {
	pool    syncTxBeginner
	queries *generated.Queries
}

type syncTxBeginner interface {
	BeginTx(ctx context.Context, opts pgx.TxOptions) (pgx.Tx, error)
}

func NewSQLFeishuSyncRepository(pool syncTxBeginner) *SQLFeishuSyncRepository {
	repository := &SQLFeishuSyncRepository{pool: pool}
	if db, ok := pool.(generated.DBTX); ok {
		repository.queries = generated.New(db)
	}
	return repository
}

func (r *SQLFeishuSyncRepository) ClaimFeishuSync(ctx context.Context, documentID uuid.UUID, expectedRemoteRevision *string, lease time.Duration) (generated.Document, error) {
	return r.queries.ClaimFeishuSync(ctx, generated.ClaimFeishuSyncParams{ID: documentID, ExpectedRemoteRevision: expectedRemoteRevision, LeaseSeconds: durationSecondsCeil(lease)})
}

func (r *SQLFeishuSyncRepository) ListStaleFeishuSyncs(ctx context.Context, lease time.Duration, afterUpdatedAt time.Time, afterID uuid.UUID, batchSize int32) ([]generated.Document, error) {
	return r.queries.ListStaleFeishuSyncs(ctx, generated.ListStaleFeishuSyncsParams{
		LeaseSeconds: durationSecondsCeil(lease), AfterUpdatedAt: afterUpdatedAt, AfterID: afterID, BatchSize: batchSize,
	})
}

func (r *SQLFeishuSyncRepository) CompleteUnchangedFeishuSync(ctx context.Context, input FeishuSyncExpectation) (bool, error) {
	var oldContentRef, oldChecksum, oldRevision *string
	if input.Pending != nil {
		oldContentRef, oldChecksum, oldRevision = &input.Pending.ContentRef, &input.Pending.Checksum, &input.Pending.RemoteRevision
	}
	rows, err := r.queries.CompleteUnchangedFeishuSync(ctx, generated.CompleteUnchangedFeishuSyncParams{
		ID: input.DocumentID, RemoteRevision: input.RemoteRevision, Checksum: input.Checksum, ClaimToken: input.ClaimToken,
		ExpectedPendingContentRef: oldContentRef, ExpectedPendingChecksum: oldChecksum, ExpectedPendingRemoteRevision: oldRevision,
	})
	return rows == 1, err
}

func (r *SQLFeishuSyncRepository) StageFeishuSnapshot(ctx context.Context, input StageFeishuSnapshotInput) (bool, error) {
	contentRef, checksum, revision := input.Snapshot.ContentRef, input.Snapshot.Checksum, input.Snapshot.RemoteRevision
	title, bytes := input.Snapshot.Title, input.Snapshot.Bytes
	var oldContentRef, oldChecksum, oldRevision *string
	if input.Expected.Pending != nil {
		oldContentRef, oldChecksum, oldRevision = &input.Expected.Pending.ContentRef, &input.Expected.Pending.Checksum, &input.Expected.Pending.RemoteRevision
	}
	rows, err := r.queries.StageFeishuSnapshot(ctx, generated.StageFeishuSnapshotParams{
		PendingContentRef: &contentRef, PendingChecksum: &checksum, PendingRemoteRevision: &revision,
		PendingTitle: &title, PendingBytes: &bytes, PendingMetadata: input.Snapshot.Metadata,
		ID: input.Expected.DocumentID, ExpectedRemoteRevision: input.Expected.RemoteRevision, ExpectedChecksum: input.Expected.Checksum,
		ClaimToken:                input.Expected.ClaimToken,
		ExpectedPendingContentRef: oldContentRef, ExpectedPendingChecksum: oldChecksum, ExpectedPendingRemoteRevision: oldRevision,
	})
	return rows == 1, err
}

func (r *SQLFeishuSyncRepository) StageFeishuSnapshotAndEnqueue(ctx context.Context, input StageFeishuSnapshotInput, enqueuer StagedIngestionEnqueuer) (bool, error) {
	txEnqueuer, ok := enqueuer.(StagedIngestionTxEnqueuer)
	if !ok {
		return false, errors.Join(errSnapshotStageRolledBack, errors.New("transactional ingestion enqueuer required"))
	}
	tx, err := r.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return false, errors.Join(errSnapshotStageRolledBack, err)
	}
	defer tx.Rollback(ctx) //nolint:errcheck
	txRepo := &SQLFeishuSyncRepository{queries: generated.New(tx)}
	staged, err := txRepo.StageFeishuSnapshot(ctx, input)
	if err != nil {
		return false, errors.Join(errSnapshotStageRolledBack, err)
	}
	if !staged {
		return false, nil
	}
	if err := txEnqueuer.EnqueueStagedIngestionTx(ctx, tx, input.Snapshot); err != nil {
		return false, errors.Join(errSnapshotStageRolledBack, err)
	}
	if err := tx.Commit(ctx); err != nil {
		if errors.Is(err, pgx.ErrTxCommitRollback) {
			return false, errors.Join(errSnapshotStageRolledBack, err)
		}
		return false, err
	}
	return true, nil
}

func (r *SQLFeishuSyncRepository) PromoteFeishuSnapshot(ctx context.Context, snapshot PendingFeishuSnapshot) (bool, error) {
	contentRef, checksum, revision := snapshot.ContentRef, snapshot.Checksum, snapshot.RemoteRevision
	rows, err := r.queries.PromoteFeishuSnapshot(ctx, generated.PromoteFeishuSnapshotParams{
		ID: snapshot.DocumentID, PendingContentRef: &contentRef, PendingChecksum: &checksum, PendingRemoteRevision: &revision,
		ClaimToken: snapshot.ClaimToken,
	})
	return rows == 1, err
}

func durationSecondsCeil(duration time.Duration) int64 {
	if duration <= 0 {
		return 0
	}
	return int64((duration + time.Second - 1) / time.Second)
}

func (r *SQLFeishuSyncRepository) FailFeishuSync(ctx context.Context, input FailFeishuSyncInput) (bool, error) {
	var contentRef, checksum, revision *string
	if input.Pending != nil {
		contentRef, checksum, revision = &input.Pending.ContentRef, &input.Pending.Checksum, &input.Pending.RemoteRevision
	}
	rows, err := r.queries.FailFeishuSync(ctx, generated.FailFeishuSyncParams{
		SafeError: &input.SafeError, ID: input.DocumentID,
		PendingContentRef: contentRef, PendingChecksum: checksum, PendingRemoteRevision: revision,
		ClaimToken: input.ClaimToken,
	})
	return rows == 1, err
}
