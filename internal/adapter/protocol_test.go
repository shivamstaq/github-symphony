package adapter_test

import (
	"bytes"
	"encoding/json"
	"testing"

	"github.com/shivamstaq/github-symphony/internal/adapter"
)

func TestEncodeRequest(t *testing.T) {
	req := adapter.Request{
		ID:     1,
		Method: "initialize",
		Params: map[string]any{
			"protocolVersion": 1,
			"clientInfo":      map[string]any{"name": "symphony", "version": "3.0"},
		},
	}

	var buf bytes.Buffer
	enc := adapter.NewEncoder(&buf)
	if err := enc.Encode(req); err != nil {
		t.Fatal(err)
	}

	// Should be a single line of JSON followed by newline
	line := buf.String()
	if line[len(line)-1] != '\n' {
		t.Error("expected trailing newline")
	}

	var decoded adapter.Request
	if err := json.Unmarshal([]byte(line), &decoded); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
	if decoded.Method != "initialize" {
		t.Errorf("expected method=initialize, got %q", decoded.Method)
	}
}

func TestDecodeResponse(t *testing.T) {
	line := `{"id":1,"result":{"protocolVersion":1,"provider":"codex"}}` + "\n"
	dec := adapter.NewDecoder(bytes.NewReader([]byte(line)))

	msg, err := dec.Decode()
	if err != nil {
		t.Fatal(err)
	}

	if msg.ID != 1 {
		t.Errorf("expected id=1, got %v", msg.ID)
	}
	if msg.Result == nil {
		t.Fatal("expected non-nil result")
	}
}

func TestDecodeNotification(t *testing.T) {
	line := `{"method":"session/update","params":{"sessionId":"s1","update":{"kind":"progress"}}}` + "\n"
	dec := adapter.NewDecoder(bytes.NewReader([]byte(line)))

	msg, err := dec.Decode()
	if err != nil {
		t.Fatal(err)
	}

	if msg.Method != "session/update" {
		t.Errorf("expected method=session/update, got %q", msg.Method)
	}
	if msg.ID != 0 {
		t.Error("notifications should have zero id")
	}
}

func TestStopReasonConstants(t *testing.T) {
	// Verify stop reason constants exist and are the right values
	reasons := []adapter.StopReason{
		adapter.StopCompleted,
		adapter.StopFailed,
		adapter.StopCancelled,
		adapter.StopTimedOut,
		adapter.StopStalled,
		adapter.StopInputRequired,
		adapter.StopHandoff,
	}

	expected := []string{"completed", "failed", "cancelled", "timed_out", "stalled", "input_required", "handoff"}
	for i, r := range reasons {
		if string(r) != expected[i] {
			t.Errorf("stop reason %d: expected %q, got %q", i, expected[i], r)
		}
	}
}
