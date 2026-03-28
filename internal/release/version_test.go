package release_test

import (
	"testing"

	"github.com/vexil-lang/vexilbot/internal/release"
)

func TestParseVersion(t *testing.T) {
	tests := []struct {
		input string
		want  release.Version
		ok    bool
	}{
		{"0.3.1", release.Version{0, 3, 1}, true},
		{"1.0.0", release.Version{1, 0, 0}, true},
		{"0.0.1", release.Version{0, 0, 1}, true},
		{"invalid", release.Version{}, false},
		{"1.2", release.Version{}, false},
		{"", release.Version{}, false},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got, err := release.ParseVersion(tt.input)
			if tt.ok && err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if !tt.ok && err == nil {
				t.Fatal("expected error")
			}
			if got != tt.want {
				t.Errorf("got %v, want %v", got, tt.want)
			}
		})
	}
}

func TestVersion_Bump(t *testing.T) {
	v := release.Version{0, 3, 1}

	patch := v.Bump(release.BumpPatch)
	if patch != (release.Version{0, 3, 2}) {
		t.Errorf("patch bump = %v", patch)
	}

	minor := v.Bump(release.BumpMinor)
	if minor != (release.Version{0, 4, 0}) {
		t.Errorf("minor bump = %v", minor)
	}

	major := v.Bump(release.BumpMajor)
	if major != (release.Version{1, 0, 0}) {
		t.Errorf("major bump = %v", major)
	}
}

func TestVersion_String(t *testing.T) {
	v := release.Version{0, 3, 1}
	if v.String() != "0.3.1" {
		t.Errorf("String() = %q", v.String())
	}
}

func TestSuggestBump(t *testing.T) {
	tests := []struct {
		name     string
		messages []string
		want     release.BumpLevel
	}{
		{
			name:     "fix only",
			messages: []string{"fix: correct parser error", "fix: handle empty input"},
			want:     release.BumpPatch,
		},
		{
			name:     "feat present",
			messages: []string{"fix: typo", "feat: add union types", "docs: update readme"},
			want:     release.BumpMinor,
		},
		{
			name:     "breaking change footer",
			messages: []string{"feat!: redesign wire format"},
			want:     release.BumpMajor,
		},
		{
			name:     "breaking change with exclamation",
			messages: []string{"refactor!: rename public API"},
			want:     release.BumpMajor,
		},
		{
			name:     "non-conventional commits",
			messages: []string{"update readme", "misc changes"},
			want:     release.BumpPatch,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := release.SuggestBump(tt.messages)
			if got != tt.want {
				t.Errorf("got %v, want %v", got, tt.want)
			}
		})
	}
}

func TestExtractTagVersion(t *testing.T) {
	tests := []struct {
		tag     string
		crate   string
		format  string
		wantVer string
		wantOK  bool
	}{
		{"vexil-lang-v0.3.1", "vexil-lang", "{{ crate }}-v{{ version }}", "0.3.1", true},
		{"vexil-runtime-v1.0.0", "vexil-runtime", "{{ crate }}-v{{ version }}", "1.0.0", true},
		{"vexil-runtime-v1.0.0", "vexil-lang", "{{ crate }}-v{{ version }}", "", false},
		{"unrelated-tag", "vexil-lang", "{{ crate }}-v{{ version }}", "", false},
	}

	for _, tt := range tests {
		t.Run(tt.tag, func(t *testing.T) {
			ver, ok := release.ExtractTagVersion(tt.tag, tt.crate, tt.format)
			if ok != tt.wantOK {
				t.Fatalf("ok = %v, want %v", ok, tt.wantOK)
			}
			if ver != tt.wantVer {
				t.Errorf("version = %q, want %q", ver, tt.wantVer)
			}
		})
	}
}
