package config

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

// SymphonyConfig is the typed runtime configuration parsed from symphony.yaml.
type SymphonyConfig struct {
	Tracker      TrackerV2Config      `yaml:"tracker"`
	Auth         AuthConfig           `yaml:"auth"`
	Agent        AgentV2Config        `yaml:"agent"`
	Git          GitV2Config          `yaml:"git"`
	Workspace    WorkspaceV2Config    `yaml:"workspace"`
	Polling      PollingV2Config      `yaml:"polling"`
	PullRequest  PullRequestV2Config  `yaml:"pull_request"`
	Hooks        HooksV2Config        `yaml:"hooks"`
	PromptRouting PromptRoutingConfig `yaml:"prompt_routing"`
	Server       ServerV2Config       `yaml:"server"`
}

// TrackerV2Config configures the issue/project tracker.
type TrackerV2Config struct {
	Kind                string         `yaml:"kind"`                  // "github" | "linear"
	Owner               string         `yaml:"owner"`                 // GitHub org/user
	ProjectNumber       int            `yaml:"project_number"`        // GitHub Project V2 number
	ProjectScope        string         `yaml:"project_scope"`         // "organization" | "user"
	StatusFieldName     string         `yaml:"status_field_name"`
	ActiveValues        []string       `yaml:"active_values"`
	TerminalValues      []string       `yaml:"terminal_values"`
	BlockedValues       []string       `yaml:"blocked_values"`
	PriorityFieldName   string         `yaml:"priority_field_name"`
	PriorityValueMap    map[string]int `yaml:"priority_value_map"`
	ExecutableItemTypes []string       `yaml:"executable_item_types"`
	RequireIssueBacking bool           `yaml:"require_issue_backing"`
	RepoAllowlist       []string       `yaml:"repo_allowlist"`
	RepoDenylist        []string       `yaml:"repo_denylist"`
	RequiredLabels      []string       `yaml:"required_labels"`
	// Linear-specific
	LinearAPIKey string `yaml:"linear_api_key"`
	LinearTeamID string `yaml:"linear_team_id"`
}

// AuthConfig holds credentials for each service.
type AuthConfig struct {
	GitHub GitHubAuthConfig `yaml:"github"`
	Linear LinearAuthConfig `yaml:"linear"`
}

type GitHubAuthConfig struct {
	Mode           string `yaml:"mode"`            // "pat" | "app" | "auto"
	Token          string `yaml:"token"`           // PAT or $VAR
	APIURL         string `yaml:"api_url"`
	AppID          string `yaml:"app_id"`
	PrivateKey     string `yaml:"private_key"`
	InstallationID string `yaml:"installation_id"`
	WebhookSecret  string `yaml:"webhook_secret"`
	ResolvedMode   string `yaml:"-"` // computed at load time
}

type LinearAuthConfig struct {
	APIKey string `yaml:"api_key"`
}

// AgentV2Config configures agent behavior.
type AgentV2Config struct {
	Kind                  string         `yaml:"kind"`    // "claude_code" | "opencode" | "codex"
	Command               string         `yaml:"command"` // override binary path
	MaxConcurrent         int            `yaml:"max_concurrent"`
	MaxTurns              int            `yaml:"max_turns"`
	StallTimeoutMs        int            `yaml:"stall_timeout_ms"`
	MaxRetryBackoffMs     int            `yaml:"max_retry_backoff_ms"`
	MaxContinuationRetries int           `yaml:"max_continuation_retries"`
	SessionReuse          bool           `yaml:"session_reuse"`
	MaxConcurrentByStatus map[string]int `yaml:"max_concurrent_by_status"`
	MaxConcurrentByRepo   map[string]int `yaml:"max_concurrent_by_repo"`
	Budget                BudgetConfig   `yaml:"budget"`
	Claude                ClaudeV2Config `yaml:"claude"`
	OpenCode              OpenCodeV2Config `yaml:"opencode"`
	Codex                 CodexV2Config  `yaml:"codex"`
}

