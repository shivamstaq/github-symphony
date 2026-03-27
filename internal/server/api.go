package server

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/shivamstaq/github-symphony/internal/engine"
)

// EngineAPI provides the engine interface needed by the API server.
type EngineAPI interface {
	GetState() *engine.State
	Emit(engine.EngineEvent)
}

// APIServer serves the Symphony HTTP API.
type APIServer struct {
	engine        EngineAPI
	startedAt     time.Time
	router        *chi.Mux
	webhookSecret string // HMAC secret for GitHub webhook verification
}

// APIServerConfig configures the API server.
type APIServerConfig struct {
	WebhookSecret string // GitHub webhook secret for HMAC verification
}

// NewAPIServer creates the HTTP API server.
func NewAPIServer(eng EngineAPI, cfgs ...APIServerConfig) *APIServer {
	s := &APIServer{
		engine:    eng,
		startedAt: time.Now(),
		router:    chi.NewRouter(),
	}
	if len(cfgs) > 0 {
		s.webhookSecret = cfgs[0].WebhookSecret
	}
	s.routes()
	return s
}

func (s *APIServer) routes() {
	s.router.Get("/healthz", s.handleHealthz)
	s.router.Get("/metrics", s.handleMetrics)
	s.router.Get("/api/v1/state", s.handleGetState)
	s.router.Post("/api/v1/pause/{id}", s.handlePause)
	s.router.Post("/api/v1/resume/{id}", s.handleResume)
	s.router.Post("/api/v1/kill/{id}", s.handleKill)
	s.router.Post("/api/v1/refresh", s.handleRefresh)
	s.router.Post("/api/v1/webhooks/github", s.handleWebhook)
}

// Handler returns the HTTP handler.
func (s *APIServer) Handler() http.Handler {
	return s.router
}

func (s *APIServer) handleHealthz(w http.ResponseWriter, r *http.Request) {
	state := s.engine.GetState()
	uptime := time.Since(s.startedAt).Truncate(time.Second)
	writeJSON(w, map[string]any{
		"status":   "healthy",
		"uptime":   uptime.String(),
		"running":  state.RunningCount(),
		"last_poll": state.LastPollAt,
	})
}

func (s *APIServer) handleMetrics(w http.ResponseWriter, r *http.Request) {
	state := s.engine.GetState()
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")

	lines := []string{
		fmt.Sprintf("symphony_active_runs %d", state.RunningCount()),
		fmt.Sprintf("symphony_retry_queue_depth %d", len(state.RetryQueue)),
		fmt.Sprintf("symphony_dispatch_total %d", state.DispatchTotal),
		fmt.Sprintf("symphony_error_total %d", state.ErrorTotal),
		fmt.Sprintf("symphony_handoff_total %d", state.HandoffTotal),
		fmt.Sprintf("symphony_tokens_total{direction=\"input\"} %d", state.Totals.InputTokens),
		fmt.Sprintf("symphony_tokens_total{direction=\"output\"} %d", state.Totals.OutputTokens),
		fmt.Sprintf("symphony_tokens_total{direction=\"total\"} %d", state.Totals.TotalTokens),
		fmt.Sprintf("symphony_sessions_started_total %d", state.Totals.SessionsStarted),
		fmt.Sprintf("symphony_cost_usd_total %f", state.Totals.CostUSD),
	}
	fmt.Fprintln(w, strings.Join(lines, "\n"))
}

func (s *APIServer) handleGetState(w http.ResponseWriter, r *http.Request) {
	state := s.engine.GetState()

	running := make([]map[string]any, 0)
	for id, entry := range state.Running {
		running = append(running, map[string]any{
			"id":             id,
			"issue":          entry.WorkItem.IssueIdentifier,
			"phase":          entry.Phase,
			"paused":         entry.Paused,
			"tokens":         entry.TotalTokens,
			"cost_usd":       entry.CostUSD,
			"turns":          entry.TurnsCompleted,
			"started_at":     entry.StartedAt,
			"last_activity":  entry.LastActivityAt,
			"retry_attempt":  entry.RetryAttempt,
		})
	}

	retries := make([]map[string]any, 0)
	for _, re := range state.RetryQueue {
		retries = append(retries, map[string]any{
			"id":      re.WorkItemID,
			"issue":   re.IssueIdentifier,
			"attempt": re.Attempt,
			"due_at":  re.DueAt,
			"error":   re.Error,
		})
	}

	writeJSON(w, map[string]any{
		"running":        running,
		"retries":        retries,
		"handed_off":     len(state.HandedOff),
		"dispatch_total": state.DispatchTotal,
		"error_total":    state.ErrorTotal,
		"handoff_total":  state.HandoffTotal,
		"totals":         state.Totals,
	})
}

func (s *APIServer) handlePause(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	s.engine.Emit(engine.NewEvent(engine.EvtPauseRequested, id, nil))
	writeJSON(w, map[string]any{"status": "pause requested", "item": id})
}

func (s *APIServer) handleResume(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	s.engine.Emit(engine.NewEvent(engine.EvtResumeRequested, id, nil))
	writeJSON(w, map[string]any{"status": "resume requested", "item": id})
}

func (s *APIServer) handleKill(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	s.engine.Emit(engine.NewEvent(engine.EvtCancelRequested, id, nil))
	writeJSON(w, map[string]any{"status": "kill requested", "item": id})
}

func (s *APIServer) handleRefresh(w http.ResponseWriter, r *http.Request) {
	s.engine.Emit(engine.NewEvent(engine.EvtPollTick, "", engine.PollTickPayload{}))
	writeJSON(w, map[string]any{"status": "refresh triggered"})
}

func (s *APIServer) handleWebhook(w http.ResponseWriter, r *http.Request) {
	body, err := io.ReadAll(io.LimitReader(r.Body, 1<<20)) // 1MB max
	if err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}

	// Verify signature if webhook secret is configured
	if s.webhookSecret != "" {
		sig := r.Header.Get("X-Hub-Signature-256")
		if sig == "" {
			http.Error(w, "missing signature", http.StatusUnauthorized)
			return
		}
		if !verifyHMAC(body, sig, s.webhookSecret) {
			http.Error(w, "invalid signature", http.StatusUnauthorized)
			return
		}
	}

	// Trigger a poll tick — the engine's buffered channel provides natural coalescing:
	// if the channel is full, the event is dropped (duplicate poll suppressed).
	s.engine.Emit(engine.NewEvent(engine.EvtPollTick, "", engine.PollTickPayload{}))
	writeJSON(w, map[string]any{"status": "webhook received"})
}

// verifyHMAC checks the GitHub webhook HMAC-SHA256 signature.
func verifyHMAC(body []byte, signature, secret string) bool {
	// GitHub sends "sha256=<hex>"
	prefix := "sha256="
	if !strings.HasPrefix(signature, prefix) {
		return false
	}
	sigHex := signature[len(prefix):]
	sigBytes, err := hex.DecodeString(sigHex)
	if err != nil {
		return false
	}

	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(body)
	expected := mac.Sum(nil)
	return hmac.Equal(sigBytes, expected)
}

func writeJSON(w http.ResponseWriter, data any) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(data)
}
