package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"

	"cee.io/pkg/auth"
	"github.com/spf13/cobra"
)

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
				return fmt.Errorf("Invalid or unauthorized API key")
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
					return fmt.Errorf("Invalid or unauthorized API key")
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
	fmt.Println("Successfully logged out!")

	var envVars []string
	for _, env := range []string{"CEE_AUTH_TOKEN", "CEE_TOKEN", "CEE_METRICS_TOKEN", "AUTH_TOKEN", "METRICS_TOKEN"} {
		if os.Getenv(env) != "" {
			envVars = append(envVars, env)
		}
	}
	if len(envVars) > 0 {
		fmt.Println("\n⚠️  Notice: Authentication environment variable(s) detected in your current shell:")
		for _, v := range envVars {
			fmt.Printf("   export %s\n", v)
		}
		fmt.Println("To completely remove them from your shell session, run:")
		fmt.Printf("   unset %s\n", strings.Join(envVars, " "))
	}
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

	// Validate against server /api/capabilities with expected role
	client := &http.Client{Timeout: 8 * time.Second}
	req, err := http.NewRequest(http.MethodGet, fmt.Sprintf("%s/api/capabilities?role=%s", targetURL, expectedRole), nil)
	if err != nil {
		return err
	}
	req.Header.Set("X-Auth-Token", key)

	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("failed to connect to server at %s: %w\n(Verify that the server is online and accessible)", targetURL, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden {
		return fmt.Errorf("Invalid or unauthorized API key")
	}
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("server error (HTTP %d) verifying API key", resp.StatusCode)
	}

	b, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("failed reading response from server: %w", err)
	}

	var capInfo auth.CapabilityInfo
	if err := json.Unmarshal(b, &capInfo); err != nil {
		return fmt.Errorf("failed parsing server capabilities response: %w", err)
	}

	// 1. Ensure the credential is an AUTH key, not a METRICS key
	if capInfo.CredentialType != auth.TypeAuth {
		return fmt.Errorf("Invalid or unauthorized API key")
	}

	// 2. Strict Role Match: Ensure expectedRole matches the key's actual role
	actualRole := strings.ToLower(string(capInfo.Role))
	if expectedRole != "" && expectedRole != actualRole {
		return fmt.Errorf("Invalid or unauthorized API key")
	}

	cfg.APIURL = targetURL
	cfg.AuthToken = key
	cfg.Role = actualRole

	if metricsKey != "" {
		if actualRole == "guest" {
			return fmt.Errorf("Invalid or unauthorized API key")
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
				return fmt.Errorf("Invalid or unauthorized API key")
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
	fmt.Println("Successfully logged in!")
	fmt.Println("──────────────────────────────────────────────────────────")
	fmt.Printf("Server URL:      %s\n", targetURL)
	fmt.Printf("Key ID:          %s\n", capInfo.KeyID)
	fmt.Printf("Prefix:          %s\n", capInfo.Prefix)
	fmt.Println("Server Status:   Verified online & active")
	if cfg.MetricsToken != "" {
		maskedM := cfg.MetricsToken
		if len(maskedM) > 12 {
			maskedM = maskedM[:6] + "..." + maskedM[len(maskedM)-4:]
		}
		fmt.Printf("Metrics Key:     %s (configured)\n", maskedM)
	}
	fmt.Println("──────────────────────────────────────────────────────────")
	fmt.Println("Permissions:")
	if actualRole == "master" {
		fmt.Println("  [ALLOWED] API Execution (Run, Submit, Languages)")
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
		fmt.Println("  [ALLOWED] API Execution (Run, Submit, Languages)")
		fmt.Println("  [ALLOWED] Generate Guest AUTH Keys (Delegation)")
		fmt.Println("  [DENIED]  Generate Master Keys")
		fmt.Println("  [DENIED]  Generate Metrics Keys")
		fmt.Println("  [DENIED]  Revoke Keys")
		fmt.Println("  [DENIED]  Metrics Access")
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
	tokenSource := "Config file (~/.cee/config.json)"
	if os.Getenv("CEE_AUTH_TOKEN") != "" {
		tokenSource = "Environment variable (CEE_AUTH_TOKEN)"
	} else if os.Getenv("CEE_TOKEN") != "" {
		tokenSource = "Environment variable (CEE_TOKEN)"
	}

	fmt.Println("\nActive CEE Authentication Session:")
	fmt.Println("──────────────────────────────────────────────────────────")
	fmt.Printf("Server URL:      %s\n", cfg.APIURL)
	fmt.Printf("Role:            %s\n", strings.ToUpper(role))
	fmt.Printf("Auth Source:     %s\n", tokenSource)
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
		fmt.Println("Metrics Token:   Not configured")
	}
	fmt.Println("──────────────────────────────────────────────────────────")
	fmt.Println()
	return nil
}
