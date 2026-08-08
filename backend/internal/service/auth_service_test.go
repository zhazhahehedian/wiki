package service_test

import (
	"bytes"
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"

	authstore "github.com/zenith-wang/it-wiki/backend/internal/auth"
	"github.com/zenith-wang/it-wiki/backend/internal/domain"
	"github.com/zenith-wang/it-wiki/backend/internal/domain/ports"
	"github.com/zenith-wang/it-wiki/backend/internal/service"
)

func TestAESGCMProtectorRoundTripUsesRandomNonce(t *testing.T) {
	protector, err := authstore.NewAESGCMProtector([]byte("0123456789abcdef0123456789abcdef"))
	if err != nil {
		t.Fatalf("NewAESGCMProtector() error = %v", err)
	}

	first, err := protector.Encrypt("access-token")
	if err != nil {
		t.Fatalf("Encrypt() error = %v", err)
	}
	second, err := protector.Encrypt("access-token")
	if err != nil {
		t.Fatalf("Encrypt() second error = %v", err)
	}
	if bytes.Equal(first, second) {
		t.Fatal("Encrypt() reused a nonce: ciphertexts are equal")
	}
	got, err := protector.Decrypt(first)
	if err != nil {
		t.Fatalf("Decrypt() error = %v", err)
	}
	if got != "access-token" {
		t.Fatalf("Decrypt() = %q, want access-token", got)
	}
}

func TestAESGCMProtectorRejectsEmptyKeyWrongKeyAndTampering(t *testing.T) {
	if _, err := authstore.NewAESGCMProtector(nil); err == nil {
		t.Fatal("NewAESGCMProtector(nil) error = nil")
	}
	first, _ := authstore.NewAESGCMProtector([]byte("0123456789abcdef0123456789abcdef"))
	second, _ := authstore.NewAESGCMProtector([]byte("abcdef0123456789abcdef0123456789"))
	ciphertext, err := first.Encrypt("refresh-token")
	if err != nil {
		t.Fatalf("Encrypt() error = %v", err)
	}
	if _, err := second.Decrypt(ciphertext); err == nil {
		t.Fatal("Decrypt() with wrong key error = nil")
	}
	ciphertext[len(ciphertext)-1] ^= 0xff
	if _, err := first.Decrypt(ciphertext); err == nil {
		t.Fatal("Decrypt() tampered ciphertext error = nil")
	}
}

func TestMemorySessionStoreHashesTokensExpiresRevokesAndCleansUp(t *testing.T) {
	now := time.Date(2026, 8, 8, 12, 0, 0, 0, time.UTC)
	store := authstore.NewMemorySessionStore(func() time.Time { return now })

	session, token, csrf, err := store.Create(context.Background(), "user-1", now.Add(time.Hour))
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	if token == "" || csrf == "" {
		t.Fatal("Create() returned empty opaque credentials")
	}
	wantTokenHash := sha256.Sum256([]byte(token))
	wantCSRFHash := sha256.Sum256([]byte(csrf))
	if !bytes.Equal(session.TokenHash, wantTokenHash[:]) || !bytes.Equal(session.CSRFTokenHash, wantCSRFHash[:]) {
		t.Fatal("Create() did not retain SHA-256 token hashes")
	}
	if bytes.Contains(session.TokenHash, []byte(token)) || bytes.Contains(session.CSRFTokenHash, []byte(csrf)) {
		t.Fatal("session contains raw credentials")
	}
	loaded, err := store.Get(context.Background(), token)
	if err != nil || loaded.UserID != "user-1" {
		t.Fatalf("Get() = %+v, %v", loaded, err)
	}
	if err := store.Revoke(context.Background(), token); err != nil {
		t.Fatalf("Revoke() error = %v", err)
	}
	if _, err := store.Get(context.Background(), token); !errors.Is(err, authstore.ErrSessionNotFound) {
		t.Fatalf("Get() after revoke error = %v", err)
	}

	_, expiredToken, _, _ := store.Create(context.Background(), "user-2", now.Add(time.Minute))
	now = now.Add(2 * time.Minute)
	if _, err := store.Get(context.Background(), expiredToken); !errors.Is(err, authstore.ErrSessionExpired) {
		t.Fatalf("Get() expired error = %v", err)
	}
	removed, err := store.CleanupExpired(context.Background())
	if err != nil || removed != 1 {
		t.Fatalf("CleanupExpired() = %d, %v, want 1", removed, err)
	}
}

