package main

import (
	"bufio"
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
	"strconv"
	"strings"
	"syscall"
	"time"

	"cee.io/pkg/api"
	"cee.io/pkg/auth"
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
	logger := slog.New(slog.NewJSONHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelInfo}))
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
	rootCmd.AddCommand(newConnectCmd())
	rootCmd.AddCommand(newConfigCmd())
	rootCmd.AddCommand(newTokenCmd())
	rootCmd.AddCommand(newAuthCmd())
	rootCmd.AddCommand(newLogoutCmd())
}

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
			if (cmd.Flags().Changed("concurrency") || cmd.Flags().Changed("workers")) && concurrency > 0 {
				cfg.Worker.Concurrency = concurrency
			}
			if cmd.Flags().Changed("executor") && execType != "" {
				cfg.Executor.Type = execType
			}

			execEngine, err := executor.NewExecutor(cfg)
			if err != nil {
				return fmt.Errorf("failed to initialize executor: %w", err)
			}

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

			workerPool := queue.NewWorkerPool(q, execEngine, cfg.Worker.Concurrency)
			workerPool.Start()
			defer workerPool.Stop()

			keyStore, err := auth.NewStore(cfg)
			if err != nil {
				return fmt.Errorf("failed to initialize key store: %w", err)
			}
			defer keyStore.Close()

			router := api.NewRouter(cfg, q, execEngine, keyStore)
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
	cmd.Flags().StringVarP(&redisURL, "redis", "r", "", "Redis URL (leave empty for in-memory queue)")
	cmd.Flags().IntVarP(&concurrency, "concurrency", "c", 4, "Number of worker goroutines")
	cmd.Flags().IntVar(&concurrency, "workers", 4, "Number of worker goroutines (alias for --concurrency)")
	_ = cmd.Flags().MarkHidden("workers")
	cmd.Flags().StringVarP(&execType, "executor", "e", "auto", "Executor type (auto, docker, isolate, process)")
	return cmd
}

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
			if (cmd.Flags().Changed("concurrency") || cmd.Flags().Changed("workers")) && concurrency > 0 {
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

	cmd.Flags().StringVarP(&redisURL, "redis", "r", "redis://localhost:6379", "Redis connection URL")
	cmd.Flags().IntVarP(&concurrency, "concurrency", "c", 4, "Number of worker goroutines")
	cmd.Flags().IntVar(&concurrency, "workers", 4, "Number of worker goroutines (alias for --concurrency)")
	_ = cmd.Flags().MarkHidden("workers")
	cmd.Flags().StringVarP(&execType, "executor", "e", "auto", "Executor type")
	return cmd
}

