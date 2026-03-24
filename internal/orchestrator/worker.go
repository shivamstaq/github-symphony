package orchestrator

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/shivamstaq/github-symphony/internal/adapter"
	ghub "github.com/shivamstaq/github-symphony/internal/github"
	"github.com/shivamstaq/github-symphony/internal/prompt"
	"github.com/shivamstaq/github-symphony/internal/workspace"
)

// WorkerDeps holds dependencies injected into the worker runner.
type WorkerDeps struct {
	WorkspaceManager *workspace.Manager
	AdapterFactory   func(cwd string) (adapter.AdapterClient, error)
	Source           WorkItemSource
	WriteBack        *ghub.WriteBack
	PromptTemplate   string
	MaxTurns         int
	HooksBefore      string
	HooksAfter       string
	HooksTimeoutMs   int
	PullRequestCfg   PullRequestConfig
}

// PullRequestConfig for write-back decisions.
type PullRequestConfig struct {
	OpenPROnSuccess      bool
	DraftByDefault       bool
	HandoffProjectStatus string
	CommentOnIssue       bool
	ProjectID            string
	StatusFieldID        string
	HandoffOptionID      string
}

// Runner implements WorkerRunner with the full multi-turn loop.
type Runner struct {
	deps WorkerDeps
}

// NewRunner creates a worker runner with all dependencies.
func NewRunner(deps WorkerDeps) *Runner {
	if deps.MaxTurns <= 0 {
		deps.MaxTurns = 20
	}
	return &Runner{deps: deps}
}

// Run executes the full worker lifecycle for one work item.
func (r *Runner) Run(ctx context.Context, item WorkItem, attempt *int) WorkerResult {
	logger := slog.With(
		"work_item_id", item.WorkItemID,
		"issue", item.IssueIdentifier,
		"repo", item.Repository.FullName,
	)

	// 1. Create/reuse workspace
	ws, err := r.deps.WorkspaceManager.CreateForWorkItem(ctx, workspace.WorkItemRef{
		Owner:       item.Repository.Owner,
		Repo:        item.Repository.Name,
		IssueNumber: ptrVal(item.IssueNumber),
		CloneURL:    item.Repository.CloneURLHTTPS,
		BaseBranch:  item.Repository.DefaultBranch,
	})
	if err != nil {
		logger.Error("workspace creation failed", "error", err)
		return WorkerResult{WorkItemID: item.WorkItemID, Outcome: OutcomeFailure, Error: err}
	}

	hookTimeout := time.Duration(r.deps.HooksTimeoutMs) * time.Millisecond
	if hookTimeout == 0 {
		hookTimeout = 60 * time.Second
	}

	// 2. Run before_run hook
	if r.deps.HooksBefore != "" {
		if err := workspace.RunHook(ctx, "before_run", r.deps.HooksBefore, ws.Path, hookTimeout); err != nil {
			logger.Error("before_run hook failed", "error", err)
			return WorkerResult{WorkItemID: item.WorkItemID, Outcome: OutcomeFailure, Error: err}
		}
	}

	// 3. Start adapter session
	adapterClient, err := r.deps.AdapterFactory(ws.Path)
	if err != nil {
		logger.Error("adapter creation failed", "error", err)
		r.runAfterHook(ctx, ws.Path, hookTimeout)
		return WorkerResult{WorkItemID: item.WorkItemID, Outcome: OutcomeFailure, Error: err}
	}
	defer adapterClient.Close()

	if _, err := adapterClient.Initialize(ctx); err != nil {
		logger.Error("adapter initialize failed", "error", err)
		r.runAfterHook(ctx, ws.Path, hookTimeout)
		return WorkerResult{WorkItemID: item.WorkItemID, Outcome: OutcomeFailure, Error: err}
	}

	sessionID, err := adapterClient.NewSession(ctx, adapter.SessionParams{
		Cwd:   ws.Path,
		Title: fmt.Sprintf("%s: %s", item.IssueIdentifier, item.Title),
	})
	if err != nil {
		logger.Error("session creation failed", "error", err)
		r.runAfterHook(ctx, ws.Path, hookTimeout)
		return WorkerResult{WorkItemID: item.WorkItemID, Outcome: OutcomeFailure, Error: err}
	}

	// 4. Multi-turn loop
	var lastResult *adapter.PromptResult
	handoffReached := false

	for turn := 1; turn <= r.deps.MaxTurns; turn++ {
		logger.Info("starting turn", "turn", turn, "max_turns", r.deps.MaxTurns)

		// Build prompt
		promptText, err := prompt.Render(r.deps.PromptTemplate, prompt.RenderInput{
			WorkItem:   workItemToMap(item),
			Repository: repoToMap(item.Repository),
			Attempt:    attempt,
			BranchName: ws.BranchName,
			BaseBranch: ws.BaseBranch,
		})
		if err != nil {
			logger.Error("prompt render failed", "error", err)
			_ = adapterClient.CloseSession(ctx, sessionID)
			r.runAfterHook(ctx, ws.Path, hookTimeout)
			return WorkerResult{WorkItemID: item.WorkItemID, Outcome: OutcomeFailure, Error: err}
		}

		// Send prompt
		lastResult, err = adapterClient.Prompt(ctx, sessionID, promptText)
		if err != nil {
			logger.Error("prompt turn failed", "turn", turn, "error", err)
			_ = adapterClient.CloseSession(ctx, sessionID)
			r.runAfterHook(ctx, ws.Path, hookTimeout)
			return WorkerResult{WorkItemID: item.WorkItemID, Outcome: OutcomeFailure, Error: err}
		}

		logger.Info("turn completed", "turn", turn, "stop_reason", lastResult.StopReason)

		// Re-check work item state between turns
		if r.deps.Source != nil && turn < r.deps.MaxTurns {
			refreshed, err := r.deps.Source.FetchStates(ctx, []string{item.WorkItemID})
			if err != nil {
				logger.Warn("state refresh failed between turns", "error", err)
			} else if len(refreshed) > 0 {
				item = refreshed[0]

				// Check if item is no longer active
				if item.State != "open" {
					logger.Info("work item no longer active, stopping", "state", item.State)
					break
				}

				// Check handoff condition
				handoff := EvaluateHandoff(HandoffInput{
					HasPR:                len(item.LinkedPRs) > 0,
					CurrentProjectStatus: item.ProjectStatus,
					HandoffProjectStatus: r.deps.PullRequestCfg.HandoffProjectStatus,
				})
				if handoff.IsHandoff {
					logger.Info("handoff condition met between turns")
					handoffReached = true
					break
				}
			}
		}
	}

	// 5. Close adapter session
	_ = adapterClient.CloseSession(ctx, sessionID)

	// 6. Write-back (if configured and agent completed work)
	if r.deps.PullRequestCfg.OpenPROnSuccess && r.deps.WriteBack != nil && lastResult != nil && lastResult.StopReason == adapter.StopCompleted {
		if err := r.performWriteBack(ctx, item, ws, logger); err != nil {
			logger.Error("write-back failed", "error", err)
			r.runAfterHook(ctx, ws.Path, hookTimeout)
			return WorkerResult{WorkItemID: item.WorkItemID, Outcome: OutcomeFailure, Error: err}
		}

		// Re-evaluate handoff after write-back
		handoff := EvaluateHandoff(HandoffInput{
			HasPR:                true,
			CurrentProjectStatus: item.ProjectStatus,
			HandoffProjectStatus: r.deps.PullRequestCfg.HandoffProjectStatus,
		})
		if handoff.IsHandoff {
			handoffReached = true
		}
	}

	// 7. Run after_run hook (best-effort)
	r.runAfterHook(ctx, ws.Path, hookTimeout)

	// 8. Return outcome
	if handoffReached {
		logger.Info("worker completed with handoff")
		return WorkerResult{WorkItemID: item.WorkItemID, Outcome: OutcomeHandoff}
	}

	logger.Info("worker completed normally")
	return WorkerResult{WorkItemID: item.WorkItemID, Outcome: OutcomeNormal}
}

