package config_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/shivamstaq/github-symphony/internal/config"
)

func TestParseCodex(t *testing.T) {
	raw := map[string]any{
		"tracker": map[string]any{"kind": "github", "owner": "o", "project_number": 1},
		"github":  map[string]any{"token": "t"},
		"agent":   map[string]any{"kind": "codex"},
		"codex": map[string]any{
			"approval_policy":    "auto-edit",
			"thread_sandbox":     "network-only",
			"turn_sandbox_policy": "strict",
			"listen":             "ws://localhost:8080",
			"schema_bundle_dir":  "/tmp/schemas",
		},
	}
	cfg, err := config.NewServiceConfig(raw)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Codex.ApprovalPolicy != "auto-edit" {
		t.Errorf("approval_policy: %q", cfg.Codex.ApprovalPolicy)
	}
	if cfg.Codex.Listen != "ws://localhost:8080" {
		t.Errorf("listen: %q", cfg.Codex.Listen)
	}
}

func TestParseOpenCode(t *testing.T) {
	raw := map[string]any{
		"tracker":  map[string]any{"kind": "github", "owner": "o", "project_number": 1},
		"github":   map[string]any{"token": "t"},
		"agent":    map[string]any{"kind": "opencode"},
		"opencode": map[string]any{
			"model":          "gpt-4",
			"resume_session": false,
			"config_file":    "/etc/opencode.json",
		},
	}
	cfg, err := config.NewServiceConfig(raw)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.OpenCode.Model != "gpt-4" {
		t.Errorf("model: %q", cfg.OpenCode.Model)
	}
	if cfg.OpenCode.ResumeSession != false {
		t.Error("resume_session should be false")
	}
}

func TestParseAgent_ConcurrencyMaps(t *testing.T) {
	raw := map[string]any{
		"tracker": map[string]any{"kind": "github", "owner": "o", "project_number": 1},
		"github":  map[string]any{"token": "t"},
		"agent": map[string]any{
			"kind": "claude_code",
			"max_concurrent_agents_by_project_status": map[string]any{
				"todo":        2,
				"in_progress": 3,
			},
			"max_concurrent_agents_by_repo": map[string]any{
				"org/repo": 1,
			},
		},
	}
	cfg, err := config.NewServiceConfig(raw)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Agent.MaxConcurrentByStatus["todo"] != 2 {
		t.Errorf("per-status todo: %v", cfg.Agent.MaxConcurrentByStatus)
	}
	if cfg.Agent.MaxConcurrentByRepo["org/repo"] != 1 {
		t.Errorf("per-repo: %v", cfg.Agent.MaxConcurrentByRepo)
	}
}

func TestParseClaude_AllFields(t *testing.T) {
	raw := map[string]any{
		"tracker": map[string]any{"kind": "github", "owner": "o", "project_number": 1},
		"github":  map[string]any{"token": "t"},
		"agent":   map[string]any{"kind": "claude_code"},
		"claude": map[string]any{
			"allowed_tools":      []any{"bash", "edit"},
			"mcp_servers":        []any{map[string]any{"name": "test"}},
			"permission_profile": "permissive",
		},
	}
	cfg, err := config.NewServiceConfig(raw)
	if err != nil {
		t.Fatal(err)
	}
	if len(cfg.Claude.AllowedTools) != 2 {
		t.Errorf("allowed_tools: %v", cfg.Claude.AllowedTools)
	}
	if len(cfg.Claude.MCPServers) != 1 {
		t.Errorf("mcp_servers: %v", cfg.Claude.MCPServers)
	}
}

func TestParsePullRequest_RequiredChecks(t *testing.T) {
	raw := map[string]any{
		"tracker": map[string]any{"kind": "github", "owner": "o", "project_number": 1},
		"github":  map[string]any{"token": "t"},
		"agent":   map[string]any{"kind": "claude_code"},
		"pull_request": map[string]any{
			"required_before_handoff_checks": []any{"lint", "test"},
		},
	}
	cfg, err := config.NewServiceConfig(raw)
	if err != nil {
		t.Fatal(err)
	}
	if len(cfg.PullRequest.RequiredBeforeHandoff) != 2 {
		t.Errorf("required checks: %v", cfg.PullRequest.RequiredBeforeHandoff)
	}
}

