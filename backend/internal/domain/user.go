package domain

import "time"

type User struct {
	ID          string    `json:"id"`
	DisplayName string    `json:"display_name"`
	Email       string    `json:"email"`
	AvatarURL   string    `json:"avatar_url"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

type OAuthAccount struct {
	ID                    string
	UserID                string
	Provider              string
	ProviderUserID        string
	TenantKey             string
	AccessTokenEncrypted  []byte
	RefreshTokenEncrypted []byte
	AccessTokenExpiresAt  time.Time
	RefreshTokenExpiresAt *time.Time
	Scopes                []string
	ReauthRequired        bool
	CreatedAt             time.Time
	UpdatedAt             time.Time
}

type Session struct {
	ID            string
	UserID        string
	TokenHash     []byte
	CSRFTokenHash []byte
	ExpiresAt     time.Time
	RevokedAt     *time.Time
	CreatedAt     time.Time
}

type OAuthState struct {
	ID        string
	StateHash []byte
	TenantKey string
	ExpiresAt time.Time
	CreatedAt time.Time
}

type FeishuIdentity struct {
	OpenID      string
	UnionID     string
	TenantKey   string
	DisplayName string
	AvatarURL   string
	Email       string
}

type OAuthToken struct {
	AccessToken           string
	RefreshToken          string
	AccessTokenExpiresAt  time.Time
	RefreshTokenExpiresAt *time.Time
	Scopes                []string
}
