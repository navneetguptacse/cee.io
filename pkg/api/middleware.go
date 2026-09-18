package api

import (
	"encoding/json"
	"log/slog"
	"net"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

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

// AuthMiddleware validates authentication tokens against configured tokens.
func AuthMiddleware(cfg *config.Config) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if len(cfg.Auth.Tokens) == 0 {
				next.ServeHTTP(w, r)
				return
			}

			var clientToken string
			for _, header := range cfg.Auth.Headers {
				if val := r.Header.Get(header); val != "" {
					clientToken = val
					break
				}
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