func newRunCmd() *cobra.Command {
	var input string
	var stdinAlias string
	var expectedOutput string
	var outputAlias string
	var timeout float64
	var execType string
	var codeFlag string
	var langFlag string

	cmd := &cobra.Command{
		Use:   "run [source-file]",
		Short: "Compile and execute a local source file or inline code",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if input == "" && stdinAlias != "" {
				input = stdinAlias
			}
			if expectedOutput == "" && outputAlias != "" {
				expectedOutput = outputAlias
			}

			var code string
			var lang *languages.Language
			var label string

			if langFlag != "" {
				lang = parseLanguage(langFlag)
				if lang == nil {
					return fmt.Errorf("unrecognized language: %s (run 'cee languages' for supported languages)", langFlag)
				}
			}

			if codeFlag != "" {
				code = codeFlag
				label = "inline"
			} else if len(args) > 0 {
				arg := args[0]
				if fi, err := os.Stat(arg); err == nil && !fi.IsDir() {
					content, err := os.ReadFile(arg)
					if err != nil {
						return fmt.Errorf("failed reading file %s: %w", arg, err)
					}
					code = string(content)
					label = filepath.Base(arg)
					if lang == nil {
						langID := detectLanguageID(arg)
						if langID == 0 {
							return fmt.Errorf("could not detect language for file extension: %s (use -l to specify)", filepath.Ext(arg))
						}
						lang = languages.GetLanguageByID(langID)
					}
				} else if looksLikeFilePath(arg) {
					return fmt.Errorf("file not found: %s", arg)
				} else {
					code = arg
					label = "inline"
				}
			} else {
				stat, err := os.Stdin.Stat()
				if err == nil && (stat.Mode()&os.ModeCharDevice) == 0 {
					bytes, err := io.ReadAll(os.Stdin)
					if err != nil {
						return fmt.Errorf("failed reading from stdin: %w", err)
					}
					code = string(bytes)
					label = "stdin"
				} else {
					return fmt.Errorf("no source code provided: specify a file, use -c \"code\", or pipe code via stdin")
				}
			}

			if strings.TrimSpace(code) == "" {
				return fmt.Errorf("source code is empty")
			}

			if lang == nil {
				return fmt.Errorf("language not specified: please pass -l or --lang (e.g. -l py, -l js, -l go, -l cpp)")
			}

			cfg := config.Load()
			clientCfg := loadClientConfig()

			// Check host language availability and dynamically fallback
			isHostAvailable, missingBin := lang.IsHostAvailable()

			selectedExecType := execType
			if selectedExecType == "" || selectedExecType == "auto" {
				if isHostAvailable {
					selectedExecType = "process"
				} else {
					fmt.Printf("Language runtime (%s) not found on host.\n", missingBin)
					if executor.IsDockerAvailable(cfg.Docker.SocketPath) {
						fmt.Printf("-> Falling back to Docker container (%s)...\n", lang.Image)
						selectedExecType = "docker"
					} else if clientCfg.APIURL != "" && clientCfg.APIURL != "http://localhost:3000" {
						fmt.Printf("-> Docker is not running. Falling back to remote CEE server (%s)...\n", clientCfg.APIURL)
						return submitCodeToRemote(clientCfg.APIURL, clientCfg.AuthToken, code, lang, input, expectedOutput, timeout)
					} else {
						return fmt.Errorf("language '%s' is not installed locally (%s missing), and Docker is not running.\nPlease install %s or start Docker to execute this code", lang.Name, missingBin, missingBin)
					}
				}
			}

			cfg.Executor.Type = selectedExecType
			execEngine, err := executor.NewExecutor(cfg)
			if err != nil {
				return err
			}

			if timeout <= 0 {
				timeout = 5.0
			}

			sub := &executor.ExecutionSubmission{
				Token:                  "cli-run",
				SourceCode:             code,
				Language:               lang,
				Stdin:                  input,
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

			fmt.Printf("Executing %s (%s) with %s...\n", label, lang.Name, execEngine.Type())
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

			if expectedOutput != "" && res.Status.ID == languages.StatusWrongAnswer {
				fmt.Printf("\n[Expected Output]:\n%s", expectedOutput)
				if !strings.HasSuffix(expectedOutput, "\n") {
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

	cmd.Flags().StringVarP(&codeFlag, "code", "c", "", "Inline source code string")
	cmd.Flags().StringVarP(&langFlag, "lang", "l", "", "Language name or ID (e.g. py, js, go, cpp, rs)")
	cmd.Flags().StringVarP(&input, "input", "i", "", "Standard input for the program")
	cmd.Flags().StringVar(&stdinAlias, "stdin", "", "Standard input (alias for -i, --input)")
	cmd.Flags().StringVarP(&expectedOutput, "expected", "o", "", "Expected output to compare against")
	cmd.Flags().StringVar(&outputAlias, "output", "", "Expected output (alias for -o, --expected)")
	cmd.Flags().Float64VarP(&timeout, "timeout", "t", 5.0, "Execution timeout in seconds")
	cmd.Flags().StringVarP(&execType, "executor", "e", "auto", "Executor type (auto, process, docker)")
	return cmd
}

// ── 4. cee submit [file] ──────────────────────────────────────────────────────
// ── 4. cee submit [file] ──────────────────────────────────────────────────────
func newSubmitCmd() *cobra.Command {
	clientCfg := loadClientConfig()
	var apiURL string
	var authToken string
	var input string
	var stdinAlias string
	var expectedOutput string
	var outputAlias string
	var wait bool
	var codeFlag string
	var langFlag string

	cmd := &cobra.Command{
		Use:   "submit [source-file]",
		Short: "Submit code to a running CEE server",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if input == "" && stdinAlias != "" {
				input = stdinAlias
			}
			if expectedOutput == "" && outputAlias != "" {
				expectedOutput = outputAlias
			}

			var code string
			var lang *languages.Language

			if langFlag != "" {
				lang = parseLanguage(langFlag)
				if lang == nil {
					return fmt.Errorf("unrecognized language: %s (run 'cee languages' for supported languages)", langFlag)
				}
			}

			if codeFlag != "" {
				code = codeFlag
			} else if len(args) > 0 {
				arg := args[0]
				if fi, err := os.Stat(arg); err == nil && !fi.IsDir() {
					content, err := os.ReadFile(arg)
					if err != nil {
						return err
					}
					code = string(content)
					if lang == nil {
						langID := detectLanguageID(arg)
						if langID == 0 {
							return fmt.Errorf("could not detect language for file: %s (use -l to specify)", arg)
						}
						lang = languages.GetLanguageByID(langID)
					}
				} else if looksLikeFilePath(arg) {
					return fmt.Errorf("file not found: %s", arg)
				} else {
					code = arg
				}
			} else {
				stat, err := os.Stdin.Stat()
				if err == nil && (stat.Mode()&os.ModeCharDevice) == 0 {
					bytes, err := io.ReadAll(os.Stdin)
					if err != nil {
						return fmt.Errorf("failed reading from stdin: %w", err)
					}
					code = string(bytes)
				} else {
					return fmt.Errorf("no source code provided: specify a file, use -c \"code\", or pipe code via stdin")
				}
			}

			if strings.TrimSpace(code) == "" {
				return fmt.Errorf("source code is empty")
			}

			if lang == nil {
				return fmt.Errorf("language not specified: please pass -l or --lang (e.g. -l py, -l js, -l go)")
			}

			subReq := api.SubmissionRequest{
				SourceCode: &code,
				LanguageID: lang.ID,
			}
			if input != "" {
				subReq.Stdin = &input
			}
			if expectedOutput != "" {
				subReq.ExpectedOutput = &expectedOutput
			}

			bodyBytes, _ := json.Marshal(subReq)
			endpoint := fmt.Sprintf("%s/submissions", strings.TrimRight(apiURL, "/"))
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

	cmd.Flags().StringVarP(&apiURL, "url", "u", clientCfg.APIURL, "CEE server URL")
	cmd.Flags().StringVarP(&authToken, "token", "t", clientCfg.AuthToken, "Authentication token")
	cmd.Flags().StringVarP(&codeFlag, "code", "c", "", "Inline source code string")
	cmd.Flags().StringVarP(&langFlag, "lang", "l", "", "Language name or ID (e.g. python, py, js, 71)")
	cmd.Flags().StringVarP(&input, "input", "i", "", "Standard input for the program")
	cmd.Flags().StringVar(&stdinAlias, "stdin", "", "Standard input (alias for -i, --input)")
	cmd.Flags().StringVarP(&expectedOutput, "expected", "o", "", "Expected output to compare against")
	cmd.Flags().StringVar(&outputAlias, "output", "", "Expected output (alias for -o, --expected)")
	cmd.Flags().BoolVarP(&wait, "wait", "w", true, "Wait for execution completion")
	return cmd
}

func newStatusCmd() *cobra.Command {
	clientCfg := loadClientConfig()
	var apiURL string
	var authToken string

	cmd := &cobra.Command{
		Use:   "status <token>",
		Short: "Fetch status of a submission by token",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			token := args[0]
			endpoint := fmt.Sprintf("%s/submissions/%s", strings.TrimRight(apiURL, "/"), token)

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

	cmd.Flags().StringVarP(&apiURL, "url", "u", clientCfg.APIURL, "CEE server URL")
	cmd.Flags().StringVarP(&authToken, "token", "t", clientCfg.AuthToken, "Authentication token")
	return cmd
}

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

func newHealthCmd() *cobra.Command {
	clientCfg := loadClientConfig()
	var apiURL string
	cmd := &cobra.Command{
		Use:   "health",
		Short: "Check health of CEE API server",
		RunE: func(cmd *cobra.Command, args []string) error {
			resp, err := http.Get(strings.TrimRight(apiURL, "/") + "/health")
			if err != nil {
				return fmt.Errorf("health check failed: %w", err)
			}
			defer resp.Body.Close()

			b, _ := io.ReadAll(resp.Body)
			fmt.Printf("HTTP %d: %s\n", resp.StatusCode, string(b))
			return nil
		},
	}
	cmd.Flags().StringVarP(&apiURL, "url", "u", clientCfg.APIURL, "CEE server URL")
	return cmd
}

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

func parseLanguage(s string) *languages.Language {
	s = strings.TrimSpace(strings.ToLower(s))
	if s == "" {
		return nil
	}

	if id, err := strconv.Atoi(s); err == nil {
		return languages.GetLanguageByID(id)
	}

	switch s {
	case "py", "python", "python3":
		return languages.GetLanguageByID(languages.LangPython)
	case "js", "javascript", "node", "nodejs":
		return languages.GetLanguageByID(languages.LangJavaScript)
	case "ts", "typescript":
		return languages.GetLanguageByID(languages.LangTypeScript)
	case "go", "golang":
		return languages.GetLanguageByID(languages.LangGo)
	case "cpp", "c++", "g++":
		return languages.GetLanguageByID(languages.LangCPP)
	case "c", "gcc":
		return languages.GetLanguageByID(languages.LangC)
	case "java", "openjdk":
		return languages.GetLanguageByID(languages.LangJava)
	case "rs", "rust":
		return languages.GetLanguageByID(languages.LangRust)
	case "sh", "bash", "shell":
		return languages.GetLanguageByID(languages.LangBash)
	default:
		for _, l := range languages.GetAllLanguages() {
			if strings.Contains(strings.ToLower(l.Name), s) {
				return l
			}
		}
		return nil
	}
}

func looksLikeFilePath(s string) bool {
	if strings.ContainsAny(s, "/\\") {
		return true
	}
	return filepath.Ext(s) != ""
}

// ClientConfig holds local CLI client preferences and server target.
type ClientConfig struct {
	APIURL       string `json:"api_url"`
	AuthToken    string `json:"auth_token,omitempty"`
	MetricsToken string `json:"metrics_token,omitempty"`
	Role         string `json:"role,omitempty"`
}

func getClientConfigFile() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return ".cee-config.json"
	}
	return filepath.Join(home, ".cee", "config.json")
}

func loadClientConfig() ClientConfig {
	cfg := ClientConfig{
		APIURL:       os.Getenv("CEE_API_URL"),
		AuthToken:    os.Getenv("CEE_AUTH_TOKEN"),
		MetricsToken: os.Getenv("CEE_METRICS_TOKEN"),
	}
	if cfg.APIURL == "" {
		cfg.APIURL = os.Getenv("CEE_URL")
	}
	if cfg.AuthToken == "" {
		cfg.AuthToken = os.Getenv("CEE_TOKEN")
	}
	if cfg.AuthToken == "" {
		cfg.AuthToken = os.Getenv("AUTH_TOKEN")
	}
	if cfg.MetricsToken == "" {
		cfg.MetricsToken = os.Getenv("METRICS_TOKEN")
	}

	configFile := getClientConfigFile()
	if data, err := os.ReadFile(configFile); err == nil {
		var fileCfg ClientConfig
		if err := json.Unmarshal(data, &fileCfg); err == nil {
			if cfg.APIURL == "" && fileCfg.APIURL != "" {
				cfg.APIURL = fileCfg.APIURL
			}
			if cfg.AuthToken == "" && fileCfg.AuthToken != "" {
				cfg.AuthToken = fileCfg.AuthToken
			}
			if cfg.MetricsToken == "" && fileCfg.MetricsToken != "" {
				cfg.MetricsToken = fileCfg.MetricsToken
			}
			if cfg.Role == "" && fileCfg.Role != "" {
				cfg.Role = fileCfg.Role
			}
		}
	}

	if cfg.APIURL == "" {
		cfg.APIURL = "http://localhost:3000"
	}
	return cfg
}

func saveClientConfig(cfg ClientConfig) error {
	configFile := getClientConfigFile()
	if err := os.MkdirAll(filepath.Dir(configFile), 0755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(configFile, data, 0600)
}

func newConnectCmd() *cobra.Command {
	var token string

	cmd := &cobra.Command{
		Use:   "connect <api-url>",
		Short: "Connect CLI to a remote CEE server and verify connection",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			rawURL := strings.TrimRight(args[0], "/")
			if !strings.HasPrefix(rawURL, "http://") && !strings.HasPrefix(rawURL, "https://") {
				rawURL = "https://" + rawURL
			}

			fmt.Printf("Probing CEE server at %s/health...\n", rawURL)
			client := &http.Client{Timeout: 5 * time.Second}
			resp, err := client.Get(rawURL + "/health")
			if err != nil {
				return fmt.Errorf("could not connect to %s: %w", rawURL, err)
			}
			defer resp.Body.Close()

			if resp.StatusCode != http.StatusOK {
				return fmt.Errorf("server at %s returned status %d (expected 200)", rawURL, resp.StatusCode)
			}

			existingCfg := loadClientConfig()
			cfg := ClientConfig{
				APIURL:       rawURL,
				AuthToken:    token,
				MetricsToken: existingCfg.MetricsToken,
				Role:         existingCfg.Role,
			}
			if token == "" && existingCfg.AuthToken != "" {
				cfg.AuthToken = existingCfg.AuthToken
			}

			if err := saveClientConfig(cfg); err != nil {
				return fmt.Errorf("failed saving config: %w", err)
			}

			fmt.Printf("Successfully connected! CEE CLI configured to use: %s\n", rawURL)
			fmt.Printf("Configuration saved to: %s\n", getClientConfigFile())
			return nil
		},
	}

	cmd.Flags().StringVarP(&token, "token", "t", "", "Authentication token for remote API")
	return cmd
}

func newConfigCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "config",
		Short: "Manage CLI configuration (API URL, tokens)",
	}

	cmd.AddCommand(&cobra.Command{
		Use:   "show",
		Short: "Display active CLI configuration",
		Run: func(cmd *cobra.Command, args []string) {
			cfg := loadClientConfig()
			fmt.Println("Active CEE CLI Configuration:")
			fmt.Printf("  API URL:       %s\n", cfg.APIURL)
			if cfg.AuthToken != "" {
				masked := cfg.AuthToken
				if len(masked) > 8 {
					masked = masked[:4] + "..." + masked[len(masked)-4:]
				}
				roleStr := ""
				if cfg.Role != "" {
					roleStr = fmt.Sprintf(" (Role: %s)", strings.ToUpper(cfg.Role))
				}
				fmt.Printf("  Auth Token:    %s%s\n", masked, roleStr)
			} else {
				fmt.Println("  Auth Token:    (none)")
			}

			if cfg.MetricsToken != "" {
				masked := cfg.MetricsToken
				if len(masked) > 8 {
					masked = masked[:4] + "..." + masked[len(masked)-4:]
				}
				fmt.Printf("  Metrics Token: %s\n", masked)
			} else {
				fmt.Println("  Metrics Token: (none - optional)")
			}

			fmt.Printf("  Config File:   %s\n", getClientConfigFile())
		},
	})

	cmd.AddCommand(&cobra.Command{
		Use:   "set-url <url>",
		Short: "Set the remote CEE API URL",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg := loadClientConfig()
			cfg.APIURL = strings.TrimRight(args[0], "/")
			if err := saveClientConfig(cfg); err != nil {
				return err
			}
			fmt.Printf("API URL updated to: %s\n", cfg.APIURL)
			return nil
		},
	})

	cmd.AddCommand(&cobra.Command{
		Use:   "set-token <token>",
		Short: "Set the remote CEE API authentication token",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg := loadClientConfig()
			cfg.AuthToken = args[0]
			if err := saveClientConfig(cfg); err != nil {
				return err
			}
			fmt.Println("Authentication token updated successfully.")
			return nil
		},
	})

	cmd.AddCommand(&cobra.Command{
		Use:   "set-metrics <token>",
		Short: "Set the remote CEE Metrics authentication token (Master only)",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg := loadClientConfig()
			if strings.EqualFold(cfg.Role, "guest") {
				return fmt.Errorf("permission denied: METRICS token is master-only and cannot be configured for Guest credentials")
			}
			cfg.MetricsToken = args[0]
			if err := saveClientConfig(cfg); err != nil {
				return err
			}
			fmt.Println("Metrics authentication token updated successfully.")
			return nil
		},
	})

	return cmd
}

func newAuthCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "auth",
		Short: "Authenticate and log in to CEE (guest or master)",
		Long:  "Log in with an AUTH API key as Guest or Master. Master credentials can also configure an optional Metrics key.",
		RunE: func(cmd *cobra.Command, args []string) error {
			return showAuthStatus()
		},
	}

	var masterKeyFlag string
	var masterMetricsFlag string
	var masterUrlFlag string

	masterCmd := &cobra.Command{
		Use:   "master [api-key]",
		Short: "Log in with a Master AUTH API key (full control, key generation, optional metrics)",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			key := masterKeyFlag
			if len(args) > 0 {
				key = args[0]
			}
			if key == "" {
				fmt.Print("Enter Master AUTH API Key: ")
				reader := bufio.NewReader(os.Stdin)
				input, _ := reader.ReadString('\n')
				key = strings.TrimSpace(input)
			}
			if key == "" {
				return fmt.Errorf("master API key is required")
			}
			return executeLogin("master", key, masterMetricsFlag, masterUrlFlag)
		},
	}
	masterCmd.Flags().StringVarP(&masterKeyFlag, "key", "k", "", "Master AUTH API key")
	masterCmd.Flags().StringVarP(&masterMetricsFlag, "metrics", "m", "", "Optional Master METRICS API key")
	masterCmd.Flags().StringVarP(&masterUrlFlag, "url", "u", "", "CEE server URL")

	var guestKeyFlag string
	var guestUrlFlag string

	guestCmd := &cobra.Command{
		Use:   "guest [api-key]",
		Short: "Log in with a Guest AUTH API key (normal execution & guest delegation)",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			key := guestKeyFlag
			if len(args) > 0 {
				key = args[0]
			}
			if key == "" {
				fmt.Print("Enter Guest AUTH API Key: ")
				reader := bufio.NewReader(os.Stdin)
				input, _ := reader.ReadString('\n')
				key = strings.TrimSpace(input)
			}
			if key == "" {
				return fmt.Errorf("guest API key is required")
			}
			return executeLogin("guest", key, "", guestUrlFlag)
		},
	}
	guestCmd.Flags().StringVarP(&guestKeyFlag, "key", "k", "", "Guest AUTH API key")
	guestCmd.Flags().StringVarP(&guestUrlFlag, "url", "u", "", "CEE server URL")

	var metricsUrlFlag string
	metricsCmd := &cobra.Command{
		Use:     "metrics [api-key]",
		Aliases: []string{"set-metrics"},
		Short:   "Set or update Master METRICS API key (Master only)",
		Args:    cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg := loadClientConfig()
			if strings.EqualFold(cfg.Role, "guest") {
				return fmt.Errorf("permission denied: METRICS API key is restricted to Master role. Current login is GUEST")
			}

			key := ""
			if len(args) > 0 {
				key = args[0]
			} else {
				fmt.Print("Enter Master METRICS API Key: ")
				reader := bufio.NewReader(os.Stdin)
				input, _ := reader.ReadString('\n')
				key = strings.TrimSpace(input)
			}
			if key == "" {
				return fmt.Errorf("metrics API key is required")
			}

			targetURL := metricsUrlFlag
			if targetURL == "" {
				targetURL = cfg.APIURL
			}
			if targetURL == "" {
				targetURL = "http://localhost:3000"
			}
			targetURL = strings.TrimRight(targetURL, "/")

			// Test metrics access against the server
			req, _ := http.NewRequest(http.MethodGet, targetURL+"/metrics", nil)
			req.Header.Set("X-Metrics-Token", key)
			req.Header.Set("X-Auth-Token", key)
			req.Header.Set("Authorization", "Bearer "+key)
			client := &http.Client{Timeout: 5 * time.Second}
			resp, err := client.Do(req)
			if err == nil {
				defer resp.Body.Close()
				if resp.StatusCode == http.StatusOK {
					fmt.Println("✓ Verified METRICS API key with server.")
				} else if resp.StatusCode == http.StatusForbidden || resp.StatusCode == http.StatusUnauthorized {
					return fmt.Errorf("server rejected METRICS key (HTTP %d): Invalid or unauthorized metrics token\n(Ensure METRICS_TOKEN matches the server configuration and containers have been restarted)", resp.StatusCode)
				}
			}

			if metricsUrlFlag != "" {
				cfg.APIURL = targetURL
			}
			cfg.MetricsToken = key
			if err := saveClientConfig(cfg); err != nil {
				return err
			}

			fmt.Println("✓ Metrics API Key configured successfully!")
			fmt.Printf("Prometheus Endpoint: %s/metrics\n", targetURL)
			return nil
		},
	}
	metricsCmd.Flags().StringVarP(&metricsUrlFlag, "url", "u", "", "CEE server URL")

	statusCmd := &cobra.Command{
		Use:     "status",
		Aliases: []string{"whoami"},
		Short:   "Display active authentication status and role permissions",
		RunE: func(cmd *cobra.Command, args []string) error {
			return showAuthStatus()
		},
	}

	logoutCmd := &cobra.Command{
		Use:   "logout",
		Short: "Clear active authentication credentials from local config",
		RunE: func(cmd *cobra.Command, args []string) error {
			return executeLogout()
		},
	}

	cmd.AddCommand(masterCmd)
	cmd.AddCommand(guestCmd)
	cmd.AddCommand(metricsCmd)
	cmd.AddCommand(statusCmd)
	cmd.AddCommand(logoutCmd)

	return cmd
}

func newLogoutCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "logout",
		Short: "Log out and clear active authentication credentials (alias for 'cee auth logout')",
		RunE: func(cmd *cobra.Command, args []string) error {
			return executeLogout()
		},
	}
}

func executeLogout() error {
	cfg := loadClientConfig()
	cfg.AuthToken = ""
	cfg.MetricsToken = ""
	cfg.Role = ""
	if err := saveClientConfig(cfg); err != nil {
		return err
	}
	fmt.Println("Successfully logged out. Local credentials cleared.")
	return nil
}

func executeLogin(expectedRole, key, metricsKey, urlFlag string) error {
	cfg := loadClientConfig()
	targetURL := urlFlag
	if targetURL == "" {
		targetURL = cfg.APIURL
	}
	if targetURL == "" {
		targetURL = "http://localhost:3000"
	}
	targetURL = strings.TrimRight(targetURL, "/")

	// Validate against server /api/capabilities
	client := &http.Client{Timeout: 8 * time.Second}
	req, err := http.NewRequest(http.MethodGet, targetURL+"/api/capabilities", nil)
	if err != nil {
		return err
	}
	req.Header.Set("X-Auth-Token", key)

	resp, err := client.Do(req)
	var capInfo auth.CapabilityInfo
	serverVerified := false

	if err == nil {
		defer resp.Body.Close()
		if resp.StatusCode == http.StatusOK {
			b, _ := io.ReadAll(resp.Body)
			if err := json.Unmarshal(b, &capInfo); err == nil {
				serverVerified = true
			}
		} else if resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden {
			return fmt.Errorf("authentication failed (HTTP %d): Invalid or revoked API key", resp.StatusCode)
		}
	}

	actualRole := expectedRole
	if serverVerified {
		actualRole = strings.ToLower(string(capInfo.Role))
		if expectedRole == "master" && actualRole == "guest" {
			return fmt.Errorf("permission mismatch: Provided key is a GUEST credential, not a MASTER key.\nRun 'cee auth guest <key>' to log in as Guest")
		}
	}

	cfg.APIURL = targetURL
	cfg.AuthToken = key
	cfg.Role = actualRole

	if metricsKey != "" {
		if actualRole == "guest" {
			return fmt.Errorf("invalid option: METRICS API keys cannot be configured for Guest credentials (Master only)")
		}

		// Verify metrics key against server if reachable
		reqM, _ := http.NewRequest(http.MethodGet, targetURL+"/metrics", nil)
		reqM.Header.Set("X-Metrics-Token", metricsKey)
		reqM.Header.Set("X-Auth-Token", metricsKey)
		reqM.Header.Set("Authorization", "Bearer "+metricsKey)
		if respM, errM := client.Do(reqM); errM == nil {
			defer respM.Body.Close()
			if respM.StatusCode == http.StatusOK {
				fmt.Println("✓ Verified METRICS API key with server.")
			} else if respM.StatusCode == http.StatusForbidden || respM.StatusCode == http.StatusUnauthorized {
				return fmt.Errorf("server rejected METRICS key (HTTP %d): Invalid or unauthorized metrics token\n(Ensure METRICS_TOKEN matches the server configuration and containers have been restarted)", respM.StatusCode)
			}
		}

		cfg.MetricsToken = metricsKey
	} else if actualRole == "guest" {
		cfg.MetricsToken = "" // Clear any existing metrics token when logging in as guest
	}

	if err := saveClientConfig(cfg); err != nil {
		return fmt.Errorf("failed saving config: %w", err)
	}

	fmt.Println()
	fmt.Printf("✓ Successfully logged in as %s!\n", strings.ToUpper(actualRole))
	fmt.Println("──────────────────────────────────────────────────────────")
	fmt.Printf("Server URL:      %s\n", targetURL)
	if serverVerified {
		fmt.Printf("Key ID:          %s\n", capInfo.KeyID)
		fmt.Printf("Prefix:          %s\n", capInfo.Prefix)
		fmt.Println("Server Status:   Verified online & active")
	} else {
		masked := key
		if len(masked) > 12 {
			masked = masked[:6] + "..." + masked[len(masked)-4:]
		}
		fmt.Printf("API Key:         %s\n", masked)
		fmt.Println("Server Status:   Saved locally (server was unreachable)")
	}
	if cfg.MetricsToken != "" {
		maskedM := cfg.MetricsToken
		if len(maskedM) > 12 {
			maskedM = maskedM[:6] + "..." + maskedM[len(maskedM)-4:]
		}
		fmt.Printf("Metrics Key:     %s (configured)\n", maskedM)
	}
	fmt.Println("──────────────────────────────────────────────────────────")
	fmt.Println("Role Permissions:")
	if actualRole == "master" {
		fmt.Println("  [ALLOWED] Normal API Execution (Run, Submit, Languages)")
		fmt.Println("  [ALLOWED] Generate Guest AUTH Keys")
		fmt.Println("  [ALLOWED] Generate Master AUTH Keys")
		fmt.Println("  [ALLOWED] Generate Metrics Keys")
		fmt.Println("  [ALLOWED] Revoke & Manage All Keys")
		if cfg.MetricsToken != "" {
			fmt.Println("  [ALLOWED] Metrics Access (Configured via METRICS key)")
		} else {
			fmt.Println("  [OPTIONAL] Metrics Access: Not configured (use 'cee auth metrics <key>')")
		}
	} else {
		fmt.Println("  [ALLOWED] Normal API Execution (Run, Submit, Languages)")
		fmt.Println("  [ALLOWED] Generate Guest AUTH Keys (Delegation)")
		fmt.Println("  [DENIED]  Generate Master Keys")
		fmt.Println("  [DENIED]  Generate Metrics Keys")
		fmt.Println("  [DENIED]  Revoke Keys")
		fmt.Println("  [DENIED]  Metrics API Access (Master only)")
	}
	fmt.Println("──────────────────────────────────────────────────────────")
	fmt.Printf("Configuration saved to: %s\n\n", getClientConfigFile())

	return nil
}

