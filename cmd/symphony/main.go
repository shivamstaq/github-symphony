package main

import (
	"context"
	"flag"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/joho/godotenv"
	"github.com/shivamstaq/github-symphony/internal/adapter"
	"github.com/shivamstaq/github-symphony/internal/config"
	ghub "github.com/shivamstaq/github-symphony/internal/github"
	"github.com/shivamstaq/github-symphony/internal/logging"
	"github.com/shivamstaq/github-symphony/internal/orchestrator"
	"github.com/shivamstaq/github-symphony/internal/server"
	"github.com/shivamstaq/github-symphony/internal/state"
	symphonyTUI "github.com/shivamstaq/github-symphony/internal/tui"
	"github.com/shivamstaq/github-symphony/internal/webhook"
	"github.com/shivamstaq/github-symphony/internal/workspace"
)

func main() {
	// Load .env file from CWD
	_ = godotenv.Load()

	var (
		port            int
		logFormat       string
		logLevel        string
		stateDir        string
		doctor          bool
		noTUI           bool
		victoriaLogsURL string
	)

	flag.IntVar(&port, "port", 9097, "HTTP server port (0 to disable)")
	flag.StringVar(&logFormat, "log-format", "text", "Log output format: text, json")
	flag.BoolVar(&noTUI, "no-tui", false, "Disable TUI, use plain log output (for CI/Docker)")
	flag.StringVar(&logLevel, "log-level", "info", "Log level: debug, info, warn, error")
	flag.StringVar(&stateDir, "state-dir", "", "Directory for persistent state")
	flag.BoolVar(&doctor, "doctor", false, "Validate config and environment, then exit")
	flag.StringVar(&victoriaLogsURL, "victorialogs-url", "http://localhost:9428", "VictoriaLogs URL for log push (empty to disable)")
	flag.Parse()

	logger := setupLogger(logFormat, logLevel)

	// Wrap logger with VictoriaLogs pusher if configured
	var logPusher *logging.LogPusher
	if victoriaLogsURL != "" {
		logPusher = logging.NewLogPusherFromURL(logger.Handler(), victoriaLogsURL)
		logger = slog.New(logPusher)
	}

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

	// Apply CLI overrides (port default is 9097; use --port 0 to disable HTTP server)
	cfg.Server.Port = port

	// Validate
	if err := config.ValidateForDispatch(cfg); err != nil {
		logger.Error("config validation failed", "error", err)
		os.Exit(1)
	}

	// Resolve workspace paths
	wsRoot := cfg.Workspace.Root
	if wsRoot == "" {
		wsRoot = filepath.Join(os.TempDir(), "symphony_workspaces")
	}
	wsCacheDir := cfg.Workspace.RepoCacheDir
	if wsCacheDir == "" {
		wsCacheDir = filepath.Join(wsRoot, "repo_cache")
	}
	wsWorktreeDir := cfg.Workspace.WorktreeDir
	if wsWorktreeDir == "" {
		wsWorktreeDir = filepath.Join(wsRoot, "worktrees")
	}

	// Resolve state dir
	if stateDir == "" {
		stateDir = filepath.Join(wsRoot, ".symphony")
	}

	// Doctor mode
	if doctor {
		runDoctor(cfg, wsRoot, stateDir)
		return
	}

	logger.Info("symphony starting",
		"tracker.owner", cfg.Tracker.Owner,
		"tracker.project_number", cfg.Tracker.ProjectNumber,
		"agent.kind", cfg.Agent.Kind,
		"auth_mode", cfg.GitHub.ResolvedAuthMode,
		"workspace_root", wsRoot,
	)

	// Open persistent state store
	if err := os.MkdirAll(stateDir, 0755); err != nil {
		logger.Error("cannot create state directory", "path", stateDir, "error", err)
		os.Exit(1)
	}
	store, err := state.Open(filepath.Join(stateDir, "symphony.db"))
	if err != nil {
		logger.Error("state store open failed", "error", err)
		os.Exit(1)
	}
	defer func() { _ = store.Close() }()

	// Create GitHub auth provider
	authProvider := ghub.NewPATProvider(cfg.GitHub.Token)

	// Create GitHub clients
	token, err := authProvider.Token(context.Background(), ghub.RepoRef{})
	if err != nil {
		logger.Error("auth failed", "error", err)
		os.Exit(1)
	}

	apiURL := strings.TrimSuffix(cfg.GitHub.APIURL, "/")
	if apiURL == "" {
		apiURL = "https://api.github.com"
	}
	graphqlEndpoint := apiURL + "/graphql"

	gqlClient := ghub.NewGraphQLClient(graphqlEndpoint, token)
	writeBack := ghub.NewWriteBack(apiURL, graphqlEndpoint, token)

	// Fetch project field metadata for status updates (best-effort)
	var projectMeta *ghub.ProjectFieldMeta
	if cfg.PullRequest.HandoffProjectStatus != "" {
		meta, err := gqlClient.FetchProjectFieldMeta(
			context.Background(),
			cfg.Tracker.Owner,
			cfg.Tracker.ProjectNumber,
			cfg.Tracker.ProjectScope,
			cfg.Tracker.StatusFieldName,
		)
		if err != nil {
			logger.Warn("could not fetch project field metadata (status updates will be skipped)", "error", err)
		} else {
			projectMeta = meta
			logger.Info("project metadata loaded",
				"project_id", meta.ProjectID,
				"field_id", meta.FieldID,
				"options", len(meta.Options),
			)
		}
	}

	// Create GitHub source
	ghSource := ghub.NewSource(gqlClient, ghub.SourceConfig{
		Owner:            cfg.Tracker.Owner,
		ProjectNumber:    cfg.Tracker.ProjectNumber,
		ProjectScope:     cfg.Tracker.ProjectScope,
		StatusFieldName:  cfg.Tracker.StatusFieldName,
		PageSize:         cfg.GitHub.GraphQLPageSize,
		PriorityValueMap: cfg.Tracker.PriorityValueMap,
	})

	sourceBridge := orchestrator.NewSourceBridge(ghSource, cfg.Tracker.PriorityValueMap)

	// Create workspace manager
	wsMgr := workspace.NewManager(workspace.ManagerConfig{
		WorktreeDir:  wsWorktreeDir,
		RepoCacheDir: wsCacheDir,
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

	// Create worker runner
	runner := orchestrator.NewRunner(orchestrator.WorkerDeps{
		WorkspaceManager: wsMgr,
		AdapterFactory: func(cwd string) (adapter.AdapterClient, error) {
			acfg := adapter.AdapterConfig{
				Kind: cfg.Agent.Kind,
				Cwd:  cwd,
			}
			switch cfg.Agent.Kind {
			case "claude_code":
				acfg.Command = "claude" // uses locally-authenticated claude CLI
				acfg.Model = cfg.Claude.Model
				acfg.AllowedTools = cfg.Claude.AllowedTools
				permMode := "bypassPermissions"
				if p, ok := cfg.Claude.PermissionProfile.(string); ok && p != "" {
					permMode = p
				}
				acfg.PermissionMode = permMode
			case "opencode":
				acfg.Command = "opencode"
				acfg.Args = []string{"acp"}
			case "codex":
				acfg.Command = "codex"
				acfg.Args = []string{"app-server"}
			}
			if cfg.Agent.Command != "" {
				acfg.Command = "bash"
				acfg.Args = []string{"-lc", cfg.Agent.Command}
			}
			return adapter.NewAdapter(acfg)
		},
		Source:         sourceBridge,
		WriteBack:      writeBack,
		StateStore:     store,
		PromptTemplate: wf.PromptTemplate,
		MaxTurns:       cfg.Agent.MaxTurns,
		HooksBefore:    cfg.Hooks.BeforeRun,
		HooksAfter:     cfg.Hooks.AfterRun,
		HooksTimeoutMs: cfg.Hooks.TimeoutMs,
		PullRequestCfg: buildPRConfig(cfg, projectMeta),
		GitToken:       token,
	})

	// Create orchestrator
	orch := orchestrator.New(orchestrator.OrchestratorConfig{
		PollIntervalMs:      cfg.Polling.IntervalMs,
		MaxConcurrentAgents: cfg.Agent.MaxConcurrentAgents,
		StallTimeoutMs:      cfg.Agent.StallTimeoutMs,
		MaxRetryBackoffMs:   cfg.Agent.MaxRetryBackoffMs,
		Eligibility: orchestrator.EligibilityConfig{
			ActiveValues:        cfg.Tracker.ActiveValues,
			TerminalValues:      cfg.Tracker.TerminalValues,
			ExecutableItemTypes: cfg.Tracker.ExecutableItemTypes,
			RequireIssueBacking: cfg.Tracker.RequireIssueBacking,
			RepoAllowlist:       cfg.Tracker.RepoAllowlist,
			RepoDenylist:        cfg.Tracker.RepoDenylist,
			RequiredLabels:      cfg.Tracker.RequiredLabels,
			BlockedStatusValues: cfg.Tracker.BlockedStatusValues,
		},
		ActiveValues:   cfg.Tracker.ActiveValues,
		TerminalValues: cfg.Tracker.TerminalValues,
	}, sourceBridge, runner)

	// Restore persisted retries
	retries, err := store.LoadRetries()
	if err != nil {
		logger.Warn("failed to load persisted retries", "error", err)
	} else {
		for _, r := range retries {
			orch.RestoreRetry(orchestrator.RetryEntry{
				WorkItemID:      r.WorkItemID,
				IssueIdentifier: r.IssueIdentifier,
				Attempt:         r.Attempt,
				DueAt:           time.UnixMilli(r.DueAtMs),
				Error:           r.Error,
			})
		}
		if len(retries) > 0 {
			logger.Info("restored persisted retries", "count", len(retries))
		}
	}

	// Restore totals
	totals, err := store.LoadTotals()
	if err != nil {
		logger.Warn("failed to load persisted totals", "error", err)
	}
	_ = totals // TODO: apply to orchestrator state

	startedAt := time.Now()

	stateProvider := &orchestratorStateProvider{
		orch:      orch,
		authMode:  cfg.GitHub.ResolvedAuthMode,
		startedAt: startedAt,
	}

	// Start HTTP server if configured
	logger.Debug("server port check", "port", cfg.Server.Port)
	if cfg.Server.Port > 0 {

		srv := server.New(server.Config{
			Port:           cfg.Server.Port,
			Host:           cfg.Server.Host,
			ReadTimeoutMs:  cfg.Server.ReadTimeoutMs,
			WriteTimeoutMs: cfg.Server.WriteTimeoutMs,
		}, stateProvider)

		// Mount webhook handler if secret is configured
		if cfg.GitHub.WebhookSecret != "" {
			wh := webhook.NewHandler(cfg.GitHub.WebhookSecret, func(eventType string, _ []byte) {
				logger.Info("webhook received, triggering refresh", "event", eventType)
				orch.SetPendingRefresh()
			})
			srv.MountWebhook(wh)
		}

		go func() {
			logger.Info("HTTP server starting", "port", cfg.Server.Port)
			if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
				logger.Error("HTTP server failed", "error", err)
			}
		}()
	}

	// Start workflow file watcher
	watcher, err := config.NewWatcher(workflowPath, func(newWf *config.WorkflowDefinition) {
		logger.Info("workflow reloaded")
		// TODO: apply new config to running orchestrator
	})
	if err != nil {
		logger.Warn("workflow watcher failed to start", "error", err)
	} else {
		defer func() { _ = watcher.Close() }()
	}

	// Signal handling
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	sigChan := make(chan os.Signal, 2)
	signal.Notify(sigChan, syscall.SIGTERM, syscall.SIGINT)

	go func() {
		sig := <-sigChan
		logger.Info("received signal, initiating shutdown", "signal", sig)
		cancel()

		// Second signal: force exit
		sig = <-sigChan
		logger.Warn("received second signal, force exiting", "signal", sig)
		os.Exit(1)
	}()

	// Run orchestrator in background
	go orch.Run(ctx)

	// Start TUI if running interactively (terminal attached and not disabled)
	if !noTUI && isTerminal() {
		tuiModel := symphonyTUI.New(symphonyTUI.Config{
			StateProvider: stateProvider,
			EventBus:      orch.Events,
			StartedAt:     startedAt,
		})
		p := tea.NewProgram(tuiModel, tea.WithAltScreen())
		if _, err := p.Run(); err != nil {
			logger.Error("TUI error", "error", err)
		}
		cancel() // TUI quit → cancel orchestrator
	} else {
		// Non-interactive: block until signal
		<-ctx.Done()
	}

	// Graceful shutdown
	logger.Info("shutting down...")
	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer shutdownCancel()
	orch.Shutdown(shutdownCtx)

	// Persist retry state
	entries := orch.GetRetryEntries()
	for _, e := range entries {
		_ = store.SaveRetry(state.RetryRecord{
			WorkItemID:      e.WorkItemID,
			IssueIdentifier: e.IssueIdentifier,
			Attempt:         e.Attempt,
			DueAtMs:         e.DueAt.UnixMilli(),
			Error:           e.Error,
		})
	}
	if len(entries) > 0 {
		logger.Info("persisted retry state", "count", len(entries))
	}

	logger.Info("symphony stopped")

	// Flush log pusher
	if logPusher != nil {
		logPusher.Close()
	}
}

// orchestratorStateProvider bridges the orchestrator to the server's StateProvider interface.
type orchestratorStateProvider struct {
	orch      *orchestrator.Orchestrator
	authMode  string
	startedAt time.Time
}

func (p *orchestratorStateProvider) GetState() orchestrator.State { return p.orch.GetState() }
func (p *orchestratorStateProvider) IsHealthy() bool             { return true }
func (p *orchestratorStateProvider) AuthMode() string            { return p.authMode }
func (p *orchestratorStateProvider) TriggerRefresh()             { p.orch.SetPendingRefresh() }
func (p *orchestratorStateProvider) StartedAt() time.Time        { return p.startedAt }

func runDoctor(cfg *config.ServiceConfig, wsRoot, stateDir string) {
	fmt.Println("PASS: workflow file loaded and parsed")
	fmt.Println("PASS: config validation passed")
	fmt.Printf("  tracker.kind: %s\n", cfg.Tracker.Kind)
	fmt.Printf("  tracker.owner: %s\n", cfg.Tracker.Owner)
	fmt.Printf("  tracker.project_number: %d\n", cfg.Tracker.ProjectNumber)
	fmt.Printf("  agent.kind: %s\n", cfg.Agent.Kind)
	fmt.Printf("  auth_mode: %s\n", cfg.GitHub.ResolvedAuthMode)
	fmt.Printf("  workspace_root: %s\n", wsRoot)
	fmt.Printf("  state_dir: %s\n", stateDir)

	// Check agent runtime
	switch cfg.Agent.Kind {
	case "claude_code":
		if err := checkBinaryExists("claude"); err != nil {
			fmt.Printf("FAIL: claude CLI not found on PATH: %v\n", err)
			fmt.Println("  Install: https://docs.anthropic.com/en/docs/claude-code")
			fmt.Println("  Then run: claude login")
			os.Exit(1)
		}
		fmt.Println("PASS: claude CLI found on PATH")
	case "opencode":
		if err := checkBinaryExists("opencode"); err != nil {
			fmt.Printf("FAIL: opencode not found on PATH: %v\n", err)
			os.Exit(1)
		}
		fmt.Println("PASS: opencode found on PATH")
	case "codex":
		if err := checkBinaryExists("codex"); err != nil {
			fmt.Printf("FAIL: codex not found on PATH: %v\n", err)
			os.Exit(1)
		}
		fmt.Println("PASS: codex found on PATH")
	}

	// Check git
	if err := checkBinaryExists("git"); err != nil {
		fmt.Printf("FAIL: git not found on PATH: %v\n", err)
		os.Exit(1)
	}
	fmt.Println("PASS: git found on PATH")

	// Check GitHub connectivity
	if cfg.GitHub.Token != "" {
		apiURL := cfg.GitHub.APIURL
		if apiURL == "" {
			apiURL = "https://api.github.com"
		}
		provider := ghub.NewPATProvider(cfg.GitHub.Token)
		client, err := provider.HTTPClient(context.Background(), ghub.RepoRef{})
		if err != nil {
			fmt.Printf("FAIL: GitHub auth: %v\n", err)
			os.Exit(1)
		}
		resp, err := client.Get(apiURL + "/user")
		if err != nil {
			fmt.Printf("FAIL: GitHub connectivity: %v\n", err)
			os.Exit(1)
		}
		_ = resp.Body.Close()
		if resp.StatusCode == 200 {
			fmt.Println("PASS: GitHub API connectivity verified")
		} else {
			fmt.Printf("WARN: GitHub API returned status %d\n", resp.StatusCode)
		}
	}

	fmt.Println("\nAll checks passed.")
}

func checkBinaryExists(name string) error {
	_, err := lookPath(name)
	return err
}

// lookPath is os/exec.LookPath but avoids importing os/exec in main
// just for this one function when we already have it available.
func lookPath(name string) (string, error) {
	// Simple PATH search
	pathEnv := os.Getenv("PATH")
	for _, dir := range filepath.SplitList(pathEnv) {
		full := filepath.Join(dir, name)
		if info, err := os.Stat(full); err == nil && !info.IsDir() {
			return full, nil
		}
	}
	return "", fmt.Errorf("%s not found in PATH", name)
}

func isTerminal() bool {
	fi, err := os.Stdout.Stat()
	if err != nil {
		return false
	}
	return fi.Mode()&os.ModeCharDevice != 0
}

func buildPRConfig(cfg *config.ServiceConfig, meta *ghub.ProjectFieldMeta) orchestrator.PullRequestConfig {
	pc := orchestrator.PullRequestConfig{
		OpenPROnSuccess:      cfg.PullRequest.OpenPROnSuccess,
		DraftByDefault:       cfg.PullRequest.DraftByDefault,
		HandoffProjectStatus: cfg.PullRequest.HandoffProjectStatus,
		CommentOnIssue:       cfg.PullRequest.CommentOnIssueWithPR,
	}
	if meta != nil {
		pc.ProjectID = meta.ProjectID
		pc.StatusFieldID = meta.FieldID
		if optID, ok := meta.Options[cfg.PullRequest.HandoffProjectStatus]; ok {
			pc.HandoffOptionID = optID
		}
	}
	return pc
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
