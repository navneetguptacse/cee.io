package main

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"

	"cee.io/pkg/config"
	"cee.io/pkg/executor"
	"cee.io/pkg/languages"
	"github.com/spf13/cobra"
)

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