func TestOAuthStateIsOneTimeFiveMinutesAndTenantBound(t *testing.T) {
	now := time.Date(2026, 8, 8, 12, 0, 0, 0, time.UTC)
	store := authstore.NewMemoryOAuthStateStore(func() time.Time { return now })

	state, raw, err := store.Create(context.Background(), "tenant-a", now.Add(5*time.Minute))
	if err != nil || state.ExpiresAt.Sub(now) != 5*time.Minute || raw == "" {
		t.Fatalf("Create() = %+v, %q, %v", state, raw, err)
	}
	if _, err := store.Consume(context.Background(), raw, "tenant-b"); !errors.Is(err, authstore.ErrOAuthStateTenantMismatch) {
		t.Fatalf("Consume() tenant mismatch error = %v", err)
	}
	if _, err := store.Consume(context.Background(), raw, "tenant-a"); !errors.Is(err, authstore.ErrOAuthStateNotFound) {
		t.Fatalf("Consume() replay after mismatch error = %v", err)
	}

	_, raw, _ = store.Create(context.Background(), "tenant-a", now.Add(5*time.Minute))
	if _, err := store.Consume(context.Background(), raw, "tenant-a"); err != nil {
		t.Fatalf("Consume() error = %v", err)
	}
	if _, err := store.Consume(context.Background(), raw, "tenant-a"); !errors.Is(err, authstore.ErrOAuthStateNotFound) {
		t.Fatalf("Consume() replay error = %v", err)
	}

	_, raw, _ = store.Create(context.Background(), "tenant-a", now.Add(5*time.Minute))
	now = now.Add(5*time.Minute + time.Nanosecond)
	if _, err := store.Consume(context.Background(), raw, "tenant-a"); !errors.Is(err, authstore.ErrOAuthStateExpired) {
		t.Fatalf("Consume() expiry error = %v", err)
	}
}

func TestAuthCompleteOAuthRejectsTenantMismatchAndExchangeErrors(t *testing.T) {
	now := time.Date(2026, 8, 8, 12, 0, 0, 0, time.UTC)
	stateStore := authstore.NewMemoryOAuthStateStore(func() time.Time { return now })
	sessions := authstore.NewMemorySessionStore(func() time.Time { return now })
	protector, _ := authstore.NewAESGCMProtector([]byte("0123456789abcdef0123456789abcdef"))
	repo := newFakeAuthRepository()

	client := &fakeOAuthClient{exchangeErr: errors.New("exchange unavailable")}
	svc := service.NewAuth(service.AuthConfig{
		TenantKey: "tenant-a", RequiredScopes: []string{"docs:document:readonly"},
		SessionTTL: time.Hour, Now: func() time.Time { return now },
	}, client, protector, sessions, stateStore, repo)
	rawState, err := svc.BeginOAuth(context.Background())
	if err != nil {
		t.Fatalf("BeginOAuth() error = %v", err)
	}
	if _, err := svc.CompleteOAuth(context.Background(), rawState, "code"); !errors.Is(err, client.exchangeErr) {
		t.Fatalf("CompleteOAuth() exchange error = %v", err)
	}

	client.exchangeErr = nil
	client.token = domain.OAuthToken{AccessToken: "access", RefreshToken: "refresh", AccessTokenExpiresAt: now.Add(time.Hour), Scopes: []string{"docs:document:readonly"}}
	client.identity = domain.FeishuIdentity{OpenID: "ou_1", TenantKey: "tenant-b", DisplayName: "User"}
	rawState, _ = svc.BeginOAuth(context.Background())
	if _, err := svc.CompleteOAuth(context.Background(), rawState, "code"); !service.IsAuthError(err, service.AuthErrorTenantNotAllowed) {
		t.Fatalf("CompleteOAuth() tenant error = %v", err)
	}
	if repo.upsertCalls != 0 {
		t.Fatalf("repository upsert calls = %d, want 0", repo.upsertCalls)
	}
}

