package config

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

// ServiceConfig is the typed runtime configuration derived from WORKFLOW.md front matter.
type ServiceConfig struct {
	Tracker     TrackerConfig
	GitHub      GitHubConfig
	Git         GitConfig
	Polling     PollingConfig
	Workspace   WorkspaceConfig
	Hooks       HooksConfig
	Agent       AgentConfig
	Codex       CodexConfig
	Claude      ClaudeConfig
	OpenCode    OpenCodeConfig
	PullRequest PullRequestConfig
	Server      ServerConfig
}

type TrackerConfig struct {
	Kind                    string
	Owner                   string
	ProjectNumber           int
	ProjectScope            string
	StatusFieldName         string
	ActiveValues            []string
	TerminalValues          []string
	PriorityFieldName       string
	PriorityValueMap        map[string]int
	ExecutableItemTypes     []string
	RequireIssueBacking     bool
	AllowDraftIssueConvert  bool
	RepoAllowlist           []string
	RepoDenylist            []string
	RequiredLabels          []string
	BlockedStatusValues     []string
}

type GitHubConfig struct {
	AuthMode          string
	APIURL            string
	Token             string
	AppID             string
	PrivateKey        string
	WebhookSecret     string
	InstallationID    string
	TokenRefreshSkew  int
	GraphQLPageSize   int
	RequestTimeoutMs  int
	RateLimitQPS      int
	ResolvedAuthMode  string // computed: "pat" or "app"
}

type GitConfig struct {
	BaseBranch         string
	BranchPrefix       string
	FetchDepth         int
	ReuseRepoCache     bool
	UseWorktrees       bool
	CleanUntracked     bool
	PushRemoteName     string
	CommitAuthorName   string
	CommitAuthorEmail  string
}

type PollingConfig struct {
	IntervalMs int
}

type WorkspaceConfig struct {
	Root           string
	RepoCacheDir   string
	WorktreeDir    string
	RemoveOnTerminal bool
}

type HooksConfig struct {
	AfterCreate  string
	BeforeRun    string
	AfterRun     string
	BeforeRemove string
	TimeoutMs    int
}

type AgentConfig struct {
	Kind                        string
	LaunchMode                  string
	Command                     string
	DefaultModel                string
	MaxConcurrentAgents         int
	MaxTurns                    int
	MaxRetryBackoffMs           int
	SessionReuseMode            string
	ReadTimeoutMs               int
	TurnTimeoutMs               int
	StallTimeoutMs              int
	EnableClientTools           bool
	EnableMCP                   bool
	MaxConcurrentByStatus       map[string]int
	MaxConcurrentByRepo         map[string]int
	ProviderParams              map[string]any
}

type CodexConfig struct {
	ApprovalPolicy    string
	ThreadSandbox     string
	TurnSandboxPolicy string
	Listen            string
	SchemaBundleDir   string
	ProviderParams    map[string]any
}

type ClaudeConfig struct {
	Model             string
	AllowedTools      []string
	MCPServers        []any
	ContinueOnPause   bool
	PermissionProfile any
	EnableSubagents   bool
	ProviderParams    map[string]any
}

type OpenCodeConfig struct {
	Model             string
	PermissionProfile any
	ConfigFile        string
	ResumeSession     bool
	MCPServers        []any
	ProviderParams    map[string]any
}

type PullRequestConfig struct {
	OpenPROnSuccess        bool
	DraftByDefault         bool
	ReuseExistingPR        bool
	HandoffProjectStatus   string
	CommentOnIssueWithPR   bool
	RequiredBeforeHandoff  []string
	CloseIssueOnMerge      bool
}

type ServerConfig struct {
	Port           int
	Host           string
	ReadTimeoutMs  int
	WriteTimeoutMs int
	CORSOrigins    []string
}

