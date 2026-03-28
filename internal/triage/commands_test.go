package triage_test

import (
	"context"
	"testing"

	"github.com/vexil-lang/vexilbot/internal/triage"
)

type mockIssueAPI struct {
	addedLabels   []string
	removedLabels []string
	assignees     []string
	closed        bool
	reopened      bool
	reactions     []string
	comments      []string
}

func (m *mockIssueAPI) AddLabels(ctx context.Context, owner, repo string, number int, labels []string) error {
	m.addedLabels = append(m.addedLabels, labels...)
	return nil
}

func (m *mockIssueAPI) RemoveLabel(ctx context.Context, owner, repo string, number int, label string) error {
	m.removedLabels = append(m.removedLabels, label)
	return nil
}

func (m *mockIssueAPI) AddAssignees(ctx context.Context, owner, repo string, number int, assignees []string) error {
	m.assignees = append(m.assignees, assignees...)
	return nil
}

func (m *mockIssueAPI) CloseIssue(ctx context.Context, owner, repo string, number int) error {
	m.closed = true
	return nil
}

func (m *mockIssueAPI) ReopenIssue(ctx context.Context, owner, repo string, number int) error {
	m.reopened = true
	return nil
}

func (m *mockIssueAPI) AddReaction(ctx context.Context, owner, repo string, commentID int64, reaction string) error {
	m.reactions = append(m.reactions, reaction)
	return nil
}

func (m *mockIssueAPI) CreateComment(ctx context.Context, owner, repo string, number int, body string) error {
	m.comments = append(m.comments, body)
	return nil
}

func TestExecute_Label(t *testing.T) {
	api := &mockIssueAPI{}
	cmd := triage.Command{Name: "label", Args: []string{"bug", "enhancement"}}
	err := triage.Execute(context.Background(), api, cmd, "org", "repo", 1, 100)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(api.addedLabels) != 2 || api.addedLabels[0] != "bug" {
		t.Errorf("added labels = %v", api.addedLabels)
	}
	if len(api.reactions) != 1 || api.reactions[0] != "+1" {
		t.Errorf("reactions = %v", api.reactions)
	}
}

func TestExecute_Unlabel(t *testing.T) {
	api := &mockIssueAPI{}
	cmd := triage.Command{Name: "unlabel", Args: []string{"wontfix"}}
	err := triage.Execute(context.Background(), api, cmd, "org", "repo", 1, 100)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(api.removedLabels) != 1 || api.removedLabels[0] != "wontfix" {
		t.Errorf("removed labels = %v", api.removedLabels)
	}
}

func TestExecute_Assign(t *testing.T) {
	api := &mockIssueAPI{}
	cmd := triage.Command{Name: "assign", Args: []string{"alice"}}
	err := triage.Execute(context.Background(), api, cmd, "org", "repo", 1, 100)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(api.assignees) != 1 || api.assignees[0] != "alice" {
		t.Errorf("assignees = %v", api.assignees)
	}
}

func TestExecute_Prioritize(t *testing.T) {
	api := &mockIssueAPI{}
	cmd := triage.Command{Name: "prioritize", Args: []string{"p0"}}
	err := triage.Execute(context.Background(), api, cmd, "org", "repo", 1, 100)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	wantRemoved := map[string]bool{"p1": true, "p2": true, "p3": true}
	for _, l := range api.removedLabels {
		if !wantRemoved[l] {
			t.Errorf("unexpected removed label %q", l)
		}
	}
	if len(api.addedLabels) != 1 || api.addedLabels[0] != "p0" {
		t.Errorf("added labels = %v", api.addedLabels)
	}
}

func TestExecute_Close(t *testing.T) {
	api := &mockIssueAPI{}
	cmd := triage.Command{Name: "close", Args: nil}
	err := triage.Execute(context.Background(), api, cmd, "org", "repo", 1, 100)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !api.closed {
		t.Error("issue should be closed")
	}
}

func TestExecute_Reopen(t *testing.T) {
	api := &mockIssueAPI{}
	cmd := triage.Command{Name: "reopen", Args: nil}
	err := triage.Execute(context.Background(), api, cmd, "org", "repo", 1, 100)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !api.reopened {
		t.Error("issue should be reopened")
	}
}

func TestExecute_UnknownCommand(t *testing.T) {
	api := &mockIssueAPI{}
	cmd := triage.Command{Name: "explode", Args: nil}
	err := triage.Execute(context.Background(), api, cmd, "org", "repo", 1, 100)
	if err == nil {
		t.Fatal("expected error for unknown command")
	}
}

func TestExecute_LabelNoArgs(t *testing.T) {
	api := &mockIssueAPI{}
	cmd := triage.Command{Name: "label", Args: nil}
	err := triage.Execute(context.Background(), api, cmd, "org", "repo", 1, 100)
	if err == nil {
		t.Fatal("expected error for label with no args")
	}
}
