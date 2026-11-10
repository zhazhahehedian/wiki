package repo

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/zenith-wang/it-wiki/backend/internal/registry"
	"github.com/zenith-wang/it-wiki/backend/internal/repo/generated"
)

// BundleUploads stores browser part-upload sessions for large MCP packages.
// Every lookup is scoped to the uploading user.
type BundleUploads struct{ q *generated.Queries }

func NewBundleUploads(q generated.DBTX) *BundleUploads { return &BundleUploads{q: generated.New(q)} }

func bundleUpload(row generated.CapabilityBundleUpload) (registry.BundleUpload, error) {
	u := registry.BundleUpload{ID: row.ID.String(), FileName: row.FileName, Size: row.Size, PartSize: row.PartSize, ObjectKey: row.ObjectKey, StorageUploadID: row.StorageUploadID, CreatedAt: row.CreatedAt, ExpiresAt: row.ExpiresAt, Completed: row.CompletedAt.Valid}
	if row.BundleSha256 != nil {
		u.BundleSHA256 = *row.BundleSha256
	}
	if len(row.BundleFindings) > 0 {
		if e := json.Unmarshal(row.BundleFindings, &u.BundleFindings); e != nil {
			return registry.BundleUpload{}, e
		}
	}
	return u, nil
}

func uploadIDs(user, id string) (uuid.UUID, uuid.UUID, error) {
	u, e := uuid.Parse(user)
	if e != nil {
		return uuid.Nil, uuid.Nil, registry.ErrIdentity
	}
	i, e := uuid.Parse(id)
	if e != nil {
		return uuid.Nil, uuid.Nil, registry.ErrNotFound
	}
	return u, i, nil
}

func (r *BundleUploads) CreateUpload(ctx context.Context, user string, u registry.BundleUpload, maxActive int) (registry.BundleUpload, error) {
	userID, id, e := uploadIDs(user, u.ID)
	if e != nil {
		return registry.BundleUpload{}, e
	}
	row, e := r.q.BundleUploadCreate(ctx, generated.BundleUploadCreateParams{ID: id, UserID: userID, FileName: u.FileName, Size: u.Size, PartSize: u.PartSize, ObjectKey: u.ObjectKey, StorageUploadID: u.StorageUploadID, ExpiresAt: u.ExpiresAt, MaxActive: int32(maxActive)})
	if errors.Is(e, pgx.ErrNoRows) {
		return registry.BundleUpload{}, registry.ErrUploadQuota
	}
	if e != nil {
		return registry.BundleUpload{}, e
	}
	return bundleUpload(row)
}

func (r *BundleUploads) GetUpload(ctx context.Context, user, id string) (registry.BundleUpload, error) {
	userID, uploadID, e := uploadIDs(user, id)
	if e != nil {
		return registry.BundleUpload{}, e
	}
	row, e := r.q.BundleUploadGet(ctx, generated.BundleUploadGetParams{ID: uploadID, UserID: userID})
	if e != nil {
		return registry.BundleUpload{}, registryError(e)
	}
	return bundleUpload(row)
}

func (r *BundleUploads) CompleteUpload(ctx context.Context, user, id, sha string, findings []registry.BundleFinding) (registry.BundleUpload, error) {
	userID, uploadID, e := uploadIDs(user, id)
	if e != nil {
		return registry.BundleUpload{}, e
	}
	var raw []byte
	if findings != nil {
		if raw, e = json.Marshal(findings); e != nil {
			return registry.BundleUpload{}, e
		}
	}
	row, e := r.q.BundleUploadComplete(ctx, generated.BundleUploadCompleteParams{ID: uploadID, UserID: userID, BundleSha256: &sha, BundleFindings: raw})
	if errors.Is(e, pgx.ErrNoRows) {
		return registry.BundleUpload{}, registry.ErrConflict
	}
	if e != nil {
		return registry.BundleUpload{}, e
	}
	return bundleUpload(row)
}

func (r *BundleUploads) DeleteUpload(ctx context.Context, user, id string) error {
	userID, uploadID, e := uploadIDs(user, id)
	if e != nil {
		return e
	}
	return r.q.BundleUploadDelete(ctx, generated.BundleUploadDeleteParams{ID: uploadID, UserID: userID})
}

func (r *BundleUploads) ExpiredUploads(ctx context.Context, now time.Time) ([]registry.BundleUpload, error) {
	rows, e := r.q.BundleUploadsExpired(ctx, now)
	if e != nil {
		return nil, e
	}
	out := make([]registry.BundleUpload, 0, len(rows))
	for _, row := range rows {
		u, e := bundleUpload(row)
		if e != nil {
			return nil, e
		}
		out = append(out, u)
	}
	return out, nil
}

func (r *BundleUploads) DeleteExpiredUpload(ctx context.Context, id string, now time.Time) error {
	uploadID, e := uuid.Parse(id)
	if e != nil {
		return registry.ErrNotFound
	}
	return r.q.BundleUploadDeleteExpired(ctx, generated.BundleUploadDeleteExpiredParams{ID: uploadID, ExpiresAt: now})
}