type BudgetConfig struct {
	MaxCostPerItemUSD float64 `yaml:"max_cost_per_item_usd"`
	MaxCostTotalUSD   float64 `yaml:"max_cost_total_usd"`
	MaxTokensPerItem  int     `yaml:"max_tokens_per_item"`
}

type ClaudeV2Config struct {
	Model             string   `yaml:"model"`
	PermissionProfile string   `yaml:"permission_profile"`
	AllowedTools      []string `yaml:"allowed_tools"`
}

type OpenCodeV2Config struct {
	Model      string `yaml:"model"`
	ConfigFile string `yaml:"config_file"`
}

type CodexV2Config struct {
	ApprovalPolicy string `yaml:"approval_policy"`
}

// GitV2Config configures git behavior.
type GitV2Config struct {
	BranchPrefix string `yaml:"branch_prefix"`
	FetchDepth   int    `yaml:"fetch_depth"`
	UseWorktrees bool   `yaml:"use_worktrees"`
	PushRemote   string `yaml:"push_remote"`
	AuthorName   string `yaml:"author_name"`
	AuthorEmail  string `yaml:"author_email"`
}

// WorkspaceV2Config configures workspace directories.
type WorkspaceV2Config struct {
	Root             string `yaml:"root"`
	RepoCacheDir     string `yaml:"repo_cache_dir"`
	WorktreeDir      string `yaml:"worktree_dir"`
	RemoveOnTerminal bool   `yaml:"remove_on_terminal"`
}

// PollingV2Config configures poll intervals.
type PollingV2Config struct {
	IntervalMs int `yaml:"interval_ms"`
}

// PullRequestV2Config configures PR creation and handoff.
type PullRequestV2Config struct {
	OpenOnSuccess   bool     `yaml:"open_on_success"`
	DraftByDefault  bool     `yaml:"draft_by_default"`
	ReuseExisting   bool     `yaml:"reuse_existing"`
	HandoffStatus   string   `yaml:"handoff_status"`
	CommentOnIssue  bool     `yaml:"comment_on_issue"`
	RequiredChecks  []string `yaml:"required_checks"`
}

// HooksV2Config configures lifecycle hooks.
type HooksV2Config struct {
	AfterCreate  string `yaml:"after_create"`
	BeforeRun    string `yaml:"before_run"`
	AfterRun     string `yaml:"after_run"`
	BeforeRemove string `yaml:"before_remove"`
	TimeoutMs    int    `yaml:"timeout_ms"`
}

// PromptRoutingConfig configures custom field-based prompt routing.
type PromptRoutingConfig struct {
	FieldName string            `yaml:"field_name"` // GitHub Project custom field
	Routes    map[string]string `yaml:"routes"`     // field value -> template filename
	Default   string            `yaml:"default"`    // fallback template
}

// ServerV2Config configures the HTTP API server.
type ServerV2Config struct {
	Port int    `yaml:"port"`
	Host string `yaml:"host"`
}

// LoadSymphonyConfig reads and parses a symphony.yaml file.
func LoadSymphonyConfig(path string) (*SymphonyConfig, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read config: %w", err)
	}
	return ParseSymphonyConfig(data)
}

// ParseSymphonyConfig parses symphony.yaml content.
func ParseSymphonyConfig(data []byte) (*SymphonyConfig, error) {
	cfg := &SymphonyConfig{}
	cfg.applyDefaults()

	if err := yaml.Unmarshal(data, cfg); err != nil {
		return nil, fmt.Errorf("parse config: %w", err)
	}

	cfg.resolveEnvVars()
	cfg.resolvePathExpansion()
	cfg.deriveWorkspaceDefaults()
	cfg.resolveAuthMode()

	return cfg, nil
}

