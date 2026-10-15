package repo

import (
	"context"
	"encoding/json"
	"errors"
	"slices"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
	k "github.com/zenith-wang/it-wiki/backend/internal/knowledge"
	"github.com/zenith-wang/it-wiki/backend/internal/registry"
	"github.com/zenith-wang/it-wiki/backend/internal/repo/generated"
)

// Enqueue schedules indexing of a version inside the staging transaction.
type Enqueue func(context.Context, pgx.Tx, string) error

type KnowledgeRepository struct {
	pool    *pgxpool.Pool
	enqueue Enqueue
}

func NewKnowledgeRepository(p *pgxpool.Pool, enqueue Enqueue) *KnowledgeRepository {
	return &KnowledgeRepository{pool: p, enqueue: enqueue}
}
func ku(s string) uuid.UUID   { v, _ := uuid.Parse(s); return v }
func kn(s string) pgtype.UUID { v, e := uuid.Parse(s); return pgtype.UUID{Bytes: v, Valid: e == nil} }
func ks(v pgtype.UUID) string {
	if !v.Valid {
		return ""
	}
	return uuid.UUID(v.Bytes).String()
}
func ke(e error) error {
	if errors.Is(e, pgx.ErrNoRows) {
		return k.ErrNotFound
	}
	return e
}

// mutate runs fn in one transaction. Concurrent writers are ordered by row
// locks rather than a global lock: base FOR UPDATE (save/delete/upload, so the
// checksum check sees in-flight uploads), base FOR SHARE (grants vs. unshare),
// document FOR UPDATE (stage/publish) and application FOR UPDATE (credentials).
func (r *KnowledgeRepository) mutate(ctx context.Context, fn func(*generated.Queries, pgx.Tx) error) error {
	tx, e := r.pool.Begin(ctx)
	if e != nil {
		return e
	}
	defer tx.Rollback(context.Background())
	if e = fn(generated.New(tx), tx); e != nil {
		return e
	}
	return tx.Commit(ctx)
}
func base(b generated.KnowledgeBase, manage bool) k.KB {
	return k.KB{ID: b.ID.String(), Name: b.Name, Description: b.Description, Visibility: b.Visibility, Department: b.Department, Members: b.Members, Manage: manage, ApplicationShared: b.ApplicationShared, Model: b.EmbedModel, Dim: int(b.EmbedDim)}
}

// viewer is a principal's resolved authority, loaded once per operation.
type viewer struct {
	userID string
	who    registry.Identity
	app    bool
	grants []uuid.UUID
}

func loadViewer(ctx context.Context, q *generated.Queries, p k.Principal) (viewer, error) {
	if p.ApplicationID != "" {
		valid, e := q.KnowledgeCredentialCurrent(ctx, generated.KnowledgeCredentialCurrentParams{ID: ku(p.CredentialID), ApplicationID: ku(p.ApplicationID)})
		if e != nil {
			return viewer{}, e
		}
		if !valid {
			return viewer{}, k.ErrUnauthenticated
		}
		ids, e := q.KnowledgeGrants(ctx, ku(p.ApplicationID))
		return viewer{app: true, grants: ids}, e
	}
	who, e := identity(ctx, q, p.UserID)
	if errors.Is(e, registry.ErrIdentity) {
		return viewer{}, k.ErrForbidden
	}
	return viewer{userID: p.UserID, who: who}, e
}

