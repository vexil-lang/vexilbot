package configoverride_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/vexil-lang/vexilbot/internal/configoverride"
	"github.com/vexil-lang/vexilbot/internal/repoconfig"
)

func TestMerge_NoOverrideFile(t *testing.T) {
	base := &repoconfig.Config{}
	base.Welcome.PRMessage = "hello"
	got, err := configoverride.Merge(base, "/nonexistent/path.toml")
	if err != nil {
		t.Fatal(err)
	}
	if got.Welcome.PRMessage != "hello" {
		t.Fatalf("expected base value preserved, got %q", got.Welcome.PRMessage)
	}
}

func TestMerge_OverridesWelcome(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "override.toml")
	if err := os.WriteFile(path, []byte("[welcome]\npr_message = \"overridden\"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	base := &repoconfig.Config{}
	base.Welcome.PRMessage = "original"
	base.Welcome.IssueMessage = "keep-me"
	got, err := configoverride.Merge(base, path)
	if err != nil {
		t.Fatal(err)
	}
	if got.Welcome.PRMessage != "overridden" {
		t.Fatalf("expected overridden, got %q", got.Welcome.PRMessage)
	}
	if got.Welcome.IssueMessage != "keep-me" {
		t.Fatalf("expected keep-me (base), got %q", got.Welcome.IssueMessage)
	}
}

func TestSaveLoad(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test.toml")
	content := "[welcome]\npr_message = \"saved\"\n"
	if err := configoverride.Save(path, []byte(content)); err != nil {
		t.Fatal(err)
	}
	got, err := configoverride.Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != content {
		t.Fatalf("got %q want %q", got, content)
	}
}

func TestDelete(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test.toml")
	_ = os.WriteFile(path, []byte("x=1"), 0o644)
	if err := configoverride.Delete(path); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatal("expected file deleted")
	}
	// Delete of nonexistent should not error
	if err := configoverride.Delete(path); err != nil {
		t.Fatalf("delete nonexistent: %v", err)
	}
}
