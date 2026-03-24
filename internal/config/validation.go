package config

import "fmt"

// ValidationError represents a config validation failure.
type ValidationError struct {
	Field   string
	Message string
}

func (e *ValidationError) Error() string {
	return fmt.Sprintf("config validation: %s: %s", e.Field, e.Message)
}

// ValidateForDispatch checks that the config is sufficient to start dispatching work.
func ValidateForDispatch(cfg *ServiceConfig) error {
	if cfg.Tracker.Kind == "" {
		return &ValidationError{Field: "tracker.kind", Message: "required"}
	}
	if cfg.Tracker.Kind != "github" {
		return &ValidationError{Field: "tracker.kind", Message: fmt.Sprintf("unsupported value %q, must be \"github\"", cfg.Tracker.Kind)}
	}
	if cfg.Tracker.Owner == "" {
		return &ValidationError{Field: "tracker.owner", Message: "required"}
	}
	if cfg.Tracker.ProjectNumber == 0 {
		return &ValidationError{Field: "tracker.project_number", Message: "required"}
	}

	// Auth validation
	if cfg.GitHub.ResolvedAuthMode == "" {
		hasPAT := cfg.GitHub.Token != ""
		hasApp := cfg.GitHub.AppID != "" && cfg.GitHub.PrivateKey != ""
		if !hasPAT && !hasApp {
			return &ValidationError{Field: "github", Message: "no auth credentials: set github.token ($GITHUB_TOKEN) or github.app_id + github.private_key"}
		}
	}
	if cfg.GitHub.ResolvedAuthMode == "app" {
		if cfg.GitHub.AppID == "" {
			return &ValidationError{Field: "github.app_id", Message: "required for app auth mode"}
		}
		if cfg.GitHub.PrivateKey == "" {
			return &ValidationError{Field: "github.private_key", Message: "required for app auth mode"}
		}
	}

	// Agent validation
	if cfg.Agent.Kind == "" {
		return &ValidationError{Field: "agent.kind", Message: "required"}
	}
	switch cfg.Agent.Kind {
	case "codex", "claude_code", "opencode":
		// valid
	default:
		return &ValidationError{Field: "agent.kind", Message: fmt.Sprintf("unsupported value %q", cfg.Agent.Kind)}
	}

	return nil
}
