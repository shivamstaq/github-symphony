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

var eventsCmd = &cobra.Command{
	Use:   "events",
	Short: "Query the FSM event log",
	Long:  `Reads .symphony/state/events.jsonl and displays FSM state transitions.`,
	RunE:  runEvents,
}

var eventsItem string

func init() {
	eventsCmd.Flags().StringVar(&eventsItem, "item", "", "Filter by work item ID")
	rootCmd.AddCommand(eventsCmd)
}

func runEvents(cmd *cobra.Command, args []string) error {
	cwd, _ := os.Getwd()
	eventsPath := filepath.Join(cwd, ".symphony", "state", "events.jsonl")

	f, err := os.Open(eventsPath)
	if err != nil {
		return fmt.Errorf("open events: %w (has Symphony run yet?)", err)
	}
	defer func() { _ = f.Close() }()

	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := scanner.Text()
		if eventsItem != "" && !strings.Contains(line, eventsItem) {
			continue
		}

		var evt map[string]any
		if err := json.Unmarshal([]byte(line), &evt); err != nil {
			fmt.Println(line)
			continue
		}

		ts, _ := evt["timestamp"].(string)
		itemID, _ := evt["item_id"].(string)
		from, _ := evt["from"].(string)
		to, _ := evt["to"].(string)
		event, _ := evt["event"].(string)

		if len(ts) > 19 {
			ts = ts[11:19]
		}

		fmt.Printf("%s  %-20s  %s -> %s  [%s]\n", ts, itemID, from, to, event)
	}

	return nil
}
