package config_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/shivamstaq/github-symphony/internal/config"
)

func TestNewServiceConfig_Defaults(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "WORKFLOW.md")
	content := `---
tracker:
  kind: github
  owner: myorg
  project_number: 1
agent:
  kind: claude_code
---
prompt
`
	os.WriteFile(path, []byte(content), 0644)

	wf, err := config.LoadWorkflow(path)
	if err != nil {
		t.Fatal(err)
	}

	cfg, err := config.NewServiceConfig(wf.Config)
	if err != nil {
		t.Fatal(err)
	}

	// Tracker defaults
	if cfg.Tracker.StatusFieldName != "Status" {
		t.Errorf("expected default StatusFieldName=Status, got %q", cfg.Tracker.StatusFieldName)
	}
	if len(cfg.Tracker.ActiveValues) != 3 {
		t.Errorf("expected 3 default active values, got %d", len(cfg.Tracker.ActiveValues))
	}
	if cfg.Tracker.ProjectScope != "organization" {
		t.Errorf("expected default ProjectScope=organization, got %q", cfg.Tracker.ProjectScope)
	}

	// Agent defaults
	if cfg.Agent.MaxConcurrentAgents != 10 {
		t.Errorf("expected default MaxConcurrentAgents=10, got %d", cfg.Agent.MaxConcurrentAgents)
	}
	if cfg.Agent.MaxTurns != 20 {
		t.Errorf("expected default MaxTurns=20, got %d", cfg.Agent.MaxTurns)
	}
	if cfg.Agent.StallTimeoutMs != 300000 {
		t.Errorf("expected default StallTimeoutMs=300000, got %d", cfg.Agent.StallTimeoutMs)
	}

	// Git defaults
	if cfg.Git.BranchPrefix != "symphony/" {
		t.Errorf("expected default BranchPrefix=symphony/, got %q", cfg.Git.BranchPrefix)
	}
	if cfg.Git.UseWorktrees != true {
		t.Error("expected default UseWorktrees=true")
	}

	// Polling defaults
	if cfg.Polling.IntervalMs != 30000 {
		t.Errorf("expected default IntervalMs=30000, got %d", cfg.Polling.IntervalMs)
	}

	// PR defaults
	if cfg.PullRequest.OpenPROnSuccess != true {
		t.Error("expected default OpenPROnSuccess=true")
	}
	if cfg.PullRequest.DraftByDefault != true {
		t.Error("expected default DraftByDefault=true")
	}
}

func TestNewServiceConfig_EnvVarResolution(t *testing.T) {
	t.Setenv("TEST_GITHUB_TOKEN", "ghp_test123")

	dir := t.TempDir()
	path := filepath.Join(dir, "WORKFLOW.md")
	content := `---
tracker:
  kind: github
  owner: myorg
  project_number: 1
github:
  token: $TEST_GITHUB_TOKEN
agent:
  kind: claude_code
---
prompt
`
	os.WriteFile(path, []byte(content), 0644)

	wf, err := config.LoadWorkflow(path)
	if err != nil {
		t.Fatal(err)
	}

	cfg, err := config.NewServiceConfig(wf.Config)
	if err != nil {
		t.Fatal(err)
	}

	if cfg.GitHub.Token != "ghp_test123" {
		t.Errorf("expected resolved token=ghp_test123, got %q", cfg.GitHub.Token)
	}
}

func TestNewServiceConfig_AuthModeAuto_PAT(t *testing.T) {
	t.Setenv("MY_TOKEN", "ghp_abc")

	raw := map[string]any{
		"tracker": map[string]any{
			"kind":           "github",
			"owner":          "org",
			"project_number": 1,
		},
		"github": map[string]any{
			"token": "$MY_TOKEN",
		},
		"agent": map[string]any{
			"kind": "claude_code",
		},
	}

	cfg, err := config.NewServiceConfig(raw)
	if err != nil {
		t.Fatal(err)
	}

	if cfg.GitHub.ResolvedAuthMode != "pat" {
		t.Errorf("expected resolved auth mode=pat, got %q", cfg.GitHub.ResolvedAuthMode)
	}
}
