package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"
)

var statusCmd = &cobra.Command{
	Use:   "status",
	Short: "Show current orchestrator state (non-interactive)",
	Long:  `Reads the latest event log and state to show a one-shot status dump in JSON format.`,
	RunE:  runStatus,
}

func init() {
	rootCmd.AddCommand(statusCmd)
}

func runStatus(cmd *cobra.Command, args []string) error {
	cwd, _ := os.Getwd()
	eventsPath := filepath.Join(cwd, ".symphony", "state", "events.jsonl")

	if _, err := os.Stat(eventsPath); os.IsNotExist(err) {
		fmt.Println("No state found. Is Symphony running?")
		return nil
	}

	// Read last few events to show recent state
	data, err := os.ReadFile(eventsPath)
	if err != nil {
		return fmt.Errorf("read events: %w", err)
	}

	// Parse events
	var events []map[string]any
	for _, line := range splitLines(string(data)) {
		if line == "" {
			continue
		}
		var evt map[string]any
		if err := json.Unmarshal([]byte(line), &evt); err == nil {
			events = append(events, evt)
		}
	}

	// Show last 20 events
	start := 0
	if len(events) > 20 {
		start = len(events) - 20
	}

	status := map[string]any{
		"total_events": len(events),
		"recent":       events[start:],
	}

	out, _ := json.MarshalIndent(status, "", "  ")
	fmt.Println(string(out))
	return nil
}

func splitLines(s string) []string {
	var lines []string
	start := 0
	for i := 0; i < len(s); i++ {
		if s[i] == '\n' {
			lines = append(lines, s[start:i])
			start = i + 1
		}
	}
	if start < len(s) {
		lines = append(lines, s[start:])
	}
	return lines
}
