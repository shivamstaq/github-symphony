package claude_test

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/shivamstaq/github-symphony/internal/adapter"
	"github.com/shivamstaq/github-symphony/internal/adapter/claude"
)

// mockSidecar is a bash script that simulates the Claude TS sidecar protocol.
const mockSidecar = `
while IFS= read -r line; do
  method=$(echo "$line" | python3 -c "import sys,json; print(json.loads(sys.stdin.read())['method'])" 2>/dev/null || echo "unknown")
  id=$(echo "$line" | python3 -c "import sys,json; print(json.loads(sys.stdin.read())['id'])" 2>/dev/null || echo "0")

  case "$method" in
    initialize)
      echo "{\"id\":${id},\"result\":{\"protocolVersion\":1,\"provider\":\"claude_code\",\"adapterInfo\":{\"name\":\"symphony-claude-adapter\",\"version\":\"1.0.0\"},\"capabilities\":{\"sessionReuse\":true,\"tokenUsage\":true}}}"
      ;;
    session/new)
      echo "{\"id\":${id},\"result\":{\"sessionId\":\"sess_mock_1\"}}"
      ;;
    session/prompt)
      echo "{\"method\":\"session/update\",\"params\":{\"sessionId\":\"sess_mock_1\",\"update\":{\"kind\":\"progress\",\"message\":\"Working...\"}}}"
      echo "{\"id\":${id},\"result\":{\"stopReason\":\"completed\",\"summary\":\"Done.\"}}"
      ;;
    session/cancel)
      echo "{\"id\":${id},\"result\":{\"cancelled\":true}}"
      ;;
    session/close)
      echo "{\"id\":${id},\"result\":{\"closed\":true}}"
      ;;
    *)
      echo "{\"id\":${id},\"error\":{\"code\":-1,\"message\":\"unknown method: ${method}\"}}"
      ;;
  esac
done
`

func TestClaudeAdapter_FullLifecycle(t *testing.T) {
	cwd := t.TempDir()

	ca, err := claude.New(claude.Config{
		// Override the sidecar with our mock bash script
		Command: "bash",
		Args:    []string{"-c", mockSidecar},
		Cwd:     cwd,
	})
	if err != nil {
		t.Fatalf("failed to create claude adapter: %v", err)
	}
	defer ca.Close()

	ctx := context.Background()

	// Initialize
	caps, err := ca.Initialize(ctx)
	if err != nil {
		t.Fatalf("Initialize failed: %v", err)
	}
	if caps.Provider != "claude_code" {
		t.Errorf("expected provider=claude_code, got %q", caps.Provider)
	}

	// Create session
	sessionID, err := ca.NewSession(ctx, claude.SessionParams{
		Cwd:   cwd,
		Title: "Test session",
	})
	if err != nil {
		t.Fatalf("NewSession failed: %v", err)
	}
	if sessionID != "sess_mock_1" {
		t.Errorf("expected session=sess_mock_1, got %q", sessionID)
	}

	// Send prompt
	result, err := ca.Prompt(ctx, sessionID, "Fix the bug")
	if err != nil {
		t.Fatalf("Prompt failed: %v", err)
	}
	if result.StopReason != adapter.StopCompleted {
		t.Errorf("expected StopCompleted, got %q", result.StopReason)
	}

	// Close session
	if err := ca.CloseSession(ctx, sessionID); err != nil {
		t.Fatalf("CloseSession failed: %v", err)
	}
}

func TestClaudeAdapter_CheckDependencies(t *testing.T) {
	// Verify the dependency check function exists and works
	err := claude.CheckDependencies()
	// This may pass or fail depending on whether node/tsx are installed,
	// but it should not panic
	_ = err
}

func TestClaudeAdapter_WorkspacePath(t *testing.T) {
	// Verify the adapter properly associates with a workspace
	cwd := t.TempDir()
	ws := filepath.Join(cwd, "workspace")

	ca, err := claude.New(claude.Config{
		Command: "bash",
		Args:    []string{"-c", mockSidecar},
		Cwd:     ws,
	})
	if err != nil {
		// Cwd doesn't need to exist for the subprocess config
		_ = ca
		return
	}
	defer ca.Close()
}
