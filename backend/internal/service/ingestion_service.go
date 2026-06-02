package service

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/zenith-wang/it-wiki/backend/internal/domain"
	"github.com/zenith-wang/it-wiki/backend/internal/domain/ports"
	"github.com/zenith-wang/it-wiki/backend/internal/infra/checksum"
	"github.com/zenith-wang/it-wiki/backend/internal/repo/generated"
)

type ErrDuplicateChecksum struct {
	ExistingDocID string
}

func (e *ErrDuplicateChecksum) Error() string {
	return "document with same checksum already exists: " + e.ExistingDocID
}

type JobEnqueuer interface {
	EnqueueIngestion(ctx context.Context, docID string) error
}

type Ingestion struct {
	queries  *generated.Queries
	storage  ports.ObjectStorage
	enqueuer JobEnqueuer
}

func NewIngestion(q *generated.Queries, storage ports.ObjectStorage, enqueuer JobEnqueuer) *Ingestion {
	return &Ingestion{queries: q, storage: storage, enqueuer: enqueuer}
}

type UploadInput struct {
	KBID     string
	Title    string
	MimeType string
	Body     io.Reader
	Size     int64
}

func (s *Ingestion) Upload(ctx context.Context, in UploadInput) (*domain.Document, error) {
	buf, err := io.ReadAll(in.Body)
	if err != nil {
		return nil, fmt.Errorf("read body: %w", err)
	}
	if int64(len(buf)) != in.Size && in.Size > 0 {
		in.Size = int64(len(buf))
	}

	sum, _, err := checksum.SHA256(bytes.NewReader(buf))
	if err != nil {
		return nil, err
	}

	kbUUID, err := uuid.Parse(in.KBID)
	if err != nil {
		return nil, fmt.Errorf("invalid kb id: %w", err)
	}
	existing, err := s.queries.FindDocumentByChecksum(ctx, generated.FindDocumentByChecksumParams{
		KbID:     kbUUID,
		Checksum: sum,
	})
	if err == nil {
		return nil, &ErrDuplicateChecksum{ExistingDocID: existing.ID.String()}
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return nil, fmt.Errorf("find by checksum: %w", err)
	}

	docID := uuid.New()
	storageKey := fmt.Sprintf("%s/%s/%s", in.KBID, docID.String(), in.Title)

	if err := s.storage.Put(ctx, storageKey, bytes.NewReader(buf), in.Size, in.MimeType); err != nil {
		return nil, fmt.Errorf("storage put: %w", err)
	}

	row, err := s.queries.CreateDocument(ctx, generated.CreateDocumentParams{
		KbID:       kbUUID,
		SourceType: "local-upload",
		SourceRef:  storageKey,
		Title:      in.Title,
		MimeType:   in.MimeType,
		Bytes:      in.Size,
		Checksum:   sum,
		Status:     string(domain.StatusPending),
		Metadata:   []byte("{}"),
	})
	if err != nil {
		_ = s.storage.Delete(ctx, storageKey)
		return nil, fmt.Errorf("create document: %w", err)
	}

	if err := s.enqueuer.EnqueueIngestion(ctx, row.ID.String()); err != nil {
		msg := err.Error()
		_ = s.queries.UpdateDocumentStatus(ctx, generated.UpdateDocumentStatusParams{
			ID:           row.ID,
			Status:       string(domain.StatusFailed),
			ErrorMessage: &msg,
		})
		return nil, fmt.Errorf("enqueue: %w", err)
	}

	return rowToDocFull(row), nil
}

func rowToDocFull(r generated.Document) *domain.Document {
	doc := &domain.Document{
		ID:         r.ID.String(),
		KBID:       r.KbID.String(),
		SourceType: r.SourceType,
		SourceRef:  r.SourceRef,
		Title:      r.Title,
		MimeType:   r.MimeType,
		Bytes:      r.Bytes,
		Checksum:   r.Checksum,
		Status:     domain.DocStatus(r.Status),
		CreatedAt:  r.CreatedAt,
		UpdatedAt:  r.UpdatedAt,
	}
	if r.ErrorMessage != nil {
		doc.ErrorMessage = r.ErrorMessage
	}
	doc.Metadata = map[string]any{}
	if len(r.Metadata) > 0 {
		_ = json.Unmarshal(r.Metadata, &doc.Metadata)
	}
	return doc
}
