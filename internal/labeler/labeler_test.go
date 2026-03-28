package labeler_test

import (
	"testing"

	"github.com/vexil-lang/vexilbot/internal/labeler"
	"github.com/vexil-lang/vexilbot/internal/repoconfig"
)

func TestMatchPathLabels(t *testing.T) {
	cfg := repoconfig.Labels{
		Paths: map[string][]string{
			"crate:lang": {"crates/vexil-lang/**"},
			"crate:cli":  {"crates/vexilc/**"},
			"spec":       {"spec/**"},
			"ci":         {".github/**"},
		},
	}

	changedFiles := []string{
		"crates/vexil-lang/src/parser.rs",
		"crates/vexil-lang/src/lexer.rs",
		".github/workflows/ci.yml",
	}

	labels := labeler.MatchPathLabels(cfg, changedFiles)

	want := map[string]bool{"crate:lang": true, "ci": true}
	if len(labels) != len(want) {
		t.Fatalf("got %d labels %v, want %d", len(labels), labels, len(want))
	}
	for _, l := range labels {
		if !want[l] {
			t.Errorf("unexpected label %q", l)
		}
	}
}

func TestMatchPathLabels_NoMatch(t *testing.T) {
	cfg := repoconfig.Labels{
		Paths: map[string][]string{
			"spec": {"spec/**"},
		},
	}

	labels := labeler.MatchPathLabels(cfg, []string{"src/main.rs"})
	if len(labels) != 0 {
		t.Errorf("got labels %v, want none", labels)
	}
}

func TestMatchKeywordLabels(t *testing.T) {
	cfg := repoconfig.Labels{
		Keywords: map[string][]string{
			"bug":         {"crash", "panic", "error"},
			"performance": {"slow", "benchmark"},
		},
	}

	labels := labeler.MatchKeywordLabels(cfg, "Parser panics on empty input", "When I pass an empty file, the parser crashes with a panic.")

	want := map[string]bool{"bug": true}
	if len(labels) != len(want) {
		t.Fatalf("got %d labels %v, want %d", len(labels), labels, len(want))
	}
	for _, l := range labels {
		if !want[l] {
			t.Errorf("unexpected label %q", l)
		}
	}
}

func TestMatchKeywordLabels_CaseInsensitive(t *testing.T) {
	cfg := repoconfig.Labels{
		Keywords: map[string][]string{
			"bug": {"crash"},
		},
	}

	labels := labeler.MatchKeywordLabels(cfg, "CRASH on startup", "")
	if len(labels) != 1 || labels[0] != "bug" {
		t.Errorf("got %v, want [bug]", labels)
	}
}
