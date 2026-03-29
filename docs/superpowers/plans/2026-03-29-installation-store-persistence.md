# Installation Store Persistence Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Persist the `installationStore` (owner/repo → GitHub App installation ID) to a `.vxb` file so the dashboard repo dropdown and GitHub API calls work immediately after a container restart.

**Architecture:** New `schemas/installation.vexil` schema, codegen'd to `internal/vexstore/gen/installation/`. The `installationStore` in `cmd/vexilbot/main.go` gains a `store *vexstore.AppendStore` field — `set()` appends a record on each webhook, `loadFromStore()` replays all records on startup (latest per repo wins). One new `installations.vxb` file alongside the existing `logs.vxb` and `events.vxb`.

**Tech Stack:** Go 1.25, `vexilc --target go` (from `~/projects/orix/vexil-lang/target/release/vexilc`), `github.com/vexil-lang/vexil/packages/runtime-go` (aliased `vexil`), existing `internal/vexstore` package.

---

## File Structure

```
schemas/
  installation.vexil               (new)

internal/vexstore/gen/
  installation/                    (new, generated — DO NOT EDIT)

cmd/vexilbot/
  main.go                          (modify — installationStore struct, set, loadFromStore, main wiring)
  installation_store_test.go       (new — tests for set + loadFromStore, package main)
```

---

## Task 1: Schema File + Codegen

**Files:**
- Create: `schemas/installation.vexil`
- Create: `internal/vexstore/gen/installation/` (generated)

No tests — verify compilation.

- [ ] **Step 1: Create `schemas/installation.vexil`**

```vexil
namespace installation

message InstallationEvent {
    owner @0 : string
    repo  @1 : string
    iid   @2 : u64
}
```

Field `iid` (installation ID) keeps the field name short and unambiguous in generated code (`Iid`).

- [ ] **Step 2: Build vexilc (if not already built)**

```bash
cd ~/projects/orix/vexil-lang && cargo build -p vexilc --release 2>&1 | tail -5
```

Expected: `Compiling vexilc` or `Finished release` — binary at `target/release/vexilc`.

- [ ] **Step 3: Run codegen**

```bash
cd ~/projects/orix/vexilbot
mkdir -p internal/vexstore/gen/installation
~/projects/orix/vexil-lang/target/release/vexilc \
    --target go \
    --out internal/vexstore/gen/installation/ \
    schemas/installation.vexil
```

Expected: a `.go` file appears in `internal/vexstore/gen/installation/`.

- [ ] **Step 4: Spot-check generated file**

```bash
cat internal/vexstore/gen/installation/*.go
```

Confirm:
- `package installation`
- `var SchemaHash = [32]byte{...}`
- `type InstallationEvent struct { Owner string; Repo string; Iid uint64; Unknown []byte }`
- Methods: `Pack(w *vexil.BitWriter) error` and `Unpack(r *vexil.BitReader) error`

If the field name for `iid` is different (e.g. `IId`), note the actual name — you will use it exactly as generated in Task 2.

- [ ] **Step 5: Verify compilation**

```bash
go build ./internal/vexstore/gen/installation/
```

Expected: exit 0, no output.

- [ ] **Step 6: Commit**

```bash
git add schemas/installation.vexil internal/vexstore/gen/installation/
git commit -m "feat(vexstore): add installation schema and codegen'd InstallationEvent type"
```

---

## Task 2: installationStore Persistence — Tests + Implementation

**Files:**
- Create: `cmd/vexilbot/installation_store_test.go`
- Modify: `cmd/vexilbot/main.go` — `installationStore` struct, `set()`, new `loadFromStore()`

The test file uses `package main` (internal test package) so it can access the unexported `installationStore` type directly.

**Key generated names to use** (adjust if spot-check in Task 1 showed different names):
- Package: `installation`
- Type: `installation.InstallationEvent`
- Fields: `Owner`, `Repo`, `Iid`
- Hash: `installation.SchemaHash`

- [ ] **Step 1: Write `cmd/vexilbot/installation_store_test.go`**

