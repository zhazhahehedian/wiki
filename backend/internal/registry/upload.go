package registry

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/zenith-wang/it-wiki/backend/internal/domain/ports"
)

// Browser uploads of large MCP packages: the client sends fixed-size parts
// that the server writes into a storage multipart upload, can resume by
// asking which parts arrived, and completes once. Completion assembles the
// object, inspects it through ranged reads and records digest and findings.
// A capability save then references the completed upload by ID.
const (
	UploadPartSize   = 8 << 20
	UploadTTL        = 12 * time.Hour // well below OrphanMinAge; see sweep.go
	MaxActiveUploads = 5
	// Each in-flight part is buffered once so the storage client can sign and
	// retry it; this bounds that memory.
	maxConcurrentParts       = 8
	maxConcurrentInspections = 2
	inspectWindow            = 8 << 20
	minZIPBytes              = 22
)

type BundleUpload struct {
	ID              string          `json:"id"`
	FileName        string          `json:"file_name"`
	Size            int64           `json:"size"`
	PartSize        int64           `json:"part_size"`
	PartCount       int             `json:"part_count"`
	ReceivedParts   []int           `json:"received_parts"`
	Completed       bool            `json:"completed"`
	BundleSHA256    string          `json:"bundle_sha256,omitempty"`
	BundleFindings  []BundleFinding `json:"bundle_findings,omitempty"`
	ExpiresAt       time.Time       `json:"expires_at"`
	CreatedAt       time.Time       `json:"-"`
	ObjectKey       string          `json:"-"`
	StorageUploadID string          `json:"-"`
}

type UploadRepository interface {
	CreateUpload(ctx context.Context, user string, u BundleUpload, maxActive int) (BundleUpload, error)
	GetUpload(ctx context.Context, user, id string) (BundleUpload, error)
	CompleteUpload(ctx context.Context, user, id, sha string, findings []BundleFinding) (BundleUpload, error)
	DeleteUpload(ctx context.Context, user, id string) error
	ExpiredUploads(ctx context.Context, now time.Time) ([]BundleUpload, error)
	DeleteExpiredUpload(ctx context.Context, id string, now time.Time) error
}

// UploadStorage is the object storage needed for part uploads.
type UploadStorage interface {
	ports.ObjectStorage
	ports.RangeReader
	ports.MultipartStorage
}

type uploads struct {
	repo        UploadRepository
	store       UploadStorage
	parts       chan struct{}
	inspections chan struct{}
	mu          sync.Mutex
	completing  map[string]bool
	now         func() time.Time
}

// WithUploads enables browser part uploads. The storage also serves the
// service's regular object access.
func (s *Service) WithUploads(repo UploadRepository, store UploadStorage) *Service {
	s.storage = store
	s.uploads = &uploads{repo: repo, store: store, parts: make(chan struct{}, maxConcurrentParts), inspections: make(chan struct{}, maxConcurrentInspections), completing: map[string]bool{}, now: time.Now}
	return s
}

func (u *BundleUpload) partCount() int {
	return int((u.Size + u.PartSize - 1) / u.PartSize)
}

func (u *BundleUpload) partBytes(number int) int64 {
	if number == u.partCount() {
		return u.Size - int64(number-1)*u.PartSize
	}
	return u.PartSize
}

func (u BundleUpload) withProgress() BundleUpload {
	u.PartCount = u.partCount()
	u.ReceivedParts = []int{}
	return u
}

