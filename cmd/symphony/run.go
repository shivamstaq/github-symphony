package main

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/joho/godotenv"
	"github.com/spf13/cobra"

	"github.com/shivamstaq/github-symphony/internal/agent"
	"github.com/shivamstaq/github-symphony/internal/agent/claude"
	agentcodex "github.com/shivamstaq/github-symphony/internal/agent/codex"
	agentmock "github.com/shivamstaq/github-symphony/internal/agent/mock"
	agentopencode "github.com/shivamstaq/github-symphony/internal/agent/opencode"
	codehostgithub "github.com/shivamstaq/github-symphony/internal/codehost/github"
	"github.com/shivamstaq/github-symphony/internal/config"
	"github.com/shivamstaq/github-symphony/internal/engine"
	"github.com/shivamstaq/github-symphony/internal/logging"
	promptpkg "github.com/shivamstaq/github-symphony/internal/prompt"
	"github.com/shivamstaq/github-symphony/internal/server"
	"github.com/shivamstaq/github-symphony/internal/state"
	"github.com/shivamstaq/github-symphony/internal/tracker"
	tui "github.com/shivamstaq/github-symphony/internal/tui/views"
	"github.com/shivamstaq/github-symphony/internal/workspace"

	// Register tracker implementations
	_ "github.com/shivamstaq/github-symphony/internal/tracker/github"
	_ "github.com/shivamstaq/github-symphony/internal/tracker/linear"
)

var runMock bool

var runCmd = &cobra.Command{
	Use:   "run",
	Short: "Start the Symphony orchestrator",
	Long:  `Starts the main orchestration loop: polls the tracker, dispatches agents, manages state.`,
	RunE:  runRun,
}

func init() {
	runCmd.Flags().BoolVar(&runMock, "mock", false, "Use mock agent instead of real CLI (for testing)")
	rootCmd.AddCommand(runCmd)
}

