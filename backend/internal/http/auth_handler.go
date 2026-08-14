package http

import (
	"context"
	"crypto/subtle"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	authstore "github.com/zenith-wang/it-wiki/backend/internal/auth"
	"github.com/zenith-wang/it-wiki/backend/internal/domain"
	"github.com/zenith-wang/it-wiki/backend/internal/domain/ports"
	"github.com/zenith-wang/it-wiki/backend/internal/service"
)

const (
	defaultFeishuAuthorizeURL = "https://accounts.feishu.cn/open-apis/authen/v1/authorize"
	OAuthStateCookieName      = "it_wiki_oauth_state"
	oauthStateCookieTTL       = 5 * time.Minute
	oauthErrorCancelled       = "oauth_cancelled"
	oauthErrorInvalidState    = "oauth_state_invalid"
	oauthErrorTenant          = "tenant_not_allowed"
	oauthErrorReauth          = "feishu_reauth_required"
	oauthErrorUnavailable     = "auth_service_unavailable"
)

type AuthFlow interface {
	BeginOAuth(ctx context.Context) (string, error)
	CompleteOAuth(ctx context.Context, rawState, code string) (service.AuthResult, error)
}

type UserResolver interface {
	User(ctx context.Context, userID string) (domain.User, error)
}

type AuthHandlerConfig struct {
	AuthorizeURL   string
	AppID          string
	RedirectURL    string
	Scopes         []string
	FrontendOrigin string
	FrontendPath   string
	CookieSecure   bool
	SessionTTL     time.Duration
}

type AuthHandler struct {
	disabled         bool
	config           AuthHandlerConfig
	flow             AuthFlow
	sessions         ports.SessionStore
	users            UserResolver
	authorizeURL     *url.URL
	frontendOrigin   string
	frontendRedirect string
	frontendLoginURL string
}

func NewDisabledAuthHandler() *AuthHandler {
	return &AuthHandler{disabled: true}
}
func NewAuthHandler(config AuthHandlerConfig, flow AuthFlow, sessions ports.SessionStore, users UserResolver) (*AuthHandler, error) {
	if flow == nil || sessions == nil || users == nil {
		return nil, errors.New("auth handler dependencies are required")
	}
	if config.AuthorizeURL == "" {
		config.AuthorizeURL = defaultFeishuAuthorizeURL
	}
	authorizeURL, err := url.Parse(config.AuthorizeURL)
	if err != nil || authorizeURL.Scheme != "https" || authorizeURL.Host == "" {
		return nil, errors.New("authorize url must be an absolute https url")
	}
	originURL, err := url.Parse(config.FrontendOrigin)
	if err != nil || (originURL.Scheme != "http" && originURL.Scheme != "https") || originURL.Host == "" ||
		originURL.User != nil || originURL.Path != "" || originURL.RawQuery != "" || originURL.Fragment != "" {
		return nil, errors.New("frontend origin must contain only an http(s) origin")
	}
	if config.FrontendPath == "" {
		config.FrontendPath = "/"
	}
	pathURL, err := url.Parse(config.FrontendPath)
	if err != nil || !strings.HasPrefix(config.FrontendPath, "/") || strings.HasPrefix(config.FrontendPath, "//") ||
		pathURL.IsAbs() || pathURL.Host != "" {
		return nil, errors.New("frontend path must be a local absolute path")
	}
	if config.SessionTTL <= 0 {
		config.SessionTTL = 24 * time.Hour
	}
	origin := strings.TrimRight(config.FrontendOrigin, "/")
	return &AuthHandler{
		config:           config,
		flow:             flow,
		sessions:         sessions,
		users:            users,
		authorizeURL:     authorizeURL,
		frontendOrigin:   origin,
		frontendRedirect: origin + config.FrontendPath,
		frontendLoginURL: origin + "/login",
	}, nil
}

func (h *AuthHandler) FrontendOrigin() string {
	return h.frontendOrigin
}
func (h *AuthHandler) Start(w http.ResponseWriter, r *http.Request) {
	if h.disabled {
		http.NotFound(w, r)
		return
	}
	state, err := h.flow.BeginOAuth(r.Context())
	if err != nil {
		WriteError(w, r, fmt.Errorf("begin oauth: %w", err))
		return
	}
	h.setOAuthStateCookie(w, state)
	destination := *h.authorizeURL
	query := destination.Query()
	query.Set("app_id", h.config.AppID)
	query.Set("client_id", h.config.AppID)
	query.Set("response_type", "code")
	query.Set("redirect_uri", h.config.RedirectURL)
	if len(h.config.Scopes) > 0 {
		query.Set("scope", strings.Join(h.config.Scopes, " "))
	}
	query.Set("state", state)
	destination.RawQuery = query.Encode()
	http.Redirect(w, r, destination.String(), http.StatusFound)
}

