package main

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"
)

var initCmd = &cobra.Command{
	Use:   "init",
	Short: "Initialize a .symphony/ directory in the current repository",
	Long: `Creates a .symphony/ directory with:
  - symphony.yaml (configuration)
  - prompts/default.md (default prompt template)
  - state/ (persistent state directory)
  - logs/ (log output directory)

Also adds .symphony/ to .gitignore if not already present.`,
	RunE: runInit,
}

func init() {
	rootCmd.AddCommand(initCmd)
}

func runInit(cmd *cobra.Command, args []string) error {
	cwd, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("get working directory: %w", err)
	}

	symphonyDir := filepath.Join(cwd, ".symphony")
	if _, err := os.Stat(symphonyDir); err == nil {
		return fmt.Errorf(".symphony/ already exists in %s — run 'symphony doctor' to validate", cwd)
	}

	reader := bufio.NewReader(os.Stdin)

	// Step 1: Tracker
	trackerKind := prompt(reader, "Tracker type", "github", []string{"github", "linear"})

	var owner, projectNum, token string
	if trackerKind == "github" {
		owner = promptFree(reader, "GitHub owner (org or user)")
		projectNum = promptFree(reader, "GitHub Project number")

		// Check for token in env
		envToken := os.Getenv("GITHUB_TOKEN")
		if envToken != "" {
			fmt.Printf("  %s GitHub token detected from $GITHUB_TOKEN\n", checkmark())
			token = "$GITHUB_TOKEN"
		} else {
			token = promptFree(reader, "GitHub token (or $VAR_NAME)")
		}
	}

	// Step 2: Agent
	agentKind := prompt(reader, "Agent CLI", "claude_code", []string{"claude_code", "opencode", "codex"})
	maxConcurrent := promptFree(reader, "Max concurrent agents (default: 3)")
	if maxConcurrent == "" {
		maxConcurrent = "3"
	}

	// Create directories
	dirs := []string{
		symphonyDir,
		filepath.Join(symphonyDir, "prompts"),
		filepath.Join(symphonyDir, "state"),
		filepath.Join(symphonyDir, "logs"),
		filepath.Join(symphonyDir, "logs", "agents"),
		filepath.Join(symphonyDir, "sockets"),
	}
	for _, d := range dirs {
		if err := os.MkdirAll(d, 0755); err != nil {
			return fmt.Errorf("create directory %s: %w", d, err)
		}
	}

	// Write symphony.yaml
	yamlContent := generateYAML(trackerKind, owner, projectNum, token, agentKind, maxConcurrent)
	yamlPath := filepath.Join(symphonyDir, "symphony.yaml")
	if err := os.WriteFile(yamlPath, []byte(yamlContent), 0644); err != nil {
		return fmt.Errorf("write symphony.yaml: %w", err)
	}

	// Write default prompt
	promptPath := filepath.Join(symphonyDir, "prompts", "default.md")
	if err := os.WriteFile(promptPath, []byte(defaultPromptTemplate), 0644); err != nil {
		return fmt.Errorf("write default prompt: %w", err)
	}

	// Add .symphony/ to .gitignore
	addToGitignore(cwd)

	fmt.Println()
	fmt.Printf("  %s Created %s\n", checkmark(), yamlPath)
	fmt.Printf("  %s Created %s\n", checkmark(), promptPath)
	fmt.Printf("  %s Created state/ and logs/ directories\n", checkmark())
	fmt.Printf("  %s Updated .gitignore\n", checkmark())
	fmt.Println()
	fmt.Println("Next: edit .symphony/symphony.yaml, then run 'symphony run'")
	fmt.Println("      or run 'symphony doctor' to validate your config")

	return nil
}

func prompt(reader *bufio.Reader, label, defaultVal string, options []string) string {
	fmt.Printf("? %s [%s] (default: %s): ", label, strings.Join(options, " / "), defaultVal)
	input, _ := reader.ReadString('\n')
	input = strings.TrimSpace(input)
	if input == "" {
		return defaultVal
	}
	for _, opt := range options {
		if strings.EqualFold(input, opt) {
			return opt
		}
	}
	return defaultVal
}