func showAuthStatus() error {
	cfg := loadClientConfig()
	if cfg.AuthToken == "" {
		fmt.Println("\nNot currently logged in.")
		fmt.Println("To authenticate:")
		fmt.Println("  cee auth master <api-key> [-m <metrics-key>]")
		fmt.Println("  cee auth guest <api-key>")
		fmt.Println()
		return nil
	}

	client := &http.Client{Timeout: 5 * time.Second}
	req, _ := http.NewRequest(http.MethodGet, cfg.APIURL+"/api/capabilities", nil)
	req.Header.Set("X-Auth-Token", cfg.AuthToken)

	resp, err := client.Do(req)
	var capInfo auth.CapabilityInfo
	serverOnline := false
	if err == nil {
		defer resp.Body.Close()
		if resp.StatusCode == http.StatusOK {
			b, _ := io.ReadAll(resp.Body)
			if err := json.Unmarshal(b, &capInfo); err == nil {
				serverOnline = true
			}
		}
	}

	role := cfg.Role
	if serverOnline && capInfo.Role != "" {
		role = string(capInfo.Role)
	}
	if role == "" {
		role = "master"
	}

	fmt.Println("\nActive CEE Authentication Session:")
	fmt.Println("──────────────────────────────────────────────────────────")
	fmt.Printf("Server URL:      %s\n", cfg.APIURL)
	fmt.Printf("Role:            %s\n", strings.ToUpper(role))
	if serverOnline {
		fmt.Printf("Key ID:          %s\n", capInfo.KeyID)
		fmt.Printf("Prefix:          %s\n", capInfo.Prefix)
		fmt.Println("Server Status:   Online & Active")
	} else {
		masked := cfg.AuthToken
		if len(masked) > 8 {
			masked = masked[:4] + "..." + masked[len(masked)-4:]
		}
		fmt.Printf("Token:           %s\n", masked)
		fmt.Println("Server Status:   Unreachable / Offline")
	}

	if strings.EqualFold(role, "master") {
		if cfg.MetricsToken != "" {
			maskedM := cfg.MetricsToken
			if len(maskedM) > 8 {
				maskedM = maskedM[:4] + "..." + maskedM[len(maskedM)-4:]
			}
			fmt.Printf("Metrics Token:   %s (Configured)\n", maskedM)
		} else {
			fmt.Println("Metrics Token:   Not configured (optional - use 'cee auth metrics <key>')")
		}
	} else {
		fmt.Println("Metrics Token:   N/A (Metrics restricted to Master role)")
	}
	fmt.Println("──────────────────────────────────────────────────────────")
	fmt.Println()
	return nil
}

