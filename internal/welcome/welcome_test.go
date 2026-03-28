package welcome_test

import (
	"context"
	"testing"

	"github.com/vexil-lang/vexilbot/internal/welcome"
)

type mockContribAPI struct {
	prCount    int
	issueCount int
	comments   []struct {
		number int
		body   string
	}
}

func (m *mockContribAPI) CountUserPRs(ctx context.Context, owner, repo, user string) (int, error) {
	return m.prCount, nil
}

func (m *mockContribAPI) CountUserIssues(ctx context.Context, owner, repo, user string) (int, error) {
	return m.issueCount, nil
}

func (m *mockContribAPI) CreateComment(ctx context.Context, owner, repo string, number int, body string) error {
	m.comments = append(m.comments, struct {
		number int
		body   string
	}{number, body})
	return nil
}

func TestWelcomePR_FirstTime(t *testing.T) {
	api := &mockContribAPI{prCount: 0, issueCount: 0}
	err := welcome.MaybeWelcomePR(context.Background(), api, "org", "repo", "alice", 1, "Welcome!")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(api.comments) != 1 {
		t.Fatalf("got %d comments, want 1", len(api.comments))
	}
	if api.comments[0].body != "Welcome!" {
		t.Errorf("comment = %q, want %q", api.comments[0].body, "Welcome!")
	}
}

func TestWelcomePR_ReturningContributor(t *testing.T) {
	api := &mockContribAPI{prCount: 3, issueCount: 0}
	err := welcome.MaybeWelcomePR(context.Background(), api, "org", "repo", "bob", 5, "Welcome!")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(api.comments) != 0 {
		t.Errorf("got %d comments, want 0 for returning contributor", len(api.comments))
	}
}

func TestWelcomeIssue_FirstTime(t *testing.T) {
	api := &mockContribAPI{prCount: 0, issueCount: 0}
	err := welcome.MaybeWelcomeIssue(context.Background(), api, "org", "repo", "carol", 10, "Thanks!")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(api.comments) != 1 {
		t.Fatalf("got %d comments, want 1", len(api.comments))
	}
}

func TestWelcomeIssue_HasPriorIssues(t *testing.T) {
	api := &mockContribAPI{prCount: 0, issueCount: 2}
	err := welcome.MaybeWelcomeIssue(context.Background(), api, "org", "repo", "dave", 11, "Thanks!")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(api.comments) != 0 {
		t.Errorf("got %d comments, want 0", len(api.comments))
	}
}

func TestWelcome_EmptyMessage(t *testing.T) {
	api := &mockContribAPI{prCount: 0, issueCount: 0}
	err := welcome.MaybeWelcomePR(context.Background(), api, "org", "repo", "eve", 1, "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(api.comments) != 0 {
		t.Errorf("got %d comments, want 0 for empty message", len(api.comments))
	}
}