```go
package main

import (
	"path/filepath"
	"testing"

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
}

func TestInstallationStoreLoadFromStore(t *testing.T) {
	vs := openInstallStore(t)

	// Write two repos
	s := &installationStore{entries: make(map[string]int64), store: vs}
	s.set("vexil-lang", "vexil", 10)
	s.set("vexil-lang", "vexilbot", 20)

	// Load into a fresh store
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

	// Same repo, two different installation IDs (reinstall scenario)
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
	// nil store must not panic — in-memory only behavior unchanged
	s := &installationStore{entries: make(map[string]int64)}
	s.set("vexil-lang", "vexil", 99)
	id, ok := s.get("vexil-lang", "vexil")
	if !ok || id != 99 {
		t.Errorf("want 99, got %d (ok=%v)", id, ok)
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

```bash
go test ./cmd/vexilbot/ -run TestInstallationStore -v
```

Expected: compilation error — `installationStore` has no `store` field, `loadFromStore` undefined.

- [ ] **Step 3: Add `store` field to `installationStore` in `cmd/vexilbot/main.go`**

Replace (lines ~40-43):

```go
type installationStore struct {
	mu      sync.RWMutex
	entries map[string]int64
}
```

With:

```go
type installationStore struct {
	mu      sync.RWMutex
	entries map[string]int64
	store   *vexstore.AppendStore // nil = in-memory only
}
```

- [ ] **Step 4: Modify `set()` to append to the store**

Replace the existing `set` method:

```go
func (s *installationStore) set(owner, repo string, id int64) {
	s.mu.Lock()
	s.entries[owner+"/"+repo] = id
	s.mu.Unlock()
	if s.store != nil {
		ev := &installation.InstallationEvent{
			Owner: owner,
			Repo:  repo,
			Iid:   uint64(id),
		}
		bw := vexil.NewBitWriter()
		if ev.Pack(bw) == nil {
			if err := s.store.Append(bw.Finish()); err != nil {
				slog.Error("installation store append", "error", err)
			}
		}
	}
}
```

- [ ] **Step 5: Add `loadFromStore()` method**

Add after `list()` in `cmd/vexilbot/main.go`:

```go
func (s *installationStore) loadFromStore(store *vexstore.AppendStore) error {
	records, err := store.ReadAll()
	if err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, rec := range records {
		r := vexil.NewBitReader(rec)
		var ev installation.InstallationEvent
		if err := ev.Unpack(r); err != nil {
			slog.Warn("installation store: skip bad record", "error", err)
			continue
		}
		s.entries[ev.Owner+"/"+ev.Repo] = int64(ev.Iid)
	}
	return nil
}
```

- [ ] **Step 6: Add the `installation` import to `cmd/vexilbot/main.go`**

In the import block, add alongside the other generated packages:

```go
"github.com/vexil-lang/vexilbot/internal/vexstore/gen/installation"
```

- [ ] **Step 7: Run tests to verify they pass**

```bash
go test ./cmd/vexilbot/ -run TestInstallationStore -v
```

Expected: all four `TestInstallationStore*` tests PASS.

- [ ] **Step 8: Run full suite**

```bash
go test ./...
```

Expected: all packages PASS.

- [ ] **Step 9: Commit**

```bash
git add cmd/vexilbot/main.go cmd/vexilbot/installation_store_test.go
git commit -m "feat: persist installationStore to installations.vxb — survives restarts"
```

---

## Task 3: Wire `installations.vxb` in `main()` + Deploy

**Files:**
- Modify: `cmd/vexilbot/main.go` — open store, call `loadFromStore`, pass to `installationStore`

- [ ] **Step 1: Open `installations.vxb` in `main()`**

In `cmd/vexilbot/main.go`, after the `scheduledRelStore` block (around line 101), add:

```go
installStore, err := vexstore.OpenAppendStore(cfg.Server.DataDir+"/installations.vxb", installation.SchemaHash)
if err != nil {
	fmt.Fprintf(os.Stderr, "open installation store: %v\n", err)
	os.Exit(1)
}
defer installStore.Close()
```

- [ ] **Step 2: Pass store to `installationStore` and replay**

Replace (around line 111):

```go
store := &installationStore{entries: make(map[string]int64)}
```

With:

```go
store := &installationStore{entries: make(map[string]int64), store: installStore}
if err := store.loadFromStore(installStore); err != nil {
	slog.Warn("load installation store", "error", err)
}
```

Note: the `slog.Warn` call here happens before `slog.SetDefault(logger)` in the current file ordering. Verify that `logger` is initialized before this block; if not, swap the order so stores open after `slog.SetDefault`. The logger init block is around line 102 — move the `installStore` opening to after it if needed.

- [ ] **Step 3: Build and test**

```bash
go build ./cmd/vexilbot/ && go test ./...
```

Expected: build succeeds, all tests PASS.

- [ ] **Step 4: Commit**

```bash
git add cmd/vexilbot/main.go
git commit -m "feat: open installations.vxb on startup and replay known repos into installationStore"
```

- [ ] **Step 5: Push and deploy**

```bash
git push origin master
```

Then on the production server (using PowerShell + SSH key):

```powershell
ssh -i "$env:USERPROFILE\.ssh\orix_prod" root@46.224.214.53 "cd /opt/apps/vexilbot && docker compose pull && docker compose up -d"
```

- [ ] **Step 6: Verify**

```powershell
ssh -i "$env:USERPROFILE\.ssh\orix_prod" root@46.224.214.53 "docker compose -f /opt/apps/vexilbot/docker-compose.yml logs --tail=10 vexilbot"
```

Expected: container starts cleanly, no `open installation store` error. After the first webhook arrives, restart the container and confirm the repo dropdown still shows the repo without waiting for another webhook.

---

## Coverage Matrix

| Requirement | Task |
|---|---|
| `schemas/installation.vexil` schema | 1 |
| `vexilc --target go` codegen | 1 |
| `InstallationEvent` Pack/Unpack | 1 (generated) |
| `SchemaHash` per package | 1 (generated) |
| `installationStore.store` field | 2 |
| `set()` appends to store (best-effort) | 2 |
| `set()` nil-safe (in-memory still works) | 2 |
| `loadFromStore()` replays all records | 2 |
| Latest record wins per owner/repo | 2 |
| Bad records skipped with warning | 2 |
| `installations.vxb` opened on startup | 3 |
| `loadFromStore` called before server starts | 3 |
| Deploy to production | 3 |
