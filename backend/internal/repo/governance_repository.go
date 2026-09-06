package repo

import (
	"context"
	"encoding/json"
	"errors"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/zenith-wang/it-wiki/backend/internal/registry"
	"github.com/zenith-wang/it-wiki/backend/internal/repo/generated"
)

func identity(ctx context.Context, q *generated.Queries, user string) (registry.Identity, error) {
	id, e := uuid.Parse(user)
	if e != nil {
		return registry.Identity{}, registry.ErrIdentity
	}
	row, e := q.GovernanceIdentity(ctx, id)
	if errors.Is(e, pgx.ErrNoRows) {
		return registry.Identity{}, registry.ErrIdentity
	}
	if e != nil {
		return registry.Identity{}, e
	}
	return registry.Identity{OpenID: row.OpenID, IsAdmin: row.IsAdmin, Department: row.Department, Revision: row.Revision}, nil
}
func (r *RegistryRepository) Identity(ctx context.Context, user string) (registry.Identity, error) {
	return identity(ctx, r.q, user)
}
func (r *RegistryRepository) BootstrapAdmin(ctx context.Context, openID string) error {
	openID = strings.TrimSpace(openID)
	if openID == "" {
		return nil
	}
	if len(openID) > 128 || strings.ContainsAny(openID, " \t\r\n") {
		return registry.ErrInvalid
	}
	tx, e := r.pool.Begin(ctx)
	if e != nil {
		return e
	}
	defer tx.Rollback(context.Background())
	q := r.q.WithTx(tx)
	if e = q.LockAdminBootstrap(ctx); e != nil {
		return e
	}
	if e = q.BootstrapPlatformAdmin(ctx, openID); e != nil {
		return e
	}
	return tx.Commit(ctx)
}
func writeAudit(ctx context.Context, q *generated.Queries, actor, action, targetType, target string, detail map[string]any) error {
	raw, e := json.Marshal(detail)
	if e != nil {
		return e
	}
	return q.InsertGovernanceAudit(ctx, generated.InsertGovernanceAuditParams{ActorOpenID: actor, Action: action, TargetType: targetType, TargetID: target, Detail: raw, RequestID: registry.RequestID(ctx)})
}
func publishedMetadata(c generated.Capability) (registry.PublishedMetadata, error) {
	var m registry.PublishedMetadata
	if len(c.PublishedMetadata) == 0 {
		return m, registry.ErrNotFound
	}
	e := json.Unmarshal(c.PublishedMetadata, &m)
	return m, e
}
func publishedCapability(c generated.Capability) (registry.Capability, error) {
	m, e := publishedMetadata(c)
	if e != nil {
		return registry.Capability{}, e
	}
	out := capability(c)
	out.Name = m.Name
	out.Description = m.Description
	out.Department = m.Department
	out.Visibility = m.Visibility
	out.Status = "published"
	out.DraftVersionID = ""
	out.ReviewReason = ""
	out.Revision = 0
	return out, nil
}
func (r *RegistryRepository) Read(ctx context.Context, user, slug string, publicOnly bool) (registry.Detail, error) {
	tx, e := r.pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.RepeatableRead, AccessMode: pgx.ReadOnly})
	if e != nil {
		return registry.Detail{}, e
	}
	defer tx.Rollback(context.Background())
	q := r.q.WithTx(tx)
	who, e := identity(ctx, q, user)
	if e != nil {
		return registry.Detail{}, e
	}
	c, e := q.GetCapability(ctx, slug)
	if e != nil {
		return registry.Detail{}, registryError(e)
	}
	manage := !publicOnly && (who.IsAdmin || who.OpenID == c.OwnerOpenID)
	if !manage {
		m, e := publishedMetadata(c)
		if e != nil {
			return registry.Detail{}, e
		}
		if !registry.CanViewPublished(who, c.OwnerOpenID, c.IsLive, m) {
			return registry.Detail{}, registry.ErrNotFound
		}
	}
	d, e := registryDetail(ctx, q, c)
	if e != nil {
		return registry.Detail{}, e
	}
	if manage {
		d.IsOwner = who.OpenID == c.OwnerOpenID
		d.IsAdmin = who.IsAdmin
	} else {
		d.Capability, e = publishedCapability(c)
		if e != nil {
			return registry.Detail{}, e
		}
		versions := []registry.Version{}
		for _, v := range d.Versions {
			if v.ID == d.CurrentVersionID {
				versions = append(versions, v)
			}
		}
		d.Versions = versions
		d.Allowlist = nil
		d.Published = nil
	}
	return d, tx.Commit(ctx)
}
func (r *RegistryRepository) Catalog(ctx context.Context, user string, f registry.Filter) (registry.Page, error) {
	who, e := r.Identity(ctx, user)
	if e != nil {
		return registry.Page{}, e
	}
	rows, e := r.q.ListCatalogCapabilities(ctx, generated.ListCatalogCapabilitiesParams{IsAdmin: who.IsAdmin, OpenID: who.OpenID, Department: who.Department, FilterType: f.Type, FilterStatus: f.Status, FilterDepartment: f.Department, Search: f.Search, PageLimit: f.Limit + 1, PageOffset: f.Offset})
	if e != nil {
		return registry.Page{}, e
	}
	page := registry.Page{Items: []registry.Capability{}, HasMore: len(rows) > int(f.Limit)}
	if page.HasMore {
		rows = rows[:f.Limit]
	}
	for _, c := range rows {
		v, e := publishedCapability(c)
		if e != nil {
			return page, e
		}
		page.Items = append(page.Items, v)
	}
	return page, nil
}
func (r *RegistryRepository) Reviews(ctx context.Context, user string, f registry.Filter) (registry.Page, error) {
	who, e := r.Identity(ctx, user)
	if e != nil {
		return registry.Page{}, e
	}
	if !who.IsAdmin {
		return registry.Page{}, registry.ErrForbidden
	}
	rows, e := r.q.ListReviewCapabilities(ctx, generated.ListReviewCapabilitiesParams{FilterType: f.Type, FilterDepartment: f.Department, Search: f.Search, PageLimit: f.Limit + 1, PageOffset: f.Offset})
	if e != nil {
		return registry.Page{}, e
	}
	page := registry.Page{Items: []registry.Capability{}, HasMore: len(rows) > int(f.Limit)}
	if page.HasMore {
		rows = rows[:f.Limit]
	}
	for _, c := range rows {
		page.Items = append(page.Items, capability(c))
	}
	return page, nil
}
func versionID(id pgtype.UUID) string {
	if !id.Valid {
		return ""
	}
	return uuid.UUID(id.Bytes).String()
}
func (r *RegistryRepository) Act(ctx context.Context, user, slug, action string, input registry.ActionRequest) (result registry.Detail, err error) {
	defer func() { err = registryError(err) }()
	tx, e := r.pool.Begin(ctx)
	if e != nil {
		return result, e
	}
	defer tx.Rollback(context.Background())
	q := r.q.WithTx(tx)
	who, e := identity(ctx, q, user)
	if e != nil {
		return result, e
	}
	c, e := q.LockCapability(ctx, slug)
	if e != nil {
		return result, e
	}
	owner := who.OpenID == c.OwnerOpenID
	if !who.IsAdmin && !owner {
		return result, registry.ErrNotFound
	}
	switch action {
	case "approve", "reject", "force-publish", "visibility":
		if !who.IsAdmin {
			return result, registry.ErrForbidden
		}
	case "submit", "new-draft":
		if !owner {
			return result, registry.ErrForbidden
		}
	case "offline":
	default:
		return result, registry.ErrInvalid
	}
	if c.Revision != input.Revision {
		return result, registry.ErrConflict
	}
	before := map[string]any{"status": c.Status, "is_live": c.IsLive, "current_version_id": versionID(c.CurrentVersionID), "draft_version_id": versionID(c.DraftVersionID), "revision": c.Revision}
	candidate := c.DraftVersionID
	if !candidate.Valid && c.Status == "offline" {
		candidate = c.CurrentVersionID
	}
	expected := candidate
	if action == "new-draft" || action == "offline" || action == "visibility" {
		expected = c.CurrentVersionID
	}
	if versionID(expected) != input.VersionID {
		return result, registry.ErrConflict
	}
	switch action {
	case "new-draft":
		if c.Status != "published" && c.Status != "offline" {
			return result, registry.ErrConflict
		}
		c.Status = "draft"
		c.DraftVersionID = pgtype.UUID{}
		c.ReviewReason = ""
	case "submit":
		if (c.Status != "draft" && c.Status != "offline") || !candidate.Valid {
			return result, registry.ErrConflict
		}
		c.Status = "in_review"
		c.DraftVersionID = candidate
		c.ReviewReason = ""
	case "approve", "force-publish":
		if !candidate.Valid || (action == "approve" && c.Status != "in_review") || (action == "force-publish" && c.Status != "draft" && c.Status != "in_review" && c.Status != "offline") {
			return result, registry.ErrConflict
		}
		allowlist, e := q.ListCapabilityAllowlist(ctx, c.ID)
		if e != nil {
			return result, e
		}
		c.PublishedMetadata, e = json.Marshal(registry.PublishedMetadata{Name: c.Name, Description: c.Description, Department: c.Department, Visibility: c.Visibility, Allowlist: allowlist})
		if e != nil {
			return result, e
		}
		c.Status = "published"
		c.IsLive = true
		c.CurrentVersionID = candidate
		c.DraftVersionID = pgtype.UUID{}
		c.ReviewReason = ""
		if e = q.MarkVersionPublished(ctx, uuid.UUID(candidate.Bytes)); e != nil {
			return result, e
		}
	case "reject":
		if c.Status != "in_review" {
			return result, registry.ErrConflict
		}
		c.Status = "draft"
		c.ReviewReason = input.Reason
	case "offline":
		if !c.IsLive {
			return result, registry.ErrConflict
		}
		c.Status = "offline"
		c.IsLive = false
		c.ReviewReason = input.Reason
	case "visibility":
		if !c.IsLive || c.Status == "in_review" {
			return result, registry.ErrConflict
		}
		meta, e := publishedMetadata(c)
		if e != nil {
			return result, e
		}
		before["visibility"] = meta.Visibility
		before["department"] = meta.Department
		meta.Visibility = input.Visibility
		meta.Department = input.Department
		meta.Allowlist = input.Allowlist
		c.PublishedMetadata, e = json.Marshal(meta)
		if e != nil {
			return result, e
		}
		// With no distinct draft, keep the next offline resubmission aligned
		// with the admin override. A separate candidate remains a review proposal.
		if !c.DraftVersionID.Valid || c.DraftVersionID == c.CurrentVersionID {
			updated, e := q.SetCapabilityVisibility(ctx, generated.SetCapabilityVisibilityParams{ID: c.ID, Visibility: input.Visibility, Department: input.Department})
			if e != nil {
				return result, e
			}
			c.Visibility = updated.Visibility
			c.Department = updated.Department
			if e = q.ClearCapabilityAllowlist(ctx, c.ID); e != nil {
				return result, e
			}
			for _, id := range input.Allowlist {
				if e = q.AddCapabilityAllowlist(ctx, generated.AddCapabilityAllowlistParams{CapabilityID: c.ID, OpenID: id}); e != nil {
					return result, e
				}
			}
		}
	}
	c, e = q.SetCapabilityGovernance(ctx, generated.SetCapabilityGovernanceParams{ID: c.ID, Status: c.Status, IsLive: c.IsLive, CurrentVersionID: c.CurrentVersionID, DraftVersionID: c.DraftVersionID, PublishedMetadata: c.PublishedMetadata, ReviewReason: c.ReviewReason})
	if e != nil {
		return result, e
	}
	detail := map[string]any{"before": before, "status": c.Status, "is_live": c.IsLive, "revision": c.Revision, "version_id": input.VersionID, "reason": input.Reason}
	if action == "visibility" {
		detail["visibility"] = input.Visibility
		detail["department"] = input.Department
		detail["allowlist_count"] = len(input.Allowlist)
	}
	if e = writeAudit(ctx, q, who.OpenID, action, "capability", c.ID.String(), detail); e != nil {
		return result, e
	}
	result, e = registryDetail(ctx, q, c)
	if e != nil {
		return result, e
	}
	result.IsOwner = owner
	result.IsAdmin = who.IsAdmin
	if e = tx.Commit(ctx); e != nil {
		return result, errors.Join(registry.ErrCommitUnknown, e)
	}
	return result, nil
}
func (r *RegistryRepository) Audit(ctx context.Context, user string, f registry.AuditFilter) (registry.AuditPage, error) {
	who, e := r.Identity(ctx, user)
	if e != nil {
		return registry.AuditPage{}, e
	}
	if !who.IsAdmin {
		return registry.AuditPage{}, registry.ErrForbidden
	}
	rows, e := r.q.ListGovernanceAudit(ctx, generated.ListGovernanceAuditParams{Action: f.Action, Actor: f.Actor, Target: f.Target, PageLimit: f.Limit + 1, PageOffset: f.Offset})
	if e != nil {
		return registry.AuditPage{}, e
	}
	page := registry.AuditPage{Items: []registry.AuditEntry{}, HasMore: len(rows) > int(f.Limit)}
	if page.HasMore {
		rows = rows[:f.Limit]
	}
	for _, v := range rows {
		entry := registry.AuditEntry{ID: v.ID.String(), ActorOpenID: v.ActorOpenID, Action: v.Action, TargetType: v.TargetType, TargetID: v.TargetID, RequestID: v.RequestID, CreatedAt: v.CreatedAt}
		if e = json.Unmarshal(v.Detail, &entry.Detail); e != nil {
			return page, e
		}
		page.Items = append(page.Items, entry)
	}
	return page, nil
}
func (r *RegistryRepository) Profiles(ctx context.Context, user string, f registry.Filter) (registry.ProfilePage, error) {
	who, e := r.Identity(ctx, user)
	if e != nil {
		return registry.ProfilePage{}, e
	}
	if !who.IsAdmin {
		return registry.ProfilePage{}, registry.ErrForbidden
	}
	rows, e := r.q.ListPlatformProfiles(ctx, generated.ListPlatformProfilesParams{Search: f.Search, PageLimit: f.Limit + 1, PageOffset: f.Offset})
	if e != nil {
		return registry.ProfilePage{}, e
	}
	page := registry.ProfilePage{Items: []registry.Identity{}, HasMore: len(rows) > int(f.Limit)}
	if page.HasMore {
		rows = rows[:f.Limit]
	}
	for _, v := range rows {
		page.Items = append(page.Items, registry.Identity{OpenID: v.OpenID, DisplayName: v.DisplayName, IsAdmin: v.IsAdmin, Department: v.Department, Revision: v.Revision})
	}
	return page, nil
}
func (r *RegistryRepository) SetDepartment(ctx context.Context, user, openID string, input registry.DepartmentRequest) (result registry.Identity, err error) {
	defer func() { err = registryError(err) }()
	tx, e := r.pool.Begin(ctx)
	if e != nil {
		return result, e
	}
	defer tx.Rollback(context.Background())
	q := r.q.WithTx(tx)
	who, e := identity(ctx, q, user)
	if e != nil {
		return result, e
	}
	if !who.IsAdmin {
		return result, registry.ErrForbidden
	}
	row, e := q.SetTrustedDepartment(ctx, generated.SetTrustedDepartmentParams{OpenID: openID, Department: input.Department, Revision: input.Revision})
	if errors.Is(e, pgx.ErrNoRows) {
		return result, registry.ErrConflict
	}
	if e != nil {
		return result, e
	}
	if e = writeAudit(ctx, q, who.OpenID, "set_department", "profile", openID, map[string]any{"department": input.Department, "revision": row.Revision, "reason": input.Reason}); e != nil {
		return result, e
	}
	result = registry.Identity{OpenID: row.OpenID, IsAdmin: row.IsAdmin, Department: row.Department, Revision: row.Revision}
	return result, tx.Commit(ctx)
}
