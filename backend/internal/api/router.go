package api

import (
	"log/slog"
	"net/http"

	chimw "github.com/go-chi/chi/v5/middleware"

	"github.com/go-chi/chi/v5"

	"github.com/devwithfarshi/painless-agent/internal/api/handlers"
	"github.com/devwithfarshi/painless-agent/internal/api/middleware"
	"github.com/devwithfarshi/painless-agent/pkg/config"
)

// NewRouter builds and returns the chi router wired with all handlers and middleware.
func NewRouter(cfg *config.Config, h *handlers.Handlers, log *slog.Logger) http.Handler {
	r := chi.NewRouter()

	// Global middleware (runs before auth so healthchecks are not rate-limited).
	r.Use(chimw.RealIP)
	r.Use(chimw.RequestID)
	r.Use(chimw.Recoverer)
	r.Use(middleware.Logging(log))
	r.Use(middleware.CORS(cfg.CORSOrigins))
	r.Use(middleware.RateLimit(cfg.RateLimitRPM))

	r.Route("/api", func(r chi.Router) {
		// Auth is applied to all API routes.
		r.Use(middleware.Auth(cfg.APIKey))

		r.Get("/health", h.Health)

		r.Route("/tasks", func(r chi.Router) {
			r.Get("/", h.ListTasks)
			r.Post("/", h.CreateTask)
			r.Route("/{id}", func(r chi.Router) {
				r.Get("/", h.GetTask)
				r.Delete("/", h.CancelTask)
				r.Get("/stream", h.StreamTask)
			})
		})

		r.Route("/memory", func(r chi.Router) {
			r.Get("/search", h.SearchMemory)
		})

		r.Route("/skills", func(r chi.Router) {
			r.Get("/", h.ListSkills)
			r.Route("/{id}", func(r chi.Router) {
				r.Get("/", h.GetSkill)
				r.Delete("/", h.DeleteSkill)
			})
		})

		r.Post("/config/provider", h.SetProvider)

		r.Route("/workspace", func(r chi.Router) {
			r.Get("/", h.ListWorkspaceFiles)
			r.Get("/file", h.GetWorkspaceFile)
		})
	})

	return r
}