// NewServiceConfig builds a typed config from raw WORKFLOW.md front matter.
func NewServiceConfig(raw map[string]any) (*ServiceConfig, error) {
	// Re-marshal and unmarshal through a structured intermediate to handle
	// nested type conversions cleanly.
	data, err := yaml.Marshal(raw)
	if err != nil {
		return nil, fmt.Errorf("config marshal: %w", err)
	}

	var intermediate struct {
		Tracker     map[string]any `yaml:"tracker"`
		GitHub      map[string]any `yaml:"github"`
		Git         map[string]any `yaml:"git"`
		Polling     map[string]any `yaml:"polling"`
		Workspace   map[string]any `yaml:"workspace"`
		Hooks       map[string]any `yaml:"hooks"`
		Agent       map[string]any `yaml:"agent"`
		Codex       map[string]any `yaml:"codex"`
		Claude      map[string]any `yaml:"claude"`
		OpenCode    map[string]any `yaml:"opencode"`
		PullRequest map[string]any `yaml:"pull_request"`
		Server      map[string]any `yaml:"server"`
	}
	if err := yaml.Unmarshal(data, &intermediate); err != nil {
		return nil, fmt.Errorf("config unmarshal: %w", err)
	}

	cfg := &ServiceConfig{}
	cfg.applyDefaults()

	// Apply values from raw config
	cfg.Tracker = parseTracker(intermediate.Tracker, cfg.Tracker)
	cfg.GitHub = parseGitHub(intermediate.GitHub, cfg.GitHub)
	cfg.Git = parseGit(intermediate.Git, cfg.Git)
	cfg.Polling = parsePolling(intermediate.Polling, cfg.Polling)
	cfg.Workspace = parseWorkspace(intermediate.Workspace, cfg.Workspace)
	cfg.Hooks = parseHooks(intermediate.Hooks, cfg.Hooks)
	cfg.Agent = parseAgent(intermediate.Agent, cfg.Agent)
	cfg.Codex = parseCodex(intermediate.Codex, cfg.Codex)
	cfg.Claude = parseClaude(intermediate.Claude, cfg.Claude)
	cfg.OpenCode = parseOpenCode(intermediate.OpenCode, cfg.OpenCode)
	cfg.PullRequest = parsePullRequest(intermediate.PullRequest, cfg.PullRequest)
	cfg.Server = parseServer(intermediate.Server, cfg.Server)

	// Resolve environment variables
	cfg.resolveEnvVars()

	// Resolve path expansion
	cfg.resolvePathExpansion()

	// Derive workspace defaults
	cfg.deriveWorkspaceDefaults()

	// Resolve auth mode
	cfg.resolveAuthMode()

	return cfg, nil
}

func (c *ServiceConfig) applyDefaults() {
	c.Tracker = TrackerConfig{
		ProjectScope:        "organization",
		StatusFieldName:     "Status",
		ActiveValues:        []string{"Todo", "Ready", "In Progress"},
		TerminalValues:      []string{"Done", "Closed", "Cancelled", "Canceled", "Duplicate"},
		PriorityFieldName:   "Priority",
		ExecutableItemTypes:  []string{"issue"},
		RequireIssueBacking: true,
	}
	c.GitHub = GitHubConfig{
		AuthMode:         "auto",
		APIURL:           "https://api.github.com",
		TokenRefreshSkew: 300000,
		GraphQLPageSize:  50,
		RequestTimeoutMs: 30000,
		RateLimitQPS:     10,
	}
	c.Git = GitConfig{
		BranchPrefix:   "symphony/",
		FetchDepth:     0,
		ReuseRepoCache: true,
		UseWorktrees:   true,
		PushRemoteName: "origin",
	}
	c.Polling = PollingConfig{IntervalMs: 30000}
	c.Workspace = WorkspaceConfig{
		RemoveOnTerminal: true,
	}
	c.Hooks = HooksConfig{TimeoutMs: 60000}
	c.Agent = AgentConfig{
		MaxConcurrentAgents: 10,
		MaxTurns:            20,
		MaxRetryBackoffMs:   300000,
		SessionReuseMode:    "continue_if_supported",
		ReadTimeoutMs:       5000,
		TurnTimeoutMs:       3600000,
		StallTimeoutMs:      300000,
		EnableClientTools:   true,
		EnableMCP:           true,
	}
	c.Claude = ClaudeConfig{
		ContinueOnPause: true,
	}
	c.OpenCode = OpenCodeConfig{
		ResumeSession: true,
	}
	c.Codex = CodexConfig{
		Listen: "stdio://",
	}
	c.PullRequest = PullRequestConfig{
		OpenPROnSuccess:      true,
		DraftByDefault:       true,
		ReuseExistingPR:      true,
		CommentOnIssueWithPR: true,
	}
	c.Server = ServerConfig{
		Host:           "0.0.0.0",
		ReadTimeoutMs:  30000,
		WriteTimeoutMs: 60000,
	}
}

func (c *ServiceConfig) resolveEnvVars() {
	c.GitHub.Token = resolveEnv(c.GitHub.Token)
	c.GitHub.AppID = resolveEnv(c.GitHub.AppID)
	c.GitHub.PrivateKey = resolveEnv(c.GitHub.PrivateKey)
	c.GitHub.WebhookSecret = resolveEnv(c.GitHub.WebhookSecret)
	c.GitHub.InstallationID = resolveEnv(c.GitHub.InstallationID)
}