func (h *AuthHandler) Callback(w http.ResponseWriter, r *http.Request) {
	if h.disabled {
		http.NotFound(w, r)
		return
	}
	state := r.URL.Query().Get("state")
	stateCookie, cookieErr := r.Cookie(OAuthStateCookieName)
	if state == "" || cookieErr != nil ||
		subtle.ConstantTimeCompare([]byte(state), []byte(stateCookie.Value)) != 1 {
		h.redirectOAuthError(w, r, oauthErrorInvalidState)
		return
	}
	h.clearOAuthStateCookie(w)
	if r.URL.Query().Get("error") != "" {
		h.redirectOAuthError(w, r, oauthErrorCancelled)
		return
	}
	code := r.URL.Query().Get("code")
	if code == "" {
		h.redirectOAuthError(w, r, oauthErrorInvalidState)
		return
	}
	result, err := h.flow.CompleteOAuth(r.Context(), state, code)
	if err != nil {
		h.writeOAuthError(w, r, err)
		return
	}
	h.setAuthCookies(w, result)
	http.Redirect(w, r, h.frontendRedirect, http.StatusFound)
}

func (h *AuthHandler) Me(w http.ResponseWriter, r *http.Request) {
	user, ok := CurrentUserFromContext(r.Context())
	if !ok {
		writeUnauthenticated(w, r)
		return
	}
	WriteJSON(w, http.StatusOK, user)
}

func (h *AuthHandler) Logout(w http.ResponseWriter, r *http.Request) {
	cookie, err := r.Cookie(SessionCookieName)
	if err == nil && cookie.Value != "" {
		if revokeErr := h.sessions.Revoke(r.Context(), cookie.Value); revokeErr != nil && !errors.Is(revokeErr, authstore.ErrSessionNotFound) {
			WriteError(w, r, fmt.Errorf("revoke session: %w", revokeErr))
			return
		}
	}
	h.clearAuthCookies(w)
	w.WriteHeader(http.StatusNoContent)
}

func (h *AuthHandler) setOAuthStateCookie(w http.ResponseWriter, state string) {
	expires := time.Now().Add(oauthStateCookieTTL)
	http.SetCookie(w, &http.Cookie{
		Name: OAuthStateCookieName, Value: state, Path: "/api/v1/auth/feishu/callback",
		HttpOnly: true, Secure: h.config.CookieSecure, SameSite: http.SameSiteLaxMode,
		MaxAge: int(oauthStateCookieTTL / time.Second), Expires: expires,
	})
}

func (h *AuthHandler) clearOAuthStateCookie(w http.ResponseWriter) {
	http.SetCookie(w, &http.Cookie{
		Name: OAuthStateCookieName, Value: "", Path: "/api/v1/auth/feishu/callback",
		HttpOnly: true, Secure: h.config.CookieSecure, SameSite: http.SameSiteLaxMode,
		MaxAge: -1, Expires: time.Unix(1, 0),
	})
}
func (h *AuthHandler) setAuthCookies(w http.ResponseWriter, result service.AuthResult) {
	expiresAt := result.Session.ExpiresAt
	if expiresAt.IsZero() {
		expiresAt = time.Now().Add(h.config.SessionTTL)
	}
	maxAge := int(time.Until(expiresAt) / time.Second)
	ttlMaxAge := int(h.config.SessionTTL / time.Second)
	if maxAge > ttlMaxAge {
		maxAge = ttlMaxAge
	}
	if maxAge < 1 {
		maxAge = 1
	}
	http.SetCookie(w, &http.Cookie{
		Name: SessionCookieName, Value: result.SessionToken, Path: "/",
		HttpOnly: true, Secure: h.config.CookieSecure, SameSite: http.SameSiteLaxMode,
		MaxAge: maxAge, Expires: expiresAt,
	})
	http.SetCookie(w, &http.Cookie{
		Name: CSRFCookieName, Value: result.CSRFToken, Path: "/",
		HttpOnly: false, Secure: h.config.CookieSecure, SameSite: http.SameSiteLaxMode,
		MaxAge: maxAge, Expires: expiresAt,
	})
}

func (h *AuthHandler) clearAuthCookies(w http.ResponseWriter) {
	expires := time.Unix(1, 0)
	for _, name := range []string{SessionCookieName, CSRFCookieName} {
		http.SetCookie(w, &http.Cookie{
			Name: name, Value: "", Path: "/", HttpOnly: name == SessionCookieName,
			Secure: h.config.CookieSecure, SameSite: http.SameSiteLaxMode,
			MaxAge: -1, Expires: expires,
		})
	}
}

func (h *AuthHandler) writeOAuthError(w http.ResponseWriter, r *http.Request, err error) {
	switch {
	case service.IsAuthError(err, service.AuthErrorInvalidState):
		h.redirectOAuthError(w, r, oauthErrorInvalidState)
	case service.IsAuthError(err, service.AuthErrorTenantNotAllowed):
		h.redirectOAuthError(w, r, oauthErrorTenant)
	case service.IsAuthError(err, service.AuthErrorInsufficientScope),
		service.IsAuthError(err, service.AuthErrorReauthRequired):
		h.redirectOAuthError(w, r, oauthErrorReauth)
	default:
		h.redirectOAuthError(w, r, oauthErrorUnavailable)
	}
}

func (h *AuthHandler) redirectOAuthError(w http.ResponseWriter, r *http.Request, code string) {
	destination, _ := url.Parse(h.frontendLoginURL)
	query := destination.Query()
	query.Set("error", code)
	destination.RawQuery = query.Encode()
	http.Redirect(w, r, destination.String(), http.StatusFound)
}
