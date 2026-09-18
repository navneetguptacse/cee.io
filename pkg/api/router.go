package api

import (
	"net/http"

	"cee.io/pkg/auth"
	"cee.io/pkg/config"
	"cee.io/pkg/executor"
	"cee.io/pkg/metrics"
	"cee.io/pkg/queue"
	"github.com/go-chi/chi/v5"
	chimiddleware "github.com/go-chi/chi/v5/middleware"
)

// NewRouter constructs and configures the HTTP router with role-based API key management.
func NewRouter(cfg *config.Config, q queue.Queue, exec executor.Executor, keyStore ...auth.Store) http.Handler {
	var store auth.Store
	if len(keyStore) > 0 && keyStore[0] != nil {
		store = keyStore[0]
	} else {
		store, _ = auth.NewStore(cfg)
	}

	r := chi.NewRouter()
	h := NewHandler(cfg, q, exec, store)

	// Global Middlewares
	r.Use(chimiddleware.RealIP)
	r.Use(Recoverer)
	r.Use(RequestLogger)
	r.Use(CORS)
	r.Use(RateLimiterMiddleware(cfg))

	// Public Endpoints (no auth required)
	r.Get("/health", h.Health)

	// Metrics Endpoint (requires Auth Master or Metrics Master)
	r.With(RequireMetrics(cfg, store)).Method(http.MethodGet, "/metrics", metrics.Handler())

	// Authenticated API Groups
	r.Group(func(pr chi.Router) {
		pr.Use(AuthMiddleware(cfg, store))

		// Capabilities / Whoami (accessible by any valid active credential)
		pr.Get("/api/capabilities", h.GetCapabilities)

		// API Key Management routes
		pr.Route("/api/keys", func(kr chi.Router) {
			kr.Post("/", h.GenerateKey) // Generates key: Master can create any; Guest can create Guest
			kr.With(RequireMasterAuth).Get("/", h.ListKeys)
			kr.With(RequireMasterAuth).Delete("/{id}", h.RevokeKey)
		})

		// Normal Application APIs (accessible by Auth Master and Auth Guest)
		pr.Group(func(ar chi.Router) {
			ar.Use(RequireNormalAPI)

			// System metadata
			ar.Get("/about", h.About)
			ar.Get("/system_info", h.SystemInfo)
			ar.Get("/config_info", h.ConfigInfo)
			ar.Get("/executor", h.ExecutorInfo)
			ar.Get("/workers", h.Workers)
			ar.Get("/statistics", h.Statistics)

			// Statuses
			ar.Get("/statuses", h.Statuses)

			// Languages
			ar.Get("/languages", h.Languages)
			ar.Get("/languages/all", h.LanguagesAll)
			ar.Get("/languages/{id}", h.LanguageByID)

			// Submissions
			ar.Route("/submissions", func(sr chi.Router) {
				sr.Post("/", h.CreateSubmission)
				sr.Post("/batch", h.CreateBatchSubmission)
				sr.Get("/batch", h.GetBatchSubmission)
				sr.Get("/{token}", h.GetSubmission)
				sr.Delete("/{token}", h.DeleteSubmission)
			})
		})
	})

	return r
}
