package publishing

import (
	"context"
	"crypto/sha256"
	"errors"
	"testing"
	"time"
)

type credentialRepo struct {
	hash      []byte
	principal Principal
	created   int
}

func (r *credentialRepo) Create(_ context.Context, _ string, in CreateRequest, hash []byte, prefix string) (Credential, error) {
	r.created++
	r.hash = hash
	return Credential{ID: "id", Name: in.Name, Scopes: in.Scopes, Prefix: prefix, ExpiresAt: in.ExpiresAt}, nil
}
func (r *credentialRepo) List(context.Context, string) ([]Credential, error) { return nil, nil }
func (r *credentialRepo) Revoke(context.Context, string, string) error       { return nil }
func (r *credentialRepo) Authenticate(context.Context, []byte) (Principal, error) {
	return r.principal, nil
}
func TestCredentialSecretAndScopeLimits(t *testing.T) {
	repo := &credentialRepo{principal: Principal{UserID: "user", Scopes: []string{"capabilities:read"}}}
	s := New(repo)
	ctx := context.Background()
	in := CreateRequest{Name: " CI ", Scopes: []string{"capabilities:read"}, ExpiresAt: time.Now().Add(time.Hour)}
	issued, e := s.Create(ctx, "user", in)
	if e != nil {
		t.Fatal(e)
	}
	digest := sha256.Sum256([]byte(issued.Token))
	if len(issued.Token) != 47 || string(repo.hash) != string(digest[:]) || issued.Name != "CI" || issued.Prefix == issued.Token {
		t.Fatal("wrong secret storage or presentation")
	}
	if _, e = s.Authenticate(ctx, issued.Token, "capabilities:submit"); !errors.Is(e, ErrForbidden) {
		t.Fatal("scope escalation", e)
	}
	if _, e = s.Authenticate(ctx, issued.Token, "capabilities:read"); e != nil {
		t.Fatal(e)
	}
	if _, e = s.Authenticate(ctx, "wrong", "capabilities:read"); !errors.Is(e, ErrUnauthenticated) {
		t.Fatal(e)
	}
	for _, scopes := range [][]string{nil, {"admin"}, {"capabilities:read", "capabilities:read"}} {
		in.Scopes = scopes
		if _, e = s.Create(ctx, "user", in); !errors.Is(e, ErrInvalid) {
			t.Fatal("invalid scope accepted", e)
		}
	}
	in.Scopes = Scopes
	for _, expires := range []time.Time{{}, time.Now().Add(-time.Second), time.Now().Add(366 * 24 * time.Hour)} {
		in.ExpiresAt = expires
		if _, e = s.Create(ctx, "user", in); !errors.Is(e, ErrInvalid) {
			t.Fatal("invalid expiry accepted", e)
		}
	}
	if repo.created != 1 {
		t.Fatal("invalid request reached repository")
	}
}
