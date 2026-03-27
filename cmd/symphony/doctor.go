package main

import (
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"text/template"
	"time"

	"github.com/joho/godotenv"
	"github.com/spf13/cobra"

	"github.com/shivamstaq/github-symphony/internal/config"
)

var doctorCmd = &cobra.Command{
	Use:   "doctor",
	Short: "Validate configuration and environment",
	Long:  `Checks symphony.yaml, verifies credentials, and tests connectivity.`,
	RunE:  runDoctor,
}

func init() {
	rootCmd.AddCommand(doctorCmd)
}

func runDoctor(cmd *cobra.Command, args []string) error {
	_ = godotenv.Load()

	cwd, err := os.Getwd()
	if err != nil {
		return err
	}

	symphonyDir := filepath.Join(cwd, ".symphony")
	configPath := filepath.Join(symphonyDir, "symphony.yaml")

	fmt.Println("Symphony Doctor")
	fmt.Println("===============")
	fmt.Println()

	// Check .symphony/ exists
	if _, err := os.Stat(symphonyDir); os.IsNotExist(err) {
		fmt.Println("  FAIL .symphony/ directory not found")
		fmt.Println("       Run 'symphony init' to create it")
		return nil
	}
	fmt.Println("  ok   .symphony/ directory exists")

	// Check symphony.yaml exists
	if _, err := os.Stat(configPath); os.IsNotExist(err) {
		fmt.Println("  FAIL symphony.yaml not found")
		return nil
	}
	fmt.Println("  ok   symphony.yaml found")

	// Parse config
	cfg, err := config.LoadSymphonyConfig(configPath)
	if err != nil {
		fmt.Printf("  FAIL config parse error: %v\n", err)
		return nil
	}
	fmt.Println("  ok   symphony.yaml parsed successfully")

	// Validate config
	if err := config.ValidateSymphonyConfig(cfg); err != nil {
		fmt.Printf("  FAIL %v\n", err)
		return nil
	}
	fmt.Println("  ok   config validation passed")

	// Check directories
	dirs := map[string]string{
		"prompts": filepath.Join(symphonyDir, "prompts"),
		"state":   filepath.Join(symphonyDir, "state"),
		"logs":    filepath.Join(symphonyDir, "logs"),
	}
	for name, path := range dirs {
		if _, err := os.Stat(path); os.IsNotExist(err) {
			fmt.Printf("  WARN %s/ directory missing\n", name)
		} else {
			fmt.Printf("  ok   %s/ directory exists\n", name)
		}
	}

	// Check default prompt template
	defaultPrompt := filepath.Join(symphonyDir, "prompts", cfg.PromptRouting.Default)
	if _, err := os.Stat(defaultPrompt); os.IsNotExist(err) {
		fmt.Printf("  FAIL default prompt template not found: %s\n", defaultPrompt)
	} else {
		fmt.Printf("  ok   default prompt template: %s\n", cfg.PromptRouting.Default)
	}

	// Check agent binary
	agentBin := cfg.Agent.Command
	if agentBin == "" {
		switch cfg.Agent.Kind {
		case "claude_code":
			agentBin = "claude"
		case "opencode":
			agentBin = "opencode"
		case "codex":
			agentBin = "codex"
		}
	}
	if path, err := exec.LookPath(agentBin); err != nil {
		fmt.Printf("  WARN agent binary not found: %s\n", agentBin)
	} else {
		fmt.Printf("  ok   agent binary: %s\n", path)
	}

	// Check git
	if _, err := exec.LookPath("git"); err != nil {
		fmt.Println("  FAIL git not found on PATH")
	} else {
		fmt.Println("  ok   git available")
	}

	// Check prompt template is parseable
	if _, statErr := os.Stat(defaultPrompt); statErr == nil {
		tmplData, readErr := os.ReadFile(defaultPrompt)
		if readErr == nil {
			if _, parseErr := template.New("check").Option("missingkey=error").Parse(string(tmplData)); parseErr != nil {
				fmt.Printf("  WARN prompt template has parse errors: %v\n", parseErr)
			} else {
				fmt.Println("  ok   prompt template parses correctly")
			}
		}
	}

	// Auth check
	if cfg.Tracker.Kind == "github" {
		if cfg.Auth.GitHub.Token != "" {
			fmt.Println("  ok   GitHub token configured")
		} else if cfg.Auth.GitHub.AppID != "" {
			fmt.Println("  ok   GitHub App credentials configured")
		} else {
			fmt.Println("  FAIL no GitHub credentials")
		}
	}

	// GitHub API connectivity check
	if cfg.Tracker.Kind == "github" && cfg.Auth.GitHub.Token != "" {
		apiURL := cfg.Auth.GitHub.APIURL
		if apiURL == "" {
			apiURL = "https://api.github.com"
		}
		client := &http.Client{Timeout: 10 * time.Second}
		req, _ := http.NewRequest("GET", apiURL+"/user", nil)
		req.Header.Set("Authorization", "Bearer "+cfg.Auth.GitHub.Token)
		resp, err := client.Do(req)
		if err != nil {
			fmt.Printf("  WARN GitHub API unreachable: %v\n", err)
		} else {
			resp.Body.Close()
			if resp.StatusCode == 200 {
				fmt.Println("  ok   GitHub API reachable (authenticated)")
			} else {
				fmt.Printf("  WARN GitHub API returned status %d\n", resp.StatusCode)
			}
		}
	}

	fmt.Println()
	fmt.Printf("Tracker: %s | Agent: %s | Max concurrent: %d\n",
		cfg.Tracker.Kind, cfg.Agent.Kind, cfg.Agent.MaxConcurrent)

	return nil
}
