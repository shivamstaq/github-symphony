// Package mock provides a configurable mock agent for testing.
package mock

import (
	"context"
	"time"

	"github.com/google/uuid"
	"github.com/shivamstaq/github-symphony/internal/agent"
)

// MockAgent is a configurable mock that simulates agent behavior.
type MockAgent struct {
	// Behavior configuration
	StopReason agent.StopReason
	CostUSD    float64
	NumTurns   int
	HasCommits bool
	Error      error
	Delay      time.Duration // simulate work time per turn
}

// NewSuccessAgent returns a mock that completes successfully with commits.
func NewSuccessAgent() *MockAgent {
	return &MockAgent{
		StopReason: agent.StopCompleted,
		CostUSD:    0.05,
		NumTurns:   3,
		HasCommits: true,
	}
}

// NewFailAgent returns a mock that fails with an error.
func NewFailAgent(err error) *MockAgent {
	return &MockAgent{
		StopReason: agent.StopFailed,
		Error:      err,
	}
}

// NewNoCommitsAgent returns a mock that completes but produces no commits.
func NewNoCommitsAgent() *MockAgent {
	return &MockAgent{
		StopReason: agent.StopCompleted,
		NumTurns:   2,
		HasCommits: false,
	}
}

// MakeUpdate creates an agent.Update for testing.
func MakeUpdate(kind string, totalTokens int) agent.Update {
	return agent.Update{
		Kind: agent.UpdateKind(kind),
		Tokens: agent.TokenUsage{
			Total: totalTokens,
		},
		Timestamp: time.Now(),
	}
}

func (m *MockAgent) Start(ctx context.Context, cfg agent.StartConfig) (*agent.Session, error) {
	sessionID := uuid.New().String()
	updates := make(chan agent.Update, 100)
	done := make(chan agent.Result, 1)

	go func() {
		defer close(updates)
		defer close(done)

		numTurns := m.NumTurns
		if numTurns == 0 {
			numTurns = 1
		}

		for i := 1; i <= numTurns; i++ {
			select {
			case <-ctx.Done():
				done <- agent.Result{
					StopReason: agent.StopCancelled,
					SessionID:  sessionID,
				}
				return
			default:
			}

			updates <- agent.Update{
				Kind:      agent.UpdateTurnStarted,
				Message:   "turn started",
				Timestamp: time.Now(),
			}

			if m.Delay > 0 {
				select {
				case <-time.After(m.Delay):
				case <-ctx.Done():
					done <- agent.Result{
						StopReason: agent.StopCancelled,
						SessionID:  sessionID,
					}
					return
				}
			}

			updates <- agent.Update{
				Kind:    agent.UpdateTurnDone,
				Message: "turn completed",
				Tokens: agent.TokenUsage{
					Input:  400,
					Output: 200,
					Total:  600,
				},
				Timestamp: time.Now(),
			}
		}

		done <- agent.Result{
			StopReason: m.StopReason,
			SessionID:  sessionID,
			CostUSD:    m.CostUSD,
			NumTurns:   numTurns,
			HasCommits: m.HasCommits,
			Error:      m.Error,
		}
	}()

	return &agent.Session{
		ID:      sessionID,
		Updates: updates,
		Done:    done,
	}, nil
}