func (c *ServiceConfig) resolveAuthMode() {
	mode := c.GitHub.AuthMode
	if mode == "" {
		mode = "auto"
	}

	switch mode {
	case "pat":
		c.GitHub.ResolvedAuthMode = "pat"
	case "app":
		c.GitHub.ResolvedAuthMode = "app"
	case "auto":
		hasApp := c.GitHub.AppID != "" && c.GitHub.PrivateKey != ""
		hasPAT := c.GitHub.Token != ""
		if hasApp {
			c.GitHub.ResolvedAuthMode = "app"
		} else if hasPAT {
			c.GitHub.ResolvedAuthMode = "pat"
		}
		// If neither, leave empty — validation will catch it
	}
}

// resolveEnv resolves a $VAR_NAME reference to its environment variable value.
func resolveEnv(val string) string {
	if strings.HasPrefix(val, "$") {
		envName := val[1:]
		return os.Getenv(envName)
	}
	return val
}

// Helper functions to parse individual config sections from raw maps.

func parseTracker(raw map[string]any, def TrackerConfig) TrackerConfig {
	if raw == nil {
		return def
	}
	if v, ok := raw["kind"].(string); ok { def.Kind = v }
	if v, ok := raw["owner"].(string); ok { def.Owner = v }
	if v, ok := raw["project_number"].(int); ok { def.ProjectNumber = v }
	if v, ok := raw["project_scope"].(string); ok { def.ProjectScope = v }
	if v, ok := raw["status_field_name"].(string); ok { def.StatusFieldName = v }
	if v, ok := raw["active_values"].([]any); ok { def.ActiveValues = toStringSlice(v) }
	if v, ok := raw["terminal_values"].([]any); ok { def.TerminalValues = toStringSlice(v) }
	if v, ok := raw["priority_field_name"].(string); ok { def.PriorityFieldName = v }
	if v, ok := raw["priority_value_map"].(map[string]any); ok { def.PriorityValueMap = toIntMap(v) }
	if v, ok := raw["executable_item_types"].([]any); ok { def.ExecutableItemTypes = toStringSlice(v) }
	if v, ok := raw["require_issue_backing"].(bool); ok { def.RequireIssueBacking = v }
	if v, ok := raw["allow_draft_issue_conversion"].(bool); ok { def.AllowDraftIssueConvert = v }
	if v, ok := raw["repo_allowlist"].([]any); ok { def.RepoAllowlist = toStringSlice(v) }
	if v, ok := raw["repo_denylist"].([]any); ok { def.RepoDenylist = toStringSlice(v) }
	if v, ok := raw["required_labels"].([]any); ok { def.RequiredLabels = toStringSlice(v) }
	if v, ok := raw["blocked_status_values"].([]any); ok { def.BlockedStatusValues = toStringSlice(v) }
	return def
}

func parseGitHub(raw map[string]any, def GitHubConfig) GitHubConfig {
	if raw == nil {
		return def
	}
	if v, ok := raw["auth_mode"].(string); ok { def.AuthMode = v }
	if v, ok := raw["api_url"].(string); ok { def.APIURL = v }
	if v, ok := raw["token"].(string); ok { def.Token = v }
	if v, ok := raw["app_id"].(string); ok { def.AppID = v }
	if v, ok := raw["private_key"].(string); ok { def.PrivateKey = v }
	if v, ok := raw["webhook_secret"].(string); ok { def.WebhookSecret = v }
	if v, ok := raw["installation_id"].(string); ok { def.InstallationID = v }
	if v, ok := raw["token_refresh_skew_ms"].(int); ok { def.TokenRefreshSkew = v }
	if v, ok := raw["graphql_page_size"].(int); ok { def.GraphQLPageSize = v }
	if v, ok := raw["request_timeout_ms"].(int); ok { def.RequestTimeoutMs = v }
	if v, ok := raw["rate_limit_qps"].(int); ok { def.RateLimitQPS = v }
	return def
}

func parseGit(raw map[string]any, def GitConfig) GitConfig {
	if raw == nil {
		return def
	}
	if v, ok := raw["base_branch"].(string); ok { def.BaseBranch = v }
	if v, ok := raw["branch_prefix"].(string); ok { def.BranchPrefix = v }
	if v, ok := raw["fetch_depth"].(int); ok { def.FetchDepth = v }
	if v, ok := raw["reuse_repo_cache"].(bool); ok { def.ReuseRepoCache = v }
	if v, ok := raw["use_worktrees"].(bool); ok { def.UseWorktrees = v }
	if v, ok := raw["clean_untracked_before_run"].(bool); ok { def.CleanUntracked = v }
	if v, ok := raw["push_remote_name"].(string); ok { def.PushRemoteName = v }
	if v, ok := raw["commit_author_name"].(string); ok { def.CommitAuthorName = v }
	if v, ok := raw["commit_author_email"].(string); ok { def.CommitAuthorEmail = v }
	return def
}

