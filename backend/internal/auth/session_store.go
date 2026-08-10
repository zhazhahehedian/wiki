package auth

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/zenith-wang/it-wiki/backend/internal/domain"
)

var (
	ErrSessionNotFound          = errors.New("session not found")
	ErrSessionExpired           = errors.New("session expired")
	ErrOAuthStateNotFound       = errors.New("oauth state not found")
	ErrOAuthStateExpired        = errors.New("oauth state expired")
	ErrOAuthStateTenantMismatch = errors.New("oauth state tenant mismatch")
)

type MemorySessionStore struct {
	mu       sync.Mutex
	now      func() time.Time
	sessions map[[sha256.Size]byte]domain.Session
}

func NewMemorySessionStore(now func() time.Time) *MemorySessionStore {
	if now == nil {
		now = time.Now
	}
	return &MemorySessionStore{now: now, sessions: make(map[[sha256.Size]byte]domain.Session)}
}

func (s *MemorySessionStore) Create(_ context.Context, userID string, expiresAt time.Time) (domain.Session, string, string, error) {
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
	session := domain.Session{
		ID:            uuid.NewString(),
		UserID:        userID,
		TokenHash:     append([]byte(nil), tokenHash[:]...),
		CSRFTokenHash: append([]byte(nil), csrfHash[:]...),
		ExpiresAt:     expiresAt,
		CreatedAt:     s.now(),
	}
	s.mu.Lock()
	s.sessions[tokenHash] = cloneSession(session)
	s.mu.Unlock()
	return cloneSession(session), token, csrf, nil
}

func (s *MemorySessionStore) Get(_ context.Context, rawToken string) (domain.Session, error) {
	hash := sha256.Sum256([]byte(rawToken))
	s.mu.Lock()
	defer s.mu.Unlock()
	session, ok := s.sessions[hash]
	if !ok || session.RevokedAt != nil {
		return domain.Session{}, ErrSessionNotFound
	}
	if !s.now().Before(session.ExpiresAt) {
		return domain.Session{}, ErrSessionExpired
	}
	return cloneSession(session), nil
}

func (s *MemorySessionStore) Revoke(_ context.Context, rawToken string) error {
	hash := sha256.Sum256([]byte(rawToken))
	s.mu.Lock()
	defer s.mu.Unlock()
	session, ok := s.sessions[hash]
	if !ok {
		return ErrSessionNotFound
	}
	now := s.now()
	session.RevokedAt = &now
	s.sessions[hash] = session
	return nil
}

func (s *MemorySessionStore) CleanupExpired(_ context.Context) (int, error) {
	now := s.now()
	s.mu.Lock()
	defer s.mu.Unlock()
	removed := 0
	for hash, session := range s.sessions {
		if !now.Before(session.ExpiresAt) {
			delete(s.sessions, hash)
			removed++
		}
	}
	return removed, nil
}

type MemoryOAuthStateStore struct {
	mu     sync.Mutex
	now    func() time.Time
	states map[[sha256.Size]byte]domain.OAuthState
}

func NewMemoryOAuthStateStore(now func() time.Time) *MemoryOAuthStateStore {
	if now == nil {
		now = time.Now
	}
	return &MemoryOAuthStateStore{now: now, states: make(map[[sha256.Size]byte]domain.OAuthState)}
}

func (s *MemoryOAuthStateStore) Create(_ context.Context, tenantKey string, expiresAt time.Time) (domain.OAuthState, string, error) {
	raw, err := randomToken()
	if err != nil {
		return domain.OAuthState{}, "", err
	}
	hash := sha256.Sum256([]byte(raw))
	state := domain.OAuthState{
		ID:        uuid.NewString(),
		StateHash: append([]byte(nil), hash[:]...),
		TenantKey: tenantKey,
		ExpiresAt: expiresAt,
		CreatedAt: s.now(),
	}
	s.mu.Lock()
	s.states[hash] = cloneOAuthState(state)
	s.mu.Unlock()
	return cloneOAuthState(state), raw, nil
}

func (s *MemoryOAuthStateStore) Consume(_ context.Context, rawState, tenantKey string) (domain.OAuthState, error) {
	hash := sha256.Sum256([]byte(rawState))
	s.mu.Lock()
	defer s.mu.Unlock()
	state, ok := s.states[hash]
	if !ok {
		return domain.OAuthState{}, ErrOAuthStateNotFound
	}
	delete(s.states, hash)
	if !s.now().Before(state.ExpiresAt) {
		return domain.OAuthState{}, ErrOAuthStateExpired
	}
	if state.TenantKey != tenantKey {
		return domain.OAuthState{}, ErrOAuthStateTenantMismatch
	}
	return cloneOAuthState(state), nil
}

func randomToken() (string, error) {
	value := make([]byte, 32)
	if _, err := rand.Read(value); err != nil {
		return "", fmt.Errorf("generate opaque token: %w", err)
	}
	return base64.RawURLEncoding.EncodeToString(value), nil
}

func cloneSession(session domain.Session) domain.Session {
	session.TokenHash = append([]byte(nil), session.TokenHash...)
	session.CSRFTokenHash = append([]byte(nil), session.CSRFTokenHash...)
	if session.RevokedAt != nil {
		revokedAt := *session.RevokedAt
		session.RevokedAt = &revokedAt
	}
	return session
}

func cloneOAuthState(state domain.OAuthState) domain.OAuthState {
	state.StateHash = append([]byte(nil), state.StateHash...)
	return state
}
