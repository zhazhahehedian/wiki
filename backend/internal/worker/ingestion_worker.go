package worker

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/riverqueue/river"

	"github.com/zenith-wang/it-wiki/backend/internal/domain"
	"github.com/zenith-wang/it-wiki/backend/internal/domain/ports"
	"github.com/zenith-wang/it-wiki/backend/internal/repo/generated"
)

type IngestionRepository interface {
	GetDocument(ctx context.Context, id uuid.UUID) (generated.Document, error)
	UpdateDocumentStatus(ctx context.Context, arg generated.UpdateDocumentStatusParams) error
	FailFeishuSync(ctx context.Context, arg generated.FailFeishuSyncParams) (int64, error)
}

type IngestionWorker struct {
	river.WorkerDefaults[IngestionJobArgs]

	queries      IngestionRepository
	storage      ports.ObjectStorage
	parser       ports.Parser
	splitter     ports.Splitter
	embedder     ports.Embedder
	vstore       ports.VectorStore
	stagedVStore ports.StagedVectorStore
	chunkSize    int
	overlap      int
	batchSize    int
}

type WorkerDeps struct {
	Pool         *pgxpool.Pool
	Queries      IngestionRepository
	Storage      ports.ObjectStorage
	Parser       ports.Parser
	Splitter     ports.Splitter
	Embedder     ports.Embedder
	VStore       ports.VectorStore
	StagedVStore ports.StagedVectorStore
	ChunkSize    int
	Overlap      int
	BatchSize    int
}

func NewIngestionWorker(d WorkerDeps) *IngestionWorker {
	staged := d.StagedVStore
	if staged == nil {
		staged, _ = d.VStore.(ports.StagedVectorStore)
	}
	if d.BatchSize < 1 {
		d.BatchSize = 1
	}
	return &IngestionWorker{
		queries: d.Queries, storage: d.Storage, parser: d.Parser, splitter: d.Splitter,
		embedder: d.Embedder, vstore: d.VStore, stagedVStore: staged,
		chunkSize: d.ChunkSize, overlap: d.Overlap, batchSize: d.BatchSize,
	}
}

