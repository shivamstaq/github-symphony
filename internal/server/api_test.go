package server

import (
	"bytes"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/shivamstaq/github-symphony/internal/engine"
)

// mockEngine implements EngineAPI for testing.
type mockEngine struct {
	events []engine.EngineEvent
}

func (m *mockEngine) GetState() *engine.State {
	return engine.NewState()
}

func (m *mockEngine) Emit(evt engine.EngineEvent) {
	m.events = append(m.events, evt)
}

func TestWebhook_ValidSignature(t *testing.T) {
	secret := "test-secret"
	eng := &mockEngine{}
	srv := NewAPIServer(eng, APIServerConfig{WebhookSecret: secret})

	body := []byte(`{"action":"opened"}`)
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(body)
	sig := "sha256=" + hex.EncodeToString(mac.Sum(nil))

	req := httptest.NewRequest("POST", "/api/v1/webhooks/github", bytes.NewReader(body))
	req.Header.Set("X-Hub-Signature-256", sig)
	w := httptest.NewRecorder()

	srv.Handler().ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	if len(eng.events) != 1 {
		t.Errorf("expected 1 event emitted, got %d", len(eng.events))
	}
	if eng.events[0].Type != engine.EvtPollTick {
		t.Errorf("expected poll tick event, got %s", eng.events[0].Type)
	}
}

func TestWebhook_InvalidSignature(t *testing.T) {
	eng := &mockEngine{}
	srv := NewAPIServer(eng, APIServerConfig{WebhookSecret: "real-secret"})

	body := []byte(`{"action":"opened"}`)
	req := httptest.NewRequest("POST", "/api/v1/webhooks/github", bytes.NewReader(body))
	req.Header.Set("X-Hub-Signature-256", "sha256=badhex")
	w := httptest.NewRecorder()

	srv.Handler().ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", w.Code)
	}
	if len(eng.events) != 0 {
		t.Errorf("expected no events emitted, got %d", len(eng.events))
	}
}

func TestWebhook_MissingSignature(t *testing.T) {
	eng := &mockEngine{}
	srv := NewAPIServer(eng, APIServerConfig{WebhookSecret: "secret"})

	body := []byte(`{"action":"opened"}`)
	req := httptest.NewRequest("POST", "/api/v1/webhooks/github", bytes.NewReader(body))
	w := httptest.NewRecorder()

	srv.Handler().ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", w.Code)
	}
}

func TestWebhook_NoSecretConfigured(t *testing.T) {
	eng := &mockEngine{}
	srv := NewAPIServer(eng) // no webhook secret

	body := []byte(`{"action":"opened"}`)
	req := httptest.NewRequest("POST", "/api/v1/webhooks/github", bytes.NewReader(body))
	w := httptest.NewRecorder()

	srv.Handler().ServeHTTP(w, req)

	// Without secret configured, webhook should be accepted without signature
	if w.Code != http.StatusOK {
		t.Errorf("expected 200 (no secret = no verification), got %d", w.Code)
	}
	if len(eng.events) != 1 {
		t.Errorf("expected 1 event emitted, got %d", len(eng.events))
	}
}

func TestHealthz(t *testing.T) {
	eng := &mockEngine{}
	srv := NewAPIServer(eng)

	req := httptest.NewRequest("GET", "/healthz", nil)
	w := httptest.NewRecorder()

	srv.Handler().ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}
}

func TestRefresh(t *testing.T) {
	eng := &mockEngine{}
	srv := NewAPIServer(eng)

	req := httptest.NewRequest("POST", "/api/v1/refresh", nil)
	w := httptest.NewRecorder()

	srv.Handler().ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}
	if len(eng.events) != 1 || eng.events[0].Type != engine.EvtPollTick {
		t.Errorf("expected poll tick event")
	}
}