func acquire(ctx context.Context, slots chan struct{}) (func(), error) {
	select {
	case slots <- struct{}{}:
		return func() { <-slots }, nil
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

// activeUpload loads an unexpired upload owned by user.
func (s *Service) activeUpload(ctx context.Context, user, id string) (BundleUpload, error) {
	if s.uploads == nil {
		return BundleUpload{}, ErrNotFound
	}
	u, err := s.uploads.repo.GetUpload(ctx, user, id)
	if err != nil {
		return BundleUpload{}, err
	}
	if !u.ExpiresAt.After(s.uploads.now()) {
		return BundleUpload{}, ErrNotFound
	}
	return u.withProgress(), nil
}

func (s *Service) CreateUpload(ctx context.Context, user, name string, size int64) (BundleUpload, error) {
	if s.uploads == nil {
		return BundleUpload{}, ErrNotFound
	}
	if !validPackageName(name) || len(name) > 255 || size < minZIPBytes || size > MaxMCPBundleBytes {
		return BundleUpload{}, ErrMCPPackage
	}
	u := BundleUpload{ID: uuid.NewString(), FileName: name, Size: size, PartSize: UploadPartSize, ObjectKey: bundlePrefix + uuid.NewString() + ".zip", ExpiresAt: s.uploads.now().Add(UploadTTL)}
	storageID, err := s.uploads.store.CreateMultipart(ctx, u.ObjectKey, "application/zip")
	if err != nil {
		return BundleUpload{}, err
	}
	u.StorageUploadID = storageID
	saved, err := s.uploads.repo.CreateUpload(ctx, user, u, MaxActiveUploads)
	if err != nil {
		s.abortUpload(u)
		return BundleUpload{}, err
	}
	return saved.withProgress(), nil
}

// Upload reports progress so a client can resume by sending missing parts.
func (s *Service) Upload(ctx context.Context, user, id string) (BundleUpload, error) {
	u, err := s.activeUpload(ctx, user, id)
	if err != nil || u.Completed {
		return u, err
	}
	parts, err := s.uploads.store.ListParts(ctx, u.ObjectKey, u.StorageUploadID)
	if err != nil {
		return BundleUpload{}, err
	}
	for _, p := range parts {
		if n := int(p.Number); n >= 1 && n <= u.PartCount && p.Size == u.partBytes(n) {
			u.ReceivedParts = append(u.ReceivedParts, n)
		}
	}
	return u, nil
}

// UploadPart stores one part. Re-sending a part replaces it.
func (s *Service) UploadPart(ctx context.Context, user, id string, number int, body io.Reader) error {
	u, err := s.activeUpload(ctx, user, id)
	if err != nil {
		return err
	}
	if u.Completed {
		return ErrConflict
	}
	if number < 1 || number > u.PartCount {
		return ErrInvalid
	}
	release, err := acquire(ctx, s.uploads.parts)
	if err != nil {
		return err
	}
	defer release()
	buf := make([]byte, u.partBytes(number))
	if _, err = io.ReadFull(body, buf); err != nil {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		return ErrInvalid
	}
	if _, err = io.ReadFull(body, make([]byte, 1)); err == nil {
		return ErrInvalid
	}
	return s.uploads.store.UploadPart(ctx, u.ObjectKey, u.StorageUploadID, int32(number), bytes.NewReader(buf), int64(len(buf)))
}

// CompleteUpload assembles and inspects the package. It is idempotent: a
// retry after an interrupted completion resumes from the assembled object.
// An invalid package is removed and the upload ends with ErrMCPPackage.
func (s *Service) CompleteUpload(ctx context.Context, user, id string) (BundleUpload, error) {
	u, err := s.activeUpload(ctx, user, id)
	if err != nil || u.Completed {
		return u, err
	}
	s.uploads.mu.Lock()
	if s.uploads.completing[u.ID] {
		s.uploads.mu.Unlock()
		return BundleUpload{}, ErrConflict
	}
	s.uploads.completing[u.ID] = true
	s.uploads.mu.Unlock()
	defer func() {
		s.uploads.mu.Lock()
		delete(s.uploads.completing, u.ID)
		s.uploads.mu.Unlock()
	}()

	if size, err := s.uploads.store.ObjectSize(ctx, u.ObjectKey); err != nil || size != u.Size {
		if err == nil {
			return BundleUpload{}, fmt.Errorf("assembled bundle size %d, want %d", size, u.Size)
		}
		if err = s.assemble(ctx, u); err != nil {
			return BundleUpload{}, err
		}
	}

	release, err := acquire(ctx, s.uploads.inspections)
	if err != nil {
		return BundleUpload{}, err
	}
	sum, findings, err := s.inspectStored(ctx, u.ObjectKey, u.Size)
	release()
	if errors.Is(err, ErrMCPPackage) {
		s.discardUpload(user, u)
		return BundleUpload{}, ErrMCPPackage
	}
	if err != nil {
		return BundleUpload{}, err
	}
	done, err := s.uploads.repo.CompleteUpload(ctx, user, u.ID, sum, findings)
	if err != nil {
		return BundleUpload{}, err
	}
	return done.withProgress(), nil
}

// assemble completes the storage upload once every part has arrived.
func (s *Service) assemble(ctx context.Context, u BundleUpload) error {
	parts, err := s.uploads.store.ListParts(ctx, u.ObjectKey, u.StorageUploadID)
	if err != nil {
		return err
	}
	byNumber := map[int32]ports.StoragePart{}
	for _, p := range parts {
		byNumber[p.Number] = p
	}
	ordered := make([]ports.StoragePart, 0, u.PartCount)
	for n := 1; n <= u.PartCount; n++ {
		p, ok := byNumber[int32(n)]
		if !ok || p.Size != u.partBytes(n) {
			return ErrUploadNotReady
		}
		ordered = append(ordered, p)
	}
	return s.uploads.store.CompleteMultipart(ctx, u.ObjectKey, u.StorageUploadID, ordered)
}

// inspectStored hashes the object in one streaming pass and validates the
// archive in a second pass of ranged reads; neither holds the package.
func (s *Service) inspectStored(ctx context.Context, key string, size int64) (string, []BundleFinding, error) {
	body, err := s.uploads.store.Get(ctx, key)
	if err != nil {
		return "", nil, err
	}
	hash := sha256.New()
	n, err := io.Copy(hash, body)
	_ = body.Close()
	if err != nil {
		return "", nil, err
	}
	if n != size {
		return "", nil, fmt.Errorf("stored bundle size %d, want %d", n, size)
	}
	reader, err := s.objectReader(ctx, key, size, inspectWindow)
	if err != nil {
		return "", nil, err
	}
	findings, err := inspectMCPArchive(ctx, reader, size)
	if reader.failure != nil && errors.Is(err, ErrMCPPackage) {
		return "", nil, reader.failure
	}
	if err != nil {
		return "", nil, err
	}
	return hex.EncodeToString(hash.Sum(nil)), findings, nil
}

// CancelUpload discards an upload that is no longer wanted.
func (s *Service) CancelUpload(ctx context.Context, user, id string) error {
	if s.uploads == nil {
		return ErrNotFound
	}
	u, err := s.uploads.repo.GetUpload(ctx, user, id)
	if err != nil {
		return err
	}
	s.discardUpload(user, u)
	return nil
}

// discardUpload drops the session and its storage. The object is deleted only
// when no version references it.
func (s *Service) discardUpload(user string, u BundleUpload) {
	if !u.Completed {
		s.abortUpload(u)
	}
	s.cleanupBundle(u.ObjectKey)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := s.uploads.repo.DeleteUpload(ctx, user, u.ID); err != nil {
		slog.Warn("bundle upload record cleanup failed; it expires later")
	}
}

func (s *Service) abortUpload(u BundleUpload) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	// Aborting an already completed or missing upload fails harmlessly.
	_ = s.uploads.store.AbortMultipart(ctx, u.ObjectKey, u.StorageUploadID)
}

// readyUpload returns a completed, unexpired upload for a save.
func (s *Service) readyUpload(ctx context.Context, user, id string) (BundleUpload, error) {
	u, err := s.activeUpload(ctx, user, id)
	if errors.Is(err, ErrNotFound) {
		return BundleUpload{}, ErrUploadNotReady
	}
	if err != nil {
		return BundleUpload{}, err
	}
	if !u.Completed {
		return BundleUpload{}, ErrUploadNotReady
	}
	return u, nil
}

// SweepExpiredUploads ends sessions past their TTL. Unfinished storage
// uploads are aborted; completed objects are left to the orphan sweep, which
// deletes them only when no version references them.
func (s *Service) SweepExpiredUploads(ctx context.Context, now time.Time) (int, error) {
	if s.uploads == nil {
		return 0, nil
	}
	removed := 0
	for {
		expired, err := s.uploads.repo.ExpiredUploads(ctx, now)
		if err != nil {
			return removed, err
		}
		for _, u := range expired {
			if !u.Completed {
				if err := s.uploads.store.AbortMultipart(ctx, u.ObjectKey, u.StorageUploadID); err != nil && ctx.Err() == nil {
					slog.Warn("abort of expired bundle upload failed; storage expiry reclaims it")
				}
			}
			if err := s.uploads.repo.DeleteExpiredUpload(ctx, u.ID, now); err != nil {
				return removed, err
			}
			removed++
		}
		if len(expired) < 100 {
			return removed, nil
		}
	}
}
