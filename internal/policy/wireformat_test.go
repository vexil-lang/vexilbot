package policy_test

import (
	"context"
	"testing"

	"github.com/vexil-lang/vexilbot/internal/policy"
)

func TestCheckWireFormat_MatchingPaths(t *testing.T) {
	api := &mockPolicyAPI{}
	warningPaths := []string{"crates/vexil-runtime/**", "spec/vexil-spec.md"}
	changedFiles := []string{"crates/vexil-runtime/src/bitwriter.rs"}

	warned, err := policy.CheckWireFormatWarning(context.Background(), api, "org", "repo", 1, warningPaths, changedFiles)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !warned {
		t.Error("expected warning for wire format path")
	}
	if len(api.comments) != 1 {
		t.Errorf("got %d comments, want 1", len(api.comments))
	}
}

func TestCheckWireFormat_NoMatch(t *testing.T) {
	api := &mockPolicyAPI{}
	warningPaths := []string{"crates/vexil-runtime/**"}
	changedFiles := []string{"crates/vexil-lang/src/parser.rs"}

	warned, err := policy.CheckWireFormatWarning(context.Background(), api, "org", "repo", 1, warningPaths, changedFiles)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if warned {
		t.Error("expected no warning")
	}
}
