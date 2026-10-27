package directory

import (
	"context"
	"errors"
	"regexp"
	"slices"
	"time"

	"github.com/zenith-wang/it-wiki/backend/internal/domain"
)

var (
	ErrInvalid       = errors.New("invalid directory selection")
	ErrAuthorization = errors.New("directory authorization required")
	ErrUnavailable   = errors.New("directory unavailable")
)

var RequiredScopes = []string{"contact:contact.base:readonly", "contact:department.base:readonly", "contact:user.base:readonly", "contact:user.department:readonly"}
var idPattern = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9_@.\-]{0,127}$`)

func ValidID(id string) bool { return idPattern.MatchString(id) }

type Department struct {
	ID       string `json:"open_department_id"`
	Name     string `json:"name"`
	ParentID string `json:"parent_department_id,omitempty"`
}
type User struct {
	OpenID        string   `json:"open_id"`
	Name          string   `json:"name"`
	DepartmentIDs []string `json:"department_ids,omitempty"`
}
type Page[T any] struct {
	Items       []T    `json:"items"`
	HasMore     bool   `json:"has_more"`
	PageToken   string `json:"page_token"`
	LimitedRoot bool   `json:"limited_root,omitempty"`
}
type Client interface {
	Departments(context.Context, string, string, string) (Page[Department], error)
	Users(context.Context, string, string, string) (Page[User], error)
	Department(context.Context, string, string) (Department, error)
	User(context.Context, string, string) (User, error)
}
type Accounts interface {
	FeishuAccount(context.Context, string) (domain.OAuthAccount, error)
}
type Tokens interface {
	AccessToken(context.Context, string) (string, error)
}
type Membership struct {
	DepartmentIDs []string
	ExpiresAt     time.Time
}
type Repository interface {
	Membership(context.Context, string) (Membership, error)
	SaveMembership(context.Context, string, []string, time.Time) error
}
type Service struct {
	accounts Accounts
	tokens   Tokens
	client   Client
	repo     Repository
	now      func() time.Time
}

func New(accounts Accounts, tokens Tokens, client Client, repo Repository) *Service {
	return &Service{accounts: accounts, tokens: tokens, client: client, repo: repo, now: time.Now}
}
func (s *Service) token(ctx context.Context, user string) (domain.OAuthAccount, string, error) {
	a, e := s.accounts.FeishuAccount(ctx, user)
	if e != nil {
		return a, "", ErrAuthorization
	}
	token, e := s.tokens.AccessToken(ctx, a.ID)
	if e != nil {
		return a, "", ErrAuthorization
	}
	// Reload after a possible refresh; scope reduction must take effect.
	a, e = s.accounts.FeishuAccount(ctx, user)
	if e != nil || a.ReauthRequired {
		return a, "", ErrAuthorization
	}
	for _, scope := range RequiredScopes {
		if !slices.Contains(a.Scopes, scope) {
			return a, "", ErrAuthorization
		}
	}
	return a, token, nil
}
func (s *Service) Departments(ctx context.Context, user, parent, page string) (Page[Department], error) {
	if !ValidID(parent) || len(page) > 4096 {
		return Page[Department]{}, ErrInvalid
	}
	a, token, e := s.token(ctx, user)
	if e != nil {
		return Page[Department]{}, e
	}
	out, e := s.client.Departments(ctx, token, parent, page)
	if parent != "0" || page != "" || !errors.Is(e, ErrAuthorization) {
		return out, e
	}
	// Some tenants hide the organizational root even from users who can read
	// their own departments. Offer verified entry points without application
	// tokens, enumerating unrelated departments, or persisting membership.
	self, e := s.client.User(ctx, token, a.ProviderUserID)
	if e != nil {
		return Page[Department]{}, e
	}
	if self.OpenID != a.ProviderUserID || self.DepartmentIDs == nil || len(self.DepartmentIDs) > 50 {
		return Page[Department]{}, ErrAuthorization
	}
	out = Page[Department]{Items: []Department{}, LimitedRoot: true}
	seen := map[string]bool{}
	for _, id := range self.DepartmentIDs {
		if !ValidID(id) || id == "0" || seen[id] {
			continue
		}
		seen[id] = true
		d, err := s.client.Department(ctx, token, id)
		if errors.Is(err, ErrAuthorization) || errors.Is(err, ErrInvalid) {
			continue
		}
		if err != nil {
			return Page[Department]{}, err
		}
		if d.ID != id || d.Name == "" {
			return Page[Department]{}, ErrUnavailable
		}
		out.Items = append(out.Items, d)
	}
	if len(self.DepartmentIDs) > 0 && len(out.Items) == 0 {
		return Page[Department]{}, ErrAuthorization
	}
	return out, nil
}
func (s *Service) Users(ctx context.Context, user, department, page string) (Page[User], error) {
	if !ValidID(department) || len(page) > 4096 {
		return Page[User]{}, ErrInvalid
	}
	_, token, e := s.token(ctx, user)
	if e != nil {
		return Page[User]{}, e
	}
	out, e := s.client.Users(ctx, token, department, page)
	if department == "0" && page == "" && errors.Is(e, ErrAuthorization) {
		// The departments endpoint supplies accessible entry points. Do not
		// turn denied non-root requests into empty successful responses.
		return Page[User]{Items: []User{}, LimitedRoot: true}, nil
	}
	return out, e
}

// ValidateSelection resolves names on the server and verifies each ID with the
// acting user's token. Seeing a department does not grant membership in it.
func (s *Service) ValidateSelection(ctx context.Context, user, department string, users []string) (string, error) {
	if department == "" && len(users) == 0 {
		return "", nil
	}
	if len(users) > 200 || (department != "" && (!ValidID(department) || department == "0")) {
		return "", ErrInvalid
	}
	for _, id := range users {
		if !ValidID(id) {
			return "", ErrInvalid
		}
	}
	_, token, e := s.token(ctx, user)
	if e != nil {
		return "", e
	}
	name := ""
	if department != "" {
		d, e := s.client.Department(ctx, token, department)
		if e != nil {
			return "", e
		}
		if d.ID != department || d.Name == "" {
			return "", ErrInvalid
		}
		name = d.Name
	}
	for _, id := range users {
		u, e := s.client.User(ctx, token, id)
		if e != nil {
			return "", e
		}
		if u.OpenID != id {
			return "", ErrInvalid
		}
	}
	return name, nil
}

// EnsureMembership never imports membership from a list of visible departments.
// A failed refresh replaces expired data with an empty, expired record.
func (s *Service) EnsureMembership(ctx context.Context, user string) error {
	a, e := s.accounts.FeishuAccount(ctx, user)
	if e != nil {
		return ErrAuthorization
	}
	m, e := s.repo.Membership(ctx, a.ID)
	hasScopes := true
	for _, scope := range RequiredScopes {
		if !slices.Contains(a.Scopes, scope) {
			hasScopes = false
			break
		}
	}
	if e == nil && s.now().Before(m.ExpiresAt) && !a.ReauthRequired && hasScopes {
		return nil
	}
	_, token, e := s.token(ctx, user)
	if e == nil {
		var u User
		u, e = s.client.User(ctx, token, a.ProviderUserID)
		if e == nil && (u.OpenID != a.ProviderUserID || u.DepartmentIDs == nil) {
			e = ErrAuthorization
		}
		if e == nil {
			ids := []string{}
			for _, id := range u.DepartmentIDs {
				if id != "0" && ValidID(id) && !slices.Contains(ids, id) {
					ids = append(ids, id)
				}
			}
			return s.repo.SaveMembership(ctx, a.ID, ids, s.now().Add(5*time.Minute))
		}
	}
	if saveErr := s.repo.SaveMembership(ctx, a.ID, []string{}, s.now()); saveErr != nil {
		return saveErr
	}
	return e
}
