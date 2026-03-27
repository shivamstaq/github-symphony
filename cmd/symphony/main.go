package main

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

var rootCmd = &cobra.Command{
	Use:   "symphony",
	Short: "Symphony — AI agent orchestrator for GitHub projects",
	Long: `Symphony orchestrates AI coding agents (Claude, OpenCode, Codex) to work on
GitHub issues from a GitHub Project board. It manages the full lifecycle from
dispatch to PR creation and handoff.

Run 'symphony init' to set up a project, then 'symphony run' to start.`,
}

func main() {
	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
