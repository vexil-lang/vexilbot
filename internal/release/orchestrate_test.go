package release_test

import (
	"testing"

	"github.com/vexil-lang/vexilbot/internal/release"
	"github.com/vexil-lang/vexilbot/internal/repoconfig"
)

func TestResolveDependencyOrder(t *testing.T) {
	crates := map[string]repoconfig.CrateEntry{
		"vexil-lang":         {DependsOn: []string{}},
		"vexil-runtime":      {DependsOn: []string{"vexil-lang"}},
		"vexil-codegen-rust": {DependsOn: []string{"vexil-lang"}},
		"vexil-store":        {DependsOn: []string{"vexil-lang", "vexil-runtime"}},
		"vexilc":             {DependsOn: []string{"vexil-lang", "vexil-codegen-rust"}},
	}

	order, err := release.ResolveDependencyOrder(crates)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	indexOf := make(map[string]int)
	for i, name := range order {
		indexOf[name] = i
	}

	assertBefore := func(a, b string) {
		t.Helper()
		if indexOf[a] >= indexOf[b] {
			t.Errorf("%s (idx %d) should come before %s (idx %d)", a, indexOf[a], b, indexOf[b])
		}
	}

	assertBefore("vexil-lang", "vexil-runtime")
	assertBefore("vexil-lang", "vexil-codegen-rust")
	assertBefore("vexil-lang", "vexil-store")
	assertBefore("vexil-runtime", "vexil-store")
	assertBefore("vexil-codegen-rust", "vexilc")
}

func TestResolveDependencyOrder_CyclicError(t *testing.T) {
	crates := map[string]repoconfig.CrateEntry{
		"a": {DependsOn: []string{"b"}},
		"b": {DependsOn: []string{"a"}},
	}

	_, err := release.ResolveDependencyOrder(crates)
	if err == nil {
		t.Fatal("expected error for cyclic dependency")
	}
}

func TestReleaseBranchName(t *testing.T) {
	name := release.BranchName("vexil-lang", "0.4.0")
	if name != "release/vexil-lang-v0.4.0" {
		t.Errorf("branch name = %q", name)
	}
}
