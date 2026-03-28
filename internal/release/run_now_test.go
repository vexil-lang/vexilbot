package release_test

import (
	"context"
	"errors"
	"testing"

	"github.com/vexil-lang/vexilbot/internal/release"
	"github.com/vexil-lang/vexilbot/internal/repoconfig"
)

func TestRunReleaseNowUnknownPackage(t *testing.T) {
	cfg := repoconfig.Release{} // no crates or packages
	_, err := release.RunReleaseNow(context.Background(), nil, "owner", "repo", "nonexistent", cfg)
	if err == nil {
		t.Fatal("expected error for unknown package")
	}
	if !errors.Is(err, release.ErrNotFound) {
		t.Errorf("expected ErrNotFound, got %v", err)
	}
}