func (r *Runner) performWriteBack(ctx context.Context, item WorkItem, ws *workspace.Workspace, logger *slog.Logger) error {
	// Push branch
	if err := r.deps.WorkspaceManager.PushBranch(ws.Path, "origin", ws.BranchName); err != nil {
		return fmt.Errorf("push branch: %w", err)
	}

	// Upsert PR
	baseBranch := ws.BaseBranch
	if baseBranch == "" {
		baseBranch = "main"
	}

	prResult, err := r.deps.WriteBack.UpsertPR(ctx, ghub.PRParams{
		Owner:      item.Repository.Owner,
		Repo:       item.Repository.Name,
		Title:      item.Title,
		Body:       fmt.Sprintf("Automated by Symphony for %s", item.IssueIdentifier),
		HeadBranch: ws.BranchName,
		BaseBranch: baseBranch,
		Draft:      r.deps.PullRequestCfg.DraftByDefault,
	})
	if err != nil {
		return fmt.Errorf("upsert PR: %w", err)
	}

	action := "created"
	if !prResult.Created {
		action = "updated"
	}
	logger.Info("PR write-back", "action", action, "number", prResult.Number, "url", prResult.URL)

	// Comment on issue
	if r.deps.PullRequestCfg.CommentOnIssue && item.IssueNumber != nil {
		body := fmt.Sprintf("Symphony %s PR: %s", action, prResult.URL)
		_, err := r.deps.WriteBack.CommentOnIssue(ctx, item.Repository.Owner, item.Repository.Name, *item.IssueNumber, body)
		if err != nil {
			logger.Warn("issue comment failed", "error", err)
			// Non-fatal
		}
	}

	return nil
}

func (r *Runner) runAfterHook(ctx context.Context, wsPath string, timeout time.Duration) {
	if r.deps.HooksAfter != "" {
		if err := workspace.RunHook(ctx, "after_run", r.deps.HooksAfter, wsPath, timeout); err != nil {
			slog.Warn("after_run hook failed (ignored)", "error", err)
		}
	}
}

func ptrVal(p *int) int {
	if p == nil {
		return 0
	}
	return *p
}

func workItemToMap(item WorkItem) map[string]any {
	return map[string]any{
		"work_item_id":      item.WorkItemID,
		"project_item_id":   item.ProjectItemID,
		"content_type":      item.ContentType,
		"issue_id":          item.IssueID,
		"issue_number":      item.IssueNumber,
		"issue_identifier":  item.IssueIdentifier,
		"title":             item.Title,
		"description":       item.Description,
		"state":             item.State,
		"project_status":    item.ProjectStatus,
		"labels":            item.Labels,
		"assignees":         item.Assignees,
		"milestone":         item.Milestone,
		"url":               item.URL,
	}
}

func repoToMap(repo *Repository) map[string]any {
	if repo == nil {
		return nil
	}
	return map[string]any{
		"owner":          repo.Owner,
		"name":           repo.Name,
		"full_name":      repo.FullName,
		"default_branch": repo.DefaultBranch,
	}
}
