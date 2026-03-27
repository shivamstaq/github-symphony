package engine

import (
	"encoding/json"
	"fmt"
	"os"
	"sync"

	"github.com/shivamstaq/github-symphony/internal/domain"
)

// EventLog is an append-only JSONL writer for FSM state transitions.
type EventLog struct {
	mu   sync.Mutex
	file *os.File
}

// NewEventLog opens (or creates) the event log file for appending.
func NewEventLog(path string) (*EventLog, error) {
	f, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0644)
	if err != nil {
		return nil, fmt.Errorf("open event log: %w", err)
	}
	return &EventLog{file: f}, nil
}

// Append writes an FSM event to the log.
func (l *EventLog) Append(evt domain.FSMEvent) error {
	data, err := json.Marshal(evt)
	if err != nil {
		return fmt.Errorf("marshal event: %w", err)
	}
	data = append(data, '\n')

	l.mu.Lock()
	defer l.mu.Unlock()
	_, err = l.file.Write(data)
	return err
}

// Close flushes and closes the event log.
func (l *EventLog) Close() error {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.file.Close()
}