func parsePolling(raw map[string]any, def PollingConfig) PollingConfig {
	if raw == nil {
		return def
	}
	if v, ok := raw["interval_ms"].(int); ok { def.IntervalMs = v }
	return def
}

func parseWorkspace(raw map[string]any, def WorkspaceConfig) WorkspaceConfig {
	if raw == nil {
		return def
	}
	if v, ok := raw["root"].(string); ok { def.Root = v }
	if v, ok := raw["repo_cache_dir"].(string); ok { def.RepoCacheDir = v }
	if v, ok := raw["worktree_dir"].(string); ok { def.WorktreeDir = v }
	if v, ok := raw["remove_on_terminal"].(bool); ok { def.RemoveOnTerminal = v }
	return def
}

func parseHooks(raw map[string]any, def HooksConfig) HooksConfig {
	if raw == nil {
		return def
	}
	if v, ok := raw["after_create"].(string); ok { def.AfterCreate = v }
	if v, ok := raw["before_run"].(string); ok { def.BeforeRun = v }
	if v, ok := raw["after_run"].(string); ok { def.AfterRun = v }
	if v, ok := raw["before_remove"].(string); ok { def.BeforeRemove = v }
	if v, ok := raw["timeout_ms"].(int); ok { def.TimeoutMs = v }
	return def
}

func parseAgent(raw map[string]any, def AgentConfig) AgentConfig {
	if raw == nil {
		return def
	}
	if v, ok := raw["kind"].(string); ok { def.Kind = v }
	if v, ok := raw["launch_mode"].(string); ok { def.LaunchMode = v }
	if v, ok := raw["command"].(string); ok { def.Command = v }
	if v, ok := raw["default_model"].(string); ok { def.DefaultModel = v }
	if v, ok := raw["max_concurrent_agents"].(int); ok { def.MaxConcurrentAgents = v }
	if v, ok := raw["max_turns"].(int); ok { def.MaxTurns = v }
	if v, ok := raw["max_retry_backoff_ms"].(int); ok { def.MaxRetryBackoffMs = v }
	if v, ok := raw["session_reuse_mode"].(string); ok { def.SessionReuseMode = v }
	if v, ok := raw["read_timeout_ms"].(int); ok { def.ReadTimeoutMs = v }
	if v, ok := raw["turn_timeout_ms"].(int); ok { def.TurnTimeoutMs = v }
	if v, ok := raw["stall_timeout_ms"].(int); ok { def.StallTimeoutMs = v }
	if v, ok := raw["enable_client_tools"].(bool); ok { def.EnableClientTools = v }
	if v, ok := raw["enable_mcp"].(bool); ok { def.EnableMCP = v }
	if v, ok := raw["max_concurrent_agents_by_project_status"].(map[string]any); ok {
		def.MaxConcurrentByStatus = toIntMap(v)
	}
	if v, ok := raw["max_concurrent_agents_by_repo"].(map[string]any); ok {
		def.MaxConcurrentByRepo = toIntMap(v)
	}
	if v, ok := raw["provider_params"].(map[string]any); ok { def.ProviderParams = v }
	return def
}

func parseClaude(raw map[string]any, def ClaudeConfig) ClaudeConfig {
	if raw == nil {
		return def
	}
	if v, ok := raw["model"].(string); ok { def.Model = v }
	if v, ok := raw["continue_on_pause_turn"].(bool); ok { def.ContinueOnPause = v }
	if v, ok := raw["enable_subagents"].(bool); ok { def.EnableSubagents = v }
	if v, ok := raw["allowed_tools"].([]any); ok { def.AllowedTools = toStringSlice(v) }
	if v, ok := raw["mcp_servers"].([]any); ok { def.MCPServers = v }
	if v, ok := raw["permission_profile"]; ok { def.PermissionProfile = v }
	if v, ok := raw["provider_params"].(map[string]any); ok { def.ProviderParams = v }
	return def
}

