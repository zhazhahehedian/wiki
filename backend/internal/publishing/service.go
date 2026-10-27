// Package publishing authenticates account-bound credentials for the existing
// registry. It does not implement a second publication or authorization model.
package publishing

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"slices"
	"strings"
	"time"
	"unicode/utf8"
)

var (
	ErrInvalid         = errors.New("invalid publishing credential")
	ErrUnauthenticated = errors.New("invalid or expired publishing credential")
	ErrForbidden       = errors.New("publishing scope denied")
	ErrNotFound        = errors.New("publishing credential not found")
)
var Scopes = []string{"capabilities:read", "capabilities:write", "capabilities:submit"}

type Credential struct {
	ID         string     `json:"id"`
	Name       string     `json:"name"`
	Prefix     string     `json:"prefix"`
	Scopes     []string   `json:"scopes"`
	ExpiresAt  time.Time  `json:"expires_at"`
	RevokedAt  *time.Time `json:"revoked_at,omitempty"`
	LastUsedAt *time.Time `json:"last_used_at,omitempty"`
	CreatedAt  time.Time  `json:"created_at"`
}
type CreateRequest struct {
	Name      string    `json:"name"`
	Scopes    []string  `json:"scopes"`
	ExpiresAt time.Time `json:"expires_at"`
}
type Issued struct {
	Credential
	Token string `json:"token"`
}
type Principal struct {
	UserID, CredentialID string
	Scopes               []string
}
type Repository interface {
	Create(context.Context, string, CreateRequest, []byte, string) (Credential, error)
	List(context.Context, string) ([]Credential, error)
	Revoke(context.Context, string, string) error
	Authenticate(context.Context, []byte) (Principal, error)
}
type Service struct {
	repo Repository
	now  func() time.Time
}

func New(repo Repository) *Service { return &Service{repo: repo, now: time.Now} }
func (s *Service) Create(ctx context.Context, user string, r CreateRequest) (Issued, error) {
	r.Name = strings.TrimSpace(r.Name)
	if r.Name == "" || utf8.RuneCountInString(r.Name) > 80 || !r.ExpiresAt.After(s.now().Add(time.Minute)) || r.ExpiresAt.After(s.now().Add(365*24*time.Hour)) || len(r.Scopes) == 0 || len(r.Scopes) > len(Scopes) {
		return Issued{}, ErrInvalid
	}
	seen := map[string]bool{}
	for _, scope := range r.Scopes {
		if !slices.Contains(Scopes, scope) || seen[scope] {
			return Issued{}, ErrInvalid
		}
		seen[scope] = true
	}
	var random [32]byte
	if _, e := rand.Read(random[:]); e != nil {
		return Issued{}, e
	}
	token := "chp_" + base64.RawURLEncoding.EncodeToString(random[:])
	hash := sha256.Sum256([]byte(token))
	c, e := s.repo.Create(ctx, user, r, hash[:], token[:12])
	if e != nil {
		return Issued{}, e
	}
	return Issued{Credential: c, Token: token}, nil
}
func (s *Service) List(ctx context.Context, user string) ([]Credential, error) {
	return s.repo.List(ctx, user)
}
func (s *Service) Revoke(ctx context.Context, user, id string) error {
	return s.repo.Revoke(ctx, user, id)
}
func (s *Service) Authenticate(ctx context.Context, token, scope string) (Principal, error) {
	if !strings.HasPrefix(token, "chp_") || len(token) != 47 {
		return Principal{}, ErrUnauthenticated
	}
	hash := sha256.Sum256([]byte(token))
	p, e := s.repo.Authenticate(ctx, hash[:])
	if e != nil {
		return Principal{}, e
	}
	if !slices.Contains(p.Scopes, scope) {
		return Principal{}, ErrForbidden
	}
	return p, nil
}