// can reports read authority over b, or write authority when manage is set.
// Applications never manage and only read granted, application-shared bases.
func (v viewer) can(b generated.KnowledgeBase, manage bool) bool {
	if v.app {
		return !manage && b.ApplicationShared && slices.Contains(v.grants, b.ID)
	}
	if v.who.IsAdmin || ks(b.OwnerUserID) == v.userID {
		return true
	}
	return !manage && (b.Visibility == "company" || (b.Visibility == "department" && v.who.Department != "" && v.who.Department == b.Department) || (b.Visibility == "members" && slices.Contains(b.Members, v.who.OpenID)))
}
func access(ctx context.Context, q *generated.Queries, p k.Principal, b generated.KnowledgeBase, manage bool) error {
	v, e := loadViewer(ctx, q, p)
	if e != nil {
		return e
	}
	if !v.can(b, manage) {
		return k.ErrForbidden
	}
	return nil
}
func admin(ctx context.Context, q *generated.Queries, p k.Principal) error {
	if p.ApplicationID != "" {
		return k.ErrForbidden
	}
	who, e := identity(ctx, q, p.UserID)
	if e != nil || !who.IsAdmin {
		return k.ErrForbidden
	}
	return nil
}
func (r *KnowledgeRepository) ListKB(ctx context.Context, p k.Principal) ([]k.KB, error) {
	q := generated.New(r.pool)
	out := []k.KB{}
	v, e := loadViewer(ctx, q, p)
	if errors.Is(e, k.ErrForbidden) {
		return out, nil
	}
	if e != nil {
		return nil, e
	}
	rows, e := q.KnowledgeListBases(ctx)
	if e != nil {
		return nil, e
	}
	for _, b := range rows {
		if v.can(b, false) {
			out = append(out, base(b, v.can(b, true)))
		}
	}
	return out, nil
}

