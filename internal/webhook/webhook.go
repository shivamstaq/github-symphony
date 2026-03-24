package webhook

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"strings"
)

// EventCallback is called when a valid webhook event is received.
type EventCallback func(eventType string, payload []byte)

// Handler handles GitHub webhook deliveries.
type Handler struct {
	secret   string
	callback EventCallback
}

// NewHandler creates a webhook handler with signature verification.
func NewHandler(secret string, callback EventCallback) *Handler {
	return &Handler{
		secret:   secret,
		callback: callback,
	}
}

func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	body, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, "failed to read body", 400)
		return
	}

	// Verify signature
	sigHeader := r.Header.Get("X-Hub-Signature-256")
	if sigHeader == "" {
		slog.Warn("webhook: missing signature header")
		http.Error(w, "missing signature", 401)
		return
	}

	if !h.verifySignature(body, sigHeader) {
		slog.Warn("webhook: invalid signature")
		http.Error(w, "invalid signature", 401)
		return
	}

	eventType := r.Header.Get("X-GitHub-Event")
	deliveryID := r.Header.Get("X-GitHub-Delivery")

	slog.Info("webhook received",
		"event", eventType,
		"delivery_id", deliveryID,
		"size", len(body),
	)

	h.callback(eventType, body)

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]any{"ok": true})
}

func (h *Handler) verifySignature(body []byte, sigHeader string) bool {
	sig := strings.TrimPrefix(sigHeader, "sha256=")
	mac := hmac.New(sha256.New, []byte(h.secret))
	mac.Write(body)
	expected := hex.EncodeToString(mac.Sum(nil))
	return hmac.Equal([]byte(sig), []byte(expected))
}
