package internal_test

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/vexil-lang/vexilbot/internal/webhook"
)

func TestFullPipeline_PROpened(t *testing.T) {
	secret := "integration-test-secret"

	var mu sync.Mutex
	var receivedEvent *webhook.PullRequestEvent

	dispatcher := webhook.NewDispatcher()
	dispatcher.OnPullRequest(func(event webhook.PullRequestEvent) {
		mu.Lock()
		defer mu.Unlock()
		receivedEvent = &event
	})

	handler := webhook.NewHandler(secret, dispatcher)
	mux := http.NewServeMux()
	mux.Handle("POST /webhook", handler)

	payload, _ := json.Marshal(map[string]interface{}{
		"action": "opened",
		"number": 99,
		"pull_request": map[string]interface{}{
			"head": map[string]interface{}{"sha": "deadbeef"},
			"user": map[string]interface{}{"login": "testuser"},
		},
		"repository": map[string]interface{}{
			"owner": map[string]interface{}{"login": "vexil-lang"},
			"name":  "vexil",
		},
		"installation": map[string]interface{}{"id": 1},
	})

	body := string(payload)
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(body))
	sig := "sha256=" + hex.EncodeToString(mac.Sum(nil))

	req := httptest.NewRequest(http.MethodPost, "/webhook", strings.NewReader(body))
	req.Header.Set("X-Hub-Signature-256", sig)
	req.Header.Set("X-GitHub-Event", "pull_request")
	w := httptest.NewRecorder()

	mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
	}

	mu.Lock()
	defer mu.Unlock()
	if receivedEvent == nil {
		t.Fatal("pull_request handler was not called")
	}
	if receivedEvent.Number != 99 {
		t.Errorf("PR number = %d, want 99", receivedEvent.Number)
	}
	if receivedEvent.UserLogin != "testuser" {
		t.Errorf("user = %q, want %q", receivedEvent.UserLogin, "testuser")
	}
	if receivedEvent.Owner != "vexil-lang" {
		t.Errorf("owner = %q, want %q", receivedEvent.Owner, "vexil-lang")
	}
}
