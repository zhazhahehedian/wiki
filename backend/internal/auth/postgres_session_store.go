package auth

import (
	"context"
	"crypto/sha256"
	"time"

	"github.com/google/uuid"
	"github.com/zenith-wang/it-wiki/backend/internal/domain"
)

type SessionPersistence interface {
	CreateSession(ctx context.Context, session domain.Session) (domain.Session, error)
	SessionByTokenHash(ctx context.Context, tokenHash []byte) (domain.Session, error)
	DeleteSessionByTokenHash(ctx context.Context, tokenHash []byte) error
	DeleteExpiredSessions(ctx context.Context, now time.Time) (int, error)
}

type PersistentSessionStore struct {
	persistence SessionPersistence
	now         func() time.Time
}

func NewPersistentSessionStore(persistence SessionPersistence, now func() time.Time) *PersistentSessionStore {
	if now == nil {
		now = time.Now
	}
	return &PersistentSessionStore{persistence: persistence, now: now}
}

func (s *PersistentSessionStore) Create(ctx context.Context, userID string, expiresAt time.Time) (domain.Session, string, string, error) {
	token, err := randomToken()
	if err != nil {
		return domain.Session{}, "", "", err
	}
	csrf, err := randomToken()
	if err != nil {
		return domain.Session{}, "", "", err
	}
	tokenHash := sha256.Sum256([]byte(token))
	csrfHash := sha256.Sum256([]byte(csrf))
	session, err := s.persistence.CreateSession(ctx, domain.Session{
		ID:            uuid.NewString(),
		UserID:        userID,
		TokenHash:     append([]byte(nil), tokenHash[:]...),
		CSRFTokenHash: append([]byte(nil), csrfHash[:]...),
		ExpiresAt:     expiresAt,
		CreatedAt:     s.now(),
	})
	if err != nil {
		return domain.Session{}, "", "", err
	}
	return session, token, csrf, nil
}

func (s *PersistentSessionStore) Get(ctx context.Context, rawToken string) (domain.Session, error) {
	hash := sha256.Sum256([]byte(rawToken))
	session, err := s.persistence.SessionByTokenHash(ctx, hash[:])
	if err != nil {
		return domain.Session{}, err
	}
	if session.RevokedAt != nil {
		return domain.Session{}, ErrSessionNotFound
	}
	if !s.now().Before(session.ExpiresAt) {
		return domain.Session{}, ErrSessionExpired
	}
	return session, nil
}

func (s *PersistentSessionStore) Revoke(ctx context.Context, rawToken string) error {
	hash := sha256.Sum256([]byte(rawToken))
	return s.persistence.DeleteSessionByTokenHash(ctx, hash[:])
}

func (s *PersistentSessionStore) CleanupExpired(ctx context.Context) (int, error) {
	return s.persistence.DeleteExpiredSessions(ctx, s.now())
}
