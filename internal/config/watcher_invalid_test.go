package config_test

import (
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	"github.com/shivamstaq/github-symphony/internal/config"
)

func TestWatcher_InvalidReloadKeepsGoodConfig(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "WORKFLOW.md")

	// Start with valid config
	valid := "---\ntracker:\n  kind: github\n---\nprompt v1\n"
	os.WriteFile(path, []byte(valid), 0644)

	var callCount atomic.Int32

	w, err := config.NewWatcher(path, func(wf *config.WorkflowDefinition) {
		callCount.Add(1)
	})
	if err != nil {
		t.Fatal(err)
	}
	defer w.Close()

	time.Sleep(100 * time.Millisecond)

	// Write invalid YAML — should NOT trigger callback
	invalid := "---\nthis is: [broken: yaml\n---\nprompt\n"
	os.WriteFile(path, []byte(invalid), 0644)

	time.Sleep(500 * time.Millisecond)

	if callCount.Load() != 0 {
		t.Errorf("expected callback NOT to be invoked for invalid reload, but was called %d times", callCount.Load())
	}

	// Now write valid YAML again — SHOULD trigger callback
	valid2 := "---\ntracker:\n  kind: github\n  owner: org2\n---\nprompt v2\n"
	os.WriteFile(path, []byte(valid2), 0644)

	time.Sleep(500 * time.Millisecond)

	if callCount.Load() < 1 {
		t.Error("expected callback to be invoked after valid reload")
	}
}
