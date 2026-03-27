package engine

import (
	"context"
	"log/slog"
	"time"

	"github.com/shivamstaq/github-symphony/internal/agent"
	"github.com/shivamstaq/github-symphony/internal/codehost"
	"github.com/shivamstaq/github-symphony/internal/config"
	"github.com/shivamstaq/github-symphony/internal/domain"
	"github.com/shivamstaq/github-symphony/internal/prompt"
	"github.com/shivamstaq/github-symphony/internal/state"
	"github.com/shivamstaq/github-symphony/internal/tracker"
	"github.com/shivamstaq/github-symphony/internal/workspace"
)

// Engine is the central orchestration loop.
// A single goroutine processes events sequentially — no mutexes on State.
type Engine struct {
	cfg      *config.SymphonyConfig
	state    *State
	eventLog *EventLog
	tracker  tracker.Tracker
	agent    agent.Agent
	codeHost codehost.CodeHost
	store    *state.Store
	wsMgr    *workspace.Manager
	router   *prompt.Router
	eventCh  chan EngineEvent
	logger   *slog.Logger

	eligCfg EligibilityConfig
}

// Deps contains the dependencies injected into the engine.
type Deps struct {
	Config       *config.SymphonyConfig
	Tracker      tracker.Tracker
	Agent        agent.Agent
	CodeHost     codehost.CodeHost
	Store        *state.Store
	Workspace    *workspace.Manager
	PromptRouter *prompt.Router
	EventLog     *EventLog
	Logger       *slog.Logger
}

// New creates a new Engine with the given dependencies.
func New(deps Deps) *Engine {
	cfg := deps.Config
	e := &Engine{
		cfg:      cfg,
		state:    NewState(),
		eventLog: deps.EventLog,
		tracker:  deps.Tracker,
		agent:    deps.Agent,
		codeHost: deps.CodeHost,
		store:    deps.Store,
		wsMgr:    deps.Workspace,
		router:   deps.PromptRouter,
		eventCh:  make(chan EngineEvent, 256),
		logger:   deps.Logger,
		eligCfg: EligibilityConfig{
			ActiveValues:        cfg.Tracker.ActiveValues,
			TerminalValues:      cfg.Tracker.TerminalValues,
			ExecutableItemTypes: cfg.Tracker.ExecutableItemTypes,
			RequireIssueBacking: cfg.Tracker.RequireIssueBacking,
			RepoAllowlist:       cfg.Tracker.RepoAllowlist,
			RepoDenylist:        cfg.Tracker.RepoDenylist,
			RequiredLabels:      cfg.Tracker.RequiredLabels,
			BlockedStatusValues: cfg.Tracker.BlockedValues,
			MaxPerStatus:        cfg.Agent.MaxConcurrentByStatus,
			MaxPerRepo:          cfg.Agent.MaxConcurrentByRepo,
		},
	}
	return e
}

// Run starts the engine's main event loop. Blocks until ctx is cancelled.
func (e *Engine) Run(ctx context.Context) error {
	// Restore persisted state
	e.restoreState()

	pollInterval := time.Duration(e.cfg.Polling.IntervalMs) * time.Millisecond
	ticker := time.NewTicker(pollInterval)
	defer ticker.Stop()

	// Immediate first poll
	e.eventCh <- NewEvent(EvtPollTick, "", PollTickPayload{})

	for {
		select {
		case <-ctx.Done():
			e.handleShutdown()
			return ctx.Err()

		case <-ticker.C:
			e.eventCh <- NewEvent(EvtPollTick, "", PollTickPayload{})

		case evt := <-e.eventCh:
			e.handleEvent(ctx, evt)
		}
	}
}

// Emit sends an event to the engine's event channel (thread-safe).
func (e *Engine) Emit(evt EngineEvent) {
	select {
	case e.eventCh <- evt:
	default:
		e.logger.Warn("event channel full, dropping event", "type", evt.Type)
	}
}

// State returns a snapshot of the current state (for TUI/API).
func (e *Engine) GetState() *State {
	return e.state
}

// HandlePollTick is the exported version of handlePollTick for testing.
func (e *Engine) HandlePollTick(ctx context.Context) {
	e.handlePollTick(ctx)
}

// ProcessOneEvent attempts to process one event from the channel.
// Returns true if an event was processed, false if the channel was empty.
func (e *Engine) ProcessOneEvent(ctx context.Context) bool {
	select {
	case evt := <-e.eventCh:
		e.handleEvent(ctx, evt)
		return true
	default:
		return false
	}
}