// AuthorizeManage checks write authority over one base. A missing base is
// reported as forbidden so callers cannot probe for existence.
func (r *KnowledgeRepository) AuthorizeManage(ctx context.Context, p k.Principal, id string) error {
	q := generated.New(r.pool)
	b, e := q.KnowledgeGetBase(ctx, ku(id))
	if errors.Is(e, pgx.ErrNoRows) {
		return k.ErrForbidden
	}
	if e != nil {
		return e
	}
	return access(ctx, q, p, b, true)
}
func (r *KnowledgeRepository) SaveKB(ctx context.Context, p k.Principal, id string, in k.KB, model string, dim int) (out k.KB, e error) {
	e = r.mutate(ctx, func(q *generated.Queries, tx pgx.Tx) error {
		owner := kn(p.UserID)
		if id != "" {
			b, e := q.KnowledgeLockBase(ctx, ku(id))
			if e != nil {
				return ke(e)
			}
			if e = access(ctx, q, p, b, true); e != nil {
				return e
			}
			owner = b.OwnerUserID
			model = b.EmbedModel
			dim = int(b.EmbedDim)
		} else {
			if _, e := identity(ctx, q, p.UserID); e != nil {
				return k.ErrForbidden
			}
			id = uuid.NewString()
		}
		if in.ApplicationShared && in.Visibility == "private" {
			return k.ErrInvalid
		}
		// Legacy remote documents have unverified source ACLs and cannot be shared.
		if in.ApplicationShared {
			docs, e := q.KnowledgeDocuments(ctx, ku(id))
			if e != nil {
				return e
			}
			for _, d := range docs {
				if d.SourceType != "local-upload" || !d.ActiveVersionID.Valid {
					return k.ErrConflict
				}
			}
		}
		if in.Members == nil {
			in.Members = []string{}
		}
		b, e := q.KnowledgeSaveBase(ctx, generated.KnowledgeSaveBaseParams{ID: ku(id), Name: in.Name, Description: in.Description, OwnerUserID: owner, EmbedModel: model, EmbedDim: int32(dim), Visibility: in.Visibility, Department: in.Department, Members: in.Members, ApplicationShared: in.ApplicationShared})
		if e != nil {
			return e
		}
		out = base(b, true)
		if !b.ApplicationShared {
			if e = q.KnowledgeRevokeBaseGrants(ctx, b.ID); e != nil {
				return e
			}
		}
		return knowledgeAdminAudit(ctx, q, p, "knowledge.base.save", id)
	})
	return
}
func knowledgeAdminAudit(ctx context.Context, q *generated.Queries, p k.Principal, action, target string) error {
	who, e := identity(ctx, q, p.UserID)
	if e != nil {
		return e
	}
	return writeAudit(ctx, q, who.OpenID, action, "knowledge", target, map[string]any{})
}
func (r *KnowledgeRepository) DeleteKB(ctx context.Context, p k.Principal, id string) error {
	return r.mutate(ctx, func(q *generated.Queries, tx pgx.Tx) error {
		b, e := q.KnowledgeLockBase(ctx, ku(id))
		if e != nil {
			return ke(e)
		}
		if e = access(ctx, q, p, b, true); e != nil {
			return e
		}
		if e = q.KnowledgeDeleteBase(ctx, b.ID); e != nil {
			return e
		}
		if e = q.KnowledgeRevokeBaseGrants(ctx, b.ID); e != nil {
			return e
		}
		return knowledgeAdminAudit(ctx, q, p, "knowledge.base.delete", id)
	})
}
func doc(d generated.Document) k.Document {
	msg := ""
	if d.ErrorMessage != nil {
		msg = *d.ErrorMessage
	}
	status := d.Status
	if !d.ActiveVersionID.Valid && !d.PendingVersionID.Valid {
		status = "legacy_requires_reindex"
	}
	return k.Document{ID: d.ID.String(), Title: d.Title, Status: status, Error: msg, ActiveVersion: ks(d.ActiveVersionID), PendingVersion: ks(d.PendingVersionID)}
}
func (r *KnowledgeRepository) Documents(ctx context.Context, p k.Principal, id string) ([]k.Document, error) {
	q := generated.New(r.pool)
	b, e := q.KnowledgeGetBase(ctx, ku(id))
	if e != nil {
		return nil, ke(e)
	}
	if e = access(ctx, q, p, b, false); e != nil {
		return nil, e
	}
	rows, e := q.KnowledgeDocuments(ctx, b.ID)
	if e != nil {
		return nil, e
	}
	out := []k.Document{}
	for _, d := range rows {
		out = append(out, doc(d))
	}
	return out, nil
}
func (r *KnowledgeRepository) Stage(ctx context.Context, p k.Principal, kbID, docID string, v k.Version, size int64) (out k.Document, e error) {
	e = r.mutate(ctx, func(q *generated.Queries, tx pgx.Tx) error {
		// Locking the base serializes uploads within one KB, so the checksum
		// check also covers versions other uploads are staging concurrently.
		b, e := q.KnowledgeLockBase(ctx, ku(kbID))
		if e != nil {
			return ke(e)
		}
		if e = access(ctx, q, p, b, true); e != nil {
			return e
		}
		if b.EmbedModel != v.Model || int(b.EmbedDim) != v.Dim {
			return k.ErrConflict
		}
		duplicate, e := q.KnowledgeDuplicateChecksum(ctx, generated.KnowledgeDuplicateChecksumParams{KbID: b.ID, ID: ku(docID), Checksum: v.Checksum})
		if e != nil {
			return e
		}
		if duplicate {
			return k.ErrConflict
		}
		var d generated.Document
		if docID == "" {
			d, e = q.KnowledgeCreateDocument(ctx, generated.KnowledgeCreateDocumentParams{ID: uuid.New(), KbID: b.ID, SourceRef: v.ObjectKey, Title: v.Title, MimeType: "application/octet-stream", Bytes: size, Checksum: v.Checksum})
		} else {
			d, e = q.KnowledgeLockDocument(ctx, ku(docID))
			if e == nil && (d.KbID != b.ID || d.SourceType != "local-upload") {
				return k.ErrForbidden
			}
		}
		if e != nil {
			return ke(e)
		}
		if e = q.KnowledgeCreateVersion(ctx, generated.KnowledgeCreateVersionParams{ID: ku(v.ID), DocumentID: d.ID, Title: v.Title, ObjectKey: v.ObjectKey, Checksum: v.Checksum, EmbedModel: v.Model, EmbedDim: int32(v.Dim)}); e != nil {
			return e
		}
		if e = q.KnowledgeStageVersion(ctx, generated.KnowledgeStageVersionParams{ID: d.ID, PendingVersionID: kn(v.ID)}); e != nil {
			return e
		}
		if e = r.enqueue(ctx, tx, v.ID); e != nil {
			return e
		}
		d.PendingVersionID = kn(v.ID)
		d.Status = "pending"
		d.ErrorMessage = nil
		out = doc(d)
		return nil
	})
	return
}
func docAccess(ctx context.Context, q *generated.Queries, p k.Principal, id string, manage bool) (generated.Document, error) {
	d, e := q.KnowledgeDocument(ctx, ku(id))
	if e != nil {
		return d, ke(e)
	}
	b, e := q.KnowledgeGetBase(ctx, d.KbID)
	if e != nil {
		return d, ke(e)
	}
	return d, access(ctx, q, p, b, manage)
}
func (r *KnowledgeRepository) DeleteDocument(ctx context.Context, p k.Principal, id string) error {
	return r.mutate(ctx, func(q *generated.Queries, tx pgx.Tx) error {
		if _, e := docAccess(ctx, q, p, id, true); e != nil {
			return e
		}
		return q.KnowledgeDeleteDocument(ctx, ku(id))
	})
}
func version(v generated.KnowledgeVersionRow) k.Version {
	out := k.Version{ID: v.ID.String(), DocumentID: v.DocumentID.String(), KBID: v.KbID.String(), Title: v.Title, Status: v.Status, Error: v.ErrorCode, Model: v.EmbedModel, Dim: int(v.EmbedDim), ObjectKey: v.ObjectKey, Checksum: v.Checksum}
	_ = json.Unmarshal(v.Preview, &out.Preview)
	return out
}
func (r *KnowledgeRepository) Preview(ctx context.Context, p k.Principal, id string) (k.Version, error) {
	q := generated.New(r.pool)
	v, e := q.KnowledgeVersion(ctx, ku(id))
	if e != nil {
		return k.Version{}, ke(e)
	}
	d, e := docAccess(ctx, q, p, v.DocumentID.String(), false)
	if e != nil {
		return k.Version{}, e
	}
	if ks(d.ActiveVersionID) != id {
		if _, e = docAccess(ctx, q, p, v.DocumentID.String(), true); e != nil {
			return k.Version{}, e
		}
	}
	return version(v), nil
}
func (r *KnowledgeRepository) Retry(ctx context.Context, p k.Principal, id string) error {
	return r.mutate(ctx, func(q *generated.Queries, tx pgx.Tx) error {
		d, e := docAccess(ctx, q, p, id, true)
		if e != nil {
			return e
		}
		// Conditional on the failed pending version still being current, so a
		// concurrent re-upload or delete is never overwritten by a stale retry.
		n, e := q.KnowledgeRetryDocument(ctx, generated.KnowledgeRetryDocumentParams{ID: d.ID, PendingVersionID: d.PendingVersionID})
		if e != nil {
			return e
		}
		if n != 1 {
			return k.ErrConflict
		}
		return r.enqueue(ctx, tx, ks(d.PendingVersionID))
	})
}
func (r *KnowledgeRepository) LoadVersion(ctx context.Context, id string) (k.Version, error) {
	v, e := generated.New(r.pool).KnowledgeVersion(ctx, ku(id))
	return version(v), ke(e)
}
func (r *KnowledgeRepository) RecordPreview(ctx context.Context, id string, p k.Preview) error {
	raw, e := json.Marshal(p)
	if e != nil {
		return e
	}
	return r.mutate(ctx, func(q *generated.Queries, tx pgx.Tx) error {
		if e := q.KnowledgePreview(ctx, generated.KnowledgePreviewParams{ID: ku(id), Preview: raw}); e != nil {
			return e
		}
		return q.KnowledgeDocumentProcessing(ctx, kn(id))
	})
}
func (r *KnowledgeRepository) Fail(ctx context.Context, id, code string) error {
	return r.mutate(ctx, func(q *generated.Queries, tx pgx.Tx) error {
		if e := q.KnowledgeVersionFail(ctx, generated.KnowledgeVersionFailParams{ID: ku(id), ErrorCode: code}); e != nil {
			return e
		}
		return q.KnowledgeDocumentFail(ctx, generated.KnowledgeDocumentFailParams{PendingVersionID: kn(id), ErrorMessage: &code})
	})
}

