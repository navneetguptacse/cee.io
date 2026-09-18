package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"cee.io/pkg/auth"
	"github.com/spf13/cobra"
)

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

	listCmd := &cobra.Command{
		Use:   "list",
		Short: "List all managed API keys (master only)",
		RunE: func(cmd *cobra.Command, args []string) error {
			apiURL, _ := cmd.Flags().GetString("url")
			token, _ := cmd.Flags().GetString("token")
			return executeListKeys(apiURL, token)
		},
	}

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

	if resp.StatusCode == http.StatusForbidden || resp.StatusCode == http.StatusUnauthorized {
		return fmt.Errorf("Invalid or unauthorized API key")
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

	if resp.StatusCode == http.StatusForbidden || resp.StatusCode == http.StatusUnauthorized {
		return fmt.Errorf("Invalid or unauthorized API key")
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
	if resp.StatusCode == http.StatusForbidden || resp.StatusCode == http.StatusUnauthorized {
		return fmt.Errorf("Invalid or unauthorized API key")
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

