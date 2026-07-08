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

type ErrDocProcessing struct{ ID string }

func (e *ErrDocProcessing) Error() string { return "document is still being processed: " + e.ID }

// Reingest 把已入库的文档重新走一遍摄入管线。
// 仅允许 ready/failed 状态; worker 会在写入新 chunks 前删除旧 chunks。
func (s *Ingestion) Reingest(ctx context.Context, docID string) (*domain.Document, error) {
	id, err := uuid.Parse(docID)
	if err != nil {
		return nil, &ErrDocNotFound{ID: docID}
	}
	row, err := s.queries.GetDocument(ctx, id)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, &ErrDocNotFound{ID: docID}
		}
		return nil, fmt.Errorf("get document: %w", err)
	}
	switch domain.DocStatus(row.Status) {
	case domain.StatusReady, domain.StatusFailed:
	default:
		return nil, &ErrDocProcessing{ID: docID}
	}

	if err := s.queries.UpdateDocumentStatus(ctx, generated.UpdateDocumentStatusParams{
		ID: id, Status: string(domain.StatusPending),
	}); err != nil {
		return nil, fmt.Errorf("reset document status: %w", err)
	}
	if err := s.enqueuer.EnqueueIngestion(ctx, docID); err != nil {
		msg := err.Error()
		_ = s.queries.UpdateDocumentStatus(ctx, generated.UpdateDocumentStatusParams{
			ID: id, Status: string(domain.StatusFailed), ErrorMessage: &msg,
		})
		return nil, fmt.Errorf("enqueue: %w", err)
	}

	row.Status = string(domain.StatusPending)
	row.ErrorMessage = nil
	return rowToDocFull(row), nil
}

// ReingestKB 把 KB 下所有 ready/failed 文档逐个重新入队, 返回入队数量。
func (s *Ingestion) ReingestKB(ctx context.Context, kbID string) (int, error) {
	kbUUID, err := uuid.Parse(kbID)
	if err != nil {
		return 0, &ErrKBNotFound{ID: kbID}
	}
	if _, err := s.queries.GetKnowledgeBase(ctx, kbUUID); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return 0, &ErrKBNotFound{ID: kbID}
		}
		return 0, fmt.Errorf("get kb: %w", err)
	}

	enqueued := 0
	const pageSize = int32(100)
	for offset := int32(0); ; offset += pageSize {
		rows, err := s.queries.ListDocumentsByKB(ctx, generated.ListDocumentsByKBParams{
			KbID: kbUUID, Limit: pageSize, Offset: offset,
		})
		if err != nil {
			return enqueued, fmt.Errorf("list documents: %w", err)
		}
		for _, row := range rows {
			if _, err := s.Reingest(ctx, row.ID.String()); err != nil {
				var busy *ErrDocProcessing
				if errors.As(err, &busy) {
					continue // 处理中的文档跳过, 不视为失败
				}
				return enqueued, err
			}
			enqueued++
		}
		if int32(len(rows)) < pageSize {
			return enqueued, nil
		}
	}
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