// handleEvent dispatches a single event to its typed handler.
func (e *Engine) handleEvent(ctx context.Context, evt EngineEvent) {
	switch evt.Type {
	case EvtPollTick:
		e.handlePollTick(ctx)
	case EvtWorkspaceReady:
		e.handleWorkspaceReady(evt)
	case EvtAgentExited:
		e.handleAgentExited(evt)
	case EvtAgentUpdate:
		e.handleAgentUpdate(evt)
	case EvtPauseRequested:
		e.handlePause(evt)
	case EvtResumeRequested:
		e.handleResume(evt)
	case EvtCancelRequested:
		e.handleCancel(evt)
	case EvtStallDetected:
		e.handleStallDetected(evt)
	case EvtBudgetExceeded:
		e.handleBudgetExceeded(evt)
	case EvtRetryDue:
		e.handleRetryDue(ctx, evt)
	default:
		e.logger.Debug("unhandled event type", "type", evt.Type)
	}
}

// handleWorkspaceReady transitions an item from preparing to running.
func (e *Engine) handleWorkspaceReady(evt EngineEvent) {
	_, _ = e.transition(evt.ItemID, domain.EventWorkspaceReady, nil)
}

// handlePollTick fetches candidates and dispatches eligible items.
func (e *Engine) handlePollTick(ctx context.Context) {
	now := time.Now()
	e.state.LastPollAt = &now

	items, err := e.tracker.FetchCandidates(ctx)
	if err != nil {
		e.logger.Error("poll failed", "error", err)
		return
	}

	SortForDispatch(items)

	dispatched := 0
	for _, item := range items {
		eligible, reason := IsEligible(item, e.eligCfg, e.state, e.cfg.Agent.MaxConcurrent)
		if !eligible {
			e.logger.Debug("ineligible", "item", item.IssueIdentifier, "reason", reason)
			continue
		}

		if err := e.dispatchItem(ctx, item); err != nil {
			e.logger.Error("dispatch failed", "item", item.IssueIdentifier, "error", err)
			continue
		}
		dispatched++
	}

	if dispatched > 0 {
		e.logger.Info("poll complete", "candidates", len(items), "dispatched", dispatched)
	}

	// Fire due retries
	e.fireDueRetries()

	// Detect stalled workers
	e.detectStalls()

	// Reconcile running items with tracker state
	e.reconcileRunningItems(ctx)
}

// dispatchItem claims an item and launches an agent worker.
func (e *Engine) dispatchItem(ctx context.Context, item domain.WorkItem) error {
	// FSM: open -> queued
	_, err := e.transition(item.WorkItemID, domain.EventClaim, func(g domain.TransitionGuard) bool {
		return g == domain.GuardSlotAvailable || g == domain.GuardNone
	})
	if err != nil {
		return err
	}

	// FSM: queued -> preparing
	_, err = e.transition(item.WorkItemID, domain.EventDispatch, func(g domain.TransitionGuard) bool {
		return g == domain.GuardConcurrencyOK || g == domain.GuardNone
	})
	if err != nil {
		return err
	}

	e.state.DispatchTotal++

	// Launch worker goroutine
	workerCtx, cancel := context.WithCancel(ctx)
	entry := &RunningEntry{
		WorkItem:       item,
		CancelFunc:     cancel,
		Phase:          domain.StatePreparing,
		StartedAt:      time.Now(),
		LastActivityAt: time.Now(),
	}
	e.state.Running[item.WorkItemID] = entry

	// Create workspace if manager is available
	if e.wsMgr != nil && item.Repository != nil {
		issueNum := 0
		if item.IssueNumber != nil {
			issueNum = *item.IssueNumber
		}
		ws, err := e.wsMgr.CreateForWorkItem(ctx, workspace.WorkItemRef{
			Owner:       item.Repository.Owner,
			Repo:        item.Repository.Name,
			IssueNumber: issueNum,
			CloneURL:    item.Repository.CloneURLHTTPS,
			BaseBranch:  item.Repository.DefaultBranch,
		})
		if err != nil {
			e.logger.Error("workspace creation failed", "item", item.IssueIdentifier, "error", err)
			delete(e.state.Running, item.WorkItemID)
			cancel()
			_, _ = e.transition(item.WorkItemID, domain.EventError, nil)
			return err
		}
		entry.WorkspacePath = ws.Path
		entry.BranchName = ws.BranchName
	}

	// Transition to running
	_, _ = e.transition(item.WorkItemID, domain.EventWorkspaceReady, nil)
	entry.Phase = domain.StateRunning

	go e.runWorker(workerCtx, item, entry)

	return nil
}

