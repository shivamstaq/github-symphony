package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/joho/godotenv"
	"github.com/spf13/cobra"

	"github.com/shivamstaq/github-symphony/internal/config"
)

var configCmd = &cobra.Command{
	Use:   "config",
	Short: "Configuration management commands",
}

var configValidateCmd = &cobra.Command{
	Use:   "validate",
	Short: "Validate symphony.yaml",
	RunE: func(cmd *cobra.Command, args []string) error {
		_ = godotenv.Load()
		cwd, _ := os.Getwd()
		configPath := filepath.Join(cwd, ".symphony", "symphony.yaml")

		cfg, err := config.LoadSymphonyConfig(configPath)
		if err != nil {
			return fmt.Errorf("parse error: %w", err)
		}
		if err := config.ValidateSymphonyConfig(cfg); err != nil {
			return err
		}
		fmt.Println("Config is valid.")
		return nil
	},
}

var configShowCmd = &cobra.Command{
	Use:   "show",
	Short: "Show resolved configuration with defaults applied",
	RunE: func(cmd *cobra.Command, args []string) error {
		_ = godotenv.Load()
		cwd, _ := os.Getwd()
		configPath := filepath.Join(cwd, ".symphony", "symphony.yaml")

		cfg, err := config.LoadSymphonyConfig(configPath)
		if err != nil {
			return fmt.Errorf("parse error: %w", err)
		}

		// Mask sensitive values
		if cfg.Auth.GitHub.Token != "" {
			cfg.Auth.GitHub.Token = "***"
		}
		if cfg.Auth.Linear.APIKey != "" {
			cfg.Auth.Linear.APIKey = "***"
		}

		data, err := json.MarshalIndent(cfg, "", "  ")
		if err != nil {
			return err
		}
		fmt.Println(string(data))
		return nil
	},
}

func init() {
	configCmd.AddCommand(configValidateCmd)
	configCmd.AddCommand(configShowCmd)
	rootCmd.AddCommand(configCmd)
}
