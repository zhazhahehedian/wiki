package auth

import (
	"bytes"
	"context"
	"crypto/sha256"
	"errors"
	"testing"
	"time"

	"github.com/zenith-wang/it-wiki/backend/internal/domain"
)

type fakeSessionPersistence struct {
	created    domain.Session
	loaded     domain.Session
	loadErr    error
	deleted    []byte
	cleanupNow time.Time
}

func (f *fakeSessionPersistence) CreateSession(_ context.Context, session domain.Session) (domain.Session, error) {
	f.created = session
	return session, nil
}

func (f *fakeSessionPersistence) SessionByTokenHash(context.Context, []byte) (domain.Session, error) {
	return f.loaded, f.loadErr
}

func (f *fakeSessionPersistence) DeleteSessionByTokenHash(_ context.Context, hash []byte) error {
	f.deleted = append([]byte(nil), hash...)
	return f.loadErr
}

func (f *fakeSessionPersistence) DeleteExpiredSessions(_ context.Context, now time.Time) (int, error) {
	f.cleanupNow = now
	return 3, nil
}

func TestPersistentSessionStorePersistsOnlyCredentialHashes(t *testing.T) {
	now := time.Date(2026, 8, 9, 0, 0, 0, 0, time.UTC)
	persistence := &fakeSessionPersistence{}
	store := NewPersistentSessionStore(persistence, func() time.Time { return now })

	session, token, csrf, err := store.Create(context.Background(), "user-1", now.Add(time.Hour))
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	if token == "" || csrf == "" || session.UserID != "user-1" {
		t.Fatalf("session/token/csrf = %#v %q %q", session, token, csrf)
	}
	tokenHash := sha256.Sum256([]byte(token))
	csrfHash := sha256.Sum256([]byte(csrf))
	if !bytes.Equal(persistence.created.TokenHash, tokenHash[:]) || !bytes.Equal(persistence.created.CSRFTokenHash, csrfHash[:]) {
		t.Fatalf("persisted hashes = %x %x", persistence.created.TokenHash, persistence.created.CSRFTokenHash)
	}
	if bytes.Contains(persistence.created.TokenHash, []byte(token)) || bytes.Contains(persistence.created.CSRFTokenHash, []byte(csrf)) {
		t.Fatal("persistent session contains raw credentials")
	}
}

func TestPersistentSessionStoreHashesLookupRevokeAndChecksExpiry(t *testing.T) {
	now := time.Date(2026, 8, 9, 0, 0, 0, 0, time.UTC)
	persistence := &fakeSessionPersistence{loaded: domain.Session{UserID: "user-1", ExpiresAt: now.Add(time.Hour)}}
	store := NewPersistentSessionStore(persistence, func() time.Time { return now })

	if _, err := store.Get(context.Background(), "raw-token"); err != nil {
		t.Fatalf("Get() error = %v", err)
	}
	if err := store.Revoke(context.Background(), "raw-token"); err != nil {
		t.Fatalf("Revoke() error = %v", err)
	}
	wantHash := sha256.Sum256([]byte("raw-token"))
	if !bytes.Equal(persistence.deleted, wantHash[:]) {
		t.Fatalf("deleted hash = %x, want %x", persistence.deleted, wantHash)
	}

	persistence.loaded.ExpiresAt = now
	if _, err := store.Get(context.Background(), "raw-token"); !errors.Is(err, ErrSessionExpired) {
		t.Fatalf("expired Get() error = %v", err)
	}
	if count, err := store.CleanupExpired(context.Background()); err != nil || count != 3 || !persistence.cleanupNow.Equal(now) {
		t.Fatalf("CleanupExpired() = %d, %v, now=%s", count, err, persistence.cleanupNow)
	}
}