// renderPrompt builds the prompt text for a work item using the template router.
func (e *Engine) renderPrompt(item domain.WorkItem, entry *RunningEntry) string {
	promptText := item.Title + "\n\n" + item.Description
	if e.router == nil {
		return promptText
	}

	tmpl, err := e.router.SelectTemplate(item)
	if err != nil {
		return promptText
	}

	repoFullName := ""
	repoDefaultBranch := ""
	baseBranch := ""
	if item.Repository != nil {
		repoFullName = item.Repository.FullName
		repoDefaultBranch = item.Repository.DefaultBranch
		baseBranch = item.Repository.DefaultBranch
	}

	rendered, err := prompt.Render(tmpl, prompt.RenderInput{
		WorkItem: map[string]any{
			"title":       item.Title,
			"description": item.Description,
			"identifier":  item.IssueIdentifier,
		},
		Repository: map[string]any{
			"full_name":      repoFullName,
			"default_branch": repoDefaultBranch,
		},
		BranchName: entry.BranchName,
		BaseBranch: baseBranch,
	})
	if err == nil && rendered != "" {
		return rendered
	}
	return promptText
}

// drainSession reads from a session's channels until done or cancelled.
// Returns the final agent.Result.
func (e *Engine) drainSession(ctx context.Context, itemID string, session *agent.Session) agent.Result {
	for {
		select {
		case update, ok := <-session.Updates:
			if !ok {
				continue
			}
			e.Emit(NewEvent(EvtAgentUpdate, itemID, AgentUpdatePayload{Update: update}))

		case result, ok := <-session.Done:
			if !ok {
				return agent.Result{StopReason: agent.StopCompleted}
			}
			return result

		case <-ctx.Done():
			return agent.Result{StopReason: agent.StopCancelled}
		}
	}
}

// runWorker executes the agent lifecycle in a goroutine.
// Supports multi-turn: runs up to MaxTurns sessions, re-checking between turns.
func (e *Engine) runWorker(ctx context.Context, item domain.WorkItem, entry *RunningEntry) {
	maxTurns := e.cfg.Agent.MaxTurns
	if maxTurns <= 0 {
		maxTurns = 1
	}

	for turn := 1; turn <= maxTurns; turn++ {
		if ctx.Err() != nil {
			e.Emit(NewEvent(EvtAgentExited, item.WorkItemID, AgentExitedPayload{
				Result: agent.Result{StopReason: agent.StopCancelled},
			}))
			return
		}

		promptText := e.renderPrompt(item, entry)
		session, err := e.agent.Start(ctx, agent.StartConfig{
			WorkDir:  entry.WorkspacePath,
			Prompt:   promptText,
			Title:    item.Title,
			MaxTurns: 1, // single turn per session; we loop externally
		})
		if err != nil {
			e.Emit(NewEvent(EvtAgentExited, item.WorkItemID, AgentExitedPayload{
				Result: agent.Result{StopReason: agent.StopFailed, Error: err},
			}))
			return
		}

		entry.Session = session
		if turn == 1 {
			e.state.Totals.SessionsStarted++
		}

		result := e.drainSession(ctx, item.WorkItemID, session)

		// Cancelled or failed — exit immediately
		if result.StopReason == agent.StopCancelled {
			e.Emit(NewEvent(EvtAgentExited, item.WorkItemID, AgentExitedPayload{Result: result}))
			return
		}
		if result.StopReason == agent.StopFailed || result.Error != nil {
			e.Emit(NewEvent(EvtAgentExited, item.WorkItemID, AgentExitedPayload{Result: result}))
			return
		}

		// Has commits — success, exit with commits
		if result.HasCommits {
			e.Emit(NewEvent(EvtAgentExited, item.WorkItemID, AgentExitedPayload{Result: result}))
			return
		}

		// No commits yet — continue to next turn if turns remain
		if turn < maxTurns {
			entry.TurnsCompleted++
			e.logger.Info("turn complete, continuing", "item", item.IssueIdentifier, "turn", turn, "max", maxTurns)
		}
	}

	// All turns exhausted without commits
	e.Emit(NewEvent(EvtAgentExited, item.WorkItemID, AgentExitedPayload{
		Result: agent.Result{StopReason: agent.StopCompleted, HasCommits: false},
	}))
}

