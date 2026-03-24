package orchestrator

import "time"

// WorkItem is the normalized domain model for a project item.
type WorkItem struct {
	WorkItemID       string
	ProjectItemID    string
	ContentType      string // "issue", "draft_issue", "pull_request"
	IssueID          string
	IssueNumber      *int
	IssueIdentifier  string // "owner/repo#number"
	Title            string
	Description      string
	State            string // "open", "closed"
	ProjectStatus    string
	Priority         *int
	Labels           []string
	Assignees        []string
	Milestone        string
	ProjectFields    map[string]any
	BlockedBy        []BlockerRef
	SubIssues        []ChildRef
	ParentIssue      *ParentRef
	LinkedPRs        []PRRef
	Repository       *Repository
	URL              string
	CreatedAt        string
	UpdatedAt        string
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
	WorkItem           WorkItem
	WorkerDone         chan WorkerResult
	IssueIdentifier    string
	Repository         string
	SessionID          string
	AdapterPID         int
	LastAgentEvent     string
	LastAgentTimestamp  *time.Time
	LastAgentMessage   string
	InputTokens        int
	OutputTokens       int
	TotalTokens        int
	RetryAttempt       *int
	StartedAt          time.Time
}

// WorkerResult is what a worker goroutine sends back on completion.
type WorkerResult struct {
	WorkItemID string
	Outcome    WorkerOutcome
	Error      error
}

type WorkerOutcome string

const (
	OutcomeNormal   WorkerOutcome = "normal"
	OutcomeHandoff  WorkerOutcome = "handoff"
	OutcomeFailure  WorkerOutcome = "failure"
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
	AgentTotals         AgentTotals
	PendingRefresh      bool
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
