package policy_test

import (
	"context"
	"testing"

	"github.com/vexil-lang/vexilbot/internal/labeler"
	"github.com/vexil-lang/vexilbot/internal/policy"
)

type mockPolicyAPI struct {
	labels    []string
	statusSet *struct{ state, context, description string }
	comments  []string
}

func (m *mockPolicyAPI) GetLabels(ctx context.Context, owner, repo string, number int) ([]string, error) {
	return m.labels, nil
}

func (m *mockPolicyAPI) SetCommitStatus(ctx context.Context, owner, repo, sha, state, statusContext, description string) error {
	m.statusSet = &struct{ state, context, description string }{state, statusContext, description}
	return nil
}

func (m *mockPolicyAPI) CreateComment(ctx context.Context, owner, repo string, number int, body string) error {
	m.comments = append(m.comments, body)
	return nil
}

var _ = labeler.MatchGlob

func TestCheckRFCGate_RequiresRFC(t *testing.T) {
	api := &mockPolicyAPI{labels: []string{"enhancement"}}
	rfcPaths := []string{"spec/**", "corpus/valid/**"}
	changedFiles := []string{"spec/vexil-spec.md"}

	result, err := policy.CheckRFCGate(context.Background(), api, "org", "repo", 1, "abc123", rfcPaths, changedFiles)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result != policy.RFCRequired {
		t.Errorf("result = %v, want RFCRequired", result)
	}
	if api.statusSet == nil || api.statusSet.state != "pending" {
		t.Error("expected pending commit status")
	}
	if len(api.comments) != 1 {
		t.Errorf("got %d comments, want 1", len(api.comments))
	}
}

func TestCheckRFCGate_HasRFCLabel(t *testing.T) {
	api := &mockPolicyAPI{labels: []string{"rfc", "enhancement"}}
	rfcPaths := []string{"spec/**"}
	changedFiles := []string{"spec/vexil-spec.md"}

	result, err := policy.CheckRFCGate(context.Background(), api, "org", "repo", 1, "abc123", rfcPaths, changedFiles)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result != policy.RFCSatisfied {
		t.Errorf("result = %v, want RFCSatisfied", result)
	}
	if api.statusSet == nil || api.statusSet.state != "success" {
		t.Error("expected success commit status")
	}
}

func TestCheckRFCGate_NoRFCPathsTouched(t *testing.T) {
	api := &mockPolicyAPI{labels: []string{}}
	rfcPaths := []string{"spec/**"}
	changedFiles := []string{"crates/vexil-lang/src/lib.rs"}

	result, err := policy.CheckRFCGate(context.Background(), api, "org", "repo", 1, "abc123", rfcPaths, changedFiles)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result != policy.RFCNotApplicable {
		t.Errorf("result = %v, want RFCNotApplicable", result)
	}
}
