package directory

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"testing"
	"time"

	"github.com/zenith-wang/it-wiki/backend/internal/domain"
)

type fixture struct {
	account              domain.OAuthAccount
	membership           Membership
	self                 User
	fail                 error
	tokens, users, saves int
	parents              map[string][]Page[Department]
	parentErr            map[string]error
	parentCalls          int
}

func (f *fixture) FeishuAccount(context.Context, string) (domain.OAuthAccount, error) {
	return f.account, nil
}
func (f *fixture) AccessToken(context.Context, string) (string, error) {
	f.tokens++
	return "user-token", nil
}
func (f *fixture) Membership(context.Context, string) (Membership, error) { return f.membership, nil }
func (f *fixture) SaveMembership(_ context.Context, _ string, ids []string, expires time.Time) error {
	f.saves++
	f.membership = Membership{DepartmentIDs: ids, ExpiresAt: expires}
	return nil
}
func (f *fixture) Departments(context.Context, string, string, string) (Page[Department], error) {
	return Page[Department]{Items: []Department{{ID: "od-visible", Name: "Visible"}}}, f.fail
}
func (f *fixture) Users(context.Context, string, string, string) (Page[User], error) {
	return Page[User]{Items: []User{{OpenID: "ou-other", Name: "Other", DepartmentIDs: []string{"od-visible"}}}}, f.fail
}
func (f *fixture) Department(context.Context, string, string) (Department, error) {
	return Department{ID: "od-visible", Name: "Visible"}, f.fail
}
func (f *fixture) ParentDepartments(_ context.Context, _, id, page string) (Page[Department], error) {
	f.parentCalls++
	if e := f.parentErr[id]; e != nil {
		return Page[Department]{}, e
	}
	pages := f.parents[id]
	if len(pages) == 0 {
		return Page[Department]{Items: []Department{}}, nil
	}
	if page == "" {
		return pages[0], nil
	}
	return pages[1], nil
}
func (f *fixture) User(_ context.Context, _ string, id string) (User, error) {
	f.users++
	return f.self, f.fail
}
func newFixture() (*Service, *fixture) {
	f := &fixture{account: domain.OAuthAccount{ID: "account", ProviderUserID: "ou-self", Scopes: append([]string{}, RequiredScopes...)}, self: User{OpenID: "ou-self", Name: "Self", DepartmentIDs: []string{"od-actual"}}}
	return New(f, f, f, f), f
}
func TestVisibleDirectoryDoesNotGrantMembership(t *testing.T) {
	ctx := context.Background()
	s, f := newFixture()
	if _, e := s.Departments(ctx, "user", "0", ""); e != nil {
		t.Fatal(e)
	}
	if _, e := s.Users(ctx, "user", "od-visible", ""); e != nil {
		t.Fatal(e)
	}
	if f.saves != 0 {
		t.Fatal("browsing persisted membership")
	}
	if e := s.EnsureMembership(ctx, "user"); e != nil {
		t.Fatal(e)
	}
	if len(f.membership.DepartmentIDs) != 1 || f.membership.DepartmentIDs[0] != "od-actual" {
		t.Fatal("used visibility as membership")
	}
	if e := s.EnsureMembership(ctx, "user"); e != nil || f.users != 1 {
		t.Fatal("fresh membership should be reused", e)
	}
	f.membership.ExpiresAt = time.Now().Add(-time.Second)
	f.fail = ErrAuthorization
	if e := s.EnsureMembership(ctx, "user"); !errors.Is(e, ErrAuthorization) {
		t.Fatal(e)
	}
	if len(f.membership.DepartmentIDs) != 0 || f.membership.ExpiresAt.After(time.Now()) {
		t.Fatal("failed refresh retained access")
	}
}
func TestMembershipIncludesAncestorsOfOwnDepartments(t *testing.T) {
	s, f := newFixture()
	f.self.DepartmentIDs = []string{"od-actual", "od-second"}
	f.parents = map[string][]Page[Department]{
		"od-actual": {
			{Items: []Department{{ID: "od-team"}, {ID: "0"}, {ID: "../bad"}}, HasMore: true, PageToken: "more"},
			{Items: []Department{{ID: "od-division"}}},
		},
		"od-second": {{Items: []Department{{ID: "od-division"}, {ID: "od-second"}}}},
	}
	if e := s.EnsureMembership(context.Background(), "user"); e != nil {
		t.Fatal(e)
	}
	if !slices.Equal(f.membership.DepartmentIDs, []string{"od-actual", "od-second", "od-team", "od-division"}) {
		t.Fatal("wrong membership", f.membership.DepartmentIDs)
	}
	if ttl := time.Until(f.membership.ExpiresAt); ttl <= membershipRetryTTL || ttl > membershipTTL {
		t.Fatal("complete lookup should use the normal TTL", ttl)
	}
}
func TestAncestorFailureKeepsOnlyVerifiedDepartments(t *testing.T) {
	for _, tc := range []struct {
		name string
		err  error
		long bool
	}{{"hidden by policy", ErrAuthorization, true}, {"not found", ErrInvalid, true}, {"unavailable", ErrUnavailable, false}, {"timeout", context.DeadlineExceeded, false}} {
		t.Run(tc.name, func(t *testing.T) {
			s, f := newFixture()
			f.parentErr = map[string]error{"od-actual": tc.err}
			if e := s.EnsureMembership(context.Background(), "user"); e != nil {
				t.Fatal(e)
			}
			if !slices.Equal(f.membership.DepartmentIDs, []string{"od-actual"}) {
				t.Fatal("direct department lost or ancestor invented", f.membership.DepartmentIDs)
			}
			if long := time.Until(f.membership.ExpiresAt) > membershipRetryTTL; long != tc.long {
				t.Fatal("wrong retry behavior", f.membership.ExpiresAt)
			}
		})
	}
}
func TestStaleAncestorsAreReplacedOnRefresh(t *testing.T) {
	s, f := newFixture()
	f.parents = map[string][]Page[Department]{"od-actual": {{Items: []Department{{ID: "od-old-parent"}}}}}
	ctx := context.Background()
	if e := s.EnsureMembership(ctx, "user"); e != nil {
		t.Fatal(e)
	}
	// The department moved in Feishu: the old parent stops granting access.
	f.membership.ExpiresAt = time.Now().Add(-time.Second)
	f.parents = map[string][]Page[Department]{"od-actual": {{Items: []Department{{ID: "od-new-parent"}}}}}
	if e := s.EnsureMembership(ctx, "user"); e != nil {
		t.Fatal(e)
	}
	if !slices.Equal(f.membership.DepartmentIDs, []string{"od-actual", "od-new-parent"}) {
		t.Fatal("stale ancestor kept", f.membership.DepartmentIDs)
	}
}
func TestAncestorLookupIsBounded(t *testing.T) {
	s, f := newFixture()
	many := make([]Department, maxMembershipIDs+50)
	for i := range many {
		many[i].ID = fmt.Sprintf("od-%d", i)
	}
	f.parents = map[string][]Page[Department]{"od-actual": {{Items: many}}}
	if e := s.EnsureMembership(context.Background(), "user"); e != nil {
		t.Fatal(e)
	}
	if len(f.membership.DepartmentIDs) != maxMembershipIDs {
		t.Fatal("membership not capped", len(f.membership.DepartmentIDs))
	}
	// A provider that never stops paginating cannot hold the request.
	s, f = newFixture()
	f.parents = map[string][]Page[Department]{"od-actual": {
		{Items: []Department{{ID: "od-a"}}, HasMore: true, PageToken: "more"},
		{Items: []Department{{ID: "od-b"}}, HasMore: true, PageToken: "more2"},
	}}
	if e := s.EnsureMembership(context.Background(), "user"); e != nil {
		t.Fatal(e)
	}
	if f.parentCalls != maxAncestorPages {
		t.Fatal("pagination not bounded", f.parentCalls)
	}
}
func TestBrowsingAndSelectionDoNotQueryAncestors(t *testing.T) {
	s, f := newFixture()
	ctx := context.Background()
	_, _ = s.Departments(ctx, "user", "0", "")
	_, _ = s.ValidateSelection(ctx, "user", "od-visible", nil)
	if f.parentCalls != 0 || f.saves != 0 {
		t.Fatal("browsing touched membership", f.parentCalls, f.saves)
	}
}
func TestMembershipRequiresExactSubjectAndDepartmentField(t *testing.T) {
	for _, self := range []User{{OpenID: "ou-other", DepartmentIDs: []string{"od-actual"}}, {OpenID: "ou-self"}} {
		s, f := newFixture()
		f.self = self
		if e := s.EnsureMembership(context.Background(), "user"); !errors.Is(e, ErrAuthorization) {
			t.Fatal("accepted mismatched identity or missing field", e)
		}
		if len(f.membership.DepartmentIDs) != 0 {
			t.Fatal("invalid membership persisted")
		}
	}
}
func TestDirectoryValidatesScopeAndSelection(t *testing.T) {
	s, f := newFixture()
	ctx := context.Background()
	if _, e := s.Departments(ctx, "user", "../secret", ""); !errors.Is(e, ErrInvalid) || f.tokens != 0 {
		t.Fatal("invalid path reached provider", e)
	}
	if name, e := s.ValidateSelection(ctx, "user", "od-visible", nil); e != nil || name != "Visible" {
		t.Fatal(name, e)
	}
	if _, e := s.ValidateSelection(ctx, "user", "od-unknown", nil); !errors.Is(e, ErrInvalid) {
		t.Fatal("mismatched department accepted", e)
	}
	f.account.Scopes = nil
	if _, e := s.Users(ctx, "user", "0", ""); !errors.Is(e, ErrAuthorization) {
		t.Fatal("missing scopes accepted", e)
	}
}

