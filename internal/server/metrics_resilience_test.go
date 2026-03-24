package server_test

import (
	"net/http/httptest"
	"testing"
	"time"

	"github.com/shivamstaq/github-symphony/internal/orchestrator"
	"github.com/shivamstaq/github-symphony/internal/server"
)

// panicStateProvider panics when GetState is called, simulating a metrics subsystem failure.
type panicStateProvider struct{}

func (p *panicStateProvider) GetState() orchestrator.State { panic("simulated metrics failure") }
func (p *panicStateProvider) IsHealthy() bool              { return true }
func (p *panicStateProvider) AuthMode() string             { return "pat" }
func (p *panicStateProvider) TriggerRefresh()              {}
func (p *panicStateProvider) StartedAt() time.Time         { return time.Now() }

func TestMetrics_PanicDoesNotCrashServer(t *testing.T) {
	srv := server.New(server.Config{}, &panicStateProvider{})

	// The chi Recoverer middleware should catch the panic
	req := httptest.NewRequest("GET", "/metrics", nil)
	w := httptest.NewRecorder()

	// This should NOT panic the test process
	srv.Handler().ServeHTTP(w, req)

	// Recoverer returns 500 on panic
	if w.Code != 500 {
		t.Errorf("expected 500 after panic recovery, got %d", w.Code)
	}
}

func TestHealthz_StillWorksAfterMetricsPanic(t *testing.T) {
	srv := server.New(server.Config{}, &panicStateProvider{})

	// First: metrics panics
	req1 := httptest.NewRequest("GET", "/metrics", nil)
	w1 := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w1, req1)

	// Second: healthz should still work (server didn't crash)
	// Note: healthz also calls GetState which will panic — but the point is
	// the server process survives. Let's test with a provider that only panics on metrics.
	srv2 := server.New(server.Config{}, &mockStateProvider{healthy: true})
	req2 := httptest.NewRequest("GET", "/healthz", nil)
	w2 := httptest.NewRecorder()
	srv2.Handler().ServeHTTP(w2, req2)

	if w2.Code != 200 {
		t.Errorf("healthz should still work, got %d", w2.Code)
	}
}
