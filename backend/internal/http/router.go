package http

import (
	"encoding/json"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
)

type Handlers struct {
	KB           *KBHandler
	Doc          *DocumentHandler
	Chunk        *ChunkHandler
	Chat         *ChatHandler
	Auth         *AuthHandler
	Feishu       *FeishuHandler
	Playground   *PlaygroundHandler
	Registry     *RegistryHandler
	SkillBuilder *SkillBuilderHandler
}

// NewRouter retains the legacy composition for migration regression tests.
func NewRouter(h Handlers) http.Handler {
	return newRouter(h, true)
}

// NewPlatformRouter exposes only the active Capability Hub surface.
func NewPlatformRouter(auth *AuthHandler, playground ...*PlaygroundHandler) http.Handler {
	h := Handlers{Auth: auth}
	if len(playground) > 0 {
		h.Playground = playground[0]
	}
	return newRouter(h, false)
}

// NewCapabilityHubRouter composes active modules without exposing legacy routes.
func NewCapabilityHubRouter(h Handlers) http.Handler { return newRouter(h, false) }

func newRouter(h Handlers, legacy bool) http.Handler {
	if h.Auth == nil {
		panic("http router requires an auth handler")
	}

	r := chi.NewRouter()
	r.Use(middleware.RequestID)
	r.Use(middleware.RealIP)
	r.Use(SanitizedAccessLogger)
	r.Use(middleware.Recoverer)
	r.Use(middleware.Timeout(60 * time.Second))
	r.Use(func(next http.Handler) http.Handler {
		return CORS(next, h.Auth.FrontendOrigin())
	})

	r.Get("/healthz", healthz)
	r.Get("/api/healthz", healthz)

	r.Route("/api/v1", func(r chi.Router) {
		r.Route("/auth/feishu", func(r chi.Router) {
			r.Get("/start", h.Auth.Start)
			r.Get("/callback", h.Auth.Callback)
		})

		r.Group(func(r chi.Router) {
			r.Use(h.Auth.Middleware)
			r.Get("/auth/me", h.Auth.Me)
			r.Post("/auth/logout", h.Auth.Logout)
			if h.Playground != nil {
				r.Get("/playground/connection", h.Playground.Get)
				r.Put("/playground/connection", h.Playground.Save)
				r.Delete("/playground/connection", h.Playground.Delete)
				r.Post("/playground/models", h.Playground.Models)
				r.Post("/playground/chat/stream", h.Playground.Stream)
			}

			if h.Registry != nil {
				r.Get("/catalog", h.Registry.Catalog)
				r.Get("/catalog/{slug}", h.Registry.Published)
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
				r.Post("/skill-builder/turn", h.SkillBuilder.Turn)
				r.Post("/skill-builder/drafts", h.SkillBuilder.Save)
			}

			if !legacy {
				return
			}

			r.Post("/kbs", h.KB.Create)
			r.Get("/kbs", h.KB.List)
			r.Get("/kbs/{id}", h.KB.Get)
			r.Delete("/kbs/{id}", h.KB.Delete)

			r.Post("/kbs/{id}/docs", h.Doc.Upload)
			r.Post("/kbs/{kbID}/feishu-imports", h.Feishu.Import)
			r.Get("/kbs/{id}/docs", h.Doc.ListByKB)
			r.Get("/docs/{id}", h.Doc.Get)
			r.Delete("/docs/{id}", h.Doc.Delete)

			r.Get("/docs/{id}/chunks", h.Chunk.ListByDoc)

			r.Post("/docs/{id}/reingest", h.Doc.Reingest)
			r.Post("/docs/{docID}/sync", h.Feishu.Sync)
			r.Post("/kbs/{id}/reingest", h.Doc.ReingestKB)

			r.Get("/kbs/{kbID}/conversations", h.Chat.ListConversations)
			r.Post("/kbs/{kbID}/conversations", h.Chat.CreateConversation)
			r.Patch("/conversations/{conversationID}", h.Chat.UpdateConversation)
			r.Get("/conversations/{conversationID}/messages", h.Chat.ListMessages)
			r.Post("/conversations/{conversationID}/messages/stream", h.Chat.StreamMessage)
			r.Get("/kbs/{kbID}/chunks/{chunkID}/neighbors", h.Chunk.Neighbors)
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
