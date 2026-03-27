package config

import (
	"os"
	"strings"
	"testing"
)

func TestParseSymphonyConfig_Minimal(t *testing.T) {
	yaml := `
tracker:
  kind: github
  owner: myorg
  project_number: 42
auth:
  github:
    token: test-token-123
agent:
  kind: claude_code
`
	cfg, err := ParseSymphonyConfig([]byte(yaml))
	if err != nil {
		t.Fatalf("parse error: %v", err)
	}

	if cfg.Tracker.Kind != "github" {
		t.Errorf("tracker.kind = %q, want 'github'", cfg.Tracker.Kind)
	}
	if cfg.Tracker.Owner != "myorg" {
		t.Errorf("tracker.owner = %q, want 'myorg'", cfg.Tracker.Owner)
	}
	if cfg.Tracker.ProjectNumber != 42 {
		t.Errorf("tracker.project_number = %d, want 42", cfg.Tracker.ProjectNumber)
	}
	if cfg.Auth.GitHub.Token != "test-token-123" {
		t.Errorf("auth.github.token = %q, want 'test-token-123'", cfg.Auth.GitHub.Token)
	}
	if cfg.Auth.GitHub.ResolvedMode != "pat" {
		t.Errorf("auth.github.resolved_mode = %q, want 'pat'", cfg.Auth.GitHub.ResolvedMode)
	}
	if cfg.Agent.Kind != "claude_code" {
		t.Errorf("agent.kind = %q, want 'claude_code'", cfg.Agent.Kind)
	}
}

func TestParseSymphonyConfig_Defaults(t *testing.T) {
	yaml := `
tracker:
  kind: github
  owner: myorg
  project_number: 1
auth:
  github:
    token: tok
agent:
  kind: claude_code
`
	cfg, err := ParseSymphonyConfig([]byte(yaml))
	if err != nil {
		t.Fatalf("parse error: %v", err)
	}

	// Check defaults applied
	if cfg.Tracker.ProjectScope != "organization" {
		t.Errorf("default project_scope = %q, want 'organization'", cfg.Tracker.ProjectScope)
	}
	if cfg.Tracker.StatusFieldName != "Status" {
		t.Errorf("default status_field_name = %q, want 'Status'", cfg.Tracker.StatusFieldName)
	}
	if cfg.Agent.MaxConcurrent != 10 {
		t.Errorf("default max_concurrent = %d, want 10", cfg.Agent.MaxConcurrent)
	}
	if cfg.Agent.MaxTurns != 20 {
		t.Errorf("default max_turns = %d, want 20", cfg.Agent.MaxTurns)
	}
	if cfg.Git.BranchPrefix != "symphony/" {
		t.Errorf("default branch_prefix = %q, want 'symphony/'", cfg.Git.BranchPrefix)
	}
	if !cfg.Git.UseWorktrees {
		t.Error("default use_worktrees should be true")
	}
	if cfg.Polling.IntervalMs != 30000 {
		t.Errorf("default interval_ms = %d, want 30000", cfg.Polling.IntervalMs)
	}
	if !cfg.PullRequest.OpenOnSuccess {
		t.Error("default open_on_success should be true")
	}
	if !cfg.PullRequest.DraftByDefault {
		t.Error("default draft_by_default should be true")
	}
	if cfg.PromptRouting.Default != "default.md" {
		t.Errorf("default prompt_routing.default = %q, want 'default.md'", cfg.PromptRouting.Default)
	}
	if cfg.Server.Port != 9097 {
		t.Errorf("default server.port = %d, want 9097", cfg.Server.Port)
	}
}

