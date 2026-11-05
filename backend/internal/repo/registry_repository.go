package repo

import (
	"context"
	"encoding/json"
	"errors"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/zenith-wang/it-wiki/backend/internal/registry"
	"github.com/zenith-wang/it-wiki/backend/internal/repo/generated"
)

// EventSink enqueues a governance event inside the transition transaction.
type EventSink func(ctx context.Context, tx pgx.Tx, event registry.Event) error

type RegistryRepository struct {
	pool   *pgxpool.Pool
	q      *generated.Queries
	events EventSink
}

func NewRegistryRepository(pool *pgxpool.Pool) *RegistryRepository {
	return &RegistryRepository{pool: pool, q: generated.New(pool)}
}

// WithEvents enables governance notifications. Without a sink, transitions
// behave exactly as before.
func (r *RegistryRepository) WithEvents(sink EventSink) *RegistryRepository {
	r.events = sink
	return r
}
func registryError(e error) error {
	if errors.Is(e, pgx.ErrNoRows) {
		return registry.ErrNotFound
	}
	var pe *pgconn.PgError
	if errors.As(e, &pe) && (pe.Code == "23505" || pe.Code == "40001") {
		return registry.ErrConflict
	}
	return e
}
func (r *RegistryRepository) Owner(ctx context.Context, user string) (string, error) {
	id, e := uuid.Parse(user)
	if e != nil {
		return "", registry.ErrIdentity
	}
	owner, e := r.q.RegistryOwner(ctx, id)
	if errors.Is(e, pgx.ErrNoRows) {
		return "", registry.ErrIdentity
	}
	return owner, e
}
func capability(c generated.Capability) registry.Capability {
	result := registry.Capability{ID: c.ID.String(), Slug: c.Slug, Type: c.Type, Name: c.Name, Description: c.Description, OwnerOpenID: c.OwnerOpenID, Department: c.Department, Status: c.Status, Visibility: c.Visibility, IsLive: c.IsLive, ReviewReason: c.ReviewReason, Revision: c.Revision, CreatedAt: c.CreatedAt, UpdatedAt: c.UpdatedAt}
	if c.CurrentVersionID.Valid {
		// Current and draft metadata each retain their own department ID.
		result.CurrentVersionID = uuid.UUID(c.CurrentVersionID.Bytes).String()
	}
	if c.DraftVersionID.Valid {
		result.DraftVersionID = uuid.UUID(c.DraftVersionID.Bytes).String()
	}
	result.DepartmentID = c.DepartmentID
	return result
}
func textValue(v *string) string {
	if v == nil {
		return ""
	}
	return *v
}
func nullableText(v string) *string {
	if v == "" {
		return nil
	}
	return &v
}
func nullableSize(v int64) *int64 {
	if v <= 0 {
		return nil
	}
	return &v
}
func registryDetail(ctx context.Context, q *generated.Queries, c generated.Capability) (registry.Detail, error) {
	result := registry.Detail{Capability: capability(c), Versions: []registry.Version{}}
	rows, e := q.ListCapabilityVersions(ctx, c.ID)
	if e != nil {
		return result, e
	}
	for _, v := range rows {
		ver := registry.Version{ID: v.ID.String(), Version: v.Version, Changelog: v.Changelog, CreatedBy: v.CreatedBy, CreatedAt: v.CreatedAt, MCPEndpoint: textValue(v.McpEndpoint), MCPTransport: textValue(v.McpTransport), MCPAuthScheme: textValue(v.McpAuthScheme), BundleKey: textValue(v.BundleKey), HasBundle: v.BundleKey != nil, BundleSHA256: textValue(v.BundleSha256), SkillManifest: json.RawMessage(v.SkillManifest)}
		if v.BundleSize != nil {
			ver.BundleSize = *v.BundleSize
		}
		if len(v.BundleFindings) > 0 {
			if e = json.Unmarshal(v.BundleFindings, &ver.BundleFindings); e != nil {
				return result, e
			}
		}
		if v.PublishedAt.Valid {
			t := v.PublishedAt.Time
			ver.PublishedAt = &t
		}
		if len(v.Tools) > 0 {
			if e = json.Unmarshal(v.Tools, &ver.Tools); e != nil {
				return result, e
			}
		}
		result.Versions = append(result.Versions, ver)
	}
	if len(c.PublishedMetadata) > 0 {
		result.Published = &registry.PublishedMetadata{}
		if e = json.Unmarshal(c.PublishedMetadata, result.Published); e != nil {
			return result, e
		}
	}
	result.Allowlist, e = q.ListCapabilityAllowlist(ctx, c.ID)
	return result, e
}
func (r *RegistryRepository) List(ctx context.Context, owner string, f registry.Filter) (registry.Page, error) {
	rows, e := r.q.ListOwnedCapabilities(ctx, generated.ListOwnedCapabilitiesParams{OwnerOpenID: owner, FilterType: f.Type, FilterStatus: f.Status, FilterDepartment: f.Department, Search: f.Search, PageLimit: f.Limit + 1, PageOffset: f.Offset})
	page := registry.Page{Items: []registry.Capability{}}
	if e != nil {
		return page, e
	}
	page.HasMore = len(rows) > int(f.Limit)
	if page.HasMore {
		rows = rows[:f.Limit]
	}
	for _, c := range rows {
		page.Items = append(page.Items, capability(c))
	}
	return page, nil
}
func (r *RegistryRepository) Get(ctx context.Context, owner, slug string) (registry.Detail, error) {
	tx, e := r.pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.RepeatableRead, AccessMode: pgx.ReadOnly})
	if e != nil {
		return registry.Detail{}, e
	}
	defer tx.Rollback(context.Background())
	q := r.q.WithTx(tx)
	c, e := q.GetOwnedCapability(ctx, generated.GetOwnedCapabilityParams{Slug: slug, OwnerOpenID: owner})
	if e != nil {
		return registry.Detail{}, registryError(e)
	}
	detail, e := registryDetail(ctx, q, c)
	if e != nil {
		return detail, e
	}
	return detail, tx.Commit(ctx)
}
func (r *RegistryRepository) Save(ctx context.Context, owner, slug string, input registry.SaveRequest, v registry.Version) (result registry.Detail, err error) {
	defer func() { err = registryError(err) }()
	tx, e := r.pool.Begin(ctx)
	if e != nil {
		return result, e
	}
	defer tx.Rollback(context.Background())
	q := r.q.WithTx(tx)
	var c generated.Capability
	if slug == "" {
		if input.Revision != 0 {
			return result, registry.ErrConflict
		}
		c, e = q.CreateCapability(ctx, generated.CreateCapabilityParams{Slug: input.Slug, Type: input.Type, Name: input.Name, Description: input.Description, OwnerOpenID: owner, Department: input.Department, DepartmentID: input.DepartmentID, Visibility: input.Visibility})
	} else {
		c, e = q.LockOwnedCapability(ctx, generated.LockOwnedCapabilityParams{Slug: slug, OwnerOpenID: owner})
		if e != nil {
			return result, e
		}
		if c.Revision != input.Revision || c.Status != "draft" || c.Type != input.Type || c.Slug != input.Slug {
			return result, registry.ErrConflict
		}
		c, e = q.UpdateCapability(ctx, generated.UpdateCapabilityParams{ID: c.ID, Name: input.Name, Description: input.Description, Department: input.Department, DepartmentID: input.DepartmentID, Visibility: input.Visibility})
	}
	if e != nil {
		return result, e
	}
	versions, e := q.ListCapabilityVersions(ctx, c.ID)
	if e != nil {
		return result, e
	}
	var existing uuid.UUID
	for _, previous := range versions {
		if previous.Version == input.Version {
			if previous.PublishedAt.Valid || !c.DraftVersionID.Valid || uuid.UUID(c.DraftVersionID.Bytes) != previous.ID {
				return result, registry.ErrConflict
			}
			existing = previous.ID
		}
	}
	var tools []byte
	if input.Type == "mcp" {
		if v.Tools == nil {
			v.Tools = []registry.Tool{}
		}
		tools, e = json.Marshal(v.Tools)
		if e != nil {
			return result, e
		}
	}
	var findings []byte
	if v.BundleFindings != nil {
		if findings, e = json.Marshal(v.BundleFindings); e != nil {
			return result, e
		}
	}
	var saved generated.CapabilityVersion
	if existing == uuid.Nil {
		saved, e = q.CreateCapabilityVersion(ctx, generated.CreateCapabilityVersionParams{CapabilityID: c.ID, Version: input.Version, Changelog: v.Changelog, CreatedBy: owner, McpEndpoint: nullableText(v.MCPEndpoint), McpTransport: nullableText(v.MCPTransport), McpAuthScheme: nullableText(v.MCPAuthScheme), Tools: tools, BundleKey: nullableText(v.BundleKey), SkillManifest: v.SkillManifest, BundleSha256: nullableText(v.BundleSHA256), BundleSize: nullableSize(v.BundleSize), BundleFindings: findings})
	} else {
		saved, e = q.UpdateCapabilityVersion(ctx, generated.UpdateCapabilityVersionParams{ID: existing, Changelog: v.Changelog, McpEndpoint: nullableText(v.MCPEndpoint), McpTransport: nullableText(v.MCPTransport), McpAuthScheme: nullableText(v.MCPAuthScheme), Tools: tools, BundleKey: nullableText(v.BundleKey), SkillManifest: v.SkillManifest, BundleSha256: nullableText(v.BundleSHA256), BundleSize: nullableSize(v.BundleSize), BundleFindings: findings})
	}
	if e != nil {
		return result, e
	}
	c, e = q.SetCapabilityDraft(ctx, generated.SetCapabilityDraftParams{ID: c.ID, DraftVersionID: pgtype.UUID{Bytes: saved.ID, Valid: true}})
	if e != nil {
		return result, e
	}
	if e = q.ClearCapabilityAllowlist(ctx, c.ID); e != nil {
		return result, e
	}
	for _, id := range input.Allowlist {
		if e = q.AddCapabilityAllowlist(ctx, generated.AddCapabilityAllowlistParams{CapabilityID: c.ID, OpenID: id}); e != nil {
			return result, e
		}
	}
	result, e = registryDetail(ctx, q, c)
	if e != nil {
		return result, e
	}
	action := "edit"
	if slug == "" {
		action = "create"
	}
	if e = writeAudit(ctx, q, owner, action, "capability", c.ID.String(), map[string]any{"revision": c.Revision, "version_id": saved.ID.String(), "status": c.Status}); e != nil {
		return result, e
	}
	result.IsOwner = true
	result.IsAdmin, e = q.IsPlatformAdmin(ctx, owner)
	if e != nil {
		return result, e
	}
	if e = tx.Commit(ctx); e != nil {
		return result, errors.Join(registry.ErrCommitUnknown, e)
	}
	return result, nil
}
func (r *RegistryRepository) BundleReferenced(ctx context.Context, key string) (bool, error) {
	return r.q.RegistryBundleReferenced(ctx, &key)
}

