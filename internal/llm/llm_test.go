package llm_test

import (
	"context"
	"testing"

	"github.com/vexil-lang/vexilbot/internal/llm"
)

func TestNoopClient(t *testing.T) {
	client := llm.NewNoopClient()
	result, err := client.Analyze(context.Background(), "test prompt", "test context")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result != "" {
		t.Errorf("noop client returned %q, want empty string", result)
	}
}