func promptFree(reader *bufio.Reader, label string) string {
	fmt.Printf("? %s: ", label)
	input, _ := reader.ReadString('\n')
	return strings.TrimSpace(input)
}

func checkmark() string {
	return "ok"
}

func addToGitignore(cwd string) {
	gitignorePath := filepath.Join(cwd, ".gitignore")
	content, _ := os.ReadFile(gitignorePath)
	if strings.Contains(string(content), ".symphony/") {
		return
	}
	f, err := os.OpenFile(gitignorePath, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0644)
	if err != nil {
		return
	}
	defer f.Close()
	if len(content) > 0 && !strings.HasSuffix(string(content), "\n") {
		f.WriteString("\n")
	}
	f.WriteString("\n# Symphony orchestrator state\n.symphony/\n")
}

func generateYAML(trackerKind, owner, projectNum, token, agentKind, maxConcurrent string) string {
	var sb strings.Builder
	sb.WriteString("# Symphony configuration\n")
	sb.WriteString("# Docs: https://github.com/shivamstaq/github-symphony\n\n")

	sb.WriteString("tracker:\n")
	sb.WriteString(fmt.Sprintf("  kind: %s\n", trackerKind))
	if trackerKind == "github" {
		sb.WriteString(fmt.Sprintf("  owner: %s\n", owner))
		sb.WriteString(fmt.Sprintf("  project_number: %s\n", projectNum))
		sb.WriteString("  active_values: [Todo, In Progress]\n")
		sb.WriteString("  terminal_values: [Done, Closed, Cancelled]\n")
	}

	sb.WriteString("\nauth:\n")
	if trackerKind == "github" {
		sb.WriteString("  github:\n")
		sb.WriteString(fmt.Sprintf("    token: %s\n", token))
	}

	sb.WriteString("\nagent:\n")
	sb.WriteString(fmt.Sprintf("  kind: %s\n", agentKind))
	sb.WriteString(fmt.Sprintf("  max_concurrent: %s\n", maxConcurrent))
	sb.WriteString("  max_turns: 20\n")

	if agentKind == "claude_code" {
		sb.WriteString("  claude:\n")
		sb.WriteString("    model: sonnet\n")
		sb.WriteString("    permission_profile: bypassPermissions\n")
	}

	sb.WriteString("\ngit:\n")
	sb.WriteString("  branch_prefix: symphony/\n")
	sb.WriteString("  use_worktrees: true\n")

	sb.WriteString("\npolling:\n")
	sb.WriteString("  interval_ms: 30000\n")

	sb.WriteString("\npull_request:\n")
	sb.WriteString("  open_on_success: true\n")
	sb.WriteString("  draft_by_default: true\n")
	sb.WriteString("  handoff_status: Human Review\n")

	sb.WriteString("\nprompt_routing:\n")
	sb.WriteString("  default: default.md\n")
	sb.WriteString("  # field_name: Type\n")
	sb.WriteString("  # routes:\n")
	sb.WriteString("  #   bug: bug_fix.md\n")
	sb.WriteString("  #   feature: feature.md\n")

	sb.WriteString("\nserver:\n")
	sb.WriteString("  port: 9097\n")

	return sb.String()
}

const defaultPromptTemplate = `You are working on a GitHub issue.

## Issue
**Title**: {{.work_item.title}}
**Description**: {{.work_item.description}}

## Repository
{{.repository.full_name}}

## Instructions
1. Read the issue carefully and understand what needs to be done
2. Explore the codebase to understand the relevant code
3. Implement the fix or feature
4. Write tests if appropriate
5. Ensure all existing tests pass
6. Create clear, atomic commits

Branch: {{.branch_name}}
Base: {{.base_branch}}
`
