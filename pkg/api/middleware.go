package api

import (
	"context"
	"encoding/json"
	"log/slog"
	"net"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"cee.io/pkg/auth"
	"cee.io/pkg/config"
)

// ResponseWriter wrapper to capture status code
type statusResponseWriter struct {
	http.ResponseWriter
	statusCode int
}

func (rw *statusResponseWriter) WriteHeader(code int) {
	rw.statusCode = code
	rw.ResponseWriter.WriteHeader(code)
}

// RequestLogger logs incoming HTTP requests with latency and status code.
func RequestLogger(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		srw := &statusResponseWriter{ResponseWriter: w, statusCode: http.StatusOK}

		next.ServeHTTP(srw, r)

		slog.Info("http_request",
			"method", r.Method,
			"url", r.URL.Path,
			"status", srw.statusCode,
			"duration_ms", time.Since(start).Milliseconds(),
			"ip", getClientIP(r),
		)
	})
}

// Recoverer catches panics and returns a 500 response.
func Recoverer(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if rec := recover(); rec != nil {
				slog.Error("panic_recovered", "error", rec)
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusInternalServerError)
				_ = json.NewEncoder(w).Encode(ErrorResponse{
					Error:   "Internal Server Error",
					Message: "An unexpected server error occurred",
				})
			}
		}()
		next.ServeHTTP(w, r)
	})
}

// CORS adds permissive CORS headers for API integrations.
func CORS(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, X-Auth-Token, x-rapidapi-key, Authorization")

		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusOK)
			return
		}

		next.ServeHTTP(w, r)
	})
}

type contextKey string

const (
	KeyContextKey contextKey = "cee_auth_key"
)

// GetKeyFromContext retrieves the authenticated Key from the request context.
func GetKeyFromContext(ctx context.Context) *auth.Key {
	if val := ctx.Value(KeyContextKey); val != nil {
		if k, ok := val.(*auth.Key); ok {
			return k
		}
	}
	return nil
}

func extractToken(r *http.Request, customHeaders []string) string {
	// 1. Authorization: Bearer <token>
	if authHeader := r.Header.Get("Authorization"); authHeader != "" {
		parts := strings.SplitN(authHeader, " ", 2)
		if len(parts) == 2 && strings.EqualFold(parts[0], "Bearer") {
			return strings.TrimSpace(parts[1])
		}
		return strings.TrimSpace(authHeader)
	}

	// 2. Standard token headers
	for _, h := range []string{"X-Auth-Token", "x-rapidapi-key", "X-Metrics-Token"} {
		if val := r.Header.Get(h); val != "" {
			return strings.TrimSpace(val)
		}
	}

	// 3. Custom configured headers
	for _, h := range customHeaders {
		if val := r.Header.Get(h); val != "" {
			return strings.TrimSpace(val)
		}
	}

	return ""
}

// AuthMiddleware validates authentication tokens against the auth.Store or configured tokens.
func AuthMiddleware(cfg *config.Config, store auth.Store) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			clientToken := extractToken(r, cfg.Auth.Headers)

			if len(cfg.Auth.Tokens) == 0 && clientToken == "" {
				next.ServeHTTP(w, r)
				return
			}

			if clientToken == "" {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusUnauthorized)
				_ = json.NewEncoder(w).Encode(ErrorResponse{
					Error:   "Authentication required",
					Message: "Missing authentication header",
				})
				return
			}

			if store != nil {
				hash := auth.HashKey(clientToken)
				k, err := store.GetKeyByHash(r.Context(), hash)
				if err != nil || k.Status != auth.StatusActive {
					w.Header().Set("Content-Type", "application/json")
					w.WriteHeader(http.StatusUnauthorized)
					_ = json.NewEncoder(w).Encode(ErrorResponse{
						Error:   "Authentication failed",
						Message: "Invalid or revoked authentication token",
					})
					return
				}

				// Record last used asynchronously
				go func(id string) {
					_ = store.TouchKey(context.Background(), id)
				}(k.ID)

				ctx := context.WithValue(r.Context(), KeyContextKey, k)
				next.ServeHTTP(w, r.WithContext(ctx))
				return
			}

			// Fallback legacy authentication check if store is nil
			authorized := false
			for _, token := range cfg.Auth.Tokens {
				if token == clientToken {
					authorized = true
					break
				}
			}

			if !authorized {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusUnauthorized)
				_ = json.NewEncoder(w).Encode(ErrorResponse{
					Error:   "Authentication failed",
					Message: "Invalid authentication token",
				})
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}

// RequireNormalAPI ensures the caller has permission to access normal application APIs.
func RequireNormalAPI(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		k := GetKeyFromContext(r.Context())
		if k == nil {
			next.ServeHTTP(w, r)
			return
		}
		if !auth.HasPermission(k, auth.PermAPIAccess) {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusForbidden)
			_ = json.NewEncoder(w).Encode(ErrorResponse{
				Error:   "Forbidden",
				Message: "Your credential is not authorized to access application APIs",
			})
			return
		}
		next.ServeHTTP(w, r)
	})
}