func TestAuthCompleteOAuthEncryptsTokensAndIssuesLocalSession(t *testing.T) {
	now := time.Date(2026, 8, 8, 12, 0, 0, 0, time.UTC)
	stateStore := authstore.NewMemoryOAuthStateStore(func() time.Time { return now })
	sessions := authstore.NewMemorySessionStore(func() time.Time { return now })
	protector, _ := authstore.NewAESGCMProtector([]byte("0123456789abcdef0123456789abcdef"))
	repo := newFakeAuthRepository()
	client := &fakeOAuthClient{
		token:    domain.OAuthToken{AccessToken: "access-secret", RefreshToken: "refresh-secret", AccessTokenExpiresAt: now.Add(time.Hour), RefreshTokenExpiresAt: timePtr(now.Add(30 * 24 * time.Hour)), Scopes: []string{"docs:document:readonly"}},
		identity: domain.FeishuIdentity{OpenID: "ou_1", TenantKey: "tenant-a", DisplayName: "User"},
	}
	svc := service.NewAuth(service.AuthConfig{TenantKey: "tenant-a", RequiredScopes: []string{"docs:document:readonly"}, SessionTTL: time.Hour, Now: func() time.Time { return now }}, client, protector, sessions, stateStore, repo)
	rawState, _ := svc.BeginOAuth(context.Background())

	result, err := svc.CompleteOAuth(context.Background(), rawState, "code")
	if err != nil {
		t.Fatalf("CompleteOAuth() error = %v", err)
	}
	if result.SessionToken == "" || result.CSRFToken == "" || result.User.ID != "user-1" {
		t.Fatalf("CompleteOAuth() = %+v", result)
	}
	if bytes.Contains(repo.account.AccessTokenEncrypted, []byte("access-secret")) || bytes.Contains(repo.account.RefreshTokenEncrypted, []byte("refresh-secret")) {
		t.Fatal("repository received raw provider tokens")
	}
	access, _ := protector.Decrypt(repo.account.AccessTokenEncrypted)
	refresh, _ := protector.Decrypt(repo.account.RefreshTokenEncrypted)
	if access != "access-secret" || refresh != "refresh-secret" {
		t.Fatalf("stored token round trip = %q/%q", access, refresh)
	}
	if _, err := sessions.Get(context.Background(), result.SessionToken); err != nil {
		t.Fatalf("session lookup error = %v", err)
	}
}

func TestAuthAccessTokenSerializesConcurrentRefreshAndPreservesRotatedToken(t *testing.T) {
	now := time.Date(2026, 8, 8, 12, 0, 0, 0, time.UTC)
	protector, _ := authstore.NewAESGCMProtector([]byte("0123456789abcdef0123456789abcdef"))
	accessCipher, _ := protector.Encrypt("expired-access")
	refreshCipher, _ := protector.Encrypt("refresh-v1")
	repo := newFakeAuthRepository()
	repo.account = domain.OAuthAccount{ID: "account-1", AccessTokenEncrypted: accessCipher, RefreshTokenEncrypted: refreshCipher, AccessTokenExpiresAt: now.Add(-time.Minute)}
	client := &fakeOAuthClient{refreshToken: domain.OAuthToken{AccessToken: "access-v2", RefreshToken: "refresh-v2", AccessTokenExpiresAt: now.Add(time.Hour)}}
	client.refreshGate = make(chan struct{})
	svc := service.NewAuth(service.AuthConfig{TenantKey: "tenant-a", SessionTTL: time.Hour, Now: func() time.Time { return now }}, client, protector, authstore.NewMemorySessionStore(func() time.Time { return now }), authstore.NewMemoryOAuthStateStore(func() time.Time { return now }), repo)

	const callers = 8
	results := make(chan string, callers)
	errs := make(chan error, callers)
	var wg sync.WaitGroup
	for range callers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			got, err := svc.AccessToken(context.Background(), "account-1")
			results <- got
			errs <- err
		}()
	}
	client.waitForRefresh(t)
	close(client.refreshGate)
	wg.Wait()
	close(results)
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatalf("AccessToken() error = %v", err)
		}
	}
	for got := range results {
		if got != "access-v2" {
			t.Fatalf("AccessToken() = %q", got)
		}
	}
	if client.refreshCalls != 1 {
		t.Fatalf("RefreshToken() calls = %d, want 1", client.refreshCalls)
	}
	storedRefresh, _ := protector.Decrypt(repo.account.RefreshTokenEncrypted)
	if storedRefresh != "refresh-v2" {
		t.Fatalf("stored refresh token = %q, want refresh-v2", storedRefresh)
	}
}