func (c *SymphonyConfig) applyDefaults() {
	c.Tracker = TrackerV2Config{
		ProjectScope:        "organization",
		StatusFieldName:     "Status",
		ActiveValues:        []string{"Todo", "Ready", "In Progress"},
		TerminalValues:      []string{"Done", "Closed", "Cancelled", "Canceled", "Duplicate"},
		PriorityFieldName:   "Priority",
		ExecutableItemTypes: []string{"issue"},
		RequireIssueBacking: true,
	}
	c.Auth = AuthConfig{
		GitHub: GitHubAuthConfig{
			Mode:   "auto",
			APIURL: "https://api.github.com",
		},
	}
	c.Agent = AgentV2Config{
		Kind:                   "claude_code",
		MaxConcurrent:          10,
		MaxTurns:               20,
		StallTimeoutMs:         300000,
		MaxRetryBackoffMs:      300000,
		MaxContinuationRetries: 10,
		SessionReuse:           true,
	}
	c.Git = GitV2Config{
		BranchPrefix: "symphony/",
		UseWorktrees: true,
		PushRemote:   "origin",
		AuthorName:   "Symphony",
		AuthorEmail:  "symphony@noreply.github.com",
	}
	c.Workspace = WorkspaceV2Config{
		RemoveOnTerminal: true,
	}
	c.Polling = PollingV2Config{IntervalMs: 30000}
	c.Hooks = HooksV2Config{TimeoutMs: 60000}
	c.PullRequest = PullRequestV2Config{
		OpenOnSuccess:  true,
		DraftByDefault: true,
		ReuseExisting:  true,
		CommentOnIssue: true,
	}
	c.PromptRouting = PromptRoutingConfig{
		Default: "default.md",
	}
	c.Server = ServerV2Config{
		Port: 9097,
		Host: "0.0.0.0",
	}
}

func (c *SymphonyConfig) resolveEnvVars() {
	c.Auth.GitHub.Token = resolveEnvV2(c.Auth.GitHub.Token)
	c.Auth.GitHub.AppID = resolveEnvV2(c.Auth.GitHub.AppID)
	c.Auth.GitHub.PrivateKey = resolveEnvV2(c.Auth.GitHub.PrivateKey)
	c.Auth.GitHub.WebhookSecret = resolveEnvV2(c.Auth.GitHub.WebhookSecret)
	c.Auth.GitHub.InstallationID = resolveEnvV2(c.Auth.GitHub.InstallationID)
	c.Auth.Linear.APIKey = resolveEnvV2(c.Auth.Linear.APIKey)
	c.Tracker.LinearAPIKey = resolveEnvV2(c.Tracker.LinearAPIKey)
}

func (c *SymphonyConfig) resolvePathExpansion() {
	c.Workspace.Root = expandPathV2(c.Workspace.Root)
	c.Workspace.RepoCacheDir = expandPathV2(c.Workspace.RepoCacheDir)
	c.Workspace.WorktreeDir = expandPathV2(c.Workspace.WorktreeDir)
	c.Agent.OpenCode.ConfigFile = expandPathV2(c.Agent.OpenCode.ConfigFile)
}

func (c *SymphonyConfig) deriveWorkspaceDefaults() {
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

func (c *SymphonyConfig) resolveAuthMode() {
	mode := c.Auth.GitHub.Mode
	if mode == "" {
		mode = "auto"
	}
	switch mode {
	case "pat":
		c.Auth.GitHub.ResolvedMode = "pat"
	case "app":
		c.Auth.GitHub.ResolvedMode = "app"
	case "auto":
		hasApp := c.Auth.GitHub.AppID != "" && c.Auth.GitHub.PrivateKey != ""
		hasPAT := c.Auth.GitHub.Token != ""
		if hasApp {
			c.Auth.GitHub.ResolvedMode = "app"
		} else if hasPAT {
			c.Auth.GitHub.ResolvedMode = "pat"
		}
	}
}

func resolveEnvV2(val string) string {
	if strings.HasPrefix(val, "$") {
		return os.Getenv(val[1:])
	}
	return val
}

func expandPathV2(p string) string {
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