func TestParseServer_CORSOrigins(t *testing.T) {
	raw := map[string]any{
		"tracker": map[string]any{"kind": "github", "owner": "o", "project_number": 1},
		"github":  map[string]any{"token": "t"},
		"agent":   map[string]any{"kind": "claude_code"},
		"server": map[string]any{
			"port":         9097,
			"cors_origins": []any{"http://localhost:3000"},
		},
	}
	cfg, err := config.NewServiceConfig(raw)
	if err != nil {
		t.Fatal(err)
	}
	if len(cfg.Server.CORSOrigins) != 1 {
		t.Errorf("cors_origins: %v", cfg.Server.CORSOrigins)
	}
}

func TestPathExpansion_Tilde(t *testing.T) {
	home, _ := os.UserHomeDir()
	raw := map[string]any{
		"tracker":   map[string]any{"kind": "github", "owner": "o", "project_number": 1},
		"github":    map[string]any{"token": "t"},
		"agent":     map[string]any{"kind": "claude_code"},
		"workspace": map[string]any{"root": "~/symphony_ws"},
	}
	cfg, err := config.NewServiceConfig(raw)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(cfg.Workspace.Root, home) {
		t.Errorf("expected ~ expanded to home dir, got %q", cfg.Workspace.Root)
	}
}

func TestWorkspaceDefaults_DerivedFromRoot(t *testing.T) {
	raw := map[string]any{
		"tracker": map[string]any{"kind": "github", "owner": "o", "project_number": 1},
		"github":  map[string]any{"token": "t"},
		"agent":   map[string]any{"kind": "claude_code"},
	}
	cfg, err := config.NewServiceConfig(raw)
	if err != nil {
		t.Fatal(err)
	}
	// Root should have a default
	if cfg.Workspace.Root == "" {
		t.Error("workspace.root should default to temp dir")
	}
	// Cache and worktree dirs should derive from root
	if !strings.HasPrefix(cfg.Workspace.RepoCacheDir, cfg.Workspace.Root) {
		t.Errorf("repo_cache_dir should be under root: %q vs %q", cfg.Workspace.RepoCacheDir, cfg.Workspace.Root)
	}
	if !strings.HasPrefix(cfg.Workspace.WorktreeDir, cfg.Workspace.Root) {
		t.Errorf("worktree_dir should be under root: %q vs %q", cfg.Workspace.WorktreeDir, cfg.Workspace.Root)
	}
	if cfg.Workspace.RepoCacheDir != filepath.Join(cfg.Workspace.Root, "repo_cache") {
		t.Errorf("repo_cache_dir: %q", cfg.Workspace.RepoCacheDir)
	}
}

func TestTrackerPriorityValueMap(t *testing.T) {
	raw := map[string]any{
		"tracker": map[string]any{
			"kind":           "github",
			"owner":          "o",
			"project_number": 1,
			"priority_value_map": map[string]any{
				"Critical": 1,
				"High":     2,
				"Medium":   3,
			},
		},
		"github": map[string]any{"token": "t"},
		"agent":  map[string]any{"kind": "claude_code"},
	}
	cfg, err := config.NewServiceConfig(raw)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Tracker.PriorityValueMap["Critical"] != 1 {
		t.Errorf("priority map: %v", cfg.Tracker.PriorityValueMap)
	}
}

func TestTemplateParseValidation_InvalidTemplate(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "WORKFLOW.md")
	content := "---\ntracker:\n  kind: github\n---\n{{.bad template {{syntax}}\n"
	os.WriteFile(path, []byte(content), 0644)

	_, err := config.LoadWorkflow(path)
	if err == nil {
		t.Fatal("expected error for invalid template syntax")
	}
	var wfErr *config.WorkflowError
	if !config.AsWorkflowError(err, &wfErr) {
		t.Fatalf("expected WorkflowError, got %T", err)
	}
	if wfErr.Kind != config.ErrTemplateParseError {
		t.Errorf("expected ErrTemplateParseError, got %v", wfErr.Kind)
	}
}
