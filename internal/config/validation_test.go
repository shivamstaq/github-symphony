package config_test

import (
	"testing"

	"github.com/shivamstaq/github-symphony/internal/config"
)

func TestValidate_MinimalValid(t *testing.T) {
	t.Setenv("TK", "ghp_test")
	cfg := minimalConfig(t)
	if err := config.ValidateForDispatch(cfg); err != nil {
		t.Fatalf("expected valid config, got: %v", err)
	}
}

func TestValidate_MissingTrackerKind(t *testing.T) {
	t.Setenv("TK", "ghp_test")
	cfg := minimalConfig(t)
	cfg.Tracker.Kind = ""
	err := config.ValidateForDispatch(cfg)
	if err == nil {
		t.Fatal("expected validation error for missing tracker.kind")
	}
}

func TestValidate_MissingOwner(t *testing.T) {
	t.Setenv("TK", "ghp_test")
	cfg := minimalConfig(t)
	cfg.Tracker.Owner = ""
	err := config.ValidateForDispatch(cfg)
	if err == nil {
		t.Fatal("expected validation error for missing tracker.owner")
	}
}

func TestValidate_MissingAuth(t *testing.T) {
	cfg := minimalConfig(t)
	cfg.GitHub.Token = ""
	cfg.GitHub.AppID = ""
	cfg.GitHub.PrivateKey = ""
	cfg.GitHub.ResolvedAuthMode = ""
	err := config.ValidateForDispatch(cfg)
	if err == nil {
		t.Fatal("expected validation error for missing auth credentials")
	}
}

func TestValidate_UnsupportedAgentKind(t *testing.T) {
	t.Setenv("TK", "ghp_test")
	cfg := minimalConfig(t)
	cfg.Agent.Kind = "unsupported_agent"
	err := config.ValidateForDispatch(cfg)
	if err == nil {
		t.Fatal("expected validation error for unsupported agent kind")
	}
}

func minimalConfig(t *testing.T) *config.ServiceConfig {
	t.Helper()
	raw := map[string]any{
		"tracker": map[string]any{
			"kind":           "github",
			"owner":          "org",
			"project_number": 1,
		},
		"github": map[string]any{
			"token": "$TK",
		},
		"agent": map[string]any{
			"kind": "claude_code",
		},
	}
	cfg, err := config.NewServiceConfig(raw)
	if err != nil {
		t.Fatal(err)
	}
	return cfg
}
