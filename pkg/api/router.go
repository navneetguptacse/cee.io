package api

import (
	"net/http"

	"cee.io/pkg/config"
	"cee.io/pkg/executor"
	"cee.io/pkg/metrics"
	"cee.io/pkg/queue"
	"github.com/go-chi/chi/v5"
	chimiddleware "github.com/go-chi/chi/v5/middleware"
)

// NewRouter constructs and configures the HTTP router.
func NewRouter(cfg *config.Config, q queue.Queue, exec executor.Executor) http.Handler {
	r := chi.NewRouter()
	h := NewHandler(cfg, q, exec)

	// Global Middlewares
	r.Use(chimiddleware.RealIP)
	r.Use(Recoverer)
	r.Use(RequestLogger)
	r.Use(CORS)
	r.Use(RateLimiterMiddleware(cfg))

	// Public Endpoints (no auth required)
	r.Get("/health", h.Health)
	r.Method(http.MethodGet, "/metrics", metrics.Handler())

	// Protected Endpoints
	r.Group(func(pr chi.Router) {
		pr.Use(AuthMiddleware(cfg))

		// System metadata
		pr.Get("/about", h.About)
		pr.Get("/system_info", h.SystemInfo)
		pr.Get("/config_info", h.ConfigInfo)
		pr.Get("/executor", h.ExecutorInfo)
		pr.Get("/workers", h.Workers)
		pr.Get("/statistics", h.Statistics)

		// Statuses
		pr.Get("/statuses", h.Statuses)

		// Languages
		pr.Get("/languages", h.Languages)
		pr.Get("/languages/all", h.LanguagesAll)
		pr.Get("/languages/{id}", h.LanguageByID)

		// Submissions - Notice /batch routes are mounted before /{token} routes
		pr.Route("/submissions", func(sr chi.Router) {
			sr.Post("/", h.CreateSubmission)
			sr.Post("/batch", h.CreateBatchSubmission)
			sr.Get("/batch", h.GetBatchSubmission)
			sr.Get("/{token}", h.GetSubmission)
			sr.Delete("/{token}", h.DeleteSubmission)
		})
	})

	return r
}
