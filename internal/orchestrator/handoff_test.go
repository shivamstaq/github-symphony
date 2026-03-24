package orchestrator_test

import (
	"testing"

	"github.com/shivamstaq/github-symphony/internal/orchestrator"
)

func TestIsHandoff_PRAndStatusTransition(t *testing.T) {
	result := orchestrator.EvaluateHandoff(orchestrator.HandoffInput{
		HasPR:                true,
		CurrentProjectStatus: "Human Review",
		HandoffProjectStatus: "Human Review",
	})
	if !result.IsHandoff {
		t.Error("expected handoff when PR exists and status matches handoff value")
	}
}

func TestIsHandoff_PROnly_NotSufficient(t *testing.T) {
	result := orchestrator.EvaluateHandoff(orchestrator.HandoffInput{
		HasPR:                true,
		CurrentProjectStatus: "In Progress",
		HandoffProjectStatus: "Human Review",
	})
	if result.IsHandoff {
		t.Error("PR alone should not be sufficient for handoff")
	}
}

func TestIsHandoff_StatusOnly_NotSufficient(t *testing.T) {
	result := orchestrator.EvaluateHandoff(orchestrator.HandoffInput{
		HasPR:                false,
		CurrentProjectStatus: "Human Review",
		HandoffProjectStatus: "Human Review",
	})
	if result.IsHandoff {
		t.Error("status alone without PR should not be sufficient for handoff")
	}
}

func TestIsHandoff_NoHandoffConfigured(t *testing.T) {
	result := orchestrator.EvaluateHandoff(orchestrator.HandoffInput{
		HasPR:                true,
		CurrentProjectStatus: "In Progress",
		HandoffProjectStatus: "", // not configured
	})
	if result.IsHandoff {
		t.Error("handoff should never trigger when handoff_project_status is not configured")
	}
}

func TestIsHandoff_WithRequiredChecks_AllPass(t *testing.T) {
	result := orchestrator.EvaluateHandoff(orchestrator.HandoffInput{
		HasPR:                  true,
		CurrentProjectStatus:   "Human Review",
		HandoffProjectStatus:   "Human Review",
		RequiredChecks:         []string{"lint", "test"},
		PassedChecks:           []string{"lint", "test"},
	})
	if !result.IsHandoff {
		t.Error("expected handoff when PR + status + all required checks pass")
	}
}

func TestIsHandoff_WithRequiredChecks_Missing(t *testing.T) {
	result := orchestrator.EvaluateHandoff(orchestrator.HandoffInput{
		HasPR:                  true,
		CurrentProjectStatus:   "Human Review",
		HandoffProjectStatus:   "Human Review",
		RequiredChecks:         []string{"lint", "test"},
		PassedChecks:           []string{"lint"}, // missing "test"
	})
	if result.IsHandoff {
		t.Error("handoff should not trigger when required checks are missing")
	}
}

func TestRetryBackoff(t *testing.T) {
	tests := []struct {
		attempt int
		maxMs   int
		want    int
	}{
		{1, 300000, 10000},
		{2, 300000, 20000},
		{3, 300000, 40000},
		{4, 300000, 80000},
		{5, 300000, 160000},
		{6, 300000, 300000}, // capped
		{10, 300000, 300000},
	}

	for _, tt := range tests {
		got := orchestrator.RetryBackoffMs(tt.attempt, tt.maxMs)
		if got != tt.want {
			t.Errorf("RetryBackoffMs(%d, %d) = %d, want %d", tt.attempt, tt.maxMs, got, tt.want)
		}
	}
}
