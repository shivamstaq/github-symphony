package webhook_test

import (
	"bytes"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"net/http/httptest"
	"testing"

	"github.com/shivamstaq/github-symphony/internal/webhook"
)

func TestWebhookHandler_ValidSignature(t *testing.T) {
	secret := "test_secret_123"
	var refreshed bool

	handler := webhook.NewHandler(secret, func(eventType string, payload []byte) {
		refreshed = true
		if eventType != "issues" {
			t.Errorf("expected event type 'issues', got %q", eventType)
		}
	})

	body := []byte(`{"action":"opened","issue":{"number":1}}`)
	sig := computeSignature(secret, body)

	req := httptest.NewRequest("POST", "/api/v1/webhooks/github", bytes.NewReader(body))
	req.Header.Set("X-GitHub-Event", "issues")
	req.Header.Set("X-Hub-Signature-256", "sha256="+sig)
	req.Header.Set("Content-Type", "application/json")

	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code != 200 {
		t.Errorf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	if !refreshed {
		t.Error("expected callback to be invoked")
	}
}

func TestWebhookHandler_InvalidSignature(t *testing.T) {
	handler := webhook.NewHandler("real_secret", func(string, []byte) {
		t.Error("callback should not be invoked for invalid signature")
	})

	body := []byte(`{"action":"opened"}`)
	req := httptest.NewRequest("POST", "/", bytes.NewReader(body))
	req.Header.Set("X-GitHub-Event", "issues")
	req.Header.Set("X-Hub-Signature-256", "sha256=invalid")

	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code != 401 {
		t.Errorf("expected 401, got %d", w.Code)
	}
}

func TestWebhookHandler_MissingSignature(t *testing.T) {
	handler := webhook.NewHandler("secret", func(string, []byte) {
		t.Error("should not be called")
	})

	req := httptest.NewRequest("POST", "/", bytes.NewReader([]byte(`{}`)))
	req.Header.Set("X-GitHub-Event", "issues")
	// No signature header

	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code != 401 {
		t.Errorf("expected 401, got %d", w.Code)
	}
}

func computeSignature(secret string, body []byte) string {
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(body)
	return hex.EncodeToString(mac.Sum(nil))
}