func submitCodeToRemote(apiURL, authToken, code string, lang *languages.Language, stdin, expectedOutput string, timeout float64) error {
	subReq := api.SubmissionRequest{
		SourceCode: &code,
		LanguageID: lang.ID,
	}
	if stdin != "" {
		subReq.Stdin = &stdin
	}
	if expectedOutput != "" {
		subReq.ExpectedOutput = &expectedOutput
	}
	if timeout > 0 {
		subReq.CPUTimeLimit = &timeout
		wallTimeout := timeout + 2.0
		subReq.WallTimeLimit = &wallTimeout
	}

	bodyBytes, err := json.Marshal(subReq)
	if err != nil {
		return err
	}

	endpoint := fmt.Sprintf("%s/submissions?wait=true", strings.TrimRight(apiURL, "/"))
	req, err := http.NewRequest(http.MethodPost, endpoint, bytes.NewReader(bodyBytes))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	if authToken != "" {
		req.Header.Set("X-Auth-Token", authToken)
	}

	client := &http.Client{Timeout: time.Duration(timeout+15) * time.Second}
	start := time.Now()
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("remote execution failed connecting to %s: %w", apiURL, err)
	}
	defer resp.Body.Close()

	duration := time.Since(start)

	respBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("failed reading response: %w", err)
	}

	if resp.StatusCode != http.StatusCreated && resp.StatusCode != http.StatusOK {
		return fmt.Errorf("remote API returned HTTP %d: %s", resp.StatusCode, string(respBytes))
	}

	var res api.SubmissionResponse
	if err := json.Unmarshal(respBytes, &res); err != nil {
		return fmt.Errorf("failed parsing submission response: %w", err)
	}

	fmt.Println("\n──────────────────── Execution Result (Remote) ───────────")
	fmt.Printf("Status:    %s (ID: %d)\n", res.Status.Description, res.Status.ID)
	if res.ExitCode != nil {
		fmt.Printf("Exit Code: %d\n", *res.ExitCode)
	}
	if res.WallTime != nil {
		fmt.Printf("Wall Time: %ss (total network: %v)\n", *res.WallTime, duration.Round(time.Millisecond))
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

	if expectedOutput != "" && res.Status.ID == languages.StatusWrongAnswer {
		fmt.Printf("\n[Expected Output]:\n%s", expectedOutput)
		if !strings.HasSuffix(expectedOutput, "\n") {
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
}

func newTokenCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:     "token",
		Aliases: []string{"key", "keys", "tokens"},
		Short:   "Manage API keys and access tokens",
		Long:    "Generate, list, inspect, and revoke AUTH and METRICS API keys based on role permissions.",
	}
	cmd.PersistentFlags().StringP("url", "u", "", "CEE server URL")
	cmd.PersistentFlags().StringP("token", "t", "", "Authentication token")

	genCmd := &cobra.Command{
		Use:   "generate",
		Short: "Generate a new API key",
	}
	genCmd.PersistentFlags().StringP("description", "d", "", "Description / label for the API key")

	// cee token generate auth [role]
	authCmd := &cobra.Command{
		Use:   "auth [guest|master]",
		Short: "Generate an AUTH API key (guest or master)",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			role := "guest"
			if len(args) > 0 {
				role = strings.ToLower(args[0])
			}
			desc, _ := cmd.Flags().GetString("description")
			apiURL, _ := cmd.Flags().GetString("url")
			token, _ := cmd.Flags().GetString("token")
			return executeGenerateKey(apiURL, token, "auth", role, desc)
		},
	}

	// cee token generate auth guest
	authGuestCmd := &cobra.Command{
		Use:   "guest",
		Short: "Generate a guest AUTH API key",
		RunE: func(cmd *cobra.Command, args []string) error {
			desc, _ := cmd.Flags().GetString("description")
			apiURL, _ := cmd.Flags().GetString("url")
			token, _ := cmd.Flags().GetString("token")
			return executeGenerateKey(apiURL, token, "auth", "guest", desc)
		},
	}

	// cee token generate auth master
	authMasterCmd := &cobra.Command{
		Use:   "master",
		Short: "Generate an elevated master AUTH API key (master only)",
		RunE: func(cmd *cobra.Command, args []string) error {
			desc, _ := cmd.Flags().GetString("description")
			apiURL, _ := cmd.Flags().GetString("url")
			token, _ := cmd.Flags().GetString("token")
			return executeGenerateKey(apiURL, token, "auth", "master", desc)
		},
	}

	authCmd.AddCommand(authGuestCmd)
	authCmd.AddCommand(authMasterCmd)

	// Top-level alias: cee token generate guest
	genGuestTop := &cobra.Command{
		Use:   "guest",
		Short: "Generate a guest AUTH API key (alias for: auth guest)",
		RunE: func(cmd *cobra.Command, args []string) error {
			desc, _ := cmd.Flags().GetString("description")
			apiURL, _ := cmd.Flags().GetString("url")
			token, _ := cmd.Flags().GetString("token")
			return executeGenerateKey(apiURL, token, "auth", "guest", desc)
		},
	}

	// Top-level alias: cee token generate master
	genMasterTop := &cobra.Command{
		Use:   "master",
		Short: "Generate a master AUTH API key (alias for: auth master)",
		RunE: func(cmd *cobra.Command, args []string) error {
			desc, _ := cmd.Flags().GetString("description")
			apiURL, _ := cmd.Flags().GetString("url")
			token, _ := cmd.Flags().GetString("token")
			return executeGenerateKey(apiURL, token, "auth", "master", desc)
		},
	}

	// cee token generate metrics
	metricsCmd := &cobra.Command{
		Use:   "metrics",
		Short: "Generate a master METRICS API key (master only)",
		RunE: func(cmd *cobra.Command, args []string) error {
			desc, _ := cmd.Flags().GetString("description")
			apiURL, _ := cmd.Flags().GetString("url")
			token, _ := cmd.Flags().GetString("token")
			return executeGenerateKey(apiURL, token, "metrics", "master", desc)
		},
	}

	genCmd.AddCommand(authCmd)
	genCmd.AddCommand(genGuestTop)
	genCmd.AddCommand(genMasterTop)
	genCmd.AddCommand(metricsCmd)

	// cee token list
	listCmd := &cobra.Command{
		Use:   "list",
		Short: "List all managed API keys (master only)",
		RunE: func(cmd *cobra.Command, args []string) error {
			apiURL, _ := cmd.Flags().GetString("url")
			token, _ := cmd.Flags().GetString("token")
			return executeListKeys(apiURL, token)
		},
	}

	// cee token revoke <id>
	revokeCmd := &cobra.Command{
		Use:   "revoke <id>",
		Short: "Revoke an existing API key by ID (master only)",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			apiURL, _ := cmd.Flags().GetString("url")
			token, _ := cmd.Flags().GetString("token")
			return executeRevokeKey(apiURL, token, args[0])
		},
	}

	// cee token whoami / capabilities
	whoamiCmd := &cobra.Command{
		Use:     "whoami",
		Aliases: []string{"capabilities"},
		Short:   "Display identity and permissions for current authenticated credential",
		RunE: func(cmd *cobra.Command, args []string) error {
			apiURL, _ := cmd.Flags().GetString("url")
			token, _ := cmd.Flags().GetString("token")
			return executeWhoami(apiURL, token)
		},
	}

	cmd.AddCommand(genCmd)
	cmd.AddCommand(listCmd)
	cmd.AddCommand(revokeCmd)
	cmd.AddCommand(whoamiCmd)

	return cmd
}

