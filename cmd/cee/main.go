package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"cee.io/pkg/api"
	"cee.io/pkg/config"
	"cee.io/pkg/executor"
	"cee.io/pkg/languages"
	"cee.io/pkg/queue"
	"github.com/spf13/cobra"
)

var rootCmd = &cobra.Command{
	Use:   "cee",
	Short: "CEE — High-performance Code Execution Engine & CLI",
	Long:  `CEE is an ultra-low latency, production-ready, Judge0-compatible code execution engine and CLI tool built in Go.`,
}

func main() {
	// Setup structured JSON logger
	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo}))
	slog.SetDefault(logger)

	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}

func init() {
	rootCmd.AddCommand(newServerCmd())
	rootCmd.AddCommand(newWorkerCmd())
	rootCmd.AddCommand(newRunCmd())
	rootCmd.AddCommand(newSubmitCmd())
	rootCmd.AddCommand(newStatusCmd())
	rootCmd.AddCommand(newLanguagesCmd())
	rootCmd.AddCommand(newHealthCmd())
	rootCmd.AddCommand(newSelfTestCmd())
}

// ── 1. cee server ─────────────────────────────────────────────────────────────
func newServerCmd() *cobra.Command {
	var port int
	var redisURL string
	var concurrency int
	var execType string

	cmd := &cobra.Command{
		Use:   "server",
		Short: "Start the CEE API server and execution workers",
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg := config.Load()
			if cmd.Flags().Changed("port") && port > 0 {
				cfg.Server.Port = port
			}
			if cmd.Flags().Changed("redis") && redisURL != "" {
				cfg.Redis.URL = redisURL
			}
			if cmd.Flags().Changed("workers") && concurrency > 0 {
				cfg.Worker.Concurrency = concurrency
			}
			if cmd.Flags().Changed("executor") && execType != "" {
				cfg.Executor.Type = execType
			}

			// Initialize Executor
			execEngine, err := executor.NewExecutor(cfg)
			if err != nil {
				return fmt.Errorf("failed to initialize executor: %w", err)
			}

			// Initialize Queue (Redis if configured, otherwise high-speed in-memory)
			var q queue.Queue
			if cfg.Redis.URL != "" && cfg.Redis.URL != "none" {
				rq, err := queue.NewRedisQueue(cfg.Redis.URL, cfg.Cache.ResultTTLSeconds)
				if err != nil {
					slog.Warn("redis_unavailable_fallback_memory", "error", err)
					q = queue.NewMemoryQueue(10000)
				} else {
					slog.Info("connected_to_redis", "url", cfg.Redis.URL)
					q = rq
				}
			} else {
				slog.Info("using_in_memory_channel_queue")
				q = queue.NewMemoryQueue(10000)
			}
			defer q.Close()

			// Start Worker Pool
			workerPool := queue.NewWorkerPool(q, execEngine, cfg.Worker.Concurrency)
			workerPool.Start()
			defer workerPool.Stop()

			// Setup Router
			router := api.NewRouter(cfg, q, execEngine)
			addr := fmt.Sprintf(":%d", cfg.Server.Port)
			server := &http.Server{
				Addr:         addr,
				Handler:      router,
				ReadTimeout:  60 * time.Second,
				WriteTimeout: 60 * time.Second,
			}

			slog.Info("cee_server_starting",
				"port", cfg.Server.Port,
				"env", cfg.Server.Env,
				"executor", execEngine.Type(),
				"concurrency", cfg.Worker.Concurrency,
			)

			// Graceful shutdown on SIGTERM / SIGINT
			stopCh := make(chan os.Signal, 1)
			signal.Notify(stopCh, syscall.SIGINT, syscall.SIGTERM)

			go func() {
				if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
					slog.Error("server_fatal_error", "error", err)
					os.Exit(1)
				}
			}()

			fmt.Printf("CEE API Server listening on http://localhost:%d\n", cfg.Server.Port)
			fmt.Printf("Prometheus Metrics: http://localhost:%d/metrics\n", cfg.Server.Port)
			fmt.Printf("Health Check: http://localhost:%d/health\n", cfg.Server.Port)

			<-stopCh
			slog.Info("shutting_down_cee_server")

			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()
			return server.Shutdown(ctx)
		},
	}

	cmd.Flags().IntVarP(&port, "port", "p", 3000, "Port to bind API server")
	cmd.Flags().StringVar(&redisURL, "redis", "", "Redis URL (leave empty for in-memory queue)")
	cmd.Flags().IntVarP(&concurrency, "concurrency", "c", 4, "Number of worker goroutines")
	cmd.Flags().StringVarP(&execType, "executor", "e", "auto", "Executor type (auto, docker, isolate, process)")
	return cmd
}