func (w *IngestionWorker) Work(ctx context.Context, job *river.Job[IngestionJobArgs]) error {
	docID, err := uuid.Parse(job.Args.DocumentID)
	if err != nil {
		return fmt.Errorf("invalid doc id: %w", err)
	}
	doc, err := w.queries.GetDocument(ctx, docID)
	if err != nil {
		return fmt.Errorf("get document: %w", err)
	}

	remote := strings.HasPrefix(doc.SourceType, "feishu-")
	var pending ports.PendingDocumentPromotion
	if remote {
		if job.Args.PendingContentRef == "" || job.Args.PendingChecksum == "" || job.Args.PendingRemoteRevision == "" ||
			doc.PendingContentRef == nil || *doc.PendingContentRef != job.Args.PendingContentRef ||
			doc.PendingChecksum == nil || *doc.PendingChecksum != job.Args.PendingChecksum ||
			doc.PendingRemoteRevision == nil || *doc.PendingRemoteRevision != job.Args.PendingRemoteRevision {
			return nil
		}
		pending = ports.PendingDocumentPromotion{
			DocumentID: docID, ContentRef: job.Args.PendingContentRef,
			Checksum: job.Args.PendingChecksum, RemoteRevision: job.Args.PendingRemoteRevision,
			Title: job.Args.Title, Bytes: job.Args.Bytes, Metadata: append(json.RawMessage(nil), job.Args.Metadata...),
		}
	}

	failLocal := func(stepErr error) error {
		msg := stepErr.Error()
		_ = w.queries.UpdateDocumentStatus(ctx, generated.UpdateDocumentStatusParams{
			ID: docID, Status: string(domain.StatusFailed), ErrorMessage: &msg,
		})
		log.Printf("[worker] doc %s failed: %v", docID, stepErr)
		return stepErr
	}
	failRemote := func(safe string) error {
		contentRef, checksum, revision := pending.ContentRef, pending.Checksum, pending.RemoteRevision
		_, _ = w.queries.FailFeishuSync(ctx, generated.FailFeishuSyncParams{
			SafeError: &safe, ID: docID,
			PendingContentRef: &contentRef, PendingChecksum: &checksum, PendingRemoteRevision: &revision,
		})
		return errors.New(safe)
	}

	if !remote {
		if err := w.setStatus(ctx, docID, domain.StatusParsing); err != nil {
			return failLocal(err)
		}
	}
	contentRef := doc.SourceRef
	if remote {
		contentRef = pending.ContentRef
	}
	reader, err := w.storage.Get(ctx, contentRef)
	if err != nil {
		if remote {
			return failRemote("snapshot read failed")
		}
		return failLocal(fmt.Errorf("storage get: %w", err))
	}
	defer reader.Close()

	mimeType := effectiveMimeType(doc.MimeType, doc.Title)
	if remote {
		mimeType = "text/markdown"
	}
	parsed, err := w.parser.Parse(ctx, reader, mimeType)
	if err != nil || parsed == nil || parsed.Text == "" {
		if remote {
			return failRemote("parsing failed")
		}
		if err != nil {
			return failLocal(fmt.Errorf("parse: %w", err))
		}
		return failLocal(errors.New("parser produced empty text"))
	}

	if !remote {
		if err := w.setStatus(ctx, docID, domain.StatusChunking); err != nil {
			return failLocal(err)
		}
	}
	pieces, err := buildChunkPieces(ctx, w.splitter, parsed, w.chunkSize, w.overlap)
	if err != nil || len(pieces) == 0 {
		if remote {
			return failRemote("chunking failed")
		}
		if err != nil {
			return failLocal(fmt.Errorf("split: %w", err))
		}
		return failLocal(errors.New("splitter produced 0 chunks"))
	}

	if !remote {
		if err := w.setStatus(ctx, docID, domain.StatusEmbedding); err != nil {
			return failLocal(err)
		}
	}
	allEmbeddings := make([][]float32, 0, len(pieces))
	for i := 0; i < len(pieces); i += w.batchSize {
		end := i + w.batchSize
		if end > len(pieces) {
			end = len(pieces)
		}
		texts := make([]string, 0, end-i)
		for _, piece := range pieces[i:end] {
			texts = append(texts, piece.Content)
		}
		embeddings, err := w.embedder.Embed(ctx, texts)
		if err != nil {
			if remote {
				return failRemote("embedding failed")
			}
			return failLocal(fmt.Errorf("embed batch %d-%d: %w", i, end, err))
		}
		allEmbeddings = append(allEmbeddings, embeddings...)
	}
	if len(allEmbeddings) != len(pieces) {
		if remote {
			return failRemote("embedding failed")
		}
		return failLocal(fmt.Errorf("embedding count mismatch: %d vs %d", len(allEmbeddings), len(pieces)))
	}

	baseMetadata := map[string]any{}
	if remote && len(pending.Metadata) > 0 {
		_ = json.Unmarshal(pending.Metadata, &baseMetadata)
	}
	if remote {
		baseMetadata["source_type"] = doc.SourceType
		baseMetadata["remote_revision"] = pending.RemoteRevision
	}
	items := make([]domain.ChunkWithEmbedding, 0, len(pieces))
	for i, piece := range pieces {
		metadata := make(map[string]any, len(baseMetadata)+len(piece.Metadata))
		for key, value := range baseMetadata {
			metadata[key] = value
		}
		for key, value := range piece.Metadata {
			metadata[key] = value
		}
		items = append(items, domain.ChunkWithEmbedding{
			Chunk: domain.Chunk{
				KBID: doc.KbID.String(), DocumentID: docID.String(), Seq: i,
				Content: piece.Content, TokenCount: piece.TokenCount, Metadata: metadata,
			},
			Embedding: allEmbeddings[i],
		})
	}

	if remote {
		if w.stagedVStore == nil {
			return failRemote("snapshot promotion failed")
		}
		if err := w.stagedVStore.ReplaceChunksAndPromote(ctx, pending, items); err != nil {
			if errors.Is(err, ports.ErrStaleDocumentPromotion) {
				return nil
			}
			return failRemote("snapshot promotion failed")
		}
		return nil
	}

	if err := w.vstore.ReplaceChunks(ctx, docID.String(), items); err != nil {
		return failLocal(fmt.Errorf("replace chunks: %w", err))
	}
	if err := w.setStatus(ctx, docID, domain.StatusReady); err != nil {
		return failLocal(err)
	}
	log.Printf("[worker] doc %s ready (%d chunks)", docID, len(items))
	return nil
}

func (w *IngestionWorker) setStatus(ctx context.Context, id uuid.UUID, status domain.DocStatus) error {
	return w.queries.UpdateDocumentStatus(ctx, generated.UpdateDocumentStatusParams{ID: id, Status: string(status)})
}