func runRun(cmd *cobra.Command, args []string) error {
	_ = godotenv.Load()

	cwd, err := os.Getwd()
	if err != nil {
		return err
	}

	symphonyDir := filepath.Join(cwd, ".symphony")
	configPath := filepath.Join(symphonyDir, "symphony.yaml")

	if _, err := os.Stat(configPath); os.IsNotExist(err) {
		return fmt.Errorf("no .symphony/symphony.yaml found — run 'symphony init' first")
	}

	// Load config
	cfg, err := config.LoadSymphonyConfig(configPath)
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}

	if err := config.ValidateSymphonyConfig(cfg); err != nil {
		return fmt.Errorf("config validation: %w", err)
	}

	// Setup logging — file-only to avoid corrupting TUI
	logPath := filepath.Join(symphonyDir, "logs", "orchestrator.jsonl")
	logger, logFile, err := logging.SetupJSONL(logPath, "info")
	if err != nil {
		return fmt.Errorf("setup logging: %w", err)
	}
	defer func() { _ = logFile.Close() }()
	slog.SetDefault(logger)

	// Setup event log
	evtLogPath := filepath.Join(symphonyDir, "state", "events.jsonl")
	evtLog, err := engine.NewEventLog(evtLogPath)
	if err != nil {
		return fmt.Errorf("setup event log: %w", err)
	}

	// Create tracker
	trk, err := tracker.NewTracker(cfg)
	if err != nil {
		return fmt.Errorf("create tracker: %w", err)
	}

	// Create agent
	var agentDep agent.Agent
	if runMock {
		logger.Info("using mock agent")
		agentDep = agentmock.NewSuccessAgent()
	} else {
		logger.Info("using real agent", "kind", cfg.Agent.Kind)
		logDir := filepath.Join(symphonyDir, "logs", "agents")
		socketDir := filepath.Join(symphonyDir, "sockets")
		switch cfg.Agent.Kind {
		case "claude_code":
			agentDep = claude.New(claude.Config{
				Binary:         cfg.Agent.Command,
				Model:          cfg.Agent.Claude.Model,
				AllowedTools:   cfg.Agent.Claude.AllowedTools,
				PermissionMode: cfg.Agent.Claude.PermissionProfile,
				LogDir:         logDir,
				SocketDir:      socketDir,
			})
		case "opencode":
			agentDep = agentopencode.New(agentopencode.Config{
				Binary:     cfg.Agent.Command,
				Model:      cfg.Agent.OpenCode.Model,
				ConfigFile: cfg.Agent.OpenCode.ConfigFile,
				LogDir:     logDir,
				SocketDir:  socketDir,
			})
		case "codex":
			agentDep = agentcodex.New(agentcodex.Config{
				Binary:         cfg.Agent.Command,
				ApprovalPolicy: cfg.Agent.Codex.ApprovalPolicy,
				LogDir:         logDir,
				SocketDir:      socketDir,
			})
		default:
			return fmt.Errorf("unknown agent kind: %s", cfg.Agent.Kind)
		}
	}

	// Verify agent binary is available
	if !runMock {
		switch cfg.Agent.Kind {
		case "opencode":
			if err := agentopencode.CheckDependencies(); err != nil {
				return fmt.Errorf("agent dependency check: %w", err)
			}
		case "codex":
			if err := agentcodex.CheckDependencies(); err != nil {
				return fmt.Errorf("agent dependency check: %w", err)
			}
		}
	}

	// Create workspace manager
	wsMgr := workspace.NewManager(workspace.ManagerConfig{
		WorktreeDir:  cfg.Workspace.WorktreeDir,
		RepoCacheDir: cfg.Workspace.RepoCacheDir,
		BranchPrefix: cfg.Git.BranchPrefix,
		UseWorktrees: cfg.Git.UseWorktrees,
		FetchDepth:   cfg.Git.FetchDepth,
		Hooks: workspace.HooksConfig{
			AfterCreate:  cfg.Hooks.AfterCreate,
			BeforeRun:    cfg.Hooks.BeforeRun,
			AfterRun:     cfg.Hooks.AfterRun,
			BeforeRemove: cfg.Hooks.BeforeRemove,
			TimeoutMs:    cfg.Hooks.TimeoutMs,
		},
	})

	// Create prompt router
	promptRouter := promptpkg.NewRouter(cfg.PromptRouting, filepath.Join(symphonyDir, "prompts"))

	// Create CodeHost (GitHub)
	apiURL := cfg.Auth.GitHub.APIURL
	if apiURL == "" {
		apiURL = "https://api.github.com"
	}
	codeHost := codehostgithub.New(apiURL, cfg.Auth.GitHub.Token)

	// Open state store
	storePath := filepath.Join(symphonyDir, "state", "symphony.db")
	_ = os.MkdirAll(filepath.Dir(storePath), 0755)
	store, err := state.Open(storePath)
	if err != nil {
		return fmt.Errorf("open state store: %w", err)
	}
	defer func() { _ = store.Close() }()

	eng := engine.New(engine.Deps{
		Config:       cfg,
		Tracker:      trk,
		Agent:        agentDep,
		CodeHost:     codeHost,
		Store:        store,
		Workspace:    wsMgr,
		PromptRouter: promptRouter,
		EventLog:     evtLog,
		Logger:       logger,
	})

	// Signal handling
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)

	logger.Info("symphony starting",
		"tracker", cfg.Tracker.Kind,
		"agent", cfg.Agent.Kind,
		"max_concurrent", cfg.Agent.MaxConcurrent,
		"poll_interval_ms", cfg.Polling.IntervalMs,
	)

	// Start HTTP API server
	if cfg.Server.Port > 0 {
		apiServer := server.NewAPIServer(eng, server.APIServerConfig{
			WebhookSecret: cfg.Auth.GitHub.WebhookSecret,
		})
		addr := fmt.Sprintf("%s:%d", cfg.Server.Host, cfg.Server.Port)
		httpServer := &http.Server{Addr: addr, Handler: apiServer.Handler()}
		go func() {
			logger.Info("HTTP API listening", "addr", addr)
			if err := httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
				logger.Error("HTTP server error", "error", err)
			}
		}()
		defer func() { _ = httpServer.Close() }()
	}

	// Run engine in background goroutine
	engineDone := make(chan error, 1)
	go func() {
		engineDone <- eng.Run(ctx)
	}()

	// Launch TUI in main goroutine
	tuiModel := tui.New(tui.Config{
		Engine:    eng,
		StartedAt: time.Now(),
		LogDir:    filepath.Join(symphonyDir, "logs"),
	})

	p := tea.NewProgram(tuiModel, tea.WithAltScreen())

	// Handle signals — cancel engine which will cause TUI to show shutdown
	go func() {
		select {
		case sig := <-sigCh:
			logger.Info("received signal", "signal", sig)
			cancel()
			// Give engine a moment to shut down, then quit TUI
			time.Sleep(500 * time.Millisecond)
			p.Send(tea.Quit())
		case <-ctx.Done():
		}
	}()

	// Run TUI — blocks until user quits
	if _, err := p.Run(); err != nil {
		// TUI error is non-fatal, engine may still be running
		logger.Error("TUI error", "error", err)
	}

	// TUI exited — shut down engine
	cancel()

	// Wait for engine to finish
	select {
	case err := <-engineDone:
		if err != nil && err != context.Canceled {
			return err
		}
	case <-time.After(5 * time.Second):
		logger.Warn("engine shutdown timed out")
	}

	return nil
}