// Authorize resolves the principal once and checks every requested base. A
// missing base is reported as forbidden, never as not found.
func (r *KnowledgeRepository) Authorize(ctx context.Context, p k.Principal, ids []string) ([]k.KB, error) {
	q := generated.New(r.pool)
	v, e := loadViewer(ctx, q, p)
	if e != nil {
		return nil, e
	}
	out := make([]k.KB, 0, len(ids))
	for _, id := range ids {
		b, e := q.KnowledgeGetBase(ctx, ku(id))
		if errors.Is(e, pgx.ErrNoRows) {
			return nil, k.ErrForbidden
		}
		if e != nil {
			return nil, e
		}
		if !v.can(b, false) {
			return nil, k.ErrForbidden
		}
		ready, e := q.KnowledgeBaseReadiness(ctx, b.ID)
		if e != nil {
			return nil, e
		}
		if ready.HasDocuments && !ready.HasActive {
			return nil, k.ErrNotReady
		}
		item := base(b, false)
		item.HasActive = ready.HasActive
		out = append(out, item)
	}
	return out, nil
}
func (r *KnowledgeRepository) Recheck(ctx context.Context, p k.Principal, ids []string, matches []k.Match) ([]k.Match, error) {
	if _, e := r.Authorize(ctx, p, ids); e != nil {
		return nil, e
	}
	out := []k.Match{}
	if len(matches) == 0 {
		return out, nil
	}
	documentIDs := make([]uuid.UUID, len(matches))
	for i, m := range matches {
		documentIDs[i] = ku(m.DocumentID)
	}
	kbIDs := make([]uuid.UUID, len(ids))
	for i, id := range ids {
		kbIDs[i] = ku(id)
	}
	documents, e := generated.New(r.pool).KnowledgeActiveDocuments(ctx, generated.KnowledgeActiveDocumentsParams{DocumentIds: documentIDs, KbIds: kbIDs})
	if e != nil {
		return nil, e
	}
	active := map[string]string{}
	for _, d := range documents {
		active[d.ID.String()] = ks(d.ActiveVersionID)
	}
	for _, m := range matches {
		if active[m.DocumentID] == m.VersionID && m.VersionID != "" {
			out = append(out, m)
		}
	}
	return out, nil
}
func (r *KnowledgeRepository) Authenticate(ctx context.Context, digest []byte) (k.Principal, error) {
	q := generated.New(r.pool)
	c, e := q.KnowledgeCredential(ctx, digest)
	if errors.Is(e, pgx.ErrNoRows) {
		return k.Principal{}, k.ErrUnauthenticated
	}
	if e != nil {
		return k.Principal{}, e
	}
	if !slices.Contains(c.Scopes, "knowledge:search") {
		return k.Principal{}, k.ErrForbidden
	}
	if e = q.KnowledgeCredentialUsed(ctx, c.ID); e != nil {
		return k.Principal{}, e
	}
	return k.Principal{ApplicationID: c.ApplicationID.String(), CredentialID: c.ID.String()}, nil
}
func (r *KnowledgeRepository) Applications(ctx context.Context, p k.Principal) ([]k.Application, error) {
	q := generated.New(r.pool)
	if e := admin(ctx, q, p); e != nil {
		return nil, e
	}
	rows, e := q.KnowledgeApplications(ctx)
	if e != nil {
		return nil, e
	}
	out := []k.Application{}
	for _, a := range rows {
		ids, e := q.KnowledgeGrants(ctx, a.ID)
		if e != nil {
			return nil, e
		}
		v := k.Application{ID: a.ID.String(), Name: a.Name, Enabled: a.Enabled, KBIDs: []string{}}
		for _, id := range ids {
			v.KBIDs = append(v.KBIDs, id.String())
		}
		c, e := q.KnowledgeLatestCredential(ctx, a.ID)
		if e == nil {
			v.Prefix = c.Prefix
			v.ExpiresAt = &c.ExpiresAt
			if c.LastUsedAt.Valid {
				v.LastUsedAt = &c.LastUsedAt.Time
			}
		} else if !errors.Is(e, pgx.ErrNoRows) {
			return nil, e
		}
		out = append(out, v)
	}
	return out, nil
}
func (r *KnowledgeRepository) SaveApplication(ctx context.Context, p k.Principal, id string, in k.ApplicationInput) (out k.Application, e error) {
	e = r.mutate(ctx, func(q *generated.Queries, tx pgx.Tx) error {
		if e := admin(ctx, q, p); e != nil {
			return e
		}
		if id == "" {
			id = uuid.NewString()
		}
		// FOR SHARE orders this against a concurrent unshare/delete of the base,
		// whose grant revocation then sees (and removes) the grant written here.
		for _, kbID := range in.KBIDs {
			b, e := q.KnowledgeShareLockBase(ctx, ku(kbID))
			if errors.Is(e, pgx.ErrNoRows) || (e == nil && !b.ApplicationShared) {
				return k.ErrForbidden
			}
			if e != nil {
				return e
			}
		}
		a, e := q.KnowledgeSaveApplication(ctx, generated.KnowledgeSaveApplicationParams{ID: ku(id), Name: in.Name, Enabled: in.Enabled})
		if e != nil {
			return e
		}
		if e = q.KnowledgeClearGrants(ctx, a.ID); e != nil {
			return e
		}
		for _, kbID := range in.KBIDs {
			if e = q.KnowledgeGrant(ctx, generated.KnowledgeGrantParams{ApplicationID: a.ID, KbID: ku(kbID)}); e != nil {
				return e
			}
		}
		out = k.Application{ID: id, Name: a.Name, Enabled: a.Enabled, KBIDs: in.KBIDs}
		return knowledgeAdminAudit(ctx, q, p, "knowledge.application.save", id)
	})
	return
}

