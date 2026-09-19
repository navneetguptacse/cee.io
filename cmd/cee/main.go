package main

import (
	"fmt"
	"log/slog"
	"os"
	"strings"

	"github.com/spf13/cobra"
)

var Version = "v1.0.0"

var rootCmd = &cobra.Command{
	Use:   "cee",
	Short: "CEE — High-performance Code Execution Engine & CLI",
	Long:  `CEE is an ultra-low latency, production-ready, Judge0-compatible code execution engine and CLI tool built in Go.`,
}

func newVersionCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "version",
		Short: "Print version information",
		Run: func(cmd *cobra.Command, args []string) {
			fmt.Println(Version)
		},
	}
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
	if !strings.HasPrefix(Version, "v") {
		Version = "v" + Version
	}
	rootCmd.Version = Version
	rootCmd.AddCommand(newVersionCmd())
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