// RequireMasterAuth ensures only Master AUTH credentials can access the route.
func RequireMasterAuth(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		k := GetKeyFromContext(r.Context())
		if k == nil || k.Type != auth.TypeAuth || k.Role != auth.RoleMaster {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusForbidden)
			_ = json.NewEncoder(w).Encode(ErrorResponse{
				Error:   "Forbidden",
				Message: "Master AUTH API key required for this operation",
			})
			return
		}
		next.ServeHTTP(w, r)
	})
}

// RequireMetrics protects metrics endpoints allowing only Auth Master and Metrics Master.
func RequireMetrics(cfg *config.Config, store auth.Store) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			clientToken := extractToken(r, cfg.Auth.Headers)
			if clientToken == "" {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusUnauthorized)
				_ = json.NewEncoder(w).Encode(ErrorResponse{
					Error:   "Authentication required",
					Message: "Metrics endpoint requires authentication",
				})
				return
			}

			if store != nil {
				hash := auth.HashKey(clientToken)
				k, err := store.GetKeyByHash(r.Context(), hash)
				if err != nil || k.Status != auth.StatusActive {
					w.Header().Set("Content-Type", "application/json")
					w.WriteHeader(http.StatusUnauthorized)
					_ = json.NewEncoder(w).Encode(ErrorResponse{
						Error:   "Authentication failed",
						Message: "Invalid or revoked metrics credential",
					})
					return
				}

				if !auth.HasPermission(k, auth.PermMetricsAccess) {
					w.Header().Set("Content-Type", "application/json")
					w.WriteHeader(http.StatusForbidden)
					_ = json.NewEncoder(w).Encode(ErrorResponse{
						Error:   "Forbidden",
						Message: "Guest credentials are not authorized to access metrics",
					})
					return
				}

				ctx := context.WithValue(r.Context(), KeyContextKey, k)
				next.ServeHTTP(w, r.WithContext(ctx))
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}

type rateLimiterClient struct {
	windowStart time.Time
	count       int
}

type rateLimiter struct {
	mu      sync.Mutex
	clients map[string]*rateLimiterClient
	window  time.Duration
	limit   int
}

// RateLimiterMiddleware provides sliding window IP rate limiting.
func RateLimiterMiddleware(cfg *config.Config) func(http.Handler) http.Handler {
	rl := &rateLimiter{
		clients: make(map[string]*rateLimiterClient),
		window:  time.Duration(cfg.RateLimit.WindowSeconds) * time.Second,
		limit:   cfg.RateLimit.MaxRequests,
	}

	// Periodic cleanup
	go func() {
		ticker := time.NewTicker(time.Minute)
		for range ticker.C {
			rl.mu.Lock()
			now := time.Now()
			for ip, client := range rl.clients {
				if now.Sub(client.windowStart) > rl.window {
					delete(rl.clients, ip)
				}
			}
			rl.mu.Unlock()
		}
	}()

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			ip := getClientIP(r)
			now := time.Now()

			rl.mu.Lock()
			client, exists := rl.clients[ip]
			if !exists || now.Sub(client.windowStart) > rl.window {
				client = &rateLimiterClient{windowStart: now, count: 0}
				rl.clients[ip] = client
			}
			client.count++
			currentCount := client.count
			windowStart := client.windowStart
			rl.mu.Unlock()

			remaining := rl.limit - currentCount
			if remaining < 0 {
				remaining = 0
			}
			resetUnix := windowStart.Add(rl.window).Unix()

			w.Header().Set("X-RateLimit-Limit", strconv.Itoa(rl.limit))
			w.Header().Set("X-RateLimit-Remaining", strconv.Itoa(remaining))
			w.Header().Set("X-RateLimit-Reset", strconv.FormatInt(resetUnix, 10))

			if currentCount > rl.limit {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusTooManyRequests)
				_ = json.NewEncoder(w).Encode(ErrorResponse{
					Error:   "Too Many Requests",
					Message: "Rate limit exceeded. Please try again later.",
				})
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}

var trustedProxyCIDRs []*net.IPNet

func init() {
	cidrs := []string{
		"127.0.0.0/8",
		"::1/128",
		"10.0.0.0/8",
		"172.16.0.0/12",
		"192.168.0.0/16",
	}
	for _, c := range cidrs {
		if _, ipNet, err := net.ParseCIDR(c); err == nil {
			trustedProxyCIDRs = append(trustedProxyCIDRs, ipNet)
		}
	}
}

func isTrustedProxy(remoteAddr string) bool {
	host, _, err := net.SplitHostPort(remoteAddr)
	if err != nil {
		host = remoteAddr
	}
	ip := net.ParseIP(host)
	if ip == nil {
		return false
	}
	for _, cidr := range trustedProxyCIDRs {
		if cidr.Contains(ip) {
			return true
		}
	}
	return false
}

func getClientIP(r *http.Request) string {
	remoteHost, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		remoteHost = r.RemoteAddr
	}

	// Only trust forwarding headers if the direct connecting peer is a trusted proxy
	if isTrustedProxy(r.RemoteAddr) {
		if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
			parts := strings.Split(xff, ",")
			clientIP := strings.TrimSpace(parts[0])
			if net.ParseIP(clientIP) != nil {
				return clientIP
			}
		}
		if xri := r.Header.Get("X-Real-IP"); xri != "" {
			clientIP := strings.TrimSpace(xri)
			if net.ParseIP(clientIP) != nil {
				return clientIP
			}
		}
	}

	return remoteHost
}
