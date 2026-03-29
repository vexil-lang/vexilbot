package main

import (
	"path/filepath"
	"testing"

	vexil "github.com/vexil-lang/vexil/packages/runtime-go"
	"github.com/vexil-lang/vexilbot/internal/vexstore"
	"github.com/vexil-lang/vexilbot/internal/vexstore/gen/installation"
)

func openInstallStore(t *testing.T) *vexstore.AppendStore {
	t.Helper()
	s, err := vexstore.OpenAppendStore(
		filepath.Join(t.TempDir(), "installations.vxb"),
		installation.SchemaHash,
	)
	if err != nil {
		t.Fatalf("OpenAppendStore: %v", err)
	}
	t.Cleanup(func() { s.Close() })
	return s
}

func TestInstallationStoreSetAppends(t *testing.T) {
	vs := openInstallStore(t)
	s := &installationStore{entries: make(map[string]int64), store: vs}

	s.set("vexil-lang", "vexil", 42)

	records, err := vs.ReadAll()
	if err != nil {
		t.Fatalf("ReadAll: %v", err)
	}
	if len(records) != 1 {
		t.Fatalf("want 1 record, got %d", len(records))
	}

	// Verify the encoded fields decode correctly
	r := vexil.NewBitReader(records[0])
	var ev installation.InstallationEvent
	if err := ev.Unpack(r); err != nil {
		t.Fatalf("Unpack: %v", err)
	}
	if ev.Owner != "vexil-lang" {
		t.Errorf("Owner: want %q, got %q", "vexil-lang", ev.Owner)
	}
	if ev.Repo != "vexil" {
		t.Errorf("Repo: want %q, got %q", "vexil", ev.Repo)
	}
	if ev.Iid != 42 {
		t.Errorf("Iid: want 42, got %d", ev.Iid)
	}
}

func TestInstallationStoreLoadFromStore(t *testing.T) {
	vs := openInstallStore(t)

	s := &installationStore{entries: make(map[string]int64), store: vs}
	s.set("vexil-lang", "vexil", 10)
	s.set("vexil-lang", "vexilbot", 20)

	s2 := &installationStore{entries: make(map[string]int64)}
	if err := s2.loadFromStore(vs); err != nil {
		t.Fatalf("loadFromStore: %v", err)
	}

	if id, ok := s2.get("vexil-lang", "vexil"); !ok || id != 10 {
		t.Errorf("vexil: want 10, got %d (ok=%v)", id, ok)
	}
	if id, ok := s2.get("vexil-lang", "vexilbot"); !ok || id != 20 {
		t.Errorf("vexilbot: want 20, got %d (ok=%v)", id, ok)
	}
	repos := s2.list()
	if len(repos) != 2 {
		t.Errorf("want 2 repos, got %d: %v", len(repos), repos)
	}
}

func TestInstallationStoreLoadLatestWins(t *testing.T) {
	vs := openInstallStore(t)
	s := &installationStore{entries: make(map[string]int64), store: vs}

	s.set("vexil-lang", "vexil", 100)
	s.set("vexil-lang", "vexil", 200)

	s2 := &installationStore{entries: make(map[string]int64)}
	if err := s2.loadFromStore(vs); err != nil {
		t.Fatalf("loadFromStore: %v", err)
	}
	if id, ok := s2.get("vexil-lang", "vexil"); !ok || id != 200 {
		t.Errorf("want 200 (latest), got %d (ok=%v)", id, ok)
	}
}

func TestInstallationStoreNilStore(t *testing.T) {
	s := &installationStore{entries: make(map[string]int64)}
	s.set("vexil-lang", "vexil", 99)
	id, ok := s.get("vexil-lang", "vexil")
	if !ok || id != 99 {
		t.Errorf("want 99, got %d (ok=%v)", id, ok)
	}
}
