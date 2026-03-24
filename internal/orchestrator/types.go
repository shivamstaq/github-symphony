package orchestrator

import (
	"context"
	"time"
)

// WorkItemState is the orchestrator's internal claim state for a work item.
type WorkItemState string

const (
	StateUnclaimed  WorkItemState = "unclaimed"
	StateClaimed    WorkItemState = "claimed"
	StateRunning    WorkItemState = "running"
	StateRetryQueue WorkItemState = "retry_queued"
	StateHandedOff  WorkItemState = "handed_off"
	StateReleased   WorkItemState = "released"
)

// RunAttemptPhase tracks where a run attempt is in its lifecycle.
type RunAttemptPhase string

const (
	PhasePreparingWorkspace  RunAttemptPhase = "preparing_workspace"
	PhaseSyncingRepository   RunAttemptPhase = "syncing_repository"
	PhaseBuildingPrompt      RunAttemptPhase = "building_prompt"
	PhaseLaunchingAgent      RunAttemptPhase = "launching_agent"
	PhaseInitializingSession RunAttemptPhase = "initializing_session"
	PhaseStreamingTurn       RunAttemptPhase = "streaming_turn"
	PhaseValidatingOutputs   RunAttemptPhase = "validating_outputs"
	PhaseWritingBackGitHub   RunAttemptPhase = "writing_back_github"
	PhaseFinishing           RunAttemptPhase = "finishing"
	PhaseSucceeded           RunAttemptPhase = "succeeded"
	PhaseHandedOff           RunAttemptPhase = "handed_off_for_review"
	PhaseFailed              RunAttemptPhase = "failed"
	PhaseTimedOut            RunAttemptPhase = "timed_out"
	PhaseStalled             RunAttemptPhase = "stalled"
	PhaseCanceled            RunAttemptPhase = "canceled_by_reconciliation"
)

// WorkItem is the normalized domain model for a project item.
type WorkItem struct {
	WorkItemID      string
	ProjectItemID   string
	ContentType     string // "issue", "draft_issue", "pull_request"
	IssueID         string
	IssueNumber     *int
	IssueIdentifier string // "owner/repo#number"
	Title           string
	Description     string
	State           string // "open", "closed"
	ProjectStatus   string
	Priority        *int
	Labels          []string
	Assignees       []string
	Milestone       string
	ProjectFields   map[string]any
	BlockedBy       []BlockerRef
	SubIssues       []ChildRef
	ParentIssue     *ParentRef
	LinkedPRs       []PRRef
	Repository      *Repository
	URL             string
	CreatedAt       string
	UpdatedAt       string
}

type BlockerRef struct {
	ID         string
	Identifier string
	State      string
}

type ChildRef struct {
	ID         string
	Identifier string
	State      string
}

type ParentRef struct {
	ID         string
	Identifier string
}

type PRRef struct {
	ID      string
	Number  int
	State   string
	IsDraft bool
	URL     string
}

type Repository struct {
	Owner         string
	Name          string
	FullName      string
	DefaultBranch string
	CloneURLHTTPS string
}

// RunningEntry tracks one active worker.
type RunningEntry struct {
	WorkItem          WorkItem
	CancelFunc        context.CancelFunc
	IssueIdentifier   string
	Repository        string
	SessionID         string
	AdapterPID        int
	Phase             RunAttemptPhase
	LastAgentEvent    string
	LastAgentTimestamp *time.Time
	LastAgentMessage  string
	InputTokens       int
	OutputTokens      int
	TotalTokens       int
	RetryAttempt      *int
	StartedAt         time.Time
}

// WorkerResult is what a worker goroutine sends back on completion.
type WorkerResult struct {
	WorkItemID string
	Outcome    WorkerOutcome
	Error      error
}

type WorkerOutcome string

const (
	OutcomeNormal  WorkerOutcome = "normal"
	OutcomeHandoff WorkerOutcome = "handoff"
	OutcomeFailure WorkerOutcome = "failure"
)

// RetryEntry is a scheduled retry.
type RetryEntry struct {
	WorkItemID      string
	ProjectItemID   string
	IssueIdentifier string
	Attempt         int
	DueAt           time.Time
	Error           string
}

// State is the orchestrator's authoritative runtime state.
type State struct {
	PollIntervalMs      int
	MaxConcurrentAgents int
	Running             map[string]*RunningEntry
	Claimed             map[string]bool
	RetryAttempts       map[string]*RetryEntry
	Completed           map[string]bool
	HandedOff           map[string]bool
	AgentTotals         AgentTotals
	PendingRefresh      bool
	LastPollAt          *time.Time
	RecentWebhookEvents []WebhookEvent
	DispatchTotal       int64
	ErrorTotal          int64
	HandoffTotal        int64
}

// WebhookEvent records a recent webhook delivery for observability.
type WebhookEvent struct {
	EventType  string
	DeliveryID string
	ReceivedAt time.Time
}

// AgentTotals accumulates agent metrics.
type AgentTotals struct {
	InputTokens      int64
	OutputTokens     int64
	TotalTokens      int64
	SecondsRunning   float64
	GitHubWritebacks int64
	SessionsStarted  int64
}