func resolveClientParams(apiURL, token string) (string, string) {
	cfg := loadClientConfig()
	if apiURL == "" {
		apiURL = cfg.APIURL
	}
	if apiURL == "" {
		apiURL = "http://localhost:3000"
	}
	if token == "" {
		token = cfg.AuthToken
	}
	return strings.TrimRight(apiURL, "/"), token
}

func executeGenerateKey(apiURL, token, keyType, role, desc string) error {
	apiURL, token = resolveClientParams(apiURL, token)
	if token == "" {
		return fmt.Errorf("authentication token required. Run 'cee connect <url> --token <token>' or use --token")
	}

	reqBody := auth.GenerateKeyRequest{
		Type:        auth.CredentialType(keyType),
		Role:        auth.Role(role),
		Description: desc,
	}
	b, err := json.Marshal(reqBody)
	if err != nil {
		return err
	}

	req, err := http.NewRequest(http.MethodPost, apiURL+"/api/keys", bytes.NewReader(b))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Auth-Token", token)

	client := &http.Client{Timeout: 15 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("request failed connecting to %s: %w", apiURL, err)
	}
	defer resp.Body.Close()

	respBody, _ := io.ReadAll(resp.Body)

	if resp.StatusCode == http.StatusCreated {
		var genResp auth.GenerateKeyResponse
		if err := json.Unmarshal(respBody, &genResp); err != nil {
			return fmt.Errorf("failed parsing server response: %w", err)
		}

		fmt.Println("\n Successfully generated new API Key!")
		fmt.Println("──────────────────────────────────────────────────────────")
		fmt.Printf("ID:          %s\n", genResp.ID)
		fmt.Printf("Type:        %s\n", strings.ToUpper(string(genResp.Type)))
		fmt.Printf("Role:        %s\n", strings.ToUpper(string(genResp.Role)))
		fmt.Printf("Prefix:      %s\n", genResp.Prefix)
		fmt.Printf("Created By:  %s\n", genResp.CreatedBy)
		fmt.Printf("Created At:  %s\n", genResp.CreatedAt.Format(time.RFC3339))
		fmt.Println("──────────────────────────────────────────────────────────")
		fmt.Printf("API Key:     %s\n", genResp.APIKey)
		fmt.Println("──────────────────────────────────────────────────────────")
		fmt.Println(" IMPORTANT: Save this API key now. It will never be displayed again.")
		fmt.Println()
		return nil
	}

	if resp.StatusCode == http.StatusForbidden {
		return fmt.Errorf("permission denied (HTTP 403): You do not have permission to generate %s %s credentials.\nNote: Guest credentials can only generate additional Guest AUTH keys.", keyType, role)
	}

	return fmt.Errorf("server error (HTTP %d): %s", resp.StatusCode, string(respBody))
}