func TestAuthAccessTokenMarksReauthRequiredOnInvalidGrant(t *testing.T) {
	now := time.Date(2026, 8, 8, 12, 0, 0, 0, time.UTC)
	protector, _ := authstore.NewAESGCMProtector([]byte("0123456789abcdef0123456789abcdef"))
	accessCipher, _ := protector.Encrypt("expired-access")
	refreshCipher, _ := protector.Encrypt("invalid-refresh")
	repo := newFakeAuthRepository()
	repo.account = domain.OAuthAccount{ID: "account-1", AccessTokenEncrypted: accessCipher, RefreshTokenEncrypted: refreshCipher, AccessTokenExpiresAt: now.Add(-time.Minute)}
	client := &fakeOAuthClient{refreshErr: &ports.OAuthError{Code: "invalid_grant", Message: "refresh token revoked"}}
	svc := service.NewAuth(service.AuthConfig{TenantKey: "tenant-a", SessionTTL: time.Hour, Now: func() time.Time { return now }}, client, protector, authstore.NewMemorySessionStore(func() time.Time { return now }), authstore.NewMemoryOAuthStateStore(func() time.Time { return now }), repo)

	if _, err := svc.AccessToken(context.Background(), "account-1"); !service.IsAuthError(err, service.AuthErrorReauthRequired) {
		t.Fatalf("AccessToken() error = %v", err)
	}
	if !repo.account.ReauthRequired {
		t.Fatal("oauth account reauth_required = false")
	}
}

type fakeOAuthClient struct {
	mu            sync.Mutex
	token         domain.OAuthToken
	identity      domain.FeishuIdentity
	exchangeErr   error
	refreshToken  domain.OAuthToken
	refreshErr    error
	refreshCalls  int
	refreshGate   chan struct{}
	refreshCalled chan struct{}
}

func (f *fakeOAuthClient) ExchangeCode(context.Context, string) (domain.OAuthToken, error) {
	return f.token, f.exchangeErr
}

func (f *fakeOAuthClient) UserInfo(context.Context, string) (domain.FeishuIdentity, error) {
	return f.identity, nil
}

func (f *fakeOAuthClient) RefreshToken(_ context.Context, token string) (domain.OAuthToken, error) {
	f.mu.Lock()
	f.refreshCalls++
	if f.refreshCalled == nil {
		f.refreshCalled = make(chan struct{})
		close(f.refreshCalled)
	}
	gate := f.refreshGate
	f.mu.Unlock()
	if token != "refresh-v1" && token != "invalid-refresh" {
		return domain.OAuthToken{}, fmt.Errorf("refresh token = %q", token)
	}
	if gate != nil {
		<-gate
	}
	return f.refreshToken, f.refreshErr
}

func (f *fakeOAuthClient) waitForRefresh(t *testing.T) {
	t.Helper()
	deadline := time.After(time.Second)
	for {
		f.mu.Lock()
		called := f.refreshCalled
		f.mu.Unlock()
		if called != nil {
			select {
			case <-called:
				return
			case <-deadline:
				t.Fatal("timed out waiting for refresh")
			}
		}
		select {
		case <-deadline:
			t.Fatal("timed out waiting for refresh")
		default:
			time.Sleep(time.Millisecond)
		}
	}
}

type fakeAuthRepository struct {
	mu          sync.Mutex
	user        domain.User
	account     domain.OAuthAccount
	upsertCalls int
}

func newFakeAuthRepository() *fakeAuthRepository {
	return &fakeAuthRepository{user: domain.User{ID: "user-1", DisplayName: "User"}}
}

func (r *fakeAuthRepository) UpsertOAuthIdentity(_ context.Context, identity domain.FeishuIdentity, account domain.OAuthAccount) (domain.User, domain.OAuthAccount, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.upsertCalls++
	account.ID = "account-1"
	account.UserID = r.user.ID
	account.ProviderUserID = identity.OpenID
	r.account = account
	return r.user, r.account, nil
}

func (r *fakeAuthRepository) OAuthAccount(_ context.Context, id string) (domain.OAuthAccount, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.account.ID != id {
		return domain.OAuthAccount{}, errors.New("oauth account not found")
	}
	return cloneAccount(r.account), nil
}

func (r *fakeAuthRepository) UpdateOAuthTokens(_ context.Context, account domain.OAuthAccount) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.account = cloneAccount(account)
	return nil
}

func (r *fakeAuthRepository) MarkOAuthAccountReauthRequired(_ context.Context, id string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.account.ID != id {
		return errors.New("oauth account not found")
	}
	r.account.ReauthRequired = true
	return nil
}

func cloneAccount(account domain.OAuthAccount) domain.OAuthAccount {
	account.AccessTokenEncrypted = bytes.Clone(account.AccessTokenEncrypted)
	account.RefreshTokenEncrypted = bytes.Clone(account.RefreshTokenEncrypted)
	account.Scopes = append([]string(nil), account.Scopes...)
	return account
}

func timePtr(value time.Time) *time.Time { return &value }

var _ ports.OAuthClient = (*fakeOAuthClient)(nil)
var _ ports.AuthRepository = (*fakeAuthRepository)(nil)
