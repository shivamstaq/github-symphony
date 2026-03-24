package adapter_test

import (
	"context"
	"testing"

	"github.com/shivamstaq/github-symphony/internal/adapter"
)

func TestSubprocessAdapter_EchoRoundTrip(t *testing.T) {
	// Launch a simple echo adapter: reads JSON line from stdin, writes it back to stdout
	// We use a bash one-liner that reads a line and echoes back a valid response
	script := `while IFS= read -r line; do echo '{"id":1,"result":{"protocolVersion":1,"provider":"test","capabilities":{}}}'; done`

	a, err := adapter.NewSubprocessAdapter(adapter.SubprocessConfig{
		Command: "bash",
		Args:    []string{"-c", script},
	})
	if err != nil {
		t.Fatalf("failed to create adapter: %v", err)
	}
	defer a.Close()

	// Send initialize
	resp, err := a.SendRequest(context.Background(), adapter.Request{
		ID:     1,
		Method: "initialize",
		Params: map[string]any{
			"protocolVersion": 1,
		},
	})
	if err != nil {
		t.Fatalf("SendRequest failed: %v", err)
	}

	if resp.ID != 1 {
		t.Errorf("expected id=1, got %d", resp.ID)
	}
	if resp.Result == nil {
		t.Fatal("expected non-nil result")
	}
}

func TestSubprocessAdapter_ProcessExitDetected(t *testing.T) {
	// Adapter that exits immediately
	a, err := adapter.NewSubprocessAdapter(adapter.SubprocessConfig{
		Command: "bash",
		Args:    []string{"-c", "exit 0"},
	})
	if err != nil {
		t.Fatalf("failed to create adapter: %v", err)
	}
	defer a.Close()

	_, err = a.SendRequest(context.Background(), adapter.Request{
		ID:     1,
		Method: "initialize",
	})
	if err == nil {
		t.Fatal("expected error when process exits")
	}
}
