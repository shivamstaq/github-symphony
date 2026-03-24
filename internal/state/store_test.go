package state_test

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/shivamstaq/github-symphony/internal/state"
)

func TestStore_SaveAndLoadRetries(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "symphony.db")

	store, err := state.Open(dbPath)
	if err != nil {
		t.Fatalf("failed to open store: %v", err)
	}

	// Save retry entries
	entries := []state.RetryRecord{
		{WorkItemID: "item1", Attempt: 2, DueAtMs: time.Now().Add(5 * time.Second).UnixMilli(), Error: "timeout"},
		{WorkItemID: "item2", Attempt: 1, DueAtMs: time.Now().Add(1 * time.Second).UnixMilli(), Error: ""},
	}
	for _, e := range entries {
		if err := store.SaveRetry(e); err != nil {
			t.Fatalf("SaveRetry failed: %v", err)
		}
	}

	store.Close()

	// Reopen and load
	store2, err := state.Open(dbPath)
	if err != nil {
		t.Fatalf("failed to reopen store: %v", err)
	}
	defer store2.Close()

	loaded, err := store2.LoadRetries()
	if err != nil {
		t.Fatalf("LoadRetries failed: %v", err)
	}

	if len(loaded) != 2 {
		t.Fatalf("expected 2 entries, got %d", len(loaded))
	}

	// Verify data
	found := make(map[string]state.RetryRecord)
	for _, r := range loaded {
		found[r.WorkItemID] = r
	}

	if found["item1"].Attempt != 2 {
		t.Errorf("item1 attempt: expected 2, got %d", found["item1"].Attempt)
	}
	if found["item1"].Error != "timeout" {
		t.Errorf("item1 error: expected 'timeout', got %q", found["item1"].Error)
	}
}

func TestStore_DeleteRetry(t *testing.T) {
	dir := t.TempDir()
	store, err := state.Open(filepath.Join(dir, "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	store.SaveRetry(state.RetryRecord{WorkItemID: "item1", Attempt: 1})
	store.SaveRetry(state.RetryRecord{WorkItemID: "item2", Attempt: 1})

	if err := store.DeleteRetry("item1"); err != nil {
		t.Fatalf("DeleteRetry failed: %v", err)
	}

	loaded, _ := store.LoadRetries()
	if len(loaded) != 1 {
		t.Fatalf("expected 1 entry after delete, got %d", len(loaded))
	}
	if loaded[0].WorkItemID != "item2" {
		t.Errorf("expected item2, got %s", loaded[0].WorkItemID)
	}
}

func TestStore_SaveTotals(t *testing.T) {
	dir := t.TempDir()
	store, err := state.Open(filepath.Join(dir, "test.db"))
	if err != nil {
		t.Fatal(err)
	}

	totals := state.AgentTotalsRecord{
		InputTokens:  1000,
		OutputTokens: 500,
		TotalTokens:  1500,
		SessionsStarted: 5,
	}
	if err := store.SaveTotals(totals); err != nil {
		t.Fatal(err)
	}
	store.Close()

	store2, _ := state.Open(filepath.Join(dir, "test.db"))
	defer store2.Close()

	loaded, err := store2.LoadTotals()
	if err != nil {
		t.Fatal(err)
	}
	if loaded.InputTokens != 1000 {
		t.Errorf("expected InputTokens=1000, got %d", loaded.InputTokens)
	}
	if loaded.SessionsStarted != 5 {
		t.Errorf("expected SessionsStarted=5, got %d", loaded.SessionsStarted)
	}
}
