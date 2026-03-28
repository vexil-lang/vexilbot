package labeler_test

import (
	"testing"

	"github.com/vexil-lang/vexilbot/internal/labeler"
)

func TestMatchGlob(t *testing.T) {
	tests := []struct {
		pattern string
		path    string
		want    bool
	}{
		{"crates/vexil-lang/**", "crates/vexil-lang/src/lib.rs", true},
		{"crates/vexil-lang/**", "crates/vexil-lang/src/parser/mod.rs", true},
		{"crates/vexil-lang/**", "crates/vexil-codegen-rust/src/lib.rs", false},
		{"spec/**", "spec/vexil-spec.md", true},
		{"spec/**", "spec/grammar/vexil.peg", true},
		{"spec/**", "src/spec.rs", false},
		{".github/**", ".github/workflows/ci.yml", true},
		{"*.md", "README.md", true},
		{"*.md", "src/README.md", false},
	}

	for _, tt := range tests {
		t.Run(tt.pattern+"_"+tt.path, func(t *testing.T) {
			got := labeler.MatchGlob(tt.pattern, tt.path)
			if got != tt.want {
				t.Errorf("MatchGlob(%q, %q) = %v, want %v", tt.pattern, tt.path, got, tt.want)
			}
		})
	}
}