type restrictedDirectory struct{ *fixture }

func (f *restrictedDirectory) Departments(context.Context, string, string, string) (Page[Department], error) {
	return Page[Department]{}, ErrAuthorization
}
func (f *restrictedDirectory) Users(context.Context, string, string, string) (Page[User], error) {
	return Page[User]{}, ErrAuthorization
}

func TestRestrictedRootOffersOnlyVerifiedOwnDepartments(t *testing.T) {
	s, f := newFixture()
	f.self.DepartmentIDs = []string{"od-visible", "od-visible"}
	s.client = &restrictedDirectory{f}
	ctx := context.Background()
	page, e := s.Departments(ctx, "user", "0", "")
	if e != nil || !page.LimitedRoot || page.HasMore || len(page.Items) != 1 || page.Items[0].ID != "od-visible" {
		t.Fatal("missing restricted-root entry point", page, e)
	}
	if f.saves != 0 {
		t.Fatal("directory fallback must not grant membership")
	}
	users, e := s.Users(ctx, "user", "0", "")
	if e != nil || !users.LimitedRoot || users.Items == nil || len(users.Items) != 0 {
		t.Fatal("root users must direct users into a visible department", users, e)
	}
	if _, e = s.Users(ctx, "user", "od-hidden", ""); !errors.Is(e, ErrAuthorization) {
		t.Fatal("non-root denial was hidden", e)
	}
	if _, e = s.Departments(ctx, "user", "od-hidden", ""); !errors.Is(e, ErrAuthorization) {
		t.Fatal("non-root request used a fallback", e)
	}
	if _, e = s.Departments(ctx, "user", "0", "next"); !errors.Is(e, ErrAuthorization) {
		t.Fatal("pagination failure was hidden", e)
	}
	f.fail = ErrAuthorization
	if _, e = s.Departments(ctx, "user", "0", ""); !errors.Is(e, ErrAuthorization) {
		t.Fatal("failed user lookup accepted", e)
	}
	f.fail = nil
	f.self.OpenID = "ou-someone-else"
	if _, e = s.Departments(ctx, "user", "0", ""); !errors.Is(e, ErrAuthorization) {
		t.Fatal("fallback accepted another subject", e)
	}
}
