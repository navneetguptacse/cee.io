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
	r.Get("/install.sh", h.InstallScript)
	r.Get("/download/{filename}", h.DownloadBinary)

	// Metrics Endpoint (requires Auth Master or Metrics Master)
	r.With(RequireMetrics(cfg, store)).Method(http.MethodGet, "/metrics", metrics.Handler())

	// Authenticated API Groups
	r.Group(func(pr chi.Router) {
		pr.Use(AuthMiddleware(cfg, store))

		// Capabilities / Whoami (accessible by any valid active credential)
		pr.Get("/api/capabilities", h.GetCapabilities)

		// Auth validation / login / logout endpoints
		pr.Post("/api/auth/login", h.Login)
		pr.Post("/api/auth/verify", h.Login)
		pr.Post("/api/auth/logout", h.Logout)

		// API Key Management routes
		pr.Route("/api/keys", func(kr chi.Router) {
			kr.Post("/", h.GenerateKey) // Generates key: Master can create any; Guest can create Guest
			kr.With(RequireMasterAuth).Get("/", h.ListKeys)
			kr.With(RequireMasterAuth).Delete("/{id}", h.RevokeKey)
		})

		// Normal Application APIs (accessible by Auth Master and Auth Guest)
		pr.Group(func(ar chi.Router) {
			ar.Use(RequireNormalAPI)

			mountAPIRoutes := func(r chi.Router) {
				// System metadata
				r.Get("/about", h.About)
				r.Get("/system_info", h.SystemInfo)
				r.Get("/config_info", h.ConfigInfo)
				r.Get("/executor", h.ExecutorInfo)
				r.Get("/workers", h.Workers)
				r.Get("/statistics", h.Statistics)

				// Statuses
				r.Get("/statuses", h.Statuses)

				// Languages
				r.Get("/languages", h.Languages)
				r.Get("/languages/all", h.LanguagesAll)
				r.Get("/languages/{id}", h.LanguageByID)

				// Submissions
				r.Route("/submissions", func(sr chi.Router) {
					sr.Post("/", h.CreateSubmission)
					sr.Post("/batch", h.CreateBatchSubmission)
					sr.Get("/batch", h.GetBatchSubmission)
					sr.Get("/{token}", h.GetSubmission)
					sr.Delete("/{token}", h.DeleteSubmission)
				})
			}

			// Mount on root and /v1 prefix
			mountAPIRoutes(ar)
			ar.Route("/v1", func(vr chi.Router) {
				mountAPIRoutes(vr)
			})
		})
	})

	return r
}