// RecordDownload audits a bundle download after the caller's access check.
func (r *RegistryRepository) RecordDownload(ctx context.Context, user, capabilityID, versionID string) error {
	who, e := identity(ctx, r.q, user)
	if e != nil {
		return e
	}
	return writeAudit(ctx, r.q, who.OpenID, "download", "capability", capabilityID, map[string]any{"version_id": versionID})
}

// Downloaders lists audited downloads; only the owner or an admin may see who downloaded.
func (r *RegistryRepository) Downloaders(ctx context.Context, user, slug string, f registry.Filter) (registry.DownloaderPage, error) {
	who, e := identity(ctx, r.q, user)
	if e != nil {
		return registry.DownloaderPage{}, e
	}
	c, e := r.q.GetCapability(ctx, slug)
	if e != nil {
		return registry.DownloaderPage{}, registryError(e)
	}
	if !who.IsAdmin && who.OpenID != c.OwnerOpenID {
		return registry.DownloaderPage{}, registry.ErrNotFound
	}
	rows, e := r.q.ListCapabilityDownloaders(ctx, generated.ListCapabilityDownloadersParams{TargetID: c.ID.String(), PageLimit: f.Limit + 1, PageOffset: f.Offset})
	if e != nil {
		return registry.DownloaderPage{}, e
	}
	page := registry.DownloaderPage{Items: []registry.Downloader{}, HasMore: len(rows) > int(f.Limit)}
	if page.HasMore {
		rows = rows[:f.Limit]
	}
	for _, row := range rows {
		page.Items = append(page.Items, registry.Downloader{OpenID: row.ActorOpenID, DisplayName: row.DisplayName, Downloads: row.Downloads, LastDownloadedAt: row.LastDownloadedAt, LastVersionID: row.LastVersionID})
	}
	return page, nil
}

// NotifyReviewers returns everyone who may approve submissions.
func (r *RegistryRepository) NotifyReviewers(ctx context.Context) ([]string, error) {
	return r.q.ListNotifyReviewers(ctx)
}

// NotifyDownloaders returns past downloaders with their platform user IDs.
func (r *RegistryRepository) NotifyDownloaders(ctx context.Context, capabilityID string) ([]registry.Account, error) {
	rows, e := r.q.ListDownloaderAccounts(ctx, capabilityID)
	if e != nil {
		return nil, e
	}
	accounts := make([]registry.Account, 0, len(rows))
	for _, row := range rows {
		accounts = append(accounts, registry.Account{UserID: row.UserID.String(), OpenID: row.ActorOpenID})
	}
	return accounts, nil
}

// CanSeePublished applies the consumer projection used by the market.
func (r *RegistryRepository) CanSeePublished(ctx context.Context, userID, slug string) (bool, error) {
	_, e := r.Read(ctx, userID, slug, true)
	if errors.Is(e, registry.ErrNotFound) || errors.Is(e, registry.ErrIdentity) {
		return false, nil
	}
	return e == nil, e
}
