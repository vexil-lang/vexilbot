package release_test

import (
	"context"
	"testing"

	"github.com/vexil-lang/vexilbot/internal/release"
	"github.com/vexil-lang/vexilbot/internal/repoconfig"
)

type mockGitAPI struct {
	tags    []string
	commits map[string][]release.Commit
}

func (m *mockGitAPI) ListTags(ctx context.Context, owner, repo string) ([]string, error) {
	return m.tags, nil
}

func (m *mockGitAPI) CommitsSinceTag(ctx context.Context, owner, repo, tag, path string) ([]release.Commit, error) {
	commits, ok := m.commits[tag]
	if !ok {
		return nil, nil
	}
	var filtered []release.Commit
	for _, c := range commits {
		for _, f := range c.Files {
			if len(f) >= len(path) && f[:len(path)] == path {
				filtered = append(filtered, c)
				break
			}
		}
	}
	return filtered, nil
}

func TestDetectChanges(t *testing.T) {
	api := &mockGitAPI{
		tags: []string{"vexil-lang-v0.3.1", "vexil-runtime-v0.2.0"},
		commits: map[string][]release.Commit{
			"vexil-lang-v0.3.1": {
				{Message: "feat: add union types", Files: []string{"crates/vexil-lang/src/union.rs"}},
				{Message: "fix: parser crash", Files: []string{"crates/vexil-lang/src/parser.rs"}},
			},
			"vexil-runtime-v0.2.0": {
				{Message: "docs: update readme", Files: []string{"README.md"}},
			},
		},
	}

	crates := map[string]repoconfig.CrateEntry{
		"vexil-lang": {
			Path:             "crates/vexil-lang",
			Publish:          "crates.io",
			SuggestThreshold: 1,
		},
		"vexil-runtime": {
			Path:             "crates/vexil-runtime",
			Publish:          "crates.io",
			SuggestThreshold: 1,
		},
	}

	results, err := release.DetectChanges(context.Background(), api, "org", "repo", "{{ crate }}-v{{ version }}", crates)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	langResult, ok := results["vexil-lang"]
	if !ok {
		t.Fatal("vexil-lang not in results")
	}
	if langResult.CurrentVersion != "0.3.1" {
		t.Errorf("current version = %q", langResult.CurrentVersion)
	}
	if len(langResult.Commits) != 2 {
		t.Errorf("commits = %d, want 2", len(langResult.Commits))
	}
	if langResult.SuggestedBump != release.BumpMinor {
		t.Errorf("suggested bump = %v, want minor", langResult.SuggestedBump)
	}

	rtResult, ok := results["vexil-runtime"]
	if !ok {
		t.Fatal("vexil-runtime not in results")
	}
	if len(rtResult.Commits) != 0 {
		t.Errorf("runtime commits = %d, want 0", len(rtResult.Commits))
	}
}

func TestGetStatus_NoPackages(t *testing.T) {
	cfg := repoconfig.Release{}
	results, err := release.GetStatus(context.Background(), &mockGitAPI{}, "owner", "repo", cfg)
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != 0 {
		t.Fatalf("expected 0 results, got %d", len(results))
	}
}
