package main

import (
	"flag"
	"fmt"
	"log/slog"
	"os"

	"github.com/shivamstaq/github-symphony/internal/config"
)

func main() {
	var (
		port      int
		logFormat string
		logLevel  string
		stateDir  string
		doctor    bool
	)

	flag.IntVar(&port, "port", 0, "HTTP server port (overrides server.port)")
	flag.StringVar(&logFormat, "log-format", "text", "Log output format: text, json")
	flag.StringVar(&logLevel, "log-level", "info", "Log level: debug, info, warn, error")
	flag.StringVar(&stateDir, "state-dir", "", "Directory for persistent state")
	flag.BoolVar(&doctor, "doctor", false, "Validate config and environment, then exit")
	flag.Parse()

	// Configure logger
	logger := setupLogger(logFormat, logLevel)
	slog.SetDefault(logger)

	// Resolve workflow path
	workflowPath := "WORKFLOW.md"
	if flag.NArg() > 0 {
		workflowPath = flag.Arg(0)
	}

	logger.Info("loading workflow", "path", workflowPath)

	// Load workflow
	wf, err := config.LoadWorkflow(workflowPath)
	if err != nil {
		logger.Error("workflow load failed", "error", err)
		os.Exit(1)
	}

	// Parse config
	cfg, err := config.NewServiceConfig(wf.Config)
	if err != nil {
		logger.Error("config parse failed", "error", err)
		os.Exit(1)
	}

	// Apply CLI overrides
	if port > 0 {
		cfg.Server.Port = port
	}

	// Validate
	if err := config.ValidateForDispatch(cfg); err != nil {
		logger.Error("config validation failed", "error", err)
		os.Exit(1)
	}

	if doctor {
		logger.Info("config validation passed")
		fmt.Println("PASS: workflow file loaded and parsed")
		fmt.Println("PASS: config validation passed")
		fmt.Printf("  tracker.kind: %s\n", cfg.Tracker.Kind)
		fmt.Printf("  tracker.owner: %s\n", cfg.Tracker.Owner)
		fmt.Printf("  agent.kind: %s\n", cfg.Agent.Kind)
		fmt.Printf("  auth_mode: %s\n", cfg.GitHub.ResolvedAuthMode)
		os.Exit(0)
	}

	logger.Info("symphony starting",
		"tracker.owner", cfg.Tracker.Owner,
		"tracker.project_number", cfg.Tracker.ProjectNumber,
		"agent.kind", cfg.Agent.Kind,
		"auth_mode", cfg.GitHub.ResolvedAuthMode,
	)

	// TODO: Start orchestrator event loop
	logger.Info("orchestrator not yet implemented, exiting")
}

func setupLogger(format, level string) *slog.Logger {
	var handler slog.Handler
	opts := &slog.HandlerOptions{Level: parseLogLevel(level)}

	switch format {
	case "json":
		handler = slog.NewJSONHandler(os.Stderr, opts)
	default:
		handler = slog.NewTextHandler(os.Stderr, opts)
	}

	return slog.New(handler)
}

func parseLogLevel(level string) slog.Level {
	switch level {
	case "debug":
		return slog.LevelDebug
	case "warn":
		return slog.LevelWarn
	case "error":
		return slog.LevelError
	default:
		return slog.LevelInfo
	}
}
