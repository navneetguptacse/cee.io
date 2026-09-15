package config

import (
	"os"
	"strconv"
	"strings"
)

type Config struct {
	Server struct {
		Port      int
		Env       string
		BodyLimit int64
	}
	Redis struct {
		URL string
	}
	Auth struct {
		Tokens  []string
		Headers []string
	}
	Execution struct {
		DefaultCPUTimeLimit    float64
		MaxCPUTimeLimit        float64
		DefaultWallTimeLimit   float64
		MaxWallTimeLimit       float64
		DefaultMemoryLimit     int
		MaxMemoryLimit         int
		MaxProcesses           int
		MaxSourceSize          int
		MaxAdditionalFilesSize int
		DefaultStackLimit      int
		CompileCPUTimeLimit    float64
		CompileWallTimeLimit   float64
	}
	Worker struct {
		Concurrency int
	}
	Cache struct {
		ResultTTLSeconds int
	}
	RateLimit struct {
		WindowSeconds int
		MaxRequests   int
	}
	Docker struct {
		SocketPath string
	}
	Executor struct {
		Type string // auto, docker, isolate, process
	}
}

func getEnv(key, defaultVal string) string {
	if val, ok := os.LookupEnv(key); ok && val != "" {
		return val
	}
	return defaultVal
}

func getEnvInt(key string, defaultVal int) int {
	if val, ok := os.LookupEnv(key); ok && val != "" {
		if i, err := strconv.Atoi(val); err == nil {
			return i
		}
	}
	return defaultVal
}

func getEnvFloat(key string, defaultVal float64) float64 {
	if val, ok := os.LookupEnv(key); ok && val != "" {
		if f, err := strconv.ParseFloat(val, 64); err == nil {
			return f
		}
	}
	return defaultVal
}

// Load loads configuration from environment variables with safe defaults.
func Load() *Config {
	cfg := &Config{}

	cfg.Server.Port = getEnvInt("PORT", 3000)
	cfg.Server.Env = getEnv("NODE_ENV", getEnv("ENV", "development"))
	cfg.Server.BodyLimit = int64(getEnvInt("BODY_LIMIT_MB", 50)) * 1024 * 1024

	cfg.Redis.URL = getEnv("REDIS_URL", "")

	rawTokens := getEnv("AUTH_TOKEN", "")
	if rawTokens != "" {
		cfg.Auth.Tokens = strings.Fields(rawTokens)
	}
	cfg.Auth.Headers = strings.Fields(getEnv("AUTH_HEADERS", "X-Auth-Token x-rapidapi-key"))

	cfg.Execution.DefaultCPUTimeLimit = getEnvFloat("DEFAULT_CPU_TIME_LIMIT", 5.0)
	cfg.Execution.MaxCPUTimeLimit = getEnvFloat("MAX_CPU_TIME_LIMIT", 15.0)
	cfg.Execution.DefaultWallTimeLimit = getEnvFloat("DEFAULT_WALL_TIME_LIMIT", 10.0)
	cfg.Execution.MaxWallTimeLimit = getEnvFloat("MAX_WALL_TIME_LIMIT", 30.0)
	cfg.Execution.DefaultMemoryLimit = getEnvInt("DEFAULT_MEMORY_LIMIT", 128000) // in KB
	cfg.Execution.MaxMemoryLimit = getEnvInt("MAX_MEMORY_LIMIT", 512000)         // in KB
	cfg.Execution.MaxProcesses = getEnvInt("MAX_PROCESSES", 60)
	cfg.Execution.MaxSourceSize = getEnvInt("MAX_SOURCE_SIZE", 65536)
	cfg.Execution.MaxAdditionalFilesSize = getEnvInt("MAX_ADDITIONAL_FILES_SIZE", 2097152) // 2MB base64
	cfg.Execution.DefaultStackLimit = getEnvInt("DEFAULT_STACK_LIMIT", 64000)
	cfg.Execution.CompileCPUTimeLimit = getEnvFloat("COMPILE_CPU_TIME_LIMIT", 15.0)
	cfg.Execution.CompileWallTimeLimit = getEnvFloat("COMPILE_WALL_TIME_LIMIT", 30.0)

	cfg.Worker.Concurrency = getEnvInt("WORKER_CONCURRENCY", 4)
	cfg.Cache.ResultTTLSeconds = getEnvInt("RESULT_CACHE_TTL", 3600)

	cfg.RateLimit.WindowSeconds = getEnvInt("RATE_LIMIT_WINDOW_SECONDS", 60)
	cfg.RateLimit.MaxRequests = getEnvInt("RATE_LIMIT_MAX_REQUESTS", 200)

	cfg.Docker.SocketPath = getEnv("DOCKER_SOCKET", "/var/run/docker.sock")
	cfg.Executor.Type = strings.ToLower(getEnv("EXECUTOR_TYPE", "auto"))

	return cfg
}