// handleAgentExited processes the result of a completed worker.
func (e *Engine) handleAgentExited(evt EngineEvent) {
	payload := evt.Payload.(AgentExitedPayload)
	itemID := evt.ItemID
	result := payload.Result

	entry, ok := e.state.Running[itemID]
	if !ok {
		// Worker was already removed by stall/budget/reconcile handler.
		// The FSM transition was already applied — do not overwrite it.
		e.logger.Debug("agent exited but worker already removed", "item", itemID)
		return
	}

	// Accumulate metrics
	e.state.Totals.SecondsRunning += time.Since(entry.StartedAt).Seconds()
	e.state.Totals.InputTokens += int64(entry.InputTokens)
	e.state.Totals.OutputTokens += int64(entry.OutputTokens)
	e.state.Totals.TotalTokens += int64(entry.TotalTokens)
	e.state.Totals.CostUSD += entry.CostUSD

	// Remove from running
	delete(e.state.Running, itemID)

	switch {
	case result.StopReason == agent.StopCancelled:
		_, _ = e.transition(itemID, domain.EventCancelled, nil)

	case result.StopReason == agent.StopFailed || result.Error != nil:
		item := domain.WorkItem{}
		attempt := 0
		if entry != nil {
			item = entry.WorkItem
			attempt = entry.RetryAttempt
		}
		e.handleWorkerError(itemID, item, result, attempt)

	case result.HasCommits:
		// FSM: running -> completed
		_, _ = e.transition(itemID, domain.EventAgentExitedCommits, nil)
		// Perform handoff (push branch, create PR, update status)
		handoffItem := domain.WorkItem{}
		if entry != nil {
			handoffItem = entry.WorkItem
		}
		e.performHandoff(itemID, handoffItem, entry)

	default:
		// No commits — needs human intervention
		_, _ = e.transition(itemID, domain.EventAgentExitedEmpty, nil)
	}
}

// handleAgentUpdate processes a progress update from a running agent.
func (e *Engine) handleAgentUpdate(evt EngineEvent) {
	payload := evt.Payload.(AgentUpdatePayload)
	entry, ok := e.state.Running[evt.ItemID]
	if !ok {
		return
	}
	entry.LastActivityAt = time.Now()
	entry.InputTokens += payload.Update.Tokens.Input
	entry.OutputTokens += payload.Update.Tokens.Output
	entry.TotalTokens += payload.Update.Tokens.Total

	if payload.Update.Kind == agent.UpdateTurnDone {
		entry.TurnsCompleted++
	}

	// Check budget after each update
	e.checkBudget(evt.ItemID)
}

// handlePause sets the pause flag on a running item.
func (e *Engine) handlePause(evt EngineEvent) {
	if entry, ok := e.state.Running[evt.ItemID]; ok {
		entry.Paused = true
		_, _ = e.transition(evt.ItemID, domain.EventPauseRequested, nil)
		e.logger.Info("paused", "item", evt.ItemID)
	}
}

// handleResume clears the pause flag.
func (e *Engine) handleResume(evt EngineEvent) {
	_, _ = e.transition(evt.ItemID, domain.EventResume, nil)
	if entry, ok := e.state.Running[evt.ItemID]; ok {
		entry.Paused = false
	}
	e.logger.Info("resumed", "item", evt.ItemID)
}

// handleCancel cancels a running or paused item.
func (e *Engine) handleCancel(evt EngineEvent) {
	if entry, ok := e.state.Running[evt.ItemID]; ok {
		entry.CancelFunc()
		delete(e.state.Running, evt.ItemID)
	}
	_, _ = e.transition(evt.ItemID, domain.EventCancelled, nil)
	e.logger.Info("cancelled", "item", evt.ItemID)
}

