package webhook_test

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/vexil-lang/vexilbot/internal/webhook"
)

func sign(body, secret string) string {
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(body))
	return "sha256=" + hex.EncodeToString(mac.Sum(nil))
}

func TestHandler_ValidPullRequestEvent(t *testing.T) {
	var mu sync.Mutex
	var gotEvent string
	var gotPayload []byte

	router := webhook.RouterFunc(func(eventType string, payload []byte) {
		mu.Lock()
		defer mu.Unlock()
		gotEvent = eventType
		gotPayload = payload
	})

	h := webhook.NewHandler("test-secret", router)
	body := `{"action":"opened","number":1}`
	req := httptest.NewRequest(http.MethodPost, "/webhook", strings.NewReader(body))
	req.Header.Set("X-Hub-Signature-256", sign(body, "test-secret"))
	req.Header.Set("X-GitHub-Event", "pull_request")
	w := httptest.NewRecorder()

	h.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("status = %d, want %d", w.Code, http.StatusOK)
	}
	mu.Lock()
	defer mu.Unlock()
	if gotEvent != "pull_request" {
		t.Errorf("event = %q, want %q", gotEvent, "pull_request")
	}
	if string(gotPayload) != body {
		t.Errorf("payload = %q, want %q", string(gotPayload), body)
	}
}

func TestHandler_InvalidSignature(t *testing.T) {
	router := webhook.RouterFunc(func(eventType string, payload []byte) {
		t.Fatal("router should not be called on invalid signature")
	})

	h := webhook.NewHandler("test-secret", router)
	req := httptest.NewRequest(http.MethodPost, "/webhook", strings.NewReader("body"))
	req.Header.Set("X-Hub-Signature-256", "sha256=invalid")
	req.Header.Set("X-GitHub-Event", "push")
	w := httptest.NewRecorder()

	h.ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, want %d", w.Code, http.StatusUnauthorized)
	}
}

func TestHandler_MissingEventHeader(t *testing.T) {
	router := webhook.RouterFunc(func(eventType string, payload []byte) {
		t.Fatal("router should not be called without event header")
	})

	h := webhook.NewHandler("test-secret", router)
	body := `{}`
	req := httptest.NewRequest(http.MethodPost, "/webhook", strings.NewReader(body))
	req.Header.Set("X-Hub-Signature-256", sign(body, "test-secret"))
	w := httptest.NewRecorder()

	h.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want %d", w.Code, http.StatusBadRequest)
	}
}

func TestHandler_MethodNotAllowed(t *testing.T) {
	router := webhook.RouterFunc(func(eventType string, payload []byte) {})
	h := webhook.NewHandler("test-secret", router)
	req := httptest.NewRequest(http.MethodGet, "/webhook", nil)
	w := httptest.NewRecorder()

	h.ServeHTTP(w, req)

	if w.Code != http.StatusMethodNotAllowed {
		t.Errorf("status = %d, want %d", w.Code, http.StatusMethodNotAllowed)
	}
}