func parseCodex(raw map[string]any, def CodexConfig) CodexConfig {
	if raw == nil {
		return def
	}
	if v, ok := raw["approval_policy"].(string); ok { def.ApprovalPolicy = v }
	if v, ok := raw["thread_sandbox"].(string); ok { def.ThreadSandbox = v }
	if v, ok := raw["turn_sandbox_policy"].(string); ok { def.TurnSandboxPolicy = v }
	if v, ok := raw["listen"].(string); ok { def.Listen = v }
	if v, ok := raw["schema_bundle_dir"].(string); ok { def.SchemaBundleDir = v }
	if v, ok := raw["provider_params"].(map[string]any); ok { def.ProviderParams = v }
	return def
}

func parseOpenCode(raw map[string]any, def OpenCodeConfig) OpenCodeConfig {
	if raw == nil {
		return def
	}
	if v, ok := raw["model"].(string); ok { def.Model = v }
	if v, ok := raw["permission_profile"]; ok { def.PermissionProfile = v }
	if v, ok := raw["config_file"].(string); ok { def.ConfigFile = v }
	if v, ok := raw["resume_session"].(bool); ok { def.ResumeSession = v }
	if v, ok := raw["mcp_servers"].([]any); ok { def.MCPServers = v }
	if v, ok := raw["provider_params"].(map[string]any); ok { def.ProviderParams = v }
	return def
}

func parsePullRequest(raw map[string]any, def PullRequestConfig) PullRequestConfig {
	if raw == nil {
		return def
	}
	if v, ok := raw["open_pr_on_success"].(bool); ok { def.OpenPROnSuccess = v }
	if v, ok := raw["draft_by_default"].(bool); ok { def.DraftByDefault = v }
	if v, ok := raw["reuse_existing_pr"].(bool); ok { def.ReuseExistingPR = v }
	if v, ok := raw["handoff_project_status"].(string); ok { def.HandoffProjectStatus = v }
	if v, ok := raw["comment_on_issue_with_pr"].(bool); ok { def.CommentOnIssueWithPR = v }
	if v, ok := raw["close_issue_on_merge"].(bool); ok { def.CloseIssueOnMerge = v }
	if v, ok := raw["required_before_handoff_checks"].([]any); ok { def.RequiredBeforeHandoff = toStringSlice(v) }
	return def
}

func parseServer(raw map[string]any, def ServerConfig) ServerConfig {
	if raw == nil {
		return def
	}
	if v, ok := raw["port"].(int); ok { def.Port = v }
	if v, ok := raw["host"].(string); ok { def.Host = v }
	if v, ok := raw["read_timeout_ms"].(int); ok { def.ReadTimeoutMs = v }
	if v, ok := raw["write_timeout_ms"].(int); ok { def.WriteTimeoutMs = v }
	if v, ok := raw["cors_origins"].([]any); ok { def.CORSOrigins = toStringSlice(v) }
	return def
}

func toStringSlice(raw []any) []string {
	out := make([]string, 0, len(raw))
	for _, v := range raw {
		if s, ok := v.(string); ok {
			out = append(out, s)
		}
	}
	return out
}

func toIntMap(raw map[string]any) map[string]int {
	out := make(map[string]int, len(raw))
	for k, v := range raw {
		switch val := v.(type) {
		case int:
			out[k] = val
		case float64:
			out[k] = int(val)
		}
	}
	return out
}

func (c *ServiceConfig) resolvePathExpansion() {
	c.Workspace.Root = expandPath(c.Workspace.Root)
	c.Workspace.RepoCacheDir = expandPath(c.Workspace.RepoCacheDir)
	c.Workspace.WorktreeDir = expandPath(c.Workspace.WorktreeDir)
	c.Codex.SchemaBundleDir = expandPath(c.Codex.SchemaBundleDir)
	c.OpenCode.ConfigFile = expandPath(c.OpenCode.ConfigFile)
}

func (c *ServiceConfig) deriveWorkspaceDefaults() {
	if c.Workspace.Root == "" {
		c.Workspace.Root = filepath.Join(os.TempDir(), "symphony_workspaces")
	}
	if c.Workspace.RepoCacheDir == "" {
		c.Workspace.RepoCacheDir = filepath.Join(c.Workspace.Root, "repo_cache")
	}
	if c.Workspace.WorktreeDir == "" {
		c.Workspace.WorktreeDir = filepath.Join(c.Workspace.Root, "worktrees")
	}
}

// expandPath resolves ~ to home dir and $VAR in path strings.
func expandPath(p string) string {
	if p == "" {
		return p
	}
	if strings.HasPrefix(p, "~/") || p == "~" {
		home, err := os.UserHomeDir()
		if err == nil {
			p = filepath.Join(home, p[1:])
		}
	}
	return os.ExpandEnv(p)
}
