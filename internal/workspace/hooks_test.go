package workspace_test

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"github.com/shivamstaq/github-symphony/internal/workspace"
)

func TestRunHook_Success(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("shell hooks not supported on windows")
	}

	dir := t.TempDir()
	marker := filepath.Join(dir, "hook_ran")

	err := workspace.RunHook(context.Background(), "test_hook", "touch "+marker, dir, 5*time.Second)
	if err != nil {
		t.Fatalf("hook failed: %v", err)
	}

	if _, err := os.Stat(marker); err != nil {
		t.Error("hook did not create marker file")
	}
}

func TestRunHook_Failure(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("shell hooks not supported on windows")
	}

	dir := t.TempDir()
	err := workspace.RunHook(context.Background(), "test_hook", "exit 1", dir, 5*time.Second)
	if err == nil {
		t.Fatal("expected error for failing hook")
	}
}

func TestRunHook_Timeout(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("shell hooks not supported on windows")
	}

	dir := t.TempDir()
	err := workspace.RunHook(context.Background(), "test_hook", "sleep 30", dir, 100*time.Millisecond)
	if err == nil {
		t.Fatal("expected error for timed-out hook")
	}
}

func TestRunHook_EmptyScript(t *testing.T) {
	// Empty script should be a no-op
	err := workspace.RunHook(context.Background(), "test_hook", "", t.TempDir(), 5*time.Second)
	if err != nil {
		t.Fatalf("expected no error for empty hook, got: %v", err)
	}
}