func executeListKeys(apiURL, token string) error {
	apiURL, token = resolveClientParams(apiURL, token)
	if token == "" {
		return fmt.Errorf("authentication token required. Use --token or connect CLI first")
	}

	req, err := http.NewRequest(http.MethodGet, apiURL+"/api/keys", nil)
	if err != nil {
		return err
	}
	req.Header.Set("X-Auth-Token", token)

	client := &http.Client{Timeout: 15 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	respBody, _ := io.ReadAll(resp.Body)

	if resp.StatusCode == http.StatusForbidden {
		return fmt.Errorf("permission denied (HTTP 403): Master AUTH API key required to view all keys")
	}
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("server error (HTTP %d): %s", resp.StatusCode, string(respBody))
	}

	var keys []*auth.Key
	if err := json.Unmarshal(respBody, &keys); err != nil {
		return fmt.Errorf("failed parsing keys list: %w", err)
	}

	fmt.Println("\nManaged CEE API Credentials:")
	fmt.Printf("%-24s %-8s %-8s %-24s %-8s %s\n", "ID", "TYPE", "ROLE", "PREFIX", "STATUS", "CREATED AT")
	fmt.Println("───────────────────────────────────────────────────────────────────────────────────────────")
	for _, k := range keys {
		fmt.Printf("%-24s %-8s %-8s %-24s %-8s %s\n",
			k.ID,
			k.Type,
			k.Role,
			k.Prefix,
			k.Status,
			k.CreatedAt.Format("2006-01-02 15:04"),
		)
	}
	fmt.Println()
	return nil
}

func executeRevokeKey(apiURL, token, keyID string) error {
	apiURL, token = resolveClientParams(apiURL, token)
	if token == "" {
		return fmt.Errorf("authentication token required. Use --token or connect CLI first")
	}

	req, err := http.NewRequest(http.MethodDelete, fmt.Sprintf("%s/api/keys/%s", apiURL, keyID), nil)
	if err != nil {
		return err
	}
	req.Header.Set("X-Auth-Token", token)

	client := &http.Client{Timeout: 15 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	respBody, _ := io.ReadAll(resp.Body)

	if resp.StatusCode == http.StatusConflict {
		return fmt.Errorf("lockout protection (HTTP 409): Cannot revoke the last active master AUTH API key")
	}
	if resp.StatusCode == http.StatusForbidden {
		return fmt.Errorf("permission denied (HTTP 403): Master AUTH API key required to revoke credentials")
	}
	if resp.StatusCode == http.StatusNotFound {
		return fmt.Errorf("not found (HTTP 404): Key ID '%s' does not exist", keyID)
	}
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("server error (HTTP %d): %s", resp.StatusCode, string(respBody))
	}

	fmt.Printf(" API key '%s' was successfully revoked.\n", keyID)
	return nil
}

func executeWhoami(apiURL, token string) error {
	apiURL, token = resolveClientParams(apiURL, token)
	if token == "" {
		return fmt.Errorf("authentication token required. Use --token or connect CLI first")
	}

	req, err := http.NewRequest(http.MethodGet, apiURL+"/api/capabilities", nil)
	if err != nil {
		return err
	}
	req.Header.Set("X-Auth-Token", token)

	client := &http.Client{Timeout: 15 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	respBody, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("authentication failed (HTTP %d): %s", resp.StatusCode, string(respBody))
	}

	var info auth.CapabilityInfo
	if err := json.Unmarshal(respBody, &info); err != nil {
		return fmt.Errorf("failed parsing capabilities: %w", err)
	}

	fmt.Println("\nAuthenticated Credential Identity:")
	fmt.Println("──────────────────────────────────────────────────────────")
	fmt.Printf("Key ID:          %s\n", info.KeyID)
	fmt.Printf("Prefix:          %s\n", info.Prefix)
	fmt.Printf("Credential Type: %s\n", strings.ToUpper(string(info.CredentialType)))
	fmt.Printf("Assigned Role:   %s\n", strings.ToUpper(string(info.Role)))
	fmt.Println("──────────────────────────────────────────────────────────")
	fmt.Println("Allowed Operations (Server-Enforced):")

	hasPerm := func(target string) bool {
		for _, p := range info.Permissions {
			if p == target {
				return true
			}
		}
		return false
	}

	printPerm := func(name string, allowed bool) {
		if allowed {
			fmt.Printf("  [ALLOWED] %s\n", name)
		} else {
			fmt.Printf("  [DENIED]  %s\n", name)
		}
	}

	printPerm("Application API Access (Run, Submit, Languages)", hasPerm(auth.PermAPIAccess))
	printPerm("Generate Guest AUTH Keys", hasPerm(auth.PermTokenGenerateGuest))
	printPerm("Generate Master AUTH Keys", hasPerm(auth.PermTokenGenerateMaster))
	printPerm("Generate Metrics Keys", hasPerm(auth.PermTokenGenerateMetrics))
	printPerm("Revoke & Manage API Keys", hasPerm(auth.PermTokenRevoke))
	fmt.Println("──────────────────────────────────────────────────────────")
	fmt.Println()

	return nil
}
