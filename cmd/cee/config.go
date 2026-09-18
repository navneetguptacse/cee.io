package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/spf13/cobra"
)

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
				return fmt.Errorf("Invalid or unauthorized API key")
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