func TestParseSymphonyConfig_FullSchema(t *testing.T) {
	yaml := `
tracker:
  kind: github
  owner: myorg
  project_number: 42
  project_scope: user
  status_field_name: MyStatus
  active_values: [Ready, Working]
  terminal_values: [Done]
  blocked_values: [Blocked]
  priority_field_name: Urgency
  priority_value_map:
    P0: 0
    P1: 1
  executable_item_types: [issue, draft_issue]
  require_issue_backing: false
  repo_allowlist: [myorg/repo1]
  repo_denylist: [myorg/repo2]
  required_labels: [agent-ready]
auth:
  github:
    mode: pat
    token: ghp_test123
    api_url: https://api.github.example.com
    webhook_secret: whsec_test
agent:
  kind: claude_code
  command: /usr/local/bin/claude
  max_concurrent: 5
  max_turns: 10
  stall_timeout_ms: 600000
  max_retry_backoff_ms: 120000
  max_continuation_retries: 3
  session_reuse: false
  budget:
    max_cost_per_item_usd: 10.0
    max_cost_total_usd: 100.0
    max_tokens_per_item: 500000
  claude:
    model: opus
    permission_profile: bypassPermissions
    allowed_tools: [Read, Edit, Write]
git:
  branch_prefix: agent/
  fetch_depth: 1
  use_worktrees: false
  push_remote: upstream
  author_name: Bot
  author_email: bot@example.com
polling:
  interval_ms: 60000
pull_request:
  open_on_success: true
  draft_by_default: false
  reuse_existing: false
  handoff_status: Human Review
  comment_on_issue: false
  required_checks: [ci/build, ci/test]
hooks:
  before_run: "echo before"
  after_run: "echo after"
  timeout_ms: 30000
prompt_routing:
  field_name: Type
  routes:
    bug: bug_fix.md
    feature: feature.md
  default: default.md
server:
  port: 8080
  host: 127.0.0.1
`
	cfg, err := ParseSymphonyConfig([]byte(yaml))
	if err != nil {
		t.Fatalf("parse error: %v", err)
	}

	// Spot-check overrides
	if cfg.Tracker.ProjectScope != "user" {
		t.Errorf("project_scope = %q, want 'user'", cfg.Tracker.ProjectScope)
	}
	if cfg.Agent.MaxConcurrent != 5 {
		t.Errorf("max_concurrent = %d, want 5", cfg.Agent.MaxConcurrent)
	}
	if cfg.Agent.Budget.MaxCostPerItemUSD != 10.0 {
		t.Errorf("budget.max_cost_per_item_usd = %f, want 10.0", cfg.Agent.Budget.MaxCostPerItemUSD)
	}
	if cfg.Agent.Claude.Model != "opus" {
		t.Errorf("claude.model = %q, want 'opus'", cfg.Agent.Claude.Model)
	}
	if len(cfg.Agent.Claude.AllowedTools) != 3 {
		t.Errorf("claude.allowed_tools len = %d, want 3", len(cfg.Agent.Claude.AllowedTools))
	}
	if cfg.Git.BranchPrefix != "agent/" {
		t.Errorf("branch_prefix = %q, want 'agent/'", cfg.Git.BranchPrefix)
	}
	if cfg.PullRequest.HandoffStatus != "Human Review" {
		t.Errorf("handoff_status = %q, want 'Human Review'", cfg.PullRequest.HandoffStatus)
	}
	if len(cfg.PullRequest.RequiredChecks) != 2 {
		t.Errorf("required_checks len = %d, want 2", len(cfg.PullRequest.RequiredChecks))
	}
	if cfg.PromptRouting.FieldName != "Type" {
		t.Errorf("field_name = %q, want 'Type'", cfg.PromptRouting.FieldName)
	}
	if cfg.PromptRouting.Routes["bug"] != "bug_fix.md" {
		t.Errorf("routes[bug] = %q, want 'bug_fix.md'", cfg.PromptRouting.Routes["bug"])
	}
	if cfg.Hooks.BeforeRun != "echo before" {
		t.Errorf("hooks.before_run = %q, want 'echo before'", cfg.Hooks.BeforeRun)
	}
	if cfg.Server.Port != 8080 {
		t.Errorf("server.port = %d, want 8080", cfg.Server.Port)
	}
}

func TestParseSymphonyConfig_EnvVarResolution(t *testing.T) {
	os.Setenv("SYMPHONY_TEST_TOKEN", "resolved-token-xyz")
	defer os.Unsetenv("SYMPHONY_TEST_TOKEN")

	yaml := `
tracker:
  kind: github
  owner: org
  project_number: 1
auth:
  github:
    token: $SYMPHONY_TEST_TOKEN
agent:
  kind: claude_code
`
	cfg, err := ParseSymphonyConfig([]byte(yaml))
	if err != nil {
		t.Fatalf("parse error: %v", err)
	}
	if cfg.Auth.GitHub.Token != "resolved-token-xyz" {
		t.Errorf("token = %q, want 'resolved-token-xyz'", cfg.Auth.GitHub.Token)
	}
	if cfg.Auth.GitHub.ResolvedMode != "pat" {
		t.Errorf("resolved auth mode = %q, want 'pat'", cfg.Auth.GitHub.ResolvedMode)
	}
}

func TestValidateSymphonyConfig_Valid(t *testing.T) {
	yaml := `
tracker:
  kind: github
  owner: org
  project_number: 1
auth:
  github:
    token: test-token
agent:
  kind: claude_code
`
	cfg, err := ParseSymphonyConfig([]byte(yaml))
	if err != nil {
		t.Fatalf("parse error: %v", err)
	}
	if err := ValidateSymphonyConfig(cfg); err != nil {
		t.Errorf("expected valid, got: %v", err)
	}
}

func TestValidateSymphonyConfig_MissingRequired(t *testing.T) {
	tests := []struct {
		name    string
		yaml    string
		wantErr string
	}{
		{
			"missing tracker.kind",
			`tracker: {owner: org, project_number: 1}
auth: {github: {token: t}}
agent: {kind: claude_code}`,
			"tracker.kind is required",
		},
		{
			"missing tracker.owner",
			`tracker: {kind: github, project_number: 1}
auth: {github: {token: t}}
agent: {kind: claude_code}`,
			"tracker.owner is required",
		},
		{
			"missing project_number",
			`tracker: {kind: github, owner: org}
auth: {github: {token: t}}
agent: {kind: claude_code}`,
			"tracker.project_number must be > 0",
		},
		{
			"missing auth",
			`tracker: {kind: github, owner: org, project_number: 1}
agent: {kind: claude_code}`,
			"no credentials found",
		},
		{
			"invalid agent kind",
			`tracker: {kind: github, owner: org, project_number: 1}
auth: {github: {token: t}}
agent: {kind: gpt4}`,
			"must be one of claude_code",
		},
		{
			"invalid tracker kind",
			`tracker: {kind: jira, owner: org, project_number: 1}
auth: {github: {token: t}}
agent: {kind: claude_code}`,
			"must be 'github' or 'linear'",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg, err := ParseSymphonyConfig([]byte(tt.yaml))
			if err != nil {
				t.Fatalf("parse error: %v", err)
			}
			err = ValidateSymphonyConfig(cfg)
			if err == nil {
				t.Fatal("expected validation error, got nil")
			}
			if !strings.Contains(err.Error(), tt.wantErr) {
				t.Errorf("error %q does not contain %q", err.Error(), tt.wantErr)
			}
		})
	}
}
