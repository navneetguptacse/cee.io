package main

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"cee.io/pkg/api"
	"cee.io/pkg/auth"
	"cee.io/pkg/config"
	"cee.io/pkg/executor"
	"cee.io/pkg/queue"
	"github.com/spf13/cobra"
)

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

