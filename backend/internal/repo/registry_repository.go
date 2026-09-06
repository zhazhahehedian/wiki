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

type RegistryRepository struct {
	pool *pgxpool.Pool
	q    *generated.Queries
}

func NewRegistryRepository(pool *pgxpool.Pool) *RegistryRepository {
	return &RegistryRepository{pool, generated.New(pool)}
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
		result.CurrentVersionID = uuid.UUID(c.CurrentVersionID.Bytes).String()
	}
	if c.DraftVersionID.Valid {
		result.DraftVersionID = uuid.UUID(c.DraftVersionID.Bytes).String()
	}
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
func registryDetail(ctx context.Context, q *generated.Queries, c generated.Capability) (registry.Detail, error) {
	result := registry.Detail{Capability: capability(c), Versions: []registry.Version{}}
	rows, e := q.ListCapabilityVersions(ctx, c.ID)
	if e != nil {
		return result, e
	}
	for _, v := range rows {
		ver := registry.Version{ID: v.ID.String(), Version: v.Version, Changelog: v.Changelog, CreatedBy: v.CreatedBy, CreatedAt: v.CreatedAt, MCPEndpoint: textValue(v.McpEndpoint), MCPTransport: textValue(v.McpTransport), MCPAuthScheme: textValue(v.McpAuthScheme), BundleKey: textValue(v.SkillBundleKey), HasBundle: v.SkillBundleKey != nil, SkillManifest: json.RawMessage(v.SkillManifest)}
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
		c, e = q.CreateCapability(ctx, generated.CreateCapabilityParams{Slug: input.Slug, Type: input.Type, Name: input.Name, Description: input.Description, OwnerOpenID: owner, Department: input.Department, Visibility: input.Visibility})
	} else {
		c, e = q.LockOwnedCapability(ctx, generated.LockOwnedCapabilityParams{Slug: slug, OwnerOpenID: owner})
		if e != nil {
			return result, e
		}
		if c.Revision != input.Revision || c.Status != "draft" || c.Type != input.Type || c.Slug != input.Slug {
			return result, registry.ErrConflict
		}
		c, e = q.UpdateCapability(ctx, generated.UpdateCapabilityParams{ID: c.ID, Name: input.Name, Description: input.Description, Department: input.Department, Visibility: input.Visibility})
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
	var saved generated.CapabilityVersion
	if existing == uuid.Nil {
		saved, e = q.CreateCapabilityVersion(ctx, generated.CreateCapabilityVersionParams{CapabilityID: c.ID, Version: input.Version, Changelog: v.Changelog, CreatedBy: owner, McpEndpoint: nullableText(v.MCPEndpoint), McpTransport: nullableText(v.MCPTransport), McpAuthScheme: nullableText(v.MCPAuthScheme), Tools: tools, SkillBundleKey: nullableText(v.BundleKey), SkillManifest: v.SkillManifest})
	} else {
		saved, e = q.UpdateCapabilityVersion(ctx, generated.UpdateCapabilityVersionParams{ID: existing, Changelog: v.Changelog, McpEndpoint: nullableText(v.MCPEndpoint), McpTransport: nullableText(v.MCPTransport), McpAuthScheme: nullableText(v.MCPAuthScheme), Tools: tools, SkillBundleKey: nullableText(v.BundleKey), SkillManifest: v.SkillManifest})
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
