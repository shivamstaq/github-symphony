package orchestrator

import (
	"sync"
	"time"
)

// Event represents a significant orchestrator event for the TUI/logging.
type Event struct {
	Time       time.Time
	WorkItemID string
	Issue      string // human-readable issue identifier
	Kind       EventKind
	Message    string
	Meta       map[string]any
}

// EventKind identifies the type of orchestrator event.
type EventKind string

const (
	EventDispatched       EventKind = "dispatched"
	EventTurnStarted      EventKind = "turn_started"
	EventTurnCompleted    EventKind = "turn_completed"
	EventPRCreated        EventKind = "pr_created"
	EventHandoff          EventKind = "handoff"
	EventBlocked          EventKind = "blocked"
	EventRetryScheduled   EventKind = "retry_scheduled"
	EventRetryFired       EventKind = "retry_fired"
	EventReconciled       EventKind = "reconciled"
	EventWorkspaceCreated EventKind = "workspace_created"
	EventError            EventKind = "error"
	EventShutdown         EventKind = "shutdown"
)

// EventBus distributes events to subscribers.
type EventBus struct {
	mu          sync.RWMutex
	subscribers []chan Event
	recent      []Event
	maxRecent   int
}

// NewEventBus creates an event bus with a ring buffer of recent events.
func NewEventBus(maxRecent int) *EventBus {
	if maxRecent <= 0 {
		maxRecent = 100
	}
	return &EventBus{maxRecent: maxRecent}
}

// Subscribe returns a channel that receives events.
func (eb *EventBus) Subscribe() chan Event {
	ch := make(chan Event, 50)
	eb.mu.Lock()
	eb.subscribers = append(eb.subscribers, ch)
	eb.mu.Unlock()
	return ch
}

// Emit sends an event to all subscribers and stores in recent buffer.
func (eb *EventBus) Emit(e Event) {
	if e.Time.IsZero() {
		e.Time = time.Now()
	}

	eb.mu.Lock()
	// Ring buffer
	eb.recent = append(eb.recent, e)
	if len(eb.recent) > eb.maxRecent {
		eb.recent = eb.recent[len(eb.recent)-eb.maxRecent:]
	}
	subs := make([]chan Event, len(eb.subscribers))
	copy(subs, eb.subscribers)
	eb.mu.Unlock()

	for _, ch := range subs {
		select {
		case ch <- e:
		default:
			// subscriber too slow, drop event
		}
	}
}

// Recent returns the most recent events.
func (eb *EventBus) Recent() []Event {
	eb.mu.RLock()
	defer eb.mu.RUnlock()
	result := make([]Event, len(eb.recent))
	copy(result, eb.recent)
	return result
}