// lockApplication requires an admin and an existing application, and holds the
// application row so concurrent rotations cannot leave two live credentials.
func lockApplication(ctx context.Context, q *generated.Queries, p k.Principal, id string) error {
	if e := admin(ctx, q, p); e != nil {
		return e
	}
	_, e := q.KnowledgeLockApplication(ctx, ku(id))
	return ke(e)
}
func (r *KnowledgeRepository) Rotate(ctx context.Context, p k.Principal, id string, digest []byte, prefix string, expiry time.Time) error {
	return r.mutate(ctx, func(q *generated.Queries, tx pgx.Tx) error {
		if e := lockApplication(ctx, q, p, id); e != nil {
			return e
		}
		if e := q.KnowledgeRevoke(ctx, ku(id)); e != nil {
			return e
		}
		if e := q.KnowledgeIssue(ctx, generated.KnowledgeIssueParams{ApplicationID: ku(id), Digest: digest, Prefix: prefix, ExpiresAt: expiry}); e != nil {
			return e
		}
		return knowledgeAdminAudit(ctx, q, p, "knowledge.credential.rotate", id)
	})
}
func (r *KnowledgeRepository) Revoke(ctx context.Context, p k.Principal, id string) error {
	return r.mutate(ctx, func(q *generated.Queries, tx pgx.Tx) error {
		if e := lockApplication(ctx, q, p, id); e != nil {
			return e
		}
		if e := q.KnowledgeRevoke(ctx, ku(id)); e != nil {
			return e
		}
		return knowledgeAdminAudit(ctx, q, p, "knowledge.credential.revoke", id)
	})
}

