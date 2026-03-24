package server

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/shivamstaq/github-symphony/internal/orchestrator"
)

// StateProvider is the interface the server needs from the orchestrator.
type StateProvider interface {
	GetState() orchestrator.State
	IsHealthy() bool
	AuthMode() string
	TriggerRefresh()
	StartedAt() time.Time
}

// Config for the HTTP server.
type Config struct {
	Port           int
	Host           string
	ReadTimeoutMs  int
	WriteTimeoutMs int
}

// Server is the HTTP API server.
type Server struct {
	cfg      Config
	provider StateProvider
	router   chi.Router
}

// New creates a new HTTP server.
func New(cfg Config, provider StateProvider) *Server {
	s := &Server{
		cfg:      cfg,
		provider: provider,
	}
	s.buildRouter()
	return s
}

// Handler returns the HTTP handler for testing.
func (s *Server) Handler() http.Handler {
	return s.router
}

// ListenAndServe starts the HTTP server.
func (s *Server) ListenAndServe() error {
	addr := fmt.Sprintf("%s:%d", s.cfg.Host, s.cfg.Port)
	srv := &http.Server{
		Addr:         addr,
		Handler:      s.router,
		ReadTimeout:  time.Duration(s.cfg.ReadTimeoutMs) * time.Millisecond,
		WriteTimeout: time.Duration(s.cfg.WriteTimeoutMs) * time.Millisecond,
	}
	return srv.ListenAndServe()
}

func (s *Server) buildRouter() {
	r := chi.NewRouter()
	r.Use(middleware.Recoverer)
	r.Use(middleware.RealIP)

	r.Get("/healthz", s.handleHealthz)
	r.Get("/metrics", s.handleMetrics)

	r.Route("/api/v1", func(r chi.Router) {
		r.Get("/state", s.handleState)
		r.Get("/work-items/{id}", s.handleWorkItem)
		r.Post("/refresh", s.handleRefresh)
	})

	s.router = r
}

// MountWebhook adds a webhook handler at /api/v1/webhooks/github.
func (s *Server) MountWebhook(handler http.Handler) {
	s.router.Post("/api/v1/webhooks/github", handler.ServeHTTP)
}

func (s *Server) handleHealthz(w http.ResponseWriter, _ *http.Request) {
	if !s.provider.IsHealthy() {
		w.WriteHeader(503)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"status": "unhealthy",
		})
		return
	}

	state := s.provider.GetState()
	uptime := time.Since(s.provider.StartedAt()).Seconds()

	resp := map[string]any{
		"status":         "ok",
		"uptime_seconds": int(uptime),
		"auth_mode":      s.provider.AuthMode(),
		"running_count":  len(state.Running),
	}
	if state.LastPollAt != nil {
		resp["last_poll_at"] = state.LastPollAt.Format(time.RFC3339)
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(resp)
}

func (s *Server) handleState(w http.ResponseWriter, _ *http.Request) {
	state := s.provider.GetState()

	running := make(map[string]any, len(state.Running))
	for id, entry := range state.Running {
		running[id] = map[string]any{
			"work_item_id": entry.WorkItem.WorkItemID,
			"title":        entry.WorkItem.Title,
			"repository":   entry.Repository,
			"started_at":   entry.StartedAt.Format(time.RFC3339),
			"input_tokens": entry.InputTokens,
			"output_tokens": entry.OutputTokens,
		}
	}

	retrying := make(map[string]any, len(state.RetryAttempts))
	for id, entry := range state.RetryAttempts {
		retrying[id] = map[string]any{
			"attempt": entry.Attempt,
			"due_at":  entry.DueAt.Format(time.RFC3339),
			"error":   entry.Error,
		}
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{
		"running":              running,
		"retrying":             retrying,
		"max_concurrent_agents": state.MaxConcurrentAgents,
		"agent_totals": map[string]any{
			"input_tokens":      state.AgentTotals.InputTokens,
			"output_tokens":     state.AgentTotals.OutputTokens,
			"total_tokens":      state.AgentTotals.TotalTokens,
			"seconds_running":   state.AgentTotals.SecondsRunning,
			"github_writebacks": state.AgentTotals.GitHubWritebacks,
			"sessions_started":  state.AgentTotals.SessionsStarted,
		},
		"pending_refresh": state.PendingRefresh,
	})
}

func (s *Server) handleWorkItem(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	state := s.provider.GetState()

	if entry, ok := state.Running[id]; ok {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"work_item_id": entry.WorkItem.WorkItemID,
			"title":        entry.WorkItem.Title,
			"state":        "running",
			"started_at":   entry.StartedAt.Format(time.RFC3339),
		})
		return
	}

	w.WriteHeader(404)
	_ = json.NewEncoder(w).Encode(map[string]any{"error": "work item not found"})
}

