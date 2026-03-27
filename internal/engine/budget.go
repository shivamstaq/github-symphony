package engine

import (
	"github.com/shivamstaq/github-symphony/internal/domain"
)

// checkBudget evaluates whether a running item has exceeded its cost/token budget.
// Called after each agent update with token information.
func (e *Engine) checkBudget(itemID string) {
	entry, ok := e.state.Running[itemID]
	if !ok {
		return
	}

	budget := e.cfg.Agent.Budget

	// Per-item token limit
	exceeded := budget.MaxTokensPerItem > 0 && entry.TotalTokens >= budget.MaxTokensPerItem
	// Per-item cost limit
	if !exceeded && budget.MaxCostPerItemUSD > 0 && entry.CostUSD >= budget.MaxCostPerItemUSD {
		exceeded = true
	}
	// Global cost limit
	if !exceeded && budget.MaxCostTotalUSD > 0 {
		totalCost := e.state.Totals.CostUSD + entry.CostUSD
		if totalCost >= budget.MaxCostTotalUSD {
			exceeded = true
		}
	}

	if exceeded {
		// Handle synchronously — we're already in the event loop goroutine
		e.handleBudgetExceeded(NewEvent(EvtBudgetExceeded, itemID, nil))
	}
}

// handleBudgetExceeded kills the worker and transitions to needs_human.
func (e *Engine) handleBudgetExceeded(evt EngineEvent) {
	itemID := evt.ItemID
	entry, ok := e.state.Running[itemID]
	if !ok {
		return
	}

	e.logger.Warn("budget exceeded, stopping agent",
		"item", entry.WorkItem.IssueIdentifier,
		"tokens", entry.TotalTokens,
		"cost_usd", entry.CostUSD,
	)

	// Kill the worker
	entry.CancelFunc()
	delete(e.state.Running, itemID)

	// FSM: running -> needs_human
	_, _ = e.transition(itemID, domain.EventBudgetExceeded, nil)
}
