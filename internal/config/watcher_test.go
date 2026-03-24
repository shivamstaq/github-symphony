package config_test

import (
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	"github.com/shivamstaq/github-symphony/internal/config"
)

func TestWatcher_DetectsChange(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "WORKFLOW.md")

	initial := `---
tracker:
  kind: github
  owner: org1
  project_number: 1
agent:
  kind: claude_code
---
prompt v1
`
	os.WriteFile(path, []byte(initial), 0644)

	var callCount atomic.Int32

	w, err := config.NewWatcher(path, func(wf *config.WorkflowDefinition) {
		callCount.Add(1)
	})
	if err != nil {
		t.Fatalf("NewWatcher: %v", err)
	}
	defer w.Close()

	// Modify the file
	time.Sleep(100 * time.Millisecond)
	updated := `---
tracker:
  kind: github
  owner: org2
  project_number: 2
agent:
  kind: claude_code
---
prompt v2
`
	os.WriteFile(path, []byte(updated), 0644)

	// Wait for debounced callback
	time.Sleep(500 * time.Millisecond)

	if callCount.Load() < 1 {
		t.Error("expected watcher callback to be invoked at least once")
	}
}