func (s *Server) handleRefresh(w http.ResponseWriter, _ *http.Request) {
	s.provider.TriggerRefresh()
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{"ok": true})
}

func (s *Server) handleMetrics(w http.ResponseWriter, _ *http.Request) {
	state := s.provider.GetState()
	w.Header().Set("Content-Type", "text/plain; version=0.0.4")

	var sb strings.Builder

	// Gauges
	fmt.Fprintf(&sb, "# HELP symphony_active_runs Current running work items\n")
	fmt.Fprintf(&sb, "# TYPE symphony_active_runs gauge\n")
	fmt.Fprintf(&sb, "symphony_active_runs %d\n", len(state.Running))

	fmt.Fprintf(&sb, "# HELP symphony_max_concurrent_agents Configured concurrency limit\n")
	fmt.Fprintf(&sb, "# TYPE symphony_max_concurrent_agents gauge\n")
	fmt.Fprintf(&sb, "symphony_max_concurrent_agents %d\n", state.MaxConcurrentAgents)

	fmt.Fprintf(&sb, "# HELP symphony_retry_queue_depth Current retry queue size\n")
	fmt.Fprintf(&sb, "# TYPE symphony_retry_queue_depth gauge\n")
	fmt.Fprintf(&sb, "symphony_retry_queue_depth %d\n", len(state.RetryAttempts))

	// Counters
	fmt.Fprintf(&sb, "# HELP symphony_tokens_total Cumulative token usage\n")
	fmt.Fprintf(&sb, "# TYPE symphony_tokens_total counter\n")
	fmt.Fprintf(&sb, "symphony_tokens_total{direction=\"input\"} %d\n", state.AgentTotals.InputTokens)
	fmt.Fprintf(&sb, "symphony_tokens_total{direction=\"output\"} %d\n", state.AgentTotals.OutputTokens)
	fmt.Fprintf(&sb, "symphony_tokens_total{direction=\"total\"} %d\n", state.AgentTotals.TotalTokens)

	fmt.Fprintf(&sb, "# HELP symphony_sessions_started_total Total sessions started\n")
	fmt.Fprintf(&sb, "# TYPE symphony_sessions_started_total counter\n")
	fmt.Fprintf(&sb, "symphony_sessions_started_total %d\n", state.AgentTotals.SessionsStarted)

	fmt.Fprintf(&sb, "# HELP symphony_github_writebacks_total Total write-back operations\n")
	fmt.Fprintf(&sb, "# TYPE symphony_github_writebacks_total counter\n")
	fmt.Fprintf(&sb, "symphony_github_writebacks_total %d\n", state.AgentTotals.GitHubWritebacks)

	// Dispatches total
	fmt.Fprintf(&sb, "# HELP symphony_dispatches_total Total dispatches\n")
	fmt.Fprintf(&sb, "# TYPE symphony_dispatches_total counter\n")
	fmt.Fprintf(&sb, "symphony_dispatches_total %d\n", state.DispatchTotal)

	// Work item state distribution
	fmt.Fprintf(&sb, "# HELP symphony_work_item_state Count of work items by orchestration state\n")
	fmt.Fprintf(&sb, "# TYPE symphony_work_item_state gauge\n")
	fmt.Fprintf(&sb, "symphony_work_item_state{state=\"running\"} %d\n", len(state.Running))
	fmt.Fprintf(&sb, "symphony_work_item_state{state=\"retry_queued\"} %d\n", len(state.RetryAttempts))
	handedOff := 0
	if state.HandedOff != nil {
		handedOff = len(state.HandedOff)
	}
	fmt.Fprintf(&sb, "symphony_work_item_state{state=\"handed_off\"} %d\n", handedOff)

	// Errors total
	fmt.Fprintf(&sb, "# HELP symphony_errors_total Total errors by category\n")
	fmt.Fprintf(&sb, "# TYPE symphony_errors_total counter\n")
	fmt.Fprintf(&sb, "symphony_errors_total %d\n", state.ErrorTotal)

	// PR handoffs total
	fmt.Fprintf(&sb, "# HELP symphony_pr_handoffs_total Total PR handoffs\n")
	fmt.Fprintf(&sb, "# TYPE symphony_pr_handoffs_total counter\n")
	fmt.Fprintf(&sb, "symphony_pr_handoffs_total %d\n", state.HandoffTotal)

	_, _ = w.Write([]byte(sb.String()))
}
