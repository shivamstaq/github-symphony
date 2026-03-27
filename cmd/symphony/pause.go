package main

import (
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"

	"github.com/joho/godotenv"
	"github.com/spf13/cobra"

	"github.com/shivamstaq/github-symphony/internal/config"
)

var pauseCmd = &cobra.Command{
	Use:   "pause <item-id>",
	Short: "Pause an agent between turns",
	Long:  `Sends a pause signal via the HTTP API. The current turn finishes, but the next turn will not start until resumed.`,
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		return sendControl("pause", args[0])
	},
}

var resumeCmd = &cobra.Command{
	Use:   "resume <item-id>",
	Short: "Resume a paused agent",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		return sendControl("resume", args[0])
	},
}

var killCmd = &cobra.Command{
	Use:   "kill <item-id>",
	Short: "Force-stop an agent",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		return sendControl("kill", args[0])
	},
}

func init() {
	rootCmd.AddCommand(pauseCmd)
	rootCmd.AddCommand(resumeCmd)
	rootCmd.AddCommand(killCmd)
}

func sendControl(action, itemID string) error {
	_ = godotenv.Load()
	addr := resolveAPIAddr()

	url := fmt.Sprintf("http://%s/api/v1/%s/%s", addr, action, itemID)
	resp, err := http.Post(url, "application/json", nil)
	if err != nil {
		return fmt.Errorf("failed to connect to Symphony at %s: %w\nIs 'symphony run' active?", addr, err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	fmt.Println(string(body))
	return nil
}

func resolveAPIAddr() string {
	cwd, _ := os.Getwd()
	configPath := filepath.Join(cwd, ".symphony", "symphony.yaml")

	cfg, err := config.LoadSymphonyConfig(configPath)
	if err != nil {
		return "localhost:9097"
	}

	host := cfg.Server.Host
	if host == "" || host == "0.0.0.0" {
		host = "localhost"
	}
	port := cfg.Server.Port
	if port == 0 {
		port = 9097
	}
	return fmt.Sprintf("%s:%d", host, port)
}
