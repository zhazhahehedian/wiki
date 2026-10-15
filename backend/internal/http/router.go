package http

import (
	"encoding/json"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
)

type Handlers struct {
	Knowledge    *KnowledgeHandler
	Auth         *AuthHandler
	Playground   *PlaygroundHandler
	Registry     *RegistryHandler
	SkillBuilder *SkillBuilderHandler
}

// NewPlatformRouter exposes only the active Capability Hub surface.
func NewPlatformRouter(auth *AuthHandler, playground ...*PlaygroundHandler) http.Handler {
	h := Handlers{Auth: auth}
	if len(playground) > 0 {
		h.Playground = playground[0]
	}
	return newRouter(h)
}

// NewCapabilityHubRouter composes active modules without exposing legacy routes.
func NewCapabilityHubRouter(h Handlers) http.Handler { return newRouter(h) }

func newRouter(h Handlers) http.Handler {
	if h.Auth == nil {
		panic("http router requires an auth handler")
	}

	r := chi.NewRouter()
	r.Use(middleware.RequestID)
	r.Use(middleware.RealIP)
	r.Use(SanitizedAccessLogger)
	r.Use(middleware.Recoverer)
	r.Use(func(next http.Handler) http.Handler {
		return CORS(next, h.Auth.FrontendOrigin())
	})

	r.Get("/healthz", healthz)
	r.Get("/api/healthz", healthz)

	r.Route("/api/v1", func(r chi.Router) {
		// Model streams are bounded by playground.StreamTimeout inside the
		// service; every other API route keeps the default request timeout.
		r.Group(func(r chi.Router) {
			r.Use(h.Auth.Middleware)
			if h.Playground != nil {
				r.Post("/playground/chat/stream", h.Playground.Stream)
			}
			if h.SkillBuilder != nil {
				r.Post("/skill-builder/turn", h.SkillBuilder.Turn)
			}
		})
		r = r.With(middleware.Timeout(time.Minute))

		if h.Knowledge != nil {
			r.Post("/knowledge/search", h.Knowledge.MachineSearch)
		}
		r.Route("/auth/feishu", func(r chi.Router) {
			r.Get("/start", h.Auth.Start)
			r.Get("/callback", h.Auth.Callback)
		})

		r.Group(func(r chi.Router) {
			r.Use(h.Auth.Middleware)
			r.Get("/auth/me", h.Auth.Me)
			r.Post("/auth/logout", h.Auth.Logout)
			if h.Knowledge != nil {
				r.Get("/knowledge/bases", h.Knowledge.Bases)
				r.Post("/knowledge/bases", h.Knowledge.Bases)
				r.Put("/knowledge/bases/{id}", h.Knowledge.Bases)
				r.Delete("/knowledge/bases/{id}", h.Knowledge.DeleteBase)
				r.Get("/knowledge/bases/{id}/documents", h.Knowledge.Documents)
				r.Post("/knowledge/bases/{id}/documents", h.Knowledge.Upload)
				r.Post("/knowledge/bases/{id}/documents/{documentID}/versions", h.Knowledge.Upload)
				r.Delete("/knowledge/documents/{id}", h.Knowledge.DeleteDocument)
				r.Post("/knowledge/documents/{id}/retry", h.Knowledge.Retry)
				r.Get("/knowledge/versions/{id}", h.Knowledge.Preview)
				r.Post("/knowledge/test-search", h.Knowledge.Search)
				r.Get("/knowledge/applications", h.Knowledge.Applications)
				r.Get("/knowledge/audit", h.Knowledge.Audit)
				r.Post("/knowledge/applications", h.Knowledge.Applications)
				r.Put("/knowledge/applications/{id}", h.Knowledge.Applications)
				r.Post("/knowledge/applications/{id}/credentials", h.Knowledge.Rotate)
				r.Delete("/knowledge/applications/{id}/credentials", h.Knowledge.Revoke)
			}
			if h.Playground != nil {
				r.Get("/playground/connection", h.Playground.Get)
				r.Put("/playground/connection", h.Playground.Save)
				r.Delete("/playground/connection", h.Playground.Delete)
				r.Post("/playground/models", h.Playground.Models)
			}

			if h.Registry != nil {
				r.Get("/catalog", h.Registry.Catalog)
				r.Get("/catalog/{slug}", h.Registry.Published)
				r.Get("/catalog/{slug}/template", h.Registry.SkillTemplate)
				r.Get("/governance/me", h.Registry.Identity)
				r.Get("/governance/reviews", h.Registry.Reviews)
				r.Get("/governance/audit", h.Registry.Audit)
				r.Get("/governance/profiles", h.Registry.Profiles)
				r.Put("/governance/profiles/{openID}/department", h.Registry.SetDepartment)
				for _, action := range []string{"submit", "approve", "reject", "offline", "new-draft", "force-publish", "visibility"} {
					r.Post("/capabilities/{slug}/"+action, h.Registry.Action(action))
				}
				r.Get("/capabilities/{slug}/versions/{versionID}/files", h.Registry.Files)
				r.Get("/capabilities", h.Registry.List)
				r.Post("/capabilities", h.Registry.Save)
				r.Get("/capabilities/{slug}", h.Registry.Get)
				r.Put("/capabilities/{slug}", h.Registry.Save)
				r.Get("/capabilities/{slug}/versions/{versionID}/bundle", h.Registry.Download)
			}

			if h.SkillBuilder != nil {
				r.Post("/skill-builder/drafts", h.SkillBuilder.Save)
			}
		})
	})

	return r
}

func healthz(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(map[string]any{
		"status": "ok",
		"time":   time.Now().UTC().Format(time.RFC3339),
	})
}
