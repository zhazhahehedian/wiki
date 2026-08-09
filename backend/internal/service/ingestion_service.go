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
	queries  IngestionQueries
	storage  ports.ObjectStorage
	enqueuer JobEnqueuer
}

type IngestionQueries interface {
	FindDocumentByChecksumForOwner(context.Context, generated.FindDocumentByChecksumForOwnerParams) (generated.Document, error)
	CreateDocumentForOwner(context.Context, generated.CreateDocumentForOwnerParams) (generated.Document, error)
	GetDocumentForOwner(context.Context, generated.GetDocumentForOwnerParams) (generated.Document, error)
	UpdateDocumentStatusForOwner(context.Context, generated.UpdateDocumentStatusForOwnerParams) error
	GetKnowledgeBaseForOwner(context.Context, generated.GetKnowledgeBaseForOwnerParams) (generated.KnowledgeBase, error)
	ListDocumentsByKBForOwner(context.Context, generated.ListDocumentsByKBForOwnerParams) ([]generated.Document, error)
}

func NewIngestion(q IngestionQueries, storage ports.ObjectStorage, enqueuer JobEnqueuer) *Ingestion {
	return &Ingestion{queries: q, storage: storage, enqueuer: enqueuer}
}

type UploadInput struct {
	KBID     string
	Title    string
	MimeType string
	Body     io.Reader
	Size     int64
}

func (s *Ingestion) Upload(ctx context.Context, userID string, in UploadInput) (*domain.Document, error) {
	ownerID, err := ownerUUID(userID)
	if err != nil {
		return nil, err
	}
	kbUUID, err := uuid.Parse(in.KBID)
	if err != nil {
		return nil, &ErrKBNotFound{ID: in.KBID}
	}
	if _, err := s.queries.GetKnowledgeBaseForOwner(ctx, generated.GetKnowledgeBaseForOwnerParams{ID: kbUUID, OwnerUserID: ownerID}); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, &ErrKBNotFound{ID: in.KBID}
		}
		return nil, fmt.Errorf("get kb: %w", err)
	}
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

	existing, err := s.queries.FindDocumentByChecksumForOwner(ctx, generated.FindDocumentByChecksumForOwnerParams{
		KbID: kbUUID, Checksum: sum, OwnerUserID: ownerID,
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

	row, err := s.queries.CreateDocumentForOwner(ctx, generated.CreateDocumentForOwnerParams{
		KbID:        kbUUID,
		SourceType:  "local-upload",
		SourceRef:   storageKey,
		Title:       in.Title,
		MimeType:    in.MimeType,
		Bytes:       in.Size,
		Checksum:    sum,
		Status:      string(domain.StatusPending),
		Metadata:    []byte("{}"),
		OwnerUserID: ownerID,
	})
	if err != nil {
		_ = s.storage.Delete(ctx, storageKey)
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, &ErrKBNotFound{ID: in.KBID}
		}
		return nil, fmt.Errorf("create document: %w", err)
	}

	if err := s.enqueuer.EnqueueIngestion(ctx, row.ID.String()); err != nil {
		msg := err.Error()
		_ = s.queries.UpdateDocumentStatusForOwner(ctx, generated.UpdateDocumentStatusForOwnerParams{
			ID:           row.ID,
			Status:       string(domain.StatusFailed),
			ErrorMessage: &msg,
			OwnerUserID:  ownerID,
		})
		return nil, fmt.Errorf("enqueue: %w", err)
	}

	return rowToDocFull(row), nil
}

type ErrDocProcessing struct{ ID string }

func (e *ErrDocProcessing) Error() string { return "document is still being processed: " + e.ID }

// Reingest 把已入库的文档重新走一遍摄入管线。
// 仅允许 ready/failed 状态; worker 会在写入新 chunks 前删除旧 chunks。
func (s *Ingestion) Reingest(ctx context.Context, userID, docID string) (*domain.Document, error) {
	ownerID, err := ownerUUID(userID)
	if err != nil {
		return nil, err
	}
	id, err := uuid.Parse(docID)
	if err != nil {
		return nil, &ErrDocNotFound{ID: docID}
	}
	row, err := s.queries.GetDocumentForOwner(ctx, generated.GetDocumentForOwnerParams{ID: id, OwnerUserID: ownerID})
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

	if err := s.queries.UpdateDocumentStatusForOwner(ctx, generated.UpdateDocumentStatusForOwnerParams{
		ID: id, Status: string(domain.StatusPending), OwnerUserID: ownerID,
	}); err != nil {
		return nil, fmt.Errorf("reset document status: %w", err)
	}
	if err := s.enqueuer.EnqueueIngestion(ctx, docID); err != nil {
		msg := err.Error()
		_ = s.queries.UpdateDocumentStatusForOwner(ctx, generated.UpdateDocumentStatusForOwnerParams{
			ID: id, Status: string(domain.StatusFailed), ErrorMessage: &msg, OwnerUserID: ownerID,
		})
		return nil, fmt.Errorf("enqueue: %w", err)
	}

	row.Status = string(domain.StatusPending)
	row.ErrorMessage = nil
	return rowToDocFull(row), nil
}

// ReingestKB 把 KB 下所有 ready/failed 文档逐个重新入队, 返回入队数量。
func (s *Ingestion) ReingestKB(ctx context.Context, userID, kbID string) (int, error) {
	ownerID, err := ownerUUID(userID)
	if err != nil {
		return 0, err
	}
	kbUUID, err := uuid.Parse(kbID)
	if err != nil {
		return 0, &ErrKBNotFound{ID: kbID}
	}
	if _, err := s.queries.GetKnowledgeBaseForOwner(ctx, generated.GetKnowledgeBaseForOwnerParams{ID: kbUUID, OwnerUserID: ownerID}); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return 0, &ErrKBNotFound{ID: kbID}
		}
		return 0, fmt.Errorf("get kb: %w", err)
	}

	enqueued := 0
	const pageSize = int32(100)
	for offset := int32(0); ; offset += pageSize {
		rows, err := s.queries.ListDocumentsByKBForOwner(ctx, generated.ListDocumentsByKBForOwnerParams{
			KbID: kbUUID, OwnerUserID: ownerID, Limit: pageSize, Offset: offset,
		})
		if err != nil {
			return enqueued, fmt.Errorf("list documents: %w", err)
		}
		for _, row := range rows {
			if _, err := s.Reingest(ctx, userID, row.ID.String()); err != nil {
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