// ── 2. cee worker ─────────────────────────────────────────────────────────────
func newWorkerCmd() *cobra.Command {
	var redisURL string
	var concurrency int
	var execType string

	cmd := &cobra.Command{
		Use:   "worker",
		Short: "Start a standalone CEE queue worker",
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg := config.Load()
			if cmd.Flags().Changed("redis") && redisURL != "" {
				cfg.Redis.URL = redisURL
			}
			if cmd.Flags().Changed("workers") && concurrency > 0 {
				cfg.Worker.Concurrency = concurrency
			}
			if cmd.Flags().Changed("executor") && execType != "" {
				cfg.Executor.Type = execType
			}

			execEngine, err := executor.NewExecutor(cfg)
			if err != nil {
				return err
			}

			rq, err := queue.NewRedisQueue(cfg.Redis.URL, cfg.Cache.ResultTTLSeconds)
			if err != nil {
				return fmt.Errorf("worker requires accessible Redis: %w", err)
			}
			defer rq.Close()

			workerPool := queue.NewWorkerPool(rq, execEngine, cfg.Worker.Concurrency)
			workerPool.Start()

			fmt.Printf("CEE Worker running (concurrency: %d, executor: %s)\n", cfg.Worker.Concurrency, execEngine.Type())

			stopCh := make(chan os.Signal, 1)
			signal.Notify(stopCh, syscall.SIGINT, syscall.SIGTERM)
			<-stopCh

			workerPool.Stop()
			return nil
		},
	}

	cmd.Flags().StringVar(&redisURL, "redis", "redis://localhost:6379", "Redis connection URL")
	cmd.Flags().IntVarP(&concurrency, "concurrency", "c", 4, "Number of worker goroutines")
	cmd.Flags().StringVarP(&execType, "executor", "e", "auto", "Executor type")
	return cmd
}

// ── 3. cee run <file> ─────────────────────────────────────────────────────────
func newRunCmd() *cobra.Command {
	var stdin string
	var expectedOutput string
	var timeout float64
	var execType string

	cmd := &cobra.Command{
		Use:   "run <source-file>",
		Short: "Directly compile and execute a local source file",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			filePath := args[0]
			content, err := os.ReadFile(filePath)
			if err != nil {
				return fmt.Errorf("failed reading file %s: %w", filePath, err)
			}

			langID := detectLanguageID(filePath)
			if langID == 0 {
				return fmt.Errorf("could not detect language for file extension: %s", filepath.Ext(filePath))
			}
			lang := languages.GetLanguageByID(langID)

			cfg := config.Load()
			if execType != "" {
				cfg.Executor.Type = execType
			} else {
				cfg.Executor.Type = "process" // fast native for one-shot CLI
			}

			execEngine, err := executor.NewExecutor(cfg)
			if err != nil {
				return err
			}

			if timeout <= 0 {
				timeout = 5.0
			}

			sub := &executor.ExecutionSubmission{
				Token:                  "cli-run",
				SourceCode:             string(content),
				Language:               lang,
				Stdin:                  stdin,
				ExpectedOutput:         expectedOutput,
				CPUTimeLimit:           timeout,
				CPUExtraTime:           1.0,
				WallTimeLimit:          timeout + 2.0,
				MemoryLimit:            256000,
				StackLimit:             64000,
				MaxProcesses:           60,
				MaxFileSize:            1024,
				RedirectStderrToStdout: false,
			}

			fmt.Printf("Executing %s (%s) with %s...\n", filepath.Base(filePath), lang.Name, execEngine.Type())
			start := time.Now()
			res, err := execEngine.Execute(context.Background(), sub)
			if err != nil {
				return fmt.Errorf("execution error: %w", err)
			}

			duration := time.Since(start)

			fmt.Println("\n──────────────────── Execution Result ────────────────────")
			fmt.Printf("Status:    %s (ID: %d)\n", res.Status.Description, res.Status.ID)
			if res.ExitCode != nil {
				fmt.Printf("Exit Code: %d\n", *res.ExitCode)
			}
			if res.WallTime != nil {
				fmt.Printf("Wall Time: %ss (total CLI: %v)\n", *res.WallTime, duration.Round(time.Millisecond))
			}

			if res.CompileOutput != nil && *res.CompileOutput != "" {
				fmt.Printf("\n[Compilation Output]:\n%s\n", *res.CompileOutput)
			}

			if res.Stdout != nil && *res.Stdout != "" {
				fmt.Printf("\n[Stdout]:\n%s", *res.Stdout)
				if !strings.HasSuffix(*res.Stdout, "\n") {
					fmt.Println()
				}
			}

			if res.Stderr != nil && *res.Stderr != "" {
				fmt.Printf("\n[Stderr]:\n%s", *res.Stderr)
				if !strings.HasSuffix(*res.Stderr, "\n") {
					fmt.Println()
				}
			}
			fmt.Println("──────────────────────────────────────────────────────────")

			return nil
		},
	}

	cmd.Flags().StringVar(&stdin, "stdin", "", "Standard input for the program")
	cmd.Flags().StringVar(&expectedOutput, "expected", "", "Expected output to compare against")
	cmd.Flags().Float64VarP(&timeout, "timeout", "t", 5.0, "Execution timeout in seconds")
	cmd.Flags().StringVarP(&execType, "executor", "e", "process", "Executor type (process, docker)")
	return cmd
}

