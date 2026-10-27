package directory

import (
	"context"
	"errors"
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
