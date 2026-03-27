package engine

import (
	"time"

	"github.com/shivamstaq/github-symphony/internal/agent"
	"github.com/shivamstaq/github-symphony/internal/domain"
)

// EventType classifies engine events flowing through the central event loop.
type EventType string

const (
	EvtPollTick        EventType = "poll_tick"
	EvtItemDiscovered  EventType = "item_discovered"
	EvtItemClaimed     EventType = "item_claimed"
	EvtDispatch        EventType = "dispatch"
	EvtWorkspaceReady  EventType = "workspace_ready"
	EvtTurnStarted     EventType = "turn_started"
	EvtTurnCompleted   EventType = "turn_completed"
	EvtAgentExited     EventType = "agent_exited"
	EvtAgentUpdate     EventType = "agent_update"
	EvtPauseRequested  EventType = "pause_requested"
	EvtResumeRequested EventType = "resume_requested"
	EvtCancelRequested EventType = "cancel_requested"
	EvtStallDetected   EventType = "stall_detected"
	EvtBudgetExceeded  EventType = "budget_exceeded"
	EvtPRCreated       EventType = "pr_created"
	EvtWritebackError  EventType = "writeback_error"
	EvtReconcile       EventType = "reconcile"
	EvtRetryDue        EventType = "retry_due"
	EvtWebhookReceived EventType = "webhook_received"
	EvtShutdown        EventType = "shutdown"
	EvtConfigReload    EventType = "config_reload"
)

// EngineEvent is the typed message flowing through the central event loop.
type EngineEvent struct {
	Type      EventType
	Timestamp time.Time
	ItemID    string // work item ID (empty for system events)
	Payload   any
}

// Typed payloads for each event type.

type PollTickPayload struct{}

type ItemDiscoveredPayload struct {
	Items []domain.WorkItem
}

type DispatchPayload struct {
	Item domain.WorkItem
}

type AgentExitedPayload struct {
	Result agent.Result
}

type AgentUpdatePayload struct {
	Update agent.Update
}

type PRCreatedPayload struct {
	Number int
	URL    string
}

type WritebackErrorPayload struct {
	Error error
}

type RetryDuePayload struct {
	Attempt int
}

type StallDetectedPayload struct {
	LastActivity time.Time
	Threshold    time.Duration
}

// NewEvent creates an EngineEvent with the current timestamp.
func NewEvent(typ EventType, itemID string, payload any) EngineEvent {
	return EngineEvent{
		Type:      typ,
		Timestamp: time.Now(),
		ItemID:    itemID,
		Payload:   payload,
	}
}