// ── 4. cee submit <file> ──────────────────────────────────────────────────────
func newSubmitCmd() *cobra.Command {
	var apiURL string
	var authToken string
	var stdin string
	var wait bool
	var langID int

	cmd := &cobra.Command{
		Use:   "submit <source-file>",
		Short: "Submit code to a running CEE server",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			filePath := args[0]
			content, err := os.ReadFile(filePath)
			if err != nil {
				return err
			}

			if langID == 0 {
				langID = detectLanguageID(filePath)
				if langID == 0 {
					return fmt.Errorf("specify --lang ID")
				}
			}

			codeStr := string(content)
			subReq := api.SubmissionRequest{
				SourceCode: &codeStr,
				LanguageID: langID,
			}
			if stdin != "" {
				subReq.Stdin = &stdin
			}

			bodyBytes, _ := json.Marshal(subReq)
			endpoint := fmt.Sprintf("%s/submissions", apiURL)
			if wait {
				endpoint += "?wait=true"
			}

			req, err := http.NewRequest(http.MethodPost, endpoint, bytes.NewReader(bodyBytes))
			if err != nil {
				return err
			}
			req.Header.Set("Content-Type", "application/json")
			if authToken != "" {
				req.Header.Set("X-Auth-Token", authToken)
			}

			client := &http.Client{Timeout: 35 * time.Second}
			resp, err := client.Do(req)
			if err != nil {
				return fmt.Errorf("failed connecting to CEE server: %w", err)
			}
			defer resp.Body.Close()

			respBody, _ := io.ReadAll(resp.Body)
			var prettyJSON bytes.Buffer
			_ = json.Indent(&prettyJSON, respBody, "", "  ")

			fmt.Printf("HTTP %d:\n%s\n", resp.StatusCode, prettyJSON.String())
			return nil
		},
	}

	cmd.Flags().StringVar(&apiURL, "url", "http://localhost:3000", "CEE server URL")
	cmd.Flags().StringVar(&authToken, "token", "", "Authentication token")
	cmd.Flags().StringVar(&stdin, "stdin", "", "Standard input")
	cmd.Flags().BoolVarP(&wait, "wait", "w", true, "Wait for execution completion")
	cmd.Flags().IntVarP(&langID, "lang", "l", 0, "Language ID (auto-detected if omitted)")
	return cmd
}

// ── 5. cee status <token> ─────────────────────────────────────────────────────
func newStatusCmd() *cobra.Command {
	var apiURL string
	var authToken string

	cmd := &cobra.Command{
		Use:   "status <token>",
		Short: "Fetch status of a submission by token",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			token := args[0]
			endpoint := fmt.Sprintf("%s/submissions/%s", apiURL, token)

			req, _ := http.NewRequest(http.MethodGet, endpoint, nil)
			if authToken != "" {
				req.Header.Set("X-Auth-Token", authToken)
			}

			client := &http.Client{Timeout: 10 * time.Second}
			resp, err := client.Do(req)
			if err != nil {
				return err
			}
			defer resp.Body.Close()

			respBody, _ := io.ReadAll(resp.Body)
			var prettyJSON bytes.Buffer
			_ = json.Indent(&prettyJSON, respBody, "", "  ")

			fmt.Printf("%s\n", prettyJSON.String())
			return nil
		},
	}

	cmd.Flags().StringVar(&apiURL, "url", "http://localhost:3000", "CEE server URL")
	cmd.Flags().StringVar(&authToken, "token", "", "Authentication token")
	return cmd
}

