package release_test

import (
	"testing"

	"github.com/vexil-lang/vexilbot/internal/release"
)

func TestBumpCargoVersion(t *testing.T) {
	input := `[package]
name = "vexil-lang"
version = "0.3.1"
edition = "2021"

[dependencies]
thiserror = "2"
`
	want := `[package]
name = "vexil-lang"
version = "0.4.0"
edition = "2021"

[dependencies]
thiserror = "2"
`
	got, err := release.BumpCargoVersion(input, "0.4.0")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != want {
		t.Errorf("got:\n%s\nwant:\n%s", got, want)
	}
}

func TestBumpCargoVersion_WorkspaceInherit(t *testing.T) {
	input := `[package]
name = "vexil-lang"
version.workspace = true
edition = "2021"
`
	_, err := release.BumpCargoVersion(input, "0.4.0")
	if err == nil {
		t.Fatal("expected error for workspace-inherited version")
	}
}

func TestBumpCargoDependency(t *testing.T) {
	input := `[package]
name = "vexil-codegen-rust"
version = "0.3.1"

[dependencies]
vexil-lang = { path = "../vexil-lang", version = "0.3.1" }
thiserror = "2"
`
	want := `[package]
name = "vexil-codegen-rust"
version = "0.3.1"

[dependencies]
vexil-lang = { path = "../vexil-lang", version = "0.4.0" }
thiserror = "2"
`
	got, err := release.BumpCargoDependency(input, "vexil-lang", "0.4.0")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != want {
		t.Errorf("got:\n%s\nwant:\n%s", got, want)
	}
}
