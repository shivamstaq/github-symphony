package server_test

import (
	"encoding/json"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/shivamstaq/github-symphony/internal/orchestrator"
	"github.com/shivamstaq/github-symphony/internal/server"
)

func TestHealthz_Healthy(t *testing.T) {
	srv := server.New(server.Config{}, &mockStateProvider{healthy: true})
	req := httptest.NewRequest("GET", "/healthz", nil)
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)

	if w.Code != 200 {
		t.Errorf("expected 200, got %d", w.Code)
	}

	var body map[string]any
	json.NewDecoder(w.Body).Decode(&body)
	if body["status"] != "ok" {
		t.Errorf("expected status=ok, got %v", body["status"])
	}
}

func TestHealthz_Unhealthy(t *testing.T) {
	srv := server.New(server.Config{}, &mockStateProvider{healthy: false})
	req := httptest.NewRequest("GET", "/healthz", nil)
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)

	if w.Code != 503 {
		t.Errorf("expected 503, got %d", w.Code)
	}
}

func TestAPIState(t *testing.T) {
	srv := server.New(server.Config{}, &mockStateProvider{
		healthy: true,
		state: orchestrator.State{
			MaxConcurrentAgents: 10,
			Running: map[string]*orchestrator.RunningEntry{
				"item1": {
					WorkItem: orchestrator.WorkItem{WorkItemID: "item1", Title: "Fix bug"},
					StartedAt: time.Now(),
				},
			},
		},
	})

	req := httptest.NewRequest("GET", "/api/v1/state", nil)
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)

	if w.Code != 200 {
		t.Errorf("expected 200, got %d", w.Code)
	}

	var body map[string]any
	json.NewDecoder(w.Body).Decode(&body)
	running, ok := body["running"].(map[string]any)
	if !ok {
		t.Fatalf("expected running map, got %T", body["running"])
	}
	if len(running) != 1 {
		t.Errorf("expected 1 running, got %d", len(running))
	}
}

func TestMetrics(t *testing.T) {
	srv := server.New(server.Config{}, &mockStateProvider{healthy: true})
	req := httptest.NewRequest("GET", "/metrics", nil)
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)

	if w.Code != 200 {
		t.Errorf("expected 200, got %d", w.Code)
	}

	body := w.Body.String()
	if len(body) == 0 {
		t.Error("expected non-empty metrics body")
	}
	// Should contain at least one metric
	if !containsStr(body, "symphony_") {
		t.Error("expected metrics with symphony_ prefix")
	}
}

func TestRefresh(t *testing.T) {
	provider := &mockStateProvider{healthy: true}
	srv := server.New(server.Config{}, provider)
	req := httptest.NewRequest("POST", "/api/v1/refresh", nil)
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)

	if w.Code != 200 {
		t.Errorf("expected 200, got %d", w.Code)
	}
	if !provider.refreshCalled {
		t.Error("expected refresh to be triggered")
	}
}

type mockStateProvider struct {
	healthy       bool
	state         orchestrator.State
	refreshCalled bool
}

func (m *mockStateProvider) GetState() orchestrator.State   { return m.state }
func (m *mockStateProvider) IsHealthy() bool                { return m.healthy }
func (m *mockStateProvider) AuthMode() string               { return "pat" }
func (m *mockStateProvider) TriggerRefresh()                { m.refreshCalled = true }
func (m *mockStateProvider) StartedAt() time.Time           { return time.Now().Add(-1 * time.Hour) }

func containsStr(s, substr string) bool {
	return len(s) > 0 && len(substr) > 0 && (s == substr || len(s) > len(substr) && findSubstr(s, substr))
}

func findSubstr(s, sub string) bool {
	for i := 0; i <= len(s)-len(sub); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
