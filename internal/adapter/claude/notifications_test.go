package claude_test

import (
	"context"
	"testing"
	"time"

	"github.com/shivamstaq/github-symphony/internal/adapter/claude"
)

// Mock sidecar that emits notifications before the prompt result
const notifyingSidecar = `
while IFS= read -r line; do
  method=$(echo "$line" | python3 -c "import sys,json; print(json.loads(sys.stdin.read())['method'])" 2>/dev/null || echo "unknown")
  id=$(echo "$line" | python3 -c "import sys,json; print(json.loads(sys.stdin.read())['id'])" 2>/dev/null || echo "0")

  case "$method" in
    initialize)
      echo "{\"id\":${id},\"result\":{\"protocolVersion\":1,\"provider\":\"claude_code\",\"capabilities\":{}}}"
      ;;
    session/new)
      echo "{\"id\":${id},\"result\":{\"sessionId\":\"sess_1\"}}"
      ;;
    session/prompt)
      echo "{\"method\":\"session/update\",\"params\":{\"sessionId\":\"sess_1\",\"update\":{\"kind\":\"progress\",\"message\":\"Step 1\"}}}"
      echo "{\"method\":\"session/update\",\"params\":{\"sessionId\":\"sess_1\",\"update\":{\"kind\":\"tool_call_started\",\"message\":\"Running tests\"}}}"
      echo "{\"method\":\"session/update\",\"params\":{\"sessionId\":\"sess_1\",\"update\":{\"kind\":\"completed\",\"message\":\"All done\"}}}"
      echo "{\"id\":${id},\"result\":{\"stopReason\":\"completed\",\"summary\":\"Done.\"}}"
      ;;
    *)
      echo "{\"id\":${id},\"error\":{\"code\":-1,\"message\":\"unknown\"}}"
      ;;
  esac
done
`

func TestClaudeAdapter_NotificationsStreamToGo(t *testing.T) {
	ca, err := claude.New(claude.Config{
		Command: "bash",
		Args:    []string{"-c", notifyingSidecar},
		Cwd:     t.TempDir(),
	})
	if err != nil {
		t.Fatal(err)
	}
	defer ca.Close()

	ctx := context.Background()
	if _, err := ca.Initialize(ctx); err != nil {
		t.Fatal(err)
	}
	sessionID, err := ca.NewSession(ctx, claude.SessionParams{Cwd: t.TempDir(), Title: "Test"})
	if err != nil {
		t.Fatal(err)
	}

	// Send prompt — this will cause 3 notifications before the response
	result, err := ca.Prompt(ctx, sessionID, "do work")
	if err != nil {
		t.Fatalf("Prompt failed: %v", err)
	}
	if string(result.StopReason) != "completed" {
		t.Errorf("expected completed, got %q", result.StopReason)
	}

	// Drain the updates channel — should have the 3 notifications
	var updates []string
	timeout := time.After(2 * time.Second)
	for {
		select {
		case msg := <-ca.Updates():
			if msg.Params != nil {
				if update, ok := msg.Params["update"].(map[string]any); ok {
					if kind, ok := update["kind"].(string); ok {
						updates = append(updates, kind)
					}
				}
			}
		case <-timeout:
			goto done
		default:
			if len(updates) >= 3 {
				goto done
			}
			time.Sleep(10 * time.Millisecond)
		}
	}
done:

	if len(updates) < 3 {
		t.Fatalf("expected at least 3 notifications, got %d: %v", len(updates), updates)
	}
	if updates[0] != "progress" {
		t.Errorf("first notification should be progress, got %q", updates[0])
	}
	if updates[1] != "tool_call_started" {
		t.Errorf("second notification should be tool_call_started, got %q", updates[1])
	}
	if updates[2] != "completed" {
		t.Errorf("third notification should be completed, got %q", updates[2])
	}
}