// Audit records one search; kbIDs must already be validated and deduplicated.
func (r *KnowledgeRepository) Audit(ctx context.Context, p k.Principal, kbIDs []string, requestID, status string, count int, duration time.Duration) error {
	ids := make([]uuid.UUID, len(kbIDs))
	for i, id := range kbIDs {
		ids[i] = ku(id)
	}
	return generated.New(r.pool).KnowledgeAudit(ctx, generated.KnowledgeAuditParams{ApplicationID: kn(p.ApplicationID), UserID: kn(p.UserID), KnowledgeBaseIds: ids, RequestID: requestID, Status: status, ResultCount: int32(count), DurationMs: duration.Milliseconds()})
}
func (r *KnowledgeRepository) AuditEntries(ctx context.Context, p k.Principal) ([]k.AuditEntry, error) {
	q := generated.New(r.pool)
	if e := admin(ctx, q, p); e != nil {
		return nil, e
	}
	rows, e := q.KnowledgeAuditEntries(ctx)
	if e != nil {
		return nil, e
	}
	out := []k.AuditEntry{}
	for _, row := range rows {
		ids := []string{}
		for _, id := range row.KnowledgeBaseIds {
			ids = append(ids, id.String())
		}
		out = append(out, k.AuditEntry{KBIDs: ids, RequestID: row.RequestID, ApplicationID: ks(row.ApplicationID), Status: row.Status, ResultCount: int(row.ResultCount), DurationMS: row.DurationMs, CreatedAt: row.CreatedAt})
	}
	return out, nil
}
