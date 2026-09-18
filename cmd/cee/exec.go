package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"cee.io/pkg/api"
	"cee.io/pkg/config"
	"cee.io/pkg/executor"
	"cee.io/pkg/languages"
	"github.com/spf13/cobra"
)

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
					data, err := io.ReadAll(os.Stdin)
					if err != nil {
						return fmt.Errorf("failed reading from stdin: %w", err)
					}
					code = string(data)
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
					data, err := io.ReadAll(os.Stdin)
					if err != nil {
						return fmt.Errorf("failed reading from stdin: %w", err)
					}
					code = string(data)
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

