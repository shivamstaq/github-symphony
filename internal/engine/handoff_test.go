package engine

import "testing"

func TestEvaluateHandoff_AllConditionsMet(t *testing.T) {
	result := EvaluateHandoff(HandoffInput{
		HasPR:                true,
		CurrentProjectStatus: "Human Review",
		HandoffProjectStatus: "Human Review",
	})
	if !result.IsHandoff {
		t.Errorf("expected handoff, got: %s", result.Reason)
	}
}

func TestEvaluateHandoff_NoPR(t *testing.T) {
	result := EvaluateHandoff(HandoffInput{
		HasPR:                false,
		CurrentProjectStatus: "Human Review",
		HandoffProjectStatus: "Human Review",
	})
	if result.IsHandoff {
		t.Error("should not handoff without PR")
	}
	if result.Reason != "no PR linked" {
		t.Errorf("unexpected reason: %s", result.Reason)
	}
}

func TestEvaluateHandoff_WrongStatus(t *testing.T) {
	result := EvaluateHandoff(HandoffInput{
		HasPR:                true,
		CurrentProjectStatus: "In Progress",
		HandoffProjectStatus: "Human Review",
	})
	if result.IsHandoff {
		t.Error("should not handoff with wrong status")
	}
}

func TestEvaluateHandoff_NotConfigured(t *testing.T) {
	result := EvaluateHandoff(HandoffInput{
		HasPR:                true,
		HandoffProjectStatus: "", // not configured
	})
	if result.IsHandoff {
		t.Error("should not handoff when not configured")
	}
}

func TestEvaluateHandoff_CaseInsensitive(t *testing.T) {
	result := EvaluateHandoff(HandoffInput{
		HasPR:                true,
		CurrentProjectStatus: "human review",
		HandoffProjectStatus: "Human Review",
	})
	if !result.IsHandoff {
		t.Errorf("case-insensitive match should trigger handoff: %s", result.Reason)
	}
}

func TestEvaluateHandoff_RequiredChecksMet(t *testing.T) {
	result := EvaluateHandoff(HandoffInput{
		HasPR:                true,
		CurrentProjectStatus: "Human Review",
		HandoffProjectStatus: "Human Review",
		RequiredChecks:       []string{"ci/build", "ci/test"},
		PassedChecks:         []string{"ci/build", "ci/test", "ci/lint"},
	})
	if !result.IsHandoff {
		t.Errorf("all checks passed, expected handoff: %s", result.Reason)
	}
}

func TestEvaluateHandoff_RequiredChecksMissing(t *testing.T) {
	result := EvaluateHandoff(HandoffInput{
		HasPR:                true,
		CurrentProjectStatus: "Human Review",
		HandoffProjectStatus: "Human Review",
		RequiredChecks:       []string{"ci/build", "ci/test"},
		PassedChecks:         []string{"ci/build"}, // missing ci/test
	})
	if result.IsHandoff {
		t.Error("should not handoff with missing check")
	}
	if result.Reason != "required check not passed: ci/test" {
		t.Errorf("unexpected reason: %s", result.Reason)
	}
}
