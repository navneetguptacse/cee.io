package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"runtime"
	"strconv"
	"time"

	"cee.io/pkg/auth"
	"cee.io/pkg/config"
	"cee.io/pkg/executor"
	"cee.io/pkg/languages"
	"cee.io/pkg/queue"
	"github.com/go-chi/chi/v5"
)

var appStartTime = time.Now()

type Handler struct {
	cfg      *config.Config
	queue    queue.Queue
	executor executor.Executor
	keyStore auth.Store
}

func NewHandler(cfg *config.Config, q queue.Queue, exec executor.Executor, keyStore auth.Store) *Handler {
	return &Handler{
		cfg:      cfg,
		queue:    q,
		executor: exec,
		keyStore: keyStore,
	}
}

// GET /health
func (h *Handler) Health(w http.ResponseWriter, r *http.Request) {
	respondJSON(w, http.StatusOK, map[string]any{
		"status":    "healthy",
		"uptime":    time.Since(appStartTime).Seconds(),
		"timestamp": time.Now().UTC().Format(time.RFC3339),
	})
}

// GET /about
func (h *Handler) About(w http.ResponseWriter, r *http.Request) {
	respondJSON(w, http.StatusOK, map[string]any{
		"version":     "1.0.0",
		"engine":      "CEE (Code Execution Engine in Go)",
		"homepage":    "https://github.com/navneetguptacse/cee",
		"maintainer":  "Navneet Gupta <navneetguptacse@gmail.com>",
		"compatible":  "Judge0 API v1.13.0",
		"performance": "Ultra-low latency compiled Go runtime",
	})
}

// GET /system_info
func (h *Handler) SystemInfo(w http.ResponseWriter, r *http.Request) {
	var m runtime.MemStats
	runtime.ReadMemStats(&m)

	hostname, _ := os.Hostname()

	respondJSON(w, http.StatusOK, map[string]any{
		"system": map[string]any{
			"platform": runtime.GOOS,
			"arch":     runtime.GOARCH,
			"hostname": hostname,
			"num_cpu":  runtime.NumCPU(),
		},
		"memory": map[string]any{
			"alloc_mb":       m.Alloc / 1024 / 1024,
			"total_alloc_mb": m.TotalAlloc / 1024 / 1024,
			"sys_mb":         m.Sys / 1024 / 1024,
			"num_gc":         m.NumGC,
		},
		"uptime": map[string]any{
			"process": time.Since(appStartTime).Seconds(),
		},
	})
}

// GET /config_info
func (h *Handler) ConfigInfo(w http.ResponseWriter, r *http.Request) {
	respondJSON(w, http.StatusOK, map[string]any{
		"max_cpu_time_limit":               h.cfg.Execution.MaxCPUTimeLimit,
		"max_cpu_extra_time":               5.0,
		"max_wall_time_limit":              h.cfg.Execution.MaxWallTimeLimit,
		"max_memory_limit":                 h.cfg.Execution.MaxMemoryLimit,
		"max_stack_limit":                  h.cfg.Execution.DefaultStackLimit,
		"max_max_processes_and_or_threads": h.cfg.Execution.MaxProcesses,
		"max_max_file_size":                4096,
		"cpu_time_limit":                   h.cfg.Execution.DefaultCPUTimeLimit,
		"cpu_extra_time":                   1.0,
		"wall_time_limit":                  h.cfg.Execution.DefaultWallTimeLimit,
		"memory_limit":                     h.cfg.Execution.DefaultMemoryLimit,
		"stack_limit":                      h.cfg.Execution.DefaultStackLimit,
		"max_processes_and_or_threads":     h.cfg.Execution.MaxProcesses,
		"max_file_size":                    1024,
		"enable_network":                   false,
		"allow_enable_network":             false,
		"enable_additional_files":          true,
	})
}

// GET /executor
func (h *Handler) ExecutorInfo(w http.ResponseWriter, r *http.Request) {
	caps := executor.GetSystemCapabilities(h.cfg.Docker.SocketPath, h.executor.Type())
	respondJSON(w, http.StatusOK, map[string]any{
		"current_executor": h.executor.Type(),
		"configured":       h.cfg.Executor.Type,
		"recommended":      caps.Recommended,
		"capabilities": map[string]any{
			"docker":  caps.Docker,
			"isolate": caps.Isolate,
			"process": caps.Process,
		},
		"platform": caps.Platform,
		"arch":     caps.Arch,
	})
}

// GET /workers
func (h *Handler) Workers(w http.ResponseWriter, r *http.Request) {
	workers := make([]map[string]any, h.cfg.Worker.Concurrency)
	for i := 0; i < h.cfg.Worker.Concurrency; i++ {
		workers[i] = map[string]any{
			"id":     fmt.Sprintf("worker-%d", i+1),
			"name":   fmt.Sprintf("cee-worker-%d", i+1),
			"active": true,
		}
	}
	respondJSON(w, http.StatusOK, workers)
}

// GET /statistics
func (h *Handler) Statistics(w http.ResponseWriter, r *http.Request) {
	stats, err := h.queue.GetStats(r.Context())
	if err != nil {
		respondJSON(w, http.StatusOK, map[string]any{
			"submissions": map[string]int{"total": 0, "in_queue": 0, "processing": 0, "completed": 0},
		})
		return
	}
	respondJSON(w, http.StatusOK, map[string]any{
		"submissions": stats,
	})
}

// GET /statuses
func (h *Handler) Statuses(w http.ResponseWriter, r *http.Request) {
	respondJSON(w, http.StatusOK, languages.GetAllStatuses())
}

// GET /languages
func (h *Handler) Languages(w http.ResponseWriter, r *http.Request) {
	langs := languages.GetActiveLanguages()
	res := make([]map[string]any, len(langs))
	for i, l := range langs {
		res[i] = map[string]any{
			"id":   l.ID,
			"name": l.Name,
		}
	}
	respondJSON(w, http.StatusOK, res)
}

// GET /languages/all
func (h *Handler) LanguagesAll(w http.ResponseWriter, r *http.Request) {
	langs := languages.GetAllLanguages()
	res := make([]map[string]any, len(langs))
	for i, l := range langs {
		res[i] = map[string]any{
			"id":          l.ID,
			"name":        l.Name,
			"is_archived": l.IsArchived,
		}
	}
	respondJSON(w, http.StatusOK, res)
}

// GET /languages/{id}
func (h *Handler) LanguageByID(w http.ResponseWriter, r *http.Request) {
	idStr := chi.URLParam(r, "id")
	id, err := strconv.Atoi(idStr)
	if err != nil {
		respondError(w, http.StatusBadRequest, "Bad Request", "Invalid language ID")
		return
	}

	lang := languages.GetLanguageByID(id)
	if lang == nil {
		respondError(w, http.StatusNotFound, "Not Found", fmt.Sprintf("Language with ID %d not found", id))
		return
	}

	respondJSON(w, http.StatusOK, lang)
}

func respondJSON(w http.ResponseWriter, code int, payload any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(payload)
}

func respondError(w http.ResponseWriter, code int, errType string, message string) {
	respondJSON(w, code, ErrorResponse{Error: errType, Message: message})
}