// ── 6. cee languages ──────────────────────────────────────────────────────────
func newLanguagesCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "languages",
		Short: "List all supported programming languages",
		Run: func(cmd *cobra.Command, args []string) {
			langs := languages.GetActiveLanguages()
			fmt.Println("ID    LANGUAGE                        DEFAULT COMPILER / RUNNER")
			fmt.Println("─────────────────────────────────────────────────────────────────")
			for _, l := range langs {
				runner := l.RunCmd
				if l.CompileCmd != "" {
					runner = l.CompileCmd + " && " + l.RunCmd
				}
				if len(runner) > 38 {
					runner = runner[:35] + "..."
				}
				fmt.Printf("%-5d %-31s %s\n", l.ID, l.Name, runner)
			}
		},
	}
}

// ── 7. cee health ─────────────────────────────────────────────────────────────
func newHealthCmd() *cobra.Command {
	var apiURL string
	cmd := &cobra.Command{
		Use:   "health",
		Short: "Check health of CEE API server",
		RunE: func(cmd *cobra.Command, args []string) error {
			resp, err := http.Get(apiURL + "/health")
			if err != nil {
				return fmt.Errorf("health check failed: %w", err)
			}
			defer resp.Body.Close()

			b, _ := io.ReadAll(resp.Body)
			fmt.Printf("HTTP %d: %s\n", resp.StatusCode, string(b))
			return nil
		},
	}
	cmd.Flags().StringVar(&apiURL, "url", "http://localhost:3000", "CEE server URL")
	return cmd
}

// ── 8. cee test ───────────────────────────────────────────────────────────────
func newSelfTestCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "test",
		Short: "Run self-diagnostic execution test suite",
		RunE: func(cmd *cobra.Command, args []string) error {
			fmt.Println("Running CEE Self-Diagnostics...")

			cfg := config.Load()
			cfg.Executor.Type = "process"
			execEngine, err := executor.NewExecutor(cfg)
			if err != nil {
				return err
			}

			// Test 1: Python
			pySub := &executor.ExecutionSubmission{
				Token:         "test-py",
				SourceCode:    "print(21 * 2)",
				Language:      languages.GetLanguageByID(languages.LangPython),
				CPUTimeLimit:  5,
				WallTimeLimit: 5,
			}
			res, err := execEngine.Execute(context.Background(), pySub)
			if err != nil || res.Status.ID != languages.StatusAccepted || res.Stdout == nil || strings.TrimSpace(*res.Stdout) != "42" {
				return fmt.Errorf("python self-test failed: res=%+v, err=%v", res, err)
			}
			fmt.Println("  [PASS] Python execution verified (21 * 2 = 42)")

			// Test 2: Timeout detection
			timeoutSub := &executor.ExecutionSubmission{
				Token:         "test-timeout",
				SourceCode:    "import time; time.sleep(10)",
				Language:      languages.GetLanguageByID(languages.LangPython),
				CPUTimeLimit:  1,
				WallTimeLimit: 1,
			}
			tRes, err := execEngine.Execute(context.Background(), timeoutSub)
			if err != nil || tRes.Status.ID != languages.StatusTimeLimitExceeded {
				return fmt.Errorf("timeout self-test failed: expected TLE, got %+v", tRes)
			}
			fmt.Println("  [PASS] Timeout enforcement verified (Time Limit Exceeded handled)")

			fmt.Println("All CEE core self-diagnostic checks passed.")
			return nil
		},
	}
}

func detectLanguageID(path string) int {
	ext := strings.ToLower(filepath.Ext(path))
	switch ext {
	case ".py":
		return languages.LangPython
	case ".js", ".mjs":
		return languages.LangJavaScript
	case ".ts":
		return languages.LangTypeScript
	case ".go":
		return languages.LangGo
	case ".cpp", ".cc", ".cxx":
		return languages.LangCPP
	case ".c":
		return languages.LangC
	case ".java":
		return languages.LangJava
	case ".rs":
		return languages.LangRust
	case ".sh":
		return languages.LangBash
	case ".zip":
		return languages.LangMultiFile
	default:
		return 0
	}
}
