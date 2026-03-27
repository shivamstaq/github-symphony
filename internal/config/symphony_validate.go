package config

import (
	"fmt"
	"strings"
)

// ConfigErrors collects multiple config validation failures.
type ConfigErrors struct {
	Errors []string
}

func (e *ConfigErrors) Error() string {
	return fmt.Sprintf("config validation failed:\n  - %s", strings.Join(e.Errors, "\n  - "))
}

func (e *ConfigErrors) add(msg string) {
	e.Errors = append(e.Errors, msg)
}

func (e *ConfigErrors) hasErrors() bool {
	return len(e.Errors) > 0
}

// ValidateSymphonyConfig checks that all required fields are present and values are sane.
func ValidateSymphonyConfig(cfg *SymphonyConfig) error {
	ve := &ConfigErrors{}

	// Tracker
	if cfg.Tracker.Kind == "" {
		ve.add("tracker.kind is required")
	} else {
		switch cfg.Tracker.Kind {
		case "github":
			if cfg.Tracker.Owner == "" {
				ve.add("tracker.owner is required for GitHub tracker")
			}
			if cfg.Tracker.ProjectNumber <= 0 {
				ve.add("tracker.project_number must be > 0 for GitHub tracker")
			}
		case "linear":
			// Linear validation will be added in Slice 4
		default:
			ve.add(fmt.Sprintf("tracker.kind must be 'github' or 'linear', got %q", cfg.Tracker.Kind))
		}
	}

	if len(cfg.Tracker.ActiveValues) == 0 {
		ve.add("tracker.active_values must have at least one value")
	}
	if len(cfg.Tracker.TerminalValues) == 0 {
		ve.add("tracker.terminal_values must have at least one value")
	}

	// Auth
	if cfg.Tracker.Kind == "github" {
		if cfg.Auth.GitHub.ResolvedMode == "" {
			ve.add("auth.github: no credentials found — set token ($GITHUB_TOKEN) or app_id + private_key")
		}
	}

	// Agent
	validAgentKinds := map[string]bool{"claude_code": true, "opencode": true, "codex": true}
	if cfg.Agent.Kind == "" {
		ve.add("agent.kind is required")
	} else if !validAgentKinds[cfg.Agent.Kind] {
		ve.add(fmt.Sprintf("agent.kind must be one of claude_code, opencode, codex — got %q", cfg.Agent.Kind))
	}

	if cfg.Agent.MaxConcurrent <= 0 {
		ve.add("agent.max_concurrent must be > 0")
	}
	if cfg.Agent.MaxTurns <= 0 {
		ve.add("agent.max_turns must be > 0")
	}

	// Polling
	if cfg.Polling.IntervalMs <= 0 {
		ve.add("polling.interval_ms must be > 0")
	}

	// Prompt routing
	if cfg.PromptRouting.Default == "" {
		ve.add("prompt_routing.default is required")
	}

	if ve.hasErrors() {
		return ve
	}
	return nil
}
