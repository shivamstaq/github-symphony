package orchestrator

import "strings"

// HandoffInput contains the inputs needed to evaluate whether a handoff condition is met.
type HandoffInput struct {
	HasPR                bool
	CurrentProjectStatus string
	HandoffProjectStatus string // from pull_request.handoff_project_status config
	RequiredChecks       []string
	PassedChecks         []string
}

// HandoffResult is the outcome of handoff evaluation.
type HandoffResult struct {
	IsHandoff     bool
	MissingChecks []string
}

// EvaluateHandoff determines whether a deterministic handoff condition is met.
//
// Rules (from spec Section 7.5):
// - PR alone is NOT sufficient.
// - If handoff_project_status is not configured, handoff never triggers.
// - Default: PR exists AND project status == configured handoff value.
// - Strong: PR + status + all required checks passed.
func EvaluateHandoff(input HandoffInput) HandoffResult {
	// If no handoff status configured, handoff never triggers
	if input.HandoffProjectStatus == "" {
		return HandoffResult{IsHandoff: false}
	}

	// Must have a PR
	if !input.HasPR {
		return HandoffResult{IsHandoff: false}
	}

	// Status must match handoff value (case-insensitive)
	if !strings.EqualFold(input.CurrentProjectStatus, input.HandoffProjectStatus) {
		return HandoffResult{IsHandoff: false}
	}

	// If required checks are configured, all must pass
	if len(input.RequiredChecks) > 0 {
		passedSet := make(map[string]bool, len(input.PassedChecks))
		for _, c := range input.PassedChecks {
			passedSet[strings.ToLower(c)] = true
		}

		var missing []string
		for _, req := range input.RequiredChecks {
			if !passedSet[strings.ToLower(req)] {
				missing = append(missing, req)
			}
		}
		if len(missing) > 0 {
			return HandoffResult{IsHandoff: false, MissingChecks: missing}
		}
	}

	return HandoffResult{IsHandoff: true}
}

// RetryBackoffMs computes the delay for a failure-driven retry.
// Formula: min(10000 * 2^(attempt-1), maxMs)
func RetryBackoffMs(attempt, maxMs int) int {
	delay := 10000
	for i := 1; i < attempt; i++ {
		delay *= 2
		if delay > maxMs {
			return maxMs
		}
	}
	if delay > maxMs {
		return maxMs
	}
	return delay
}
