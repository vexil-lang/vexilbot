package webhook_test

import (
	"encoding/json"
	"testing"

	"github.com/vexil-lang/vexilbot/internal/webhook"
)

func TestDispatcher_PullRequestOpened(t *testing.T) {
	var called bool
	d := webhook.NewDispatcher()
	d.OnPullRequest(func(event webhook.PullRequestEvent) {
		called = true
		if event.Action != "opened" {
			t.Errorf("action = %q, want %q", event.Action, "opened")
		}
		if event.Number != 42 {
			t.Errorf("number = %d, want %d", event.Number, 42)
		}
	})

	payload, _ := json.Marshal(map[string]interface{}{
		"action": "opened",
		"number": 42,
		"pull_request": map[string]interface{}{
			"head": map[string]interface{}{"sha": "abc123"},
			"user": map[string]interface{}{"login": "alice"},
		},
		"repository": map[string]interface{}{
			"owner": map[string]interface{}{"login": "vexil-lang"},
			"name":  "vexil",
		},
		"installation": map[string]interface{}{"id": 999},
	})

	d.Route("pull_request", payload)
	if !called {
		t.Error("pull_request handler was not called")
	}
}

func TestDispatcher_IssueComment(t *testing.T) {
	var called bool
	d := webhook.NewDispatcher()
	d.OnIssueComment(func(event webhook.IssueCommentEvent) {
		called = true
		if event.Action != "created" {
			t.Errorf("action = %q", event.Action)
		}
	})

	payload, _ := json.Marshal(map[string]interface{}{
		"action": "created",
		"comment": map[string]interface{}{
			"id":   123,
			"body": "@vexilbot label bug",
			"user": map[string]interface{}{"login": "bob"},
		},
		"issue": map[string]interface{}{"number": 10},
		"repository": map[string]interface{}{
			"owner": map[string]interface{}{"login": "vexil-lang"},
			"name":  "vexil",
		},
		"installation": map[string]interface{}{"id": 999},
	})

	d.Route("issue_comment", payload)
	if !called {
		t.Error("issue_comment handler was not called")
	}
}

func TestDispatcher_UnknownEvent(t *testing.T) {
	d := webhook.NewDispatcher()
	// Should not panic on unknown events
	d.Route("unknown_event", []byte(`{}`))
}
