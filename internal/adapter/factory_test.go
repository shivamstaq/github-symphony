package adapter_test

import (
	"context"
	"testing"

	"github.com/shivamstaq/github-symphony/internal/adapter"
)

const factoryMockServer = `
while IFS= read -r line; do
  id=$(echo "$line" | python3 -c "import sys,json; print(json.loads(sys.stdin.read()).get('id',0))" 2>/dev/null || echo "0")
  method=$(echo "$line" | python3 -c "import sys,json; print(json.loads(sys.stdin.read()).get('method',''))" 2>/dev/null || echo "")
  case "$method" in
    initialize) echo "{\"id\":${id},\"result\":{\"protocolVersion\":1,\"provider\":\"test\",\"capabilities\":{}}}" ;;
    session/new) echo "{\"id\":${id},\"result\":{\"sessionId\":\"s1\"}}" ;;
    session/prompt) echo "{\"id\":${id},\"result\":{\"stopReason\":\"completed\",\"summary\":\"done\"}}" ;;
    session/cancel) echo "{\"id\":${id},\"result\":{\"cancelled\":true}}" ;;
    session/close) echo "{\"id\":${id},\"result\":{\"closed\":true}}" ;;
    *) echo "{\"id\":${id},\"error\":{\"code\":-1,\"message\":\"unknown\"}}" ;;
  esac
done
`

func TestFactory_CreatesClaudeAdapter(t *testing.T) {
	a, err := adapter.NewAdapter(adapter.AdapterConfig{
		Kind:    "claude_code",
		Command: "bash",
		Args:    []string{"-c", factoryMockServer},
	})
	if err != nil {
		t.Fatal(err)
	}
	defer a.Close()

	result, err := a.Initialize(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if result.Provider != "test" {
		t.Errorf("expected provider=test, got %q", result.Provider)
	}
}

func TestFactory_CreatesOpenCodeAdapter(t *testing.T) {
	a, err := adapter.NewAdapter(adapter.AdapterConfig{
		Kind:    "opencode",
		Command: "bash",
		Args:    []string{"-c", factoryMockServer},
	})
	if err != nil {
		t.Fatal(err)
	}
	defer a.Close()

	_, err = a.Initialize(context.Background())
	if err != nil {
		t.Fatal(err)
	}
}

func TestFactory_CreatesCodexAdapter(t *testing.T) {
	a, err := adapter.NewAdapter(adapter.AdapterConfig{
		Kind:    "codex",
		Command: "bash",
		Args:    []string{"-c", factoryMockServer},
	})
	if err != nil {
		t.Fatal(err)
	}
	defer a.Close()

	_, err = a.Initialize(context.Background())
	if err != nil {
		t.Fatal(err)
	}
}

func TestFactory_UnsupportedKind(t *testing.T) {
	_, err := adapter.NewAdapter(adapter.AdapterConfig{Kind: "unknown_agent"})
	if err == nil {
		t.Fatal("expected error for unsupported kind")
	}
}

func TestFactory_InvalidCWD(t *testing.T) {
	_, err := adapter.NewAdapter(adapter.AdapterConfig{
		Kind:    "claude_code",
		Command: "bash",
		Args:    []string{"-c", "echo hi"},
		Cwd:     "/nonexistent/path/that/does/not/exist",
	})
	if err == nil {
		t.Fatal("expected error for invalid CWD")
	}
}

func TestFactory_FullLifecycle(t *testing.T) {
	a, err := adapter.NewAdapter(adapter.AdapterConfig{
		Kind:    "claude_code",
		Command: "bash",
		Args:    []string{"-c", factoryMockServer},
	})
	if err != nil {
		t.Fatal(err)
	}
	defer a.Close()

	ctx := context.Background()

	if _, err := a.Initialize(ctx); err != nil {
		t.Fatal("Initialize:", err)
	}
	sid, err := a.NewSession(ctx, adapter.SessionParams{Cwd: "/tmp", Title: "test"})
	if err != nil {
		t.Fatal("NewSession:", err)
	}
	if sid != "s1" {
		t.Errorf("expected session s1, got %q", sid)
	}

	result, err := a.Prompt(ctx, sid, "do work")
	if err != nil {
		t.Fatal("Prompt:", err)
	}
	if result.StopReason != adapter.StopCompleted {
		t.Errorf("expected completed, got %q", result.StopReason)
	}

	if err := a.Cancel(ctx, sid); err != nil {
		t.Fatal("Cancel:", err)
	}
	if err := a.CloseSession(ctx, sid); err != nil {
		t.Fatal("CloseSession:", err)
	}
}
