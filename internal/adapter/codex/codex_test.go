package codex_test

import (
	"context"
	"testing"

	"github.com/shivamstaq/github-symphony/internal/adapter"
	"github.com/shivamstaq/github-symphony/internal/adapter/codex"
)

const mockCodexServer = `
while IFS= read -r line; do
  method=$(echo "$line" | python3 -c "import sys,json; print(json.loads(sys.stdin.read())['method'])" 2>/dev/null || echo "unknown")
  id=$(echo "$line" | python3 -c "import sys,json; print(json.loads(sys.stdin.read())['id'])" 2>/dev/null || echo "0")

  case "$method" in
    initialize)
      echo "{\"id\":${id},\"result\":{\"protocolVersion\":1,\"provider\":\"codex\",\"capabilities\":{\"sessionReuse\":true,\"tokenUsage\":true}}}"
      ;;
    session/new)
      echo "{\"id\":${id},\"result\":{\"sessionId\":\"cx_sess_1\"}}"
      ;;
    session/prompt)
      echo "{\"id\":${id},\"result\":{\"stopReason\":\"completed\",\"summary\":\"Codex done.\"}}"
      ;;
    session/close)
      echo "{\"id\":${id},\"result\":{\"closed\":true}}"
      ;;
    *)
      echo "{\"id\":${id},\"error\":{\"code\":-1,\"message\":\"unknown\"}}"
      ;;
  esac
done
`

func TestCodexAdapter_Lifecycle(t *testing.T) {
	a, err := codex.New(codex.Config{
		Command: "bash",
		Args:    []string{"-c", mockCodexServer},
		Cwd:     t.TempDir(),
	})
	if err != nil {
		t.Fatal(err)
	}
	defer a.Close()

	ctx := context.Background()

	caps, err := a.Initialize(ctx)
	if err != nil {
		t.Fatalf("Initialize: %v", err)
	}
	if caps.Provider != "codex" {
		t.Errorf("expected provider=codex, got %q", caps.Provider)
	}

	sessionID, err := a.NewSession(ctx, codex.SessionParams{Cwd: t.TempDir(), Title: "Test"})
	if err != nil {
		t.Fatalf("NewSession: %v", err)
	}
	if sessionID != "cx_sess_1" {
		t.Errorf("expected cx_sess_1, got %q", sessionID)
	}

	result, err := a.Prompt(ctx, sessionID, "Fix it")
	if err != nil {
		t.Fatalf("Prompt: %v", err)
	}
	if result.StopReason != adapter.StopCompleted {
		t.Errorf("expected completed, got %q", result.StopReason)
	}
}
