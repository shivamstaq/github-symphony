package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"
)

var logsCmd = &cobra.Command{
	Use:   "logs",
	Short: "Tail orchestrator logs",
	Long:  `Reads and displays logs from .symphony/logs/orchestrator.jsonl.`,
	RunE:  runLogs,
}

var (
	logsFollow bool
	logsAgent  string
	logsLevel  string
	logsLines  int
)

func init() {
	logsCmd.Flags().BoolVarP(&logsFollow, "follow", "f", false, "Follow log output")
	logsCmd.Flags().StringVar(&logsAgent, "agent", "", "Filter by agent/item ID")
	logsCmd.Flags().StringVar(&logsLevel, "level", "", "Minimum log level (debug, info, warn, error)")
	logsCmd.Flags().IntVarP(&logsLines, "lines", "n", 50, "Number of lines to show")
	rootCmd.AddCommand(logsCmd)
}

func runLogs(cmd *cobra.Command, args []string) error {
	cwd, _ := os.Getwd()
	logPath := filepath.Join(cwd, ".symphony", "logs", "orchestrator.jsonl")

	f, err := os.Open(logPath)
	if err != nil {
		return fmt.Errorf("open log file: %w (is Symphony initialized?)", err)
	}
	defer func() { _ = f.Close() }()

	// Read all lines
	var lines []string
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := scanner.Text()
		if logsAgent != "" && !strings.Contains(line, logsAgent) {
			continue
		}
		if logsLevel != "" && !matchesLevel(line, logsLevel) {
			continue
		}
		lines = append(lines, line)
	}

	// Show last N lines
	start := 0
	if len(lines) > logsLines {
		start = len(lines) - logsLines
	}

	for _, line := range lines[start:] {
		fmt.Println(formatLogLine(line))
	}

	return nil
}

func matchesLevel(line, minLevel string) bool {
	levelOrder := map[string]int{
		"DEBUG": 0, "INFO": 1, "WARN": 2, "ERROR": 3,
	}
	minOrd, ok := levelOrder[strings.ToUpper(minLevel)]
	if !ok {
		return true
	}

	// Parse level from JSON line
	var entry map[string]any
	if err := json.Unmarshal([]byte(line), &entry); err != nil {
		return true
	}
	level, _ := entry["level"].(string)
	lineOrd, ok := levelOrder[strings.ToUpper(level)]
	if !ok {
		return true
	}
	return lineOrd >= minOrd
}

func formatLogLine(line string) string {
	var entry map[string]any
	if err := json.Unmarshal([]byte(line), &entry); err != nil {
		return line // not JSON, show raw
	}

	ts, _ := entry["time"].(string)
	level, _ := entry["level"].(string)
	msg, _ := entry["msg"].(string)

	if len(ts) > 19 {
		ts = ts[11:19] // extract HH:MM:SS
	}

	return fmt.Sprintf("%s %-5s %s", ts, level, msg)
}