// handleShutdown performs graceful shutdown.
// Cancels all running agent processes and persists state.
func (e *Engine) handleShutdown() {
	running := len(e.state.Running)
	e.logger.Info("shutting down engine", "running_workers", running)

	// Cancel all running workers — context cancellation sends SIGKILL to agent processes
	for id, entry := range e.state.Running {
		e.logger.Info("cancelling worker", "item", entry.WorkItem.IssueIdentifier)
		entry.CancelFunc()
		_, _ = e.transition(id, domain.EventCancelled, nil)
	}

	// Persist state before exit
	e.persistState()
	if e.eventLog != nil {
		_ = e.eventLog.Close()
	}
}

// restoreState loads persisted handoffs, retries, and totals from the store.
func (e *Engine) restoreState() {
	if e.store == nil {
		return
	}

	// Restore handoffs
	handoffs, err := e.store.LoadHandoffs()
	if err != nil {
		e.logger.Warn("failed to load handoffs", "error", err)
	} else {
		for _, id := range handoffs {
			e.state.HandedOff[id] = true
			e.state.SetItemState(id, domain.StateHandedOff)
		}
		if len(handoffs) > 0 {
			e.logger.Info("restored handoffs", "count", len(handoffs))
		}
	}

	// Restore retries
	retries, err := e.store.LoadRetries()
	if err != nil {
		e.logger.Warn("failed to load retries", "error", err)
	} else {
		for _, r := range retries {
			e.state.RetryQueue[r.WorkItemID] = &RetryEntry{
				WorkItemID: r.WorkItemID,
				Attempt:    r.Attempt,
				DueAt:      time.UnixMilli(r.DueAtMs),
				Error:      r.Error,
			}
			e.state.SetItemState(r.WorkItemID, domain.StateQueued)
		}
		if len(retries) > 0 {
			e.logger.Info("restored retries", "count", len(retries))
		}
	}

	// Restore totals
	totals, err := e.store.LoadTotals()
	if err != nil {
		e.logger.Warn("failed to load totals", "error", err)
	} else {
		e.state.Totals.InputTokens = totals.InputTokens
		e.state.Totals.OutputTokens = totals.OutputTokens
		e.state.Totals.TotalTokens = totals.TotalTokens
		e.state.Totals.SecondsRunning = totals.SecondsRunning
		e.state.Totals.SessionsStarted = totals.SessionsStarted
	}
}

// persistState saves handoffs, retries, and totals to the store.
func (e *Engine) persistState() {
	if e.store == nil {
		return
	}

	// Persist handoffs
	for id := range e.state.HandedOff {
		if err := e.store.SaveHandoff(id); err != nil {
			e.logger.Warn("failed to save handoff", "item", id, "error", err)
		}
	}

	// Persist retries
	for _, re := range e.state.RetryQueue {
		if err := e.store.SaveRetry(state.RetryRecord{
			WorkItemID: re.WorkItemID,
			Attempt:    re.Attempt,
			DueAtMs:    re.DueAt.UnixMilli(),
			Error:      re.Error,
		}); err != nil {
			e.logger.Warn("failed to save retry", "item", re.WorkItemID, "error", err)
		}
	}

	// Persist totals
	if err := e.store.SaveTotals(state.AgentTotalsRecord{
		InputTokens:    e.state.Totals.InputTokens,
		OutputTokens:   e.state.Totals.OutputTokens,
		TotalTokens:    e.state.Totals.TotalTokens,
		SecondsRunning: e.state.Totals.SecondsRunning,
		SessionsStarted: e.state.Totals.SessionsStarted,
	}); err != nil {
		e.logger.Warn("failed to save totals", "error", err)
	}

	e.logger.Info("state persisted",
		"handoffs", len(e.state.HandedOff),
		"retries", len(e.state.RetryQueue),
	)
}

// transition performs an FSM transition with event logging.
func (e *Engine) transition(itemID string, event domain.Event, guard func(domain.TransitionGuard) bool) (domain.TransitionResult, error) {
	current := e.state.ItemState(itemID)
	result, err := domain.Transition(current, event, guard)
	if err != nil {
		e.logger.Error("invalid transition", "item", itemID, "from", current, "event", event, "error", err)
		return result, err
	}

	e.state.SetItemState(itemID, result.To)

	// Log to event store
	if e.eventLog != nil {
		_ = e.eventLog.Append(domain.FSMEvent{
			Timestamp: time.Now(),
			ItemID:    itemID,
			From:      result.From,
			To:        result.To,
			Event:     event,
			Guard:     result.Guard,
		})
	}

	e.logger.Debug("transition", "item", itemID, "from", result.From, "to", result.To, "event", event)
	return result, nil
}

