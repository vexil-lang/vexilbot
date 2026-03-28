package release_test

import (
	"context"
	"testing"

	"github.com/vexil-lang/vexilbot/internal/release"
)

type mockCmdRunner struct {
	output string
	err    error
}

func (m *mockCmdRunner) Run(ctx context.Context, dir string, name string, args ...string) (string, error) {
	return m.output, m.err
}

func TestGenerateChangelog(t *testing.T) {
	runner := &mockCmdRunner{
		output: "## [0.4.0] — 2026-03-28\n\n### Features\n\n- Add union types\n",
	}

	out, err := release.GenerateChangelog(context.Background(), runner, "/repo", "vexil-lang", "vexil-lang-v0.3.1", "0.4.0")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if out == "" {
		t.Fatal("expected non-empty changelog")
	}
	if out != runner.output {
		t.Errorf("got:\n%s\nwant:\n%s", out, runner.output)
	}
}
