package release_test

import (
	"context"
	"testing"

	"github.com/vexil-lang/vexilbot/internal/release"
)

func TestPublishCrate(t *testing.T) {
	runner := &mockCmdRunner{output: ""}
	err := release.PublishCrate(context.Background(), runner, "/repo", "crates/vexil-lang", "test-token")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}
