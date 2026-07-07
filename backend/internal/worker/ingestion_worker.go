package worker

import (
	"context"
	"fmt"
	"log"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/riverqueue/river"

	"github.com/zenith-wang/it-wiki/backend/internal/domain"
	"github.com/zenith-wang/it-wiki/backend/internal/domain/ports"
	"github.com/zenith-wang/it-wiki/backend/internal/repo/generated"
)

type IngestionWorker struct {
	river.WorkerDefaults[IngestionJobArgs]

	pool      *pgxpool.Pool
	queries   *generated.Queries
	storage   ports.ObjectStorage
	parser    ports.Parser
	splitter  ports.Splitter
	embedder  ports.Embedder
	vstore    ports.VectorStore
	chunkSize int
	overlap   int
	batchSize int
}

type WorkerDeps struct {
	Pool      *pgxpool.Pool
	Queries   *generated.Queries
	Storage   ports.ObjectStorage
	Parser    ports.Parser
	Splitter  ports.Splitter
	Embedder  ports.Embedder
	VStore    ports.VectorStore
	ChunkSize int
	Overlap   int
	BatchSize int
}

func NewIngestionWorker(d WorkerDeps) *IngestionWorker {
	return &IngestionWorker{
		pool: d.Pool, queries: d.Queries, storage: d.Storage,
		parser: d.Parser, splitter: d.Splitter, embedder: d.Embedder, vstore: d.VStore,
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

	failed := func(stepErr error) error {
		msg := stepErr.Error()
		_ = w.queries.UpdateDocumentStatus(ctx, generated.UpdateDocumentStatusParams{
			ID: docID, Status: string(domain.StatusFailed), ErrorMessage: &msg,
		})
		log.Printf("[worker] doc %s failed: %v", docID, stepErr)
		return stepErr
	}

	if err := w.setStatus(ctx, docID, domain.StatusParsing); err != nil {
		return failed(err)
	}
	reader, err := w.storage.Get(ctx, doc.SourceRef)
	if err != nil {
		return failed(fmt.Errorf("storage get: %w", err))
	}
	defer reader.Close()

	parsed, err := w.parser.Parse(ctx, reader, doc.MimeType)
	if err != nil {
		return failed(fmt.Errorf("parse: %w", err))
	}
	if parsed.Text == "" {
		return failed(fmt.Errorf("parser produced empty text"))
	}

	if err := w.setStatus(ctx, docID, domain.StatusChunking); err != nil {
		return failed(err)
	}
	pieces, err := buildChunkPieces(ctx, w.splitter, parsed, w.chunkSize, w.overlap)
	if err != nil {
		return failed(fmt.Errorf("split: %w", err))
	}
	if len(pieces) == 0 {
		return failed(fmt.Errorf("splitter produced 0 chunks"))
	}

	if err := w.setStatus(ctx, docID, domain.StatusEmbedding); err != nil {
		return failed(err)
	}
	allEmbeddings := make([][]float32, 0, len(pieces))
	for i := 0; i < len(pieces); i += w.batchSize {
		end := i + w.batchSize
		if end > len(pieces) {
			end = len(pieces)
		}
		texts := make([]string, 0, end-i)
		for _, p := range pieces[i:end] {
			texts = append(texts, p.Content)
		}
		embs, err := w.embedder.Embed(ctx, texts)
		if err != nil {
			return failed(fmt.Errorf("embed batch %d-%d: %w", i, end, err))
		}
		allEmbeddings = append(allEmbeddings, embs...)
	}
	if len(allEmbeddings) != len(pieces) {
		return failed(fmt.Errorf("embedding count mismatch: %d vs %d", len(allEmbeddings), len(pieces)))
	}

	items := make([]domain.ChunkWithEmbedding, 0, len(pieces))
	for i, p := range pieces {
		items = append(items, domain.ChunkWithEmbedding{
			Chunk: domain.Chunk{
				KBID:       doc.KbID.String(),
				DocumentID: docID.String(),
				Seq:        i,
				Content:    p.Content,
				TokenCount: p.TokenCount,
				Metadata:   p.Metadata,
			},
			Embedding: allEmbeddings[i],
		})
	}
	if err := w.vstore.InsertChunks(ctx, items); err != nil {
		return failed(fmt.Errorf("insert chunks: %w", err))
	}

	if err := w.setStatus(ctx, docID, domain.StatusReady); err != nil {
		return failed(err)
	}
	log.Printf("[worker] doc %s ready (%d chunks)", docID, len(items))
	return nil
}

func (w *IngestionWorker) setStatus(ctx context.Context, id uuid.UUID, status domain.DocStatus) error {
	return w.queries.UpdateDocumentStatus(ctx, generated.UpdateDocumentStatusParams{
		ID:     id,
		Status: string(status),
	})
}
