package adapter

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
)

// StopReason indicates why a prompt turn ended.
type StopReason string

const (
	StopCompleted     StopReason = "completed"
	StopFailed        StopReason = "failed"
	StopCancelled     StopReason = "cancelled"
	StopTimedOut      StopReason = "timed_out"
	StopStalled       StopReason = "stalled"
	StopInputRequired StopReason = "input_required"
	StopHandoff       StopReason = "handoff"
)

// UpdateKind identifies the type of streaming update.
type UpdateKind string

const (
	UpdateSessionStarted  UpdateKind = "session_started"
	UpdateAssistantText   UpdateKind = "assistant_text"
	UpdateProgress        UpdateKind = "progress"
	UpdateToolCallStarted UpdateKind = "tool_call_started"
	UpdateToolCallDone    UpdateKind = "tool_call_completed"
	UpdateToolCallFailed  UpdateKind = "tool_call_failed"
	UpdateTokenUsage      UpdateKind = "token_usage"
	UpdateRateLimits      UpdateKind = "rate_limits"
	UpdateApprovalAutoApproved UpdateKind = "approval_auto_approved"
	UpdateApprovalRequested    UpdateKind = "approval_requested"
	UpdateInputRequested       UpdateKind = "input_requested"
	UpdateCompleted            UpdateKind = "completed"
	UpdateFailed               UpdateKind = "failed"
	UpdateWarning              UpdateKind = "warning"
	UpdateNotification         UpdateKind = "notification"
	UpdateMalformed            UpdateKind = "malformed"
)

// Request is a JSON-RPC-style request.
type Request struct {
	ID     int            `json:"id,omitempty"`
	Method string         `json:"method"`
	Params map[string]any `json:"params,omitempty"`
}

// Message is a JSON-RPC-style response, notification, or request from the adapter.
type Message struct {
	ID     int            `json:"id,omitempty"`
	Method string         `json:"method,omitempty"`
	Result map[string]any `json:"result,omitempty"`
	Error  *RPCError      `json:"error,omitempty"`
	Params map[string]any `json:"params,omitempty"`
}

// RPCError is a JSON-RPC error object.
type RPCError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
	Data    any    `json:"data,omitempty"`
}

// IsNotification returns true if the message has no ID (notification).
func (m *Message) IsNotification() bool {
	return m.ID == 0 && m.Method != ""
}

// IsResponse returns true if the message has a result or error.
func (m *Message) IsResponse() bool {
	return m.ID != 0 && (m.Result != nil || m.Error != nil)
}

// IsRequest returns true if the message has a method and ID (adapter -> client request).
func (m *Message) IsRequest() bool {
	return m.ID != 0 && m.Method != ""
}

// Encoder writes JSON-RPC messages as newline-delimited JSON to a writer.
type Encoder struct {
	w io.Writer
}

// NewEncoder creates a new protocol encoder.
func NewEncoder(w io.Writer) *Encoder {
	return &Encoder{w: w}
}

// Encode writes a request as a single JSON line.
func (e *Encoder) Encode(req Request) error {
	data, err := json.Marshal(req)
	if err != nil {
		return fmt.Errorf("protocol encode: %w", err)
	}
	data = append(data, '\n')
	_, err = e.w.Write(data)
	return err
}

// Decoder reads JSON-RPC messages from a reader (newline-delimited).
type Decoder struct {
	scanner *bufio.Scanner
}

// NewDecoder creates a new protocol decoder.
func NewDecoder(r io.Reader) *Decoder {
	scanner := bufio.NewScanner(r)
	scanner.Buffer(make([]byte, 10*1024*1024), 10*1024*1024) // 10MB max line
	return &Decoder{scanner: scanner}
}

// Decode reads the next message. Returns io.EOF when no more messages.
func (d *Decoder) Decode() (*Message, error) {
	if !d.scanner.Scan() {
		if err := d.scanner.Err(); err != nil {
			return nil, fmt.Errorf("protocol decode: %w", err)
		}
		return nil, io.EOF
	}

	line := d.scanner.Bytes()
	if len(line) == 0 {
		return d.Decode() // skip empty lines
	}

	var msg Message
	if err := json.Unmarshal(line, &msg); err != nil {
		return nil, fmt.Errorf("protocol decode: invalid JSON: %w", err)
	}

	return &msg, nil
}
