# Dashboard Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add a localhost-only web dashboard to vexilbot with five tabs: Logs, Events, Releases, Config, Storage — served on a second port in the same binary.

**Architecture:** Go `html/template` + HTMX, embedded templates, second `http.Server` on `127.0.0.1:8081` started in a goroutine from `main.go`. All state read from existing `.vxb` stores; a new `scheduled_releases.vxb` store added for the Releases tab.

**Tech Stack:** Go 1.25, `html/template`, HTMX 2 (CDN), `embed.FS`, vexilc generated types, `github.com/vexil-lang/vexil/packages/runtime-go`

---

## File Map

**New files:**
- `schemas/scheduled_release.vexil` — vexil schema for scheduled releases
- `internal/vexstore/gen/scheduledrelease/scheduled_release.go` — vexilc-generated (do not hand-write)
- `internal/release/run_now.go` — exported `RunReleaseNow` helper (no issue comment)
- `internal/dashboard/server.go` — `Server` struct, `Deps`, mux setup, template loader
- `internal/dashboard/handlers_logs.go` — GET /logs, GET /logs-rows
- `internal/dashboard/handlers_events.go` — GET /events
- `internal/dashboard/handlers_releases.go` — GET /releases, POST /releases, POST /releases/{id}/cancel, POST /releases/{id}/confirm, POST /releases/{id}/run
- `internal/dashboard/handlers_config.go` — GET /config, GET /config/repo
- `internal/dashboard/handlers_storage.go` — GET /storage
- `internal/dashboard/templates/base.html`
- `internal/dashboard/templates/logs.html`
- `internal/dashboard/templates/events.html`
- `internal/dashboard/templates/releases.html`
- `internal/dashboard/templates/config.html`
- `internal/dashboard/templates/storage.html`

**Modified files:**
- `internal/serverconfig/serverconfig.go` — add `DashboardPort int` to `Server`
- `deploy/config.example.toml` — add `dashboard_port = 8081`
- `cmd/vexilbot/main.go` — add `list()` to installationStore, open scheduledRelStore, wire dashboard
- `docker-compose.yml` — add `vexilbot-data` named volume
- `docker-compose.override.yml` — add volume mount for dev

---

### Task 1: ScheduledRelease schema and codegen

**Files:**
- Create: `schemas/scheduled_release.vexil`
- Create: `internal/vexstore/gen/scheduledrelease/scheduled_release.go` (generated)

- [ ] **Step 1: Write the schema file**

```
namespace scheduledrelease

enum BumpLevel : u8 {
  Patch @0
  Minor @1
  Major @2
}

enum ReleaseStatus : u8 {
  Pending   @0
  Confirmed @1
  Running   @2
  Done      @3
  Cancelled @4
}

message ScheduledRelease {
  id         @0 : string
  package    @1 : string
  bump       @2 : BumpLevel
  run_at     @3 : u64
  auto_run   @4 : bool
  status     @5 : ReleaseStatus
  created_at @6 : u64
  owner      @7 : string
  repo       @8 : string
}
```

Save to `schemas/scheduled_release.vexil`.

- [ ] **Step 2: Run vexilc**

```bash
vexilc --target go --out internal/vexstore/gen schemas/scheduled_release.vexil
```

Expected: creates `internal/vexstore/gen/scheduledrelease/scheduled_release.go` with `var SchemaHash = [32]byte{...}`, `type BumpLevel int`, `type ReleaseStatus int`, `type ScheduledRelease struct { ID string; Package string; Bump BumpLevel; RunAt uint64; AutoRun bool; Status ReleaseStatus; CreatedAt uint64; Owner string; Repo string; Unknown []byte }`, and Pack/Unpack methods.

- [ ] **Step 3: Verify it compiles**

```bash
go build ./internal/vexstore/gen/scheduledrelease/
```

Expected: no output, exit 0.

- [ ] **Step 4: Commit**

```bash
git add schemas/scheduled_release.vexil internal/vexstore/gen/scheduledrelease/
git commit -m "feat: add scheduledrelease vexil schema and generated Go types"
```

---

### Task 2: Add DashboardPort to serverconfig

**Files:**
- Modify: `internal/serverconfig/serverconfig.go`
- Modify: `deploy/config.example.toml`
- Test: `internal/serverconfig/serverconfig_test.go` (create if missing, or add test)

- [ ] **Step 1: Write the failing test**

Check if `internal/serverconfig/serverconfig_test.go` exists. If it does, add this test. If not, create the file:

```go
package serverconfig_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/vexil-lang/vexilbot/internal/serverconfig"
)

func TestDashboardPortDefault(t *testing.T) {
	dir := t.TempDir()
	f := filepath.Join(dir, "config.toml")
	os.WriteFile(f, []byte(`
[server]
listen = "0.0.0.0:8080"
webhook_secret = "s"
bot_name = "vexilbot"
[github]
app_id = 1
private_key_path = "/tmp/key"
`), 0o644)
	cfg, err := serverconfig.Load(f)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Server.DashboardPort != 8081 {
		t.Errorf("want DashboardPort=8081, got %d", cfg.Server.DashboardPort)
	}
}

func TestDashboardPortConfigured(t *testing.T) {
	dir := t.TempDir()
	f := filepath.Join(dir, "config.toml")
	os.WriteFile(f, []byte(`
[server]
listen = "0.0.0.0:8080"
webhook_secret = "s"
bot_name = "vexilbot"
dashboard_port = 9090
[github]
app_id = 1
private_key_path = "/tmp/key"
`), 0o644)
	cfg, err := serverconfig.Load(f)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Server.DashboardPort != 9090 {
		t.Errorf("want DashboardPort=9090, got %d", cfg.Server.DashboardPort)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

```bash
go test ./internal/serverconfig/ -run TestDashboardPort -v
```

Expected: FAIL — `DashboardPort` field does not exist yet.

- [ ] **Step 3: Add DashboardPort to Server struct**

In `internal/serverconfig/serverconfig.go`, add `DashboardPort int` to the `Server` struct and default it in `Load`:

```go
type Server struct {
	Listen        string `toml:"listen"`
	WebhookSecret string `toml:"webhook_secret"`
	BotName       string `toml:"bot_name"`
	DataDir       string `toml:"data_dir"`
	DashboardPort int    `toml:"dashboard_port"`
}
```

In `Load()`, after the existing `if cfg.Server.DataDir == ""` block, add:

```go
if cfg.Server.DashboardPort == 0 {
    cfg.Server.DashboardPort = 8081
}
```

- [ ] **Step 4: Run test to verify it passes**

```bash
go test ./internal/serverconfig/ -run TestDashboardPort -v
```

Expected: PASS.

- [ ] **Step 5: Update deploy/config.example.toml**

Add this line after the `data_dir` comment:

```toml
# dashboard_port = 8081  # Dashboard listen port on 127.0.0.1 (0 = disabled, default: 8081)
```

- [ ] **Step 6: Commit**

```bash
git add internal/serverconfig/ deploy/config.example.toml
git commit -m "feat: add dashboard_port config field (default 8081)"
```

---

### Task 3: Dashboard server skeleton and base template

**Files:**
- Create: `internal/dashboard/server.go`
- Create: `internal/dashboard/templates/base.html`
- Test: `internal/dashboard/server_test.go`

- [ ] **Step 1: Write the failing test**

```go
// internal/dashboard/server_test.go
package dashboard_test

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/vexil-lang/vexilbot/internal/dashboard"
	"github.com/vexil-lang/vexilbot/internal/vexstore"
	"github.com/vexil-lang/vexilbot/internal/vexstore/gen/logentry"
	"github.com/vexil-lang/vexilbot/internal/vexstore/gen/scheduledrelease"
	"github.com/vexil-lang/vexilbot/internal/vexstore/gen/webhookevent"
)

func openTestStore(t *testing.T, schemaHash [32]byte) *vexstore.AppendStore {
	t.Helper()
	f := t.TempDir() + "/test.vxb"
	s, err := vexstore.OpenAppendStore(f, schemaHash)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Close() })
	return s
}

func newTestServer(t *testing.T) *dashboard.Server {
	t.Helper()
	return dashboard.New(dashboard.Deps{
		LogStore:     openTestStore(t, logentry.SchemaHash),
		EventStore:   openTestStore(t, webhookevent.SchemaHash),
		ReleaseStore: openTestStore(t, scheduledrelease.SchemaHash),
		DataDir:      t.TempDir(),
		KnownRepos:   func() []string { return nil },
		RunRelease:   func(_ interface{}, _, _, _ string) (int, error) { return 0, nil },
	})
}

func TestServerRootRedirects(t *testing.T) {
	srv := newTestServer(t)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/", nil)
	srv.ServeHTTP(rec, req)
	if rec.Code != http.StatusFound {
		t.Errorf("want 302, got %d", rec.Code)
	}
	if !strings.HasSuffix(rec.Header().Get("Location"), "/logs") {
		t.Errorf("want redirect to /logs, got %q", rec.Header().Get("Location"))
	}
}

func TestServerLogsOK(t *testing.T) {
	srv := newTestServer(t)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/logs", nil)
	srv.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Errorf("want 200, got %d: %s", rec.Code, rec.Body.String())
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

```bash
go test ./internal/dashboard/ -run TestServer -v
```

Expected: FAIL — package does not exist yet.

- [ ] **Step 3: Create server.go**

```go
// internal/dashboard/server.go
package dashboard

import (
	"context"
	"embed"
	"html/template"
	"net/http"

	"github.com/vexil-lang/vexilbot/internal/vexstore"
)

//go:embed templates/*
var templateFS embed.FS

// Deps holds all external dependencies the dashboard needs.
type Deps struct {
	LogStore     *vexstore.AppendStore
	EventStore   *vexstore.AppendStore
	ReleaseStore *vexstore.AppendStore
	DataDir      string
	// KnownRepos returns all owner/repo strings seen so far (e.g. "owner/repo").
	KnownRepos func() []string
	// RunRelease creates a release PR for owner/repo/pkg. Returns the PR number.
	RunRelease func(ctx context.Context, owner, repo, pkg string) (int, error)
}

// Server is the dashboard HTTP server.
type Server struct {
	mux  *http.ServeMux
	tmpl *template.Template
	deps Deps
}

// New creates a dashboard Server with all routes registered.
func New(deps Deps) *Server {
	tmpl := template.Must(template.ParseFS(templateFS, "templates/*.html"))
	s := &Server{mux: http.NewServeMux(), tmpl: tmpl, deps: deps}
	s.mux.HandleFunc("GET /", s.handleRoot)
	s.mux.HandleFunc("GET /logs", s.handleLogs)
	s.mux.HandleFunc("GET /logs-rows", s.handleLogsRows)
	s.mux.HandleFunc("GET /events", s.handleEvents)
	s.mux.HandleFunc("GET /releases", s.handleReleases)
	s.mux.HandleFunc("POST /releases", s.handleReleasesCreate)
	s.mux.HandleFunc("POST /releases/{id}/cancel", s.handleReleasesCancel)
	s.mux.HandleFunc("POST /releases/{id}/confirm", s.handleReleasesConfirm)
	s.mux.HandleFunc("POST /releases/{id}/run", s.handleReleasesRun)
	s.mux.HandleFunc("GET /config", s.handleConfig)
	s.mux.HandleFunc("GET /config/repo", s.handleConfigRepo)
	s.mux.HandleFunc("GET /storage", s.handleStorage)
	return s
}

func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	s.mux.ServeHTTP(w, r)
}

func (s *Server) handleRoot(w http.ResponseWriter, r *http.Request) {
	http.Redirect(w, r, "/logs", http.StatusFound)
}

// render executes a named template pair: base.html wrapping the named page template.
// data is passed as template data.
func (s *Server) render(w http.ResponseWriter, name string, data any) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := s.tmpl.ExecuteTemplate(w, name, data); err != nil {
		http.Error(w, "template error: "+err.Error(), http.StatusInternalServerError)
	}
}
```

- [ ] **Step 4: Create base.html template**

```html
<!-- internal/dashboard/templates/base.html -->
<!DOCTYPE html>
<html lang="en">
<head>
<meta charset="UTF-8">
<meta name="viewport" content="width=device-width, initial-scale=1">
<title>vexilbot dashboard</title>
<script src="https://unpkg.com/htmx.org@2.0.4"></script>
<style>
* { box-sizing: border-box; margin: 0; padding: 0; }
body { background: #0d1117; color: #e6edf3; font-family: -apple-system, BlinkMacSystemFont, "Segoe UI", monospace; font-size: 14px; }
a { color: #58a6ff; text-decoration: none; }
.top-bar { background: #161b22; border-bottom: 1px solid #30363d; padding: 0 24px; display: flex; align-items: center; gap: 24px; height: 48px; }
.top-bar .brand { font-weight: 700; color: #58a6ff; font-size: 15px; letter-spacing: .04em; }
.tabs { display: flex; gap: 2px; }
.tabs a { padding: 6px 14px; border-radius: 4px; color: #8b949e; font-size: 13px; }
.tabs a.active { background: #1f6feb; color: #fff; font-weight: 600; }
.tabs a:hover:not(.active) { background: #21262d; color: #e6edf3; }
.content { padding: 24px; max-width: 1200px; }
table { width: 100%; border-collapse: collapse; }
th, td { padding: 8px 12px; text-align: left; border-bottom: 1px solid #21262d; font-size: 13px; }
th { color: #8b949e; font-weight: 600; font-size: 11px; text-transform: uppercase; letter-spacing: .05em; }
.badge { display: inline-block; padding: 2px 7px; border-radius: 3px; font-size: 11px; font-weight: 700; }
.badge-debug { background: #21262d; color: #8b949e; }
.badge-info  { background: #1a3a5c; color: #58a6ff; }
.badge-warn  { background: #3a2a00; color: #d29922; }
.badge-error { background: #3a1a1a; color: #f85149; }
.badge-auto   { background: #1a3a1a; color: #3fb950; }
.badge-manual { background: #3a2a00; color: #d29922; }
.stat-card { background: #161b22; border: 1px solid #30363d; border-radius: 6px; padding: 16px 20px; display: inline-block; min-width: 140px; margin: 0 8px 8px 0; vertical-align: top; }
.stat-card .label { font-size: 11px; color: #8b949e; text-transform: uppercase; letter-spacing: .05em; margin-bottom: 6px; }
.stat-card .value { font-size: 24px; font-weight: 700; }
.bar-chart { display: flex; align-items: flex-end; gap: 3px; height: 80px; margin-top: 16px; }
.bar-chart .bar { background: #1f6feb; min-width: 14px; flex: 1; border-radius: 2px 2px 0 0; }
.bar-chart .bar:hover { background: #388bfd; }
pre { background: #161b22; border: 1px solid #30363d; border-radius: 6px; padding: 16px; font-size: 12px; overflow-x: auto; color: #e6edf3; }
select, input[type=text], input[type=datetime-local] { background: #21262d; border: 1px solid #30363d; color: #e6edf3; padding: 6px 10px; border-radius: 4px; font-size: 13px; }
button, input[type=submit] { background: #21262d; border: 1px solid #30363d; color: #e6edf3; padding: 5px 12px; border-radius: 4px; font-size: 13px; cursor: pointer; }
button:hover { background: #30363d; }
button.primary { background: #1f6feb; border-color: #1f6feb; color: #fff; }
button.primary:hover { background: #388bfd; }
button.danger { background: #6e2828; border-color: #6e2828; color: #f85149; }
.form-row { display: flex; gap: 8px; align-items: center; flex-wrap: wrap; margin-bottom: 12px; }
h2 { font-size: 16px; font-weight: 600; margin-bottom: 16px; color: #e6edf3; }
</style>
</head>
<body>
{{template "page" .}}
</body>
</html>
```

Note: the `base.html` template defines the outer shell but relies on each page template defining a `"page"` template block. Each page template file will define `{{define "page"}}...{{end}}` containing the full page HTML (top-bar + content).

Since Go's `html/template` doesn't support inheritance directly, each page template defines a `"page"` named template that includes the nav and content. The `render` helper calls `ExecuteTemplate(w, name, data)` where `name` is e.g. `"logs"`.

Rewrite `base.html` to just hold the CSS and scripts as a separate `"base-css"` template:

```html
<!-- internal/dashboard/templates/base.html -->
{{define "base-css"}}
<style>
* { box-sizing: border-box; margin: 0; padding: 0; }
body { background: #0d1117; color: #e6edf3; font-family: -apple-system, BlinkMacSystemFont, "Segoe UI", monospace; font-size: 14px; }
a { color: #58a6ff; text-decoration: none; }
.top-bar { background: #161b22; border-bottom: 1px solid #30363d; padding: 0 24px; display: flex; align-items: center; gap: 24px; height: 48px; }
.top-bar .brand { font-weight: 700; color: #58a6ff; font-size: 15px; letter-spacing: .04em; }
.tabs { display: flex; gap: 2px; }
.tabs a { padding: 6px 14px; border-radius: 4px; color: #8b949e; font-size: 13px; }
.tabs a.active { background: #1f6feb; color: #fff; font-weight: 600; }
.tabs a:hover:not(.active) { background: #21262d; color: #e6edf3; }
.content { padding: 24px; max-width: 1200px; }
table { width: 100%; border-collapse: collapse; }
th, td { padding: 8px 12px; text-align: left; border-bottom: 1px solid #21262d; font-size: 13px; }
th { color: #8b949e; font-weight: 600; font-size: 11px; text-transform: uppercase; letter-spacing: .05em; }
.badge { display: inline-block; padding: 2px 7px; border-radius: 3px; font-size: 11px; font-weight: 700; }
.badge-debug { background: #21262d; color: #8b949e; }
.badge-info  { background: #1a3a5c; color: #58a6ff; }
.badge-warn  { background: #3a2a00; color: #d29922; }
.badge-error { background: #3a1a1a; color: #f85149; }
.badge-auto   { background: #1a3a1a; color: #3fb950; }
.badge-manual { background: #3a2a00; color: #d29922; }
.stat-card { background: #161b22; border: 1px solid #30363d; border-radius: 6px; padding: 16px 20px; display: inline-block; min-width: 140px; margin: 0 8px 8px 0; vertical-align: top; }
.stat-card .label { font-size: 11px; color: #8b949e; text-transform: uppercase; letter-spacing: .05em; margin-bottom: 6px; }
.stat-card .value { font-size: 24px; font-weight: 700; }
.bar-chart { display: flex; align-items: flex-end; gap: 3px; height: 80px; margin-top: 16px; }
.bar-chart .bar { background: #1f6feb; min-width: 14px; flex: 1; border-radius: 2px 2px 0 0; }
.bar-chart .bar:hover { background: #388bfd; }
pre { background: #161b22; border: 1px solid #30363d; border-radius: 6px; padding: 16px; font-size: 12px; overflow-x: auto; color: #e6edf3; }
select, input[type=text], input[type=datetime-local] { background: #21262d; border: 1px solid #30363d; color: #e6edf3; padding: 6px 10px; border-radius: 4px; font-size: 13px; }
button, input[type=submit] { background: #21262d; border: 1px solid #30363d; color: #e6edf3; padding: 5px 12px; border-radius: 4px; font-size: 13px; cursor: pointer; }
button:hover { background: #30363d; }
button.primary { background: #1f6feb; border-color: #1f6feb; color: #fff; }
button.primary:hover { background: #388bfd; }
button.danger { background: #6e2828; border-color: #6e2828; color: #f85149; }
.form-row { display: flex; gap: 8px; align-items: center; flex-wrap: wrap; margin-bottom: 12px; }
h2 { font-size: 16px; font-weight: 600; margin-bottom: 16px; color: #e6edf3; }
</style>
<script src="https://unpkg.com/htmx.org@2.0.4"></script>
{{end}}

{{define "nav"}}
<div class="top-bar">
  <span class="brand">VEXILBOT</span>
  <nav class="tabs">
    <a href="/logs"     {{if eq .Tab "logs"}}class="active"{{end}}>Logs</a>
    <a href="/events"   {{if eq .Tab "events"}}class="active"{{end}}>Events</a>
    <a href="/releases" {{if eq .Tab "releases"}}class="active"{{end}}>Releases</a>
    <a href="/config"   {{if eq .Tab "config"}}class="active"{{end}}>Config</a>
    <a href="/storage"  {{if eq .Tab "storage"}}class="active"{{end}}>Storage</a>
  </nav>
</div>
{{end}}
```

- [ ] **Step 5: Run test to verify it passes**

```bash
go test ./internal/dashboard/ -run TestServer -v
```

Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add internal/dashboard/
git commit -m "feat: dashboard server skeleton with base template and routing"
```

---

### Task 4: Logs handler

**Files:**
- Create: `internal/dashboard/handlers_logs.go`
- Create: `internal/dashboard/templates/logs.html`
- Test: add to `internal/dashboard/server_test.go`

- [ ] **Step 1: Write the failing test**

Add to `internal/dashboard/server_test.go`:

```go
func TestLogsFilterLevel(t *testing.T) {
	store := openTestStore(t, logentry.SchemaHash)
	// Append one INFO and one ERROR record
	writeLogRecord(t, store, logentry.LogEntry{Ts: 1000, Level: logentry.LogLevelInfo, Msg: "hello", Owner: "o", Repo: "r"})
	writeLogRecord(t, store, logentry.LogEntry{Ts: 2000, Level: logentry.LogLevelError, Msg: "boom", Owner: "o", Repo: "r"})

	srv := dashboard.New(dashboard.Deps{
		LogStore:     store,
		EventStore:   openTestStore(t, webhookevent.SchemaHash),
		ReleaseStore: openTestStore(t, scheduledrelease.SchemaHash),
		DataDir:      t.TempDir(),
		KnownRepos:   func() []string { return nil },
		RunRelease:   func(_ interface{}, _, _, _ string) (int, error) { return 0, nil },
	})
	rec := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/logs?level=error", nil)
	srv.ServeHTTP(rec, req)
	if rec.Code != 200 {
		t.Fatalf("want 200, got %d", rec.Code)
	}
	body := rec.Body.String()
	if !strings.Contains(body, "boom") {
		t.Error("expected 'boom' in response")
	}
	if strings.Contains(body, "hello") {
		t.Error("expected 'hello' filtered out")
	}
}

// helper: encode and append a LogEntry to a store
func writeLogRecord(t *testing.T, store *vexstore.AppendStore, e logentry.LogEntry) {
	t.Helper()
	w := vexilruntime.NewBitWriter()
	if err := e.Pack(w); err != nil {
		t.Fatal(err)
	}
	if err := store.Append(w.Finish()); err != nil {
		t.Fatal(err)
	}
}
```

Add import `vexilruntime "github.com/vexil-lang/vexil/packages/runtime-go"` to the test file imports.

- [ ] **Step 2: Run test to verify it fails**

```bash
go test ./internal/dashboard/ -run TestLogsFilter -v
```

Expected: FAIL — handlers_logs.go does not exist.

- [ ] **Step 3: Create handlers_logs.go**

```go
// internal/dashboard/handlers_logs.go
package dashboard

import (
	"net/http"
	"strings"
	"time"

	vexil "github.com/vexil-lang/vexil/packages/runtime-go"
	"github.com/vexil-lang/vexilbot/internal/vexstore/gen/logentry"
)

type logRow struct {
	Time  string
	Level string
	Msg   string
	Owner string
	Repo  string
}

type logsPageData struct {
	Tab         string
	Rows        []logRow
	FilterLevel string
	FilterOwner string
	FilterRepo  string
}

func (s *Server) handleLogs(w http.ResponseWriter, r *http.Request) {
	rows, err := s.readLogs(r)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	s.render(w, "logs", logsPageData{
		Tab:         "logs",
		Rows:        rows,
		FilterLevel: r.URL.Query().Get("level"),
		FilterOwner: r.URL.Query().Get("owner"),
		FilterRepo:  r.URL.Query().Get("repo"),
	})
}

func (s *Server) handleLogsRows(w http.ResponseWriter, r *http.Request) {
	rows, err := s.readLogs(r)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := s.tmpl.ExecuteTemplate(w, "logs-rows", rows); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

func (s *Server) readLogs(r *http.Request) ([]logRow, error) {
	records, err := s.deps.LogStore.ReadAll()
	if err != nil {
		return nil, err
	}
	filterLevel := strings.ToLower(r.URL.Query().Get("level"))
	filterOwner := r.URL.Query().Get("owner")
	filterRepo := r.URL.Query().Get("repo")

	var rows []logRow
	for _, rec := range records {
		br := vexil.NewBitReader(rec)
		var e logentry.LogEntry
		if err := e.Unpack(br); err != nil {
			continue
		}
		levelStr := logLevelStr(e.Level)
		if filterLevel != "" && strings.ToLower(levelStr) != filterLevel {
			continue
		}
		if filterOwner != "" && e.Owner != filterOwner {
			continue
		}
		if filterRepo != "" && e.Repo != filterRepo {
			continue
		}
		ts := time.Unix(0, int64(e.Ts)).UTC().Format("2006-01-02 15:04:05")
		rows = append(rows, logRow{Time: ts, Level: levelStr, Msg: e.Msg, Owner: e.Owner, Repo: e.Repo})
	}
	// Reverse so newest is first
	for i, j := 0, len(rows)-1; i < j; i, j = i+1, j-1 {
		rows[i], rows[j] = rows[j], rows[i]
	}
	return rows, nil
}

func logLevelStr(l logentry.LogLevel) string {
	switch l {
	case logentry.LogLevelDebug:
		return "DEBUG"
	case logentry.LogLevelInfo:
		return "INFO"
	case logentry.LogLevelWarn:
		return "WARN"
	case logentry.LogLevelError:
		return "ERROR"
	default:
		return "UNKNOWN"
	}
}
```

- [ ] **Step 4: Create logs.html**

```html
<!-- internal/dashboard/templates/logs.html -->
{{define "logs"}}
<!DOCTYPE html><html lang="en"><head><meta charset="UTF-8">
<title>Logs — vexilbot</title>
{{template "base-css" .}}
</head><body>
{{template "nav" .}}
<div class="content">
<h2>Logs</h2>
<form method="get" action="/logs" style="margin-bottom:16px">
  <div class="form-row">
    <select name="level" onchange="this.form.submit()">
      <option value="" {{if eq .FilterLevel ""}}selected{{end}}>All levels</option>
      <option value="debug" {{if eq .FilterLevel "debug"}}selected{{end}}>DEBUG</option>
      <option value="info"  {{if eq .FilterLevel "info"}}selected{{end}}>INFO</option>
      <option value="warn"  {{if eq .FilterLevel "warn"}}selected{{end}}>WARN</option>
      <option value="error" {{if eq .FilterLevel "error"}}selected{{end}}>ERROR</option>
    </select>
    <input type="text" name="owner" placeholder="owner" value="{{.FilterOwner}}">
    <input type="text" name="repo"  placeholder="repo"  value="{{.FilterRepo}}">
    <button type="submit">Filter</button>
  </div>
</form>
{{template "logs-rows" .Rows}}
</div>
</body></html>
{{end}}

{{define "logs-rows"}}
<table id="log-table" hx-get="/logs-rows" hx-trigger="every 3s" hx-swap="outerHTML">
  <thead><tr><th>Time</th><th>Level</th><th>Message</th><th>Owner</th><th>Repo</th></tr></thead>
  <tbody>
  {{range .}}
  <tr>
    <td style="font-family:monospace;color:#8b949e">{{.Time}}</td>
    <td><span class="badge badge-{{lower .Level}}">{{.Level}}</span></td>
    <td>{{.Msg}}</td>
    <td>{{.Owner}}</td>
    <td>{{.Repo}}</td>
  </tr>
  {{else}}
  <tr><td colspan="5" style="color:#8b949e;text-align:center">No log entries</td></tr>
  {{end}}
  </tbody>
</table>
{{end}}
```

The template uses a `lower` function. Add it to the template in `server.go`:

In `server.go`, change the `template.Must(template.ParseFS(...))` line to:

```go
tmpl := template.Must(
    template.New("").Funcs(template.FuncMap{
        "lower": strings.ToLower,
    }).ParseFS(templateFS, "templates/*.html"),
)
```

Add `"strings"` to the import in `server.go`.

- [ ] **Step 5: Run test to verify it passes**

```bash
go test ./internal/dashboard/ -run TestLogsFilter -v
```

Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add internal/dashboard/
git commit -m "feat: dashboard logs tab with level/owner/repo filtering and HTMX polling"
```

---

### Task 5: Events handler

**Files:**
- Create: `internal/dashboard/handlers_events.go`
- Create: `internal/dashboard/templates/events.html`
- Test: add to `internal/dashboard/server_test.go`

- [ ] **Step 1: Write the failing test**

Add to `internal/dashboard/server_test.go`:

```go
func TestEventsOK(t *testing.T) {
	evStore := openTestStore(t, webhookevent.SchemaHash)
	writeEventRecord(t, evStore, webhookevent.WebhookEvent{
		Ts: uint64(time.Now().UnixNano()), Kind: webhookevent.EventKindPush, Owner: "o", Repo: "r",
	})

	srv := dashboard.New(dashboard.Deps{
		LogStore:     openTestStore(t, logentry.SchemaHash),
		EventStore:   evStore,
		ReleaseStore: openTestStore(t, scheduledrelease.SchemaHash),
		DataDir:      t.TempDir(),
		KnownRepos:   func() []string { return nil },
		RunRelease:   func(_ interface{}, _, _, _ string) (int, error) { return 0, nil },
	})
	rec := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/events", nil)
	srv.ServeHTTP(rec, req)
	if rec.Code != 200 {
		t.Fatalf("want 200, got %d: %s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "Push") {
		t.Error("expected 'Push' in events response")
	}
}

func writeEventRecord(t *testing.T, store *vexstore.AppendStore, e webhookevent.WebhookEvent) {
	t.Helper()
	w := vexilruntime.NewBitWriter()
	if err := e.Pack(w); err != nil {
		t.Fatal(err)
	}
	if err := store.Append(w.Finish()); err != nil {
		t.Fatal(err)
	}
}
```

Add `"time"` to imports in the test file.

- [ ] **Step 2: Run test to verify it fails**

```bash
go test ./internal/dashboard/ -run TestEventsOK -v
```

Expected: FAIL.

- [ ] **Step 3: Create handlers_events.go**

```go
// internal/dashboard/handlers_events.go
package dashboard

import (
	"net/http"
	"time"

	vexil "github.com/vexil-lang/vexil/packages/runtime-go"
	"github.com/vexil-lang/vexilbot/internal/vexstore/gen/webhookevent"
)

type eventKindCount struct {
	Kind  string
	Count int
}

type hourBucket struct {
	Label string // "15:00"
	Count int
	Pct   int // 0-100 for bar height
}

type eventsPageData struct {
	Tab        string
	TotalToday int
	ByKind     []eventKindCount
	Hourly     []hourBucket
}

func (s *Server) handleEvents(w http.ResponseWriter, r *http.Request) {
	records, err := s.deps.EventStore.ReadAll()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	now := time.Now().UTC()
	todayStart := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.UTC)
	cutoff := now.Add(-24 * time.Hour)

	kindCounts := make(map[string]int)
	hourCounts := make(map[int]int) // hour 0-23 relative to now-24h

	var totalToday int
	for _, rec := range records {
		br := vexil.NewBitReader(rec)
		var e webhookevent.WebhookEvent
		if err := e.Unpack(br); err != nil {
			continue
		}
		ts := time.Unix(0, int64(e.Ts)).UTC()
		if ts.After(todayStart) {
			totalToday++
		}
		if ts.After(cutoff) {
			kindCounts[eventKindStr(e.Kind)]++
			// Which 1-hour bucket? (0 = oldest hour, 23 = most recent)
			hoursAgo := int(now.Sub(ts).Hours())
			if hoursAgo >= 0 && hoursAgo < 24 {
				hourCounts[23-hoursAgo]++
			}
		}
	}

	// Build ordered kind list
	kindOrder := []string{"Push", "PullRequest", "Issues", "IssueComment", "Unknown"}
	var byKind []eventKindCount
	for _, k := range kindOrder {
		if c := kindCounts[k]; c > 0 {
			byKind = append(byKind, eventKindCount{Kind: k, Count: c})
		}
	}

	// Find max for percentage scaling
	maxH := 1
	for _, c := range hourCounts {
		if c > maxH {
			maxH = c
		}
	}

	// Build 24 hourly buckets (oldest to newest)
	var hourly []hourBucket
	for i := 0; i < 24; i++ {
		label := now.Add(time.Duration(i-23) * time.Hour).Format("15:00")
		c := hourCounts[i]
		hourly = append(hourly, hourBucket{Label: label, Count: c, Pct: c * 100 / maxH})
	}

	s.render(w, "events", eventsPageData{
		Tab:        "events",
		TotalToday: totalToday,
		ByKind:     byKind,
		Hourly:     hourly,
	})
}

func eventKindStr(k webhookevent.EventKind) string {
	switch k {
	case webhookevent.EventKindPush:
		return "Push"
	case webhookevent.EventKindPullRequest:
		return "PullRequest"
	case webhookevent.EventKindIssues:
		return "Issues"
	case webhookevent.EventKindIssueComment:
		return "IssueComment"
	default:
		return "Unknown"
	}
}
```

- [ ] **Step 4: Create events.html**

```html
<!-- internal/dashboard/templates/events.html -->
{{define "events"}}
<!DOCTYPE html><html lang="en"><head><meta charset="UTF-8">
<title>Events — vexilbot</title>
{{template "base-css" .}}
</head><body>
{{template "nav" .}}
<div class="content">
<h2>Events</h2>
<div style="margin-bottom:24px">
  <div class="stat-card">
    <div class="label">Today</div>
    <div class="value">{{.TotalToday}}</div>
  </div>
  {{range .ByKind}}
  <div class="stat-card">
    <div class="label">{{.Kind}}</div>
    <div class="value">{{.Count}}</div>
  </div>
  {{end}}
</div>
<h2>Last 24 hours</h2>
<div class="bar-chart" title="hourly event count">
  {{range .Hourly}}
  <div class="bar" style="height:{{if .Pct}}{{.Pct}}%{{else}}2px{{end}}" title="{{.Label}}: {{.Count}}"></div>
  {{end}}
</div>
<div style="display:flex;justify-content:space-between;margin-top:4px;font-size:10px;color:#8b949e">
  {{with index .Hourly 0}}<span>{{.Label}}</span>{{end}}
  {{with index .Hourly 23}}<span>{{.Label}}</span>{{end}}
</div>
</div>
</body></html>
{{end}}
```

- [ ] **Step 5: Run test to verify it passes**

```bash
go test ./internal/dashboard/ -run TestEventsOK -v
```

Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add internal/dashboard/
git commit -m "feat: dashboard events tab with stat cards and 24h bar chart"
```

---

### Task 6: Releases handler and RunReleaseNow

**Files:**
- Create: `internal/release/run_now.go`
- Create: `internal/dashboard/handlers_releases.go`
- Create: `internal/dashboard/templates/releases.html`
- Test: `internal/release/run_now_test.go` + `internal/dashboard/server_test.go`

- [ ] **Step 1: Write the failing release unit test**

```go
// internal/release/run_now_test.go
package release_test

import (
	"context"
	"errors"
	"testing"

	"github.com/vexil-lang/vexilbot/internal/release"
	"github.com/vexil-lang/vexilbot/internal/repoconfig"
)

func TestRunReleaseNowUnknownPackage(t *testing.T) {
	cfg := repoconfig.Release{} // no crates or packages
	_, err := release.RunReleaseNow(context.Background(), nil, "owner", "repo", "nonexistent", cfg)
	if err == nil {
		t.Fatal("expected error for unknown package")
	}
	if !errors.Is(err, release.ErrNotFound) {
		t.Errorf("expected ErrNotFound, got %v", err)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

```bash
go test ./internal/release/ -run TestRunReleaseNowUnknownPackage -v
```

Expected: FAIL.

- [ ] **Step 3: Create internal/release/run_now.go**

```go
// internal/release/run_now.go
package release

import (
	"context"
	"errors"
	"fmt"

	"github.com/vexil-lang/vexilbot/internal/repoconfig"
)

// ErrNotFound is returned by RunReleaseNow when the package/crate name is not
// found in the repo config.
var ErrNotFound = errors.New("package not found in release config")

// RunReleaseNow creates a release PR for the named crate or npm package and
// returns the new PR number. Unlike RunRelease, it does not post a comment to
// any GitHub issue. Intended for dashboard "Run now" actions.
func RunReleaseNow(ctx context.Context, api ReleaseAPI, owner, repo, name string, cfg repoconfig.Release) (int, error) {
	if crate, ok := cfg.Crates[name]; ok {
		return createCratePR(ctx, api, owner, repo, name, crate, cfg)
	}
	if pkg, ok := cfg.Packages[name]; ok {
		return createNpmPR(ctx, api, owner, repo, name, pkg, cfg)
	}
	return 0, fmt.Errorf("%w: %s", ErrNotFound, name)
}
```

- [ ] **Step 4: Run release test to verify it passes**

```bash
go test ./internal/release/ -run TestRunReleaseNowUnknownPackage -v
```

Expected: PASS.

- [ ] **Step 5: Write the failing dashboard releases test**

Add to `internal/dashboard/server_test.go`:

```go
func TestReleasesOK(t *testing.T) {
	srv := newTestServer(t)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/releases", nil)
	srv.ServeHTTP(rec, req)
	if rec.Code != 200 {
		t.Fatalf("want 200, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestReleasesCreate(t *testing.T) {
	srv := newTestServer(t)
	form := "package=mylib&bump=patch&run_at=2026-04-01T10%3A00&auto_run=on&owner=o&repo=r"
	rec := httptest.NewRecorder()
	req := httptest.NewRequest("POST", "/releases", strings.NewReader(form))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	srv.ServeHTTP(rec, req)
	// Expect redirect back to /releases
	if rec.Code != http.StatusFound {
		t.Errorf("want 302, got %d: %s", rec.Code, rec.Body.String())
	}
}
```

- [ ] **Step 6: Run dashboard releases test to verify it fails**

```bash
go test ./internal/dashboard/ -run TestReleases -v
```

Expected: FAIL.

- [ ] **Step 7: Create handlers_releases.go**

```go
// internal/dashboard/handlers_releases.go
package dashboard

import (
	"crypto/rand"
	"fmt"
	"net/http"
	"sort"
	"time"

	vexil "github.com/vexil-lang/vexil/packages/runtime-go"
	"github.com/vexil-lang/vexilbot/internal/vexstore/gen/scheduledrelease"
)

type releaseRow struct {
	ID        string
	Package   string
	Bump      string
	Owner     string
	Repo      string
	RunAt     string
	AutoRun   bool
	Status    string
}

type releasesPageData struct {
	Tab  string
	Rows []releaseRow
}

func (s *Server) handleReleases(w http.ResponseWriter, r *http.Request) {
	rows, err := s.readReleases()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	s.render(w, "releases", releasesPageData{Tab: "releases", Rows: rows})
}

func (s *Server) handleReleasesCreate(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	pkg := r.FormValue("package")
	bumpStr := r.FormValue("bump")
	runAtStr := r.FormValue("run_at")
	autoRun := r.FormValue("auto_run") == "on"
	owner := r.FormValue("owner")
	repo := r.FormValue("repo")

	runAt, err := time.ParseInLocation("2006-01-02T15:04", runAtStr, time.UTC)
	if err != nil {
		http.Error(w, "invalid run_at: "+err.Error(), http.StatusBadRequest)
		return
	}

	sr := scheduledrelease.ScheduledRelease{
		ID:        generateUUID(),
		Package:   pkg,
		Bump:      parseBumpLevel(bumpStr),
		RunAt:     uint64(runAt.UnixNano()),
		AutoRun:   autoRun,
		Status:    scheduledrelease.ReleaseStatusPending,
		CreatedAt: uint64(time.Now().UnixNano()),
		Owner:     owner,
		Repo:      repo,
	}
	if err := s.appendRelease(&sr); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	http.Redirect(w, r, "/releases", http.StatusFound)
}

func (s *Server) handleReleasesCancel(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if err := s.updateReleaseStatus(id, scheduledrelease.ReleaseStatusCancelled); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	http.Redirect(w, r, "/releases", http.StatusFound)
}

func (s *Server) handleReleasesConfirm(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if err := s.updateReleaseStatus(id, scheduledrelease.ReleaseStatusConfirmed); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	http.Redirect(w, r, "/releases", http.StatusFound)
}

func (s *Server) handleReleasesRun(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	rows, err := s.readReleases()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	var target *releaseRow
	for i := range rows {
		if rows[i].ID == id {
			target = &rows[i]
			break
		}
	}
	if target == nil {
		http.Error(w, "release not found", http.StatusNotFound)
		return
	}
	if err := s.updateReleaseStatus(id, scheduledrelease.ReleaseStatusRunning); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	go func() {
		_, runErr := s.deps.RunRelease(r.Context(), target.Owner, target.Repo, target.Package)
		status := scheduledrelease.ReleaseStatusDone
		if runErr != nil {
			status = scheduledrelease.ReleaseStatusCancelled // mark cancelled on error
		}
		_ = s.updateReleaseStatus(id, status)
	}()
	http.Redirect(w, r, "/releases", http.StatusFound)
}

// readReleases returns the latest state per release ID, excluding cancelled,
// sorted by run_at ascending.
func (s *Server) readReleases() ([]releaseRow, error) {
	records, err := s.deps.ReleaseStore.ReadAll()
	if err != nil {
		return nil, err
	}
	latest := make(map[string]*scheduledrelease.ScheduledRelease)
	for _, rec := range records {
		br := vexil.NewBitReader(rec)
		var sr scheduledrelease.ScheduledRelease
		if err := sr.Unpack(br); err != nil {
			continue
		}
		latest[sr.ID] = &sr
	}
	var rows []releaseRow
	for _, sr := range latest {
		if sr.Status == scheduledrelease.ReleaseStatusCancelled ||
			sr.Status == scheduledrelease.ReleaseStatusDone {
			continue
		}
		rows = append(rows, releaseRow{
			ID:      sr.ID,
			Package: sr.Package,
			Bump:    bumpLevelStr(sr.Bump),
			Owner:   sr.Owner,
			Repo:    sr.Repo,
			RunAt:   time.Unix(0, int64(sr.RunAt)).UTC().Format("2006-01-02 15:04 UTC"),
			AutoRun: sr.AutoRun,
			Status:  releaseStatusStr(sr.Status),
		})
	}
	sort.Slice(rows, func(i, j int) bool { return rows[i].RunAt < rows[j].RunAt })
	return rows, nil
}

func (s *Server) appendRelease(sr *scheduledrelease.ScheduledRelease) error {
	w := vexil.NewBitWriter()
	if err := sr.Pack(w); err != nil {
		return err
	}
	return s.deps.ReleaseStore.Append(w.Finish())
}

func (s *Server) updateReleaseStatus(id string, status scheduledrelease.ReleaseStatus) error {
	records, err := s.deps.ReleaseStore.ReadAll()
	if err != nil {
		return err
	}
	var found *scheduledrelease.ScheduledRelease
	for _, rec := range records {
		br := vexil.NewBitReader(rec)
		var sr scheduledrelease.ScheduledRelease
		if sr.Unpack(br) == nil && sr.ID == id {
			found = &sr
		}
	}
	if found == nil {
		return fmt.Errorf("release %s not found", id)
	}
	found.Status = status
	return s.appendRelease(found)
}

func generateUUID() string {
	var b [16]byte
	_, _ = rand.Read(b[:])
	b[6] = (b[6] & 0x0f) | 0x40
	b[8] = (b[8] & 0x3f) | 0x80
	return fmt.Sprintf("%x-%x-%x-%x-%x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:])
}

func parseBumpLevel(s string) scheduledrelease.BumpLevel {
	switch s {
	case "minor":
		return scheduledrelease.BumpLevelMinor
	case "major":
		return scheduledrelease.BumpLevelMajor
	default:
		return scheduledrelease.BumpLevelPatch
	}
}

func bumpLevelStr(b scheduledrelease.BumpLevel) string {
	switch b {
	case scheduledrelease.BumpLevelMinor:
		return "minor"
	case scheduledrelease.BumpLevelMajor:
		return "major"
	default:
		return "patch"
	}
}

func releaseStatusStr(s scheduledrelease.ReleaseStatus) string {
	switch s {
	case scheduledrelease.ReleaseStatusConfirmed:
		return "confirmed"
	case scheduledrelease.ReleaseStatusRunning:
		return "running"
	case scheduledrelease.ReleaseStatusDone:
		return "done"
	case scheduledrelease.ReleaseStatusCancelled:
		return "cancelled"
	default:
		return "pending"
	}
}
```

- [ ] **Step 8: Create releases.html**

```html
<!-- internal/dashboard/templates/releases.html -->
{{define "releases"}}
<!DOCTYPE html><html lang="en"><head><meta charset="UTF-8">
<title>Releases — vexilbot</title>
{{template "base-css" .}}
</head><body>
{{template "nav" .}}
<div class="content">
<h2>Scheduled Releases</h2>
<table>
  <thead><tr><th>Package</th><th>Bump</th><th>Owner/Repo</th><th>Run At</th><th>Mode</th><th>Status</th><th>Actions</th></tr></thead>
  <tbody>
  {{range .Rows}}
  <tr>
    <td>{{.Package}}</td>
    <td>{{.Bump}}</td>
    <td>{{.Owner}}/{{.Repo}}</td>
    <td style="font-family:monospace;color:#8b949e">{{.RunAt}}</td>
    <td>
      {{if .AutoRun}}<span class="badge badge-auto">auto</span>
      {{else}}<span class="badge badge-manual">manual</span>{{end}}
    </td>
    <td>{{.Status}}</td>
    <td style="display:flex;gap:6px">
      {{if eq .Status "pending"}}
        <form method="post" action="/releases/{{.ID}}/run">
          <button class="primary" type="submit">Run now</button>
        </form>
        {{if not .AutoRun}}
        <form method="post" action="/releases/{{.ID}}/confirm">
          <button type="submit">Confirm</button>
        </form>
        {{end}}
      {{end}}
      <form method="post" action="/releases/{{.ID}}/cancel">
        <button class="danger" type="submit">Cancel</button>
      </form>
    </td>
  </tr>
  {{else}}
  <tr><td colspan="7" style="color:#8b949e;text-align:center">No pending releases</td></tr>
  {{end}}
  </tbody>
</table>

<details style="margin-top:24px">
  <summary style="cursor:pointer;color:#58a6ff;font-size:13px">+ Schedule a release</summary>
  <form method="post" action="/releases" style="margin-top:12px;background:#161b22;border:1px solid #30363d;border-radius:6px;padding:16px">
    <div class="form-row">
      <input type="text" name="owner" placeholder="owner" required>
      <input type="text" name="repo"  placeholder="repo"  required>
      <input type="text" name="package" placeholder="package / crate" required>
      <select name="bump">
        <option value="patch">patch</option>
        <option value="minor">minor</option>
        <option value="major">major</option>
      </select>
      <input type="datetime-local" name="run_at" required>
      <label><input type="checkbox" name="auto_run"> Auto-run</label>
      <button class="primary" type="submit">Schedule</button>
    </div>
  </form>
</details>
</div>
</body></html>
{{end}}
```

- [ ] **Step 9: Run all dashboard tests**

```bash
go test ./internal/dashboard/ ./internal/release/ -v
```

Expected: all PASS.

- [ ] **Step 10: Commit**

```bash
git add internal/dashboard/ internal/release/run_now.go internal/release/run_now_test.go
git commit -m "feat: dashboard releases tab with schedule/cancel/confirm/run-now"
```

---

### Task 7: Config handler

**Files:**
- Create: `internal/dashboard/handlers_config.go`
- Create: `internal/dashboard/templates/config.html`
- Test: add to `internal/dashboard/server_test.go`

- [ ] **Step 1: Write the failing test**

Add to `internal/dashboard/server_test.go`:

```go
func TestConfigOK(t *testing.T) {
	srv := newTestServer(t)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/config", nil)
	srv.ServeHTTP(rec, req)
	if rec.Code != 200 {
		t.Fatalf("want 200, got %d: %s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "[redacted]") {
		t.Error("expected secrets to be redacted in config view")
	}
}
```

Also update `newTestServer` to pass a non-nil `ServerConfig` with dummy secrets:

```go
func newTestServer(t *testing.T) *dashboard.Server {
	t.Helper()
	cfg := &serverconfig.Config{}
	cfg.Server.WebhookSecret = "secret"
	cfg.GitHub.PrivateKeyPath = "/tmp/key"
	cfg.Credentials.CargoRegistryToken = "token"
	cfg.LLM.AnthropicAPIKey = "key"
	return dashboard.New(dashboard.Deps{
		LogStore:     openTestStore(t, logentry.SchemaHash),
		EventStore:   openTestStore(t, webhookevent.SchemaHash),
		ReleaseStore: openTestStore(t, scheduledrelease.SchemaHash),
		DataDir:      t.TempDir(),
		ServerConfig: cfg,
		KnownRepos:   func() []string { return []string{"owner/repo"} },
		RunRelease:   func(_ context.Context, _, _, _ string) (int, error) { return 0, nil },
		FetchRepoConfig: func(_ context.Context, _, _ string) ([]byte, error) {
			return []byte(`[labels]`), nil
		},
	})
}
```

Update the `Deps` struct and `RunRelease` signature to use `context.Context` instead of `interface{}`. Fix all prior test helpers accordingly.

- [ ] **Step 2: Run test to verify it fails**

```bash
go test ./internal/dashboard/ -run TestConfigOK -v
```

Expected: FAIL.

- [ ] **Step 3: Update Deps struct in server.go**

Add two new fields to `Deps`:

```go
type Deps struct {
	LogStore        *vexstore.AppendStore
	EventStore      *vexstore.AppendStore
	ReleaseStore    *vexstore.AppendStore
	DataDir         string
	ServerConfig    *serverconfig.Config
	KnownRepos      func() []string
	RunRelease      func(ctx context.Context, owner, repo, pkg string) (int, error)
	// FetchRepoConfig fetches the raw .vexilbot.toml bytes for owner/repo from GitHub.
	FetchRepoConfig func(ctx context.Context, owner, repo string) ([]byte, error)
}
```

Add import `"github.com/vexil-lang/vexilbot/internal/serverconfig"` to server.go.

- [ ] **Step 4: Create handlers_config.go**

```go
// internal/dashboard/handlers_config.go
package dashboard

import (
	"fmt"
	"net/http"
	"strings"
)

type configPageData struct {
	Tab         string
	ServerTOML  string
	KnownRepos  []string
	SelectedOwner string
	SelectedRepo  string
	RepoCOML    string
	RepoError   string
}

func (s *Server) handleConfig(w http.ResponseWriter, r *http.Request) {
	s.render(w, "config", configPageData{
		Tab:        "config",
		ServerTOML: redactedServerTOML(s.deps.ServerConfig),
		KnownRepos: s.deps.KnownRepos(),
	})
}

func (s *Server) handleConfigRepo(w http.ResponseWriter, r *http.Request) {
	owner := r.URL.Query().Get("owner")
	repo := r.URL.Query().Get("repo")
	if owner == "" || repo == "" {
		http.Error(w, "owner and repo are required", http.StatusBadRequest)
		return
	}
	data, err := s.deps.FetchRepoConfig(r.Context(), owner, repo)
	d := configPageData{
		Tab:           "config",
		ServerTOML:    redactedServerTOML(s.deps.ServerConfig),
		KnownRepos:    s.deps.KnownRepos(),
		SelectedOwner: owner,
		SelectedRepo:  repo,
	}
	if err != nil {
		d.RepoError = err.Error()
	} else {
		d.RepoCOML = string(data)
	}
	s.render(w, "config", d)
}

// redactedServerTOML renders the server config as TOML with secrets replaced.
func redactedServerTOML(cfg *serverconfig.Config) string {
	if cfg == nil {
		return ""
	}
	var sb strings.Builder
	fmt.Fprintf(&sb, "[server]\n")
	fmt.Fprintf(&sb, "listen = %q\n", cfg.Server.Listen)
	fmt.Fprintf(&sb, "webhook_secret = \"[redacted]\"\n")
	fmt.Fprintf(&sb, "bot_name = %q\n", cfg.Server.BotName)
	fmt.Fprintf(&sb, "data_dir = %q\n", cfg.Server.DataDir)
	fmt.Fprintf(&sb, "dashboard_port = %d\n", cfg.Server.DashboardPort)
	fmt.Fprintf(&sb, "\n[github]\n")
	fmt.Fprintf(&sb, "app_id = %d\n", cfg.GitHub.AppID)
	fmt.Fprintf(&sb, "private_key_path = \"[redacted]\"\n")
	fmt.Fprintf(&sb, "\n[credentials]\n")
	if cfg.Credentials.CargoRegistryToken != "" {
		fmt.Fprintf(&sb, "cargo_registry_token = \"[redacted]\"\n")
	}
	fmt.Fprintf(&sb, "\n[llm]\n")
	if cfg.LLM.AnthropicAPIKey != "" {
		fmt.Fprintf(&sb, "anthropic_api_key = \"[redacted]\"\n")
	}
	return sb.String()
}
```

- [ ] **Step 5: Create config.html**

```html
<!-- internal/dashboard/templates/config.html -->
{{define "config"}}
<!DOCTYPE html><html lang="en"><head><meta charset="UTF-8">
<title>Config — vexilbot</title>
{{template "base-css" .}}
</head><body>
{{template "nav" .}}
<div class="content">
<h2>Server Config</h2>
<pre>{{.ServerTOML}}</pre>

<h2 style="margin-top:24px">Repo Config</h2>
<form hx-get="/config/repo" hx-target="#repo-content" hx-swap="innerHTML" hx-trigger="change" style="margin-bottom:12px">
  <select name="owner-repo" onchange="
    var parts=this.value.split('/');
    this.form.elements['owner'].value=parts[0];
    this.form.elements['repo'].value=parts[1];
    htmx.trigger(this.form,'change')
  ">
    <option value="">Select a repo…</option>
    {{range .KnownRepos}}
    <option value="{{.}}" {{if eq . (printf "%s/%s" $.SelectedOwner $.SelectedRepo)}}selected{{end}}>{{.}}</option>
    {{end}}
  </select>
  <input type="hidden" name="owner" value="{{.SelectedOwner}}">
  <input type="hidden" name="repo"  value="{{.SelectedRepo}}">
</form>
<div id="repo-content">
  {{if .RepoError}}<p style="color:#f85149">{{.RepoError}}</p>
  {{else if .RepoCOML}}<pre>{{.RepoCOML}}</pre>
  {{end}}
</div>
</div>
</body></html>
{{end}}
```

- [ ] **Step 6: Run test to verify it passes**

```bash
go test ./internal/dashboard/ -run TestConfigOK -v
```

Expected: PASS.

- [ ] **Step 7: Commit**

```bash
git add internal/dashboard/
git commit -m "feat: dashboard config tab with redacted server config and repo config viewer"
```

---

### Task 8: Storage handler

**Files:**
- Create: `internal/dashboard/handlers_storage.go`
- Create: `internal/dashboard/templates/storage.html`
- Test: add to `internal/dashboard/server_test.go`

- [ ] **Step 1: Write the failing test**

Add to `internal/dashboard/server_test.go`:

```go
func TestStorageOK(t *testing.T) {
	dir := t.TempDir()
	// Create a test .vxb file in the dir
	store, err := vexstore.OpenAppendStore(dir+"/test.vxb", logentry.SchemaHash)
	if err != nil {
		t.Fatal(err)
	}
	writeLogRecord(t, store, logentry.LogEntry{Ts: 1000, Level: logentry.LogLevelInfo, Msg: "hi"})
	store.Close()

	srv := dashboard.New(dashboard.Deps{
		LogStore:     openTestStore(t, logentry.SchemaHash),
		EventStore:   openTestStore(t, webhookevent.SchemaHash),
		ReleaseStore: openTestStore(t, scheduledrelease.SchemaHash),
		DataDir:      dir,
		ServerConfig: &serverconfig.Config{},
		KnownRepos:   func() []string { return nil },
		RunRelease:   func(_ context.Context, _, _, _ string) (int, error) { return 0, nil },
		FetchRepoConfig: func(_ context.Context, _, _ string) ([]byte, error) { return nil, nil },
	})
	rec := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/storage", nil)
	srv.ServeHTTP(rec, req)
	if rec.Code != 200 {
		t.Fatalf("want 200, got %d: %s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "test.vxb") {
		t.Error("expected 'test.vxb' in storage response")
	}
	if !strings.Contains(rec.Body.String(), "1") {
		t.Error("expected record count of 1 in storage response")
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

```bash
go test ./internal/dashboard/ -run TestStorageOK -v
```

Expected: FAIL.

- [ ] **Step 3: Create handlers_storage.go**

```go
// internal/dashboard/handlers_storage.go
package dashboard

import (
	"encoding/binary"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"time"
)

type storageRow struct {
	File       string
	Size       string
	Records    int
	SchemaHash string
	Oldest     string
	Newest     string
	Error      string
}

type storagePageData struct {
	Tab  string
	Rows []storageRow
}

func (s *Server) handleStorage(w http.ResponseWriter, r *http.Request) {
	matches, err := filepath.Glob(filepath.Join(s.deps.DataDir, "*.vxb"))
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	var rows []storageRow
	for _, path := range matches {
		rows = append(rows, inspectVxb(path))
	}
	s.render(w, "storage", storagePageData{Tab: "storage", Rows: rows})
}

func inspectVxb(path string) storageRow {
	row := storageRow{File: filepath.Base(path)}
	f, err := os.Open(path)
	if err != nil {
		row.Error = err.Error()
		return row
	}
	defer f.Close()

	info, _ := f.Stat()
	row.Size = humanSize(info.Size())

	// Read magic (4) + schema hash (32)
	var hdr [36]byte
	if _, err := io.ReadFull(f, hdr[:]); err != nil {
		row.Error = "bad header"
		return row
	}
	row.SchemaHash = fmt.Sprintf("%x", hdr[4:8]) // first 4 bytes of hash for brevity

	// Count records and track timestamps (records store u64 ts as first 8 bytes of payload)
	var count int
	var oldest, newest uint64
	var lenBuf [4]byte
	for {
		_, err := io.ReadFull(f, lenBuf[:])
		if err != nil {
			break
		}
		n := binary.LittleEndian.Uint32(lenBuf[:])
		payload := make([]byte, n)
		if _, err := io.ReadFull(f, payload); err != nil {
			break
		}
		count++
		// Extract first 8 bytes as u64 nanosecond timestamp (best effort)
		if len(payload) >= 8 {
			ts := binary.LittleEndian.Uint64(payload[:8])
			if ts > 0 {
				if oldest == 0 || ts < oldest {
					oldest = ts
				}
				if ts > newest {
					newest = ts
				}
			}
		}
	}
	row.Records = count
	if oldest > 0 {
		row.Oldest = time.Unix(0, int64(oldest)).UTC().Format("2006-01-02 15:04")
	}
	if newest > 0 {
		row.Newest = time.Unix(0, int64(newest)).UTC().Format("2006-01-02 15:04")
	}
	return row
}

func humanSize(n int64) string {
	switch {
	case n < 1024:
		return fmt.Sprintf("%d B", n)
	case n < 1024*1024:
		return fmt.Sprintf("%.1f KB", float64(n)/1024)
	default:
		return fmt.Sprintf("%.1f MB", float64(n)/1024/1024)
	}
}
```

**Note on timestamp extraction:** The storage handler reads the first 8 bytes of each payload as a little-endian u64 nanosecond timestamp. This is a best-effort heuristic that works for all three current schemas (logentry, webhookevent, scheduledrelease) since they all have `ts`/`created_at` as the first field. If a schema does not have a timestamp as field 0, oldest/newest will show wrong values but record count and size remain correct.

- [ ] **Step 4: Create storage.html**

```html
<!-- internal/dashboard/templates/storage.html -->
{{define "storage"}}
<!DOCTYPE html><html lang="en"><head><meta charset="UTF-8">
<title>Storage — vexilbot</title>
{{template "base-css" .}}
</head><body>
{{template "nav" .}}
<div class="content">
<h2>Storage</h2>
<table>
  <thead><tr><th>File</th><th>Size</th><th>Records</th><th>Schema (8 chars)</th><th>Oldest</th><th>Newest</th></tr></thead>
  <tbody>
  {{range .Rows}}
  <tr>
    <td style="font-family:monospace">{{.File}}</td>
    <td>{{.Size}}</td>
    <td>{{.Records}}</td>
    <td style="font-family:monospace;color:#8b949e">{{.SchemaHash}}</td>
    <td style="color:#8b949e">{{if .Oldest}}{{.Oldest}}{{else}}—{{end}}</td>
    <td style="color:#8b949e">{{if .Newest}}{{.Newest}}{{else}}—{{end}}</td>
  </tr>
  {{if .Error}}<tr><td colspan="6" style="color:#f85149">{{.Error}}</td></tr>{{end}}
  {{else}}
  <tr><td colspan="6" style="color:#8b949e;text-align:center">No .vxb files in data dir</td></tr>
  {{end}}
  </tbody>
</table>
</div>
</body></html>
{{end}}
```

- [ ] **Step 5: Run all dashboard tests**

```bash
go test ./internal/dashboard/ -v
```

Expected: all PASS.

- [ ] **Step 6: Commit**

```bash
git add internal/dashboard/
git commit -m "feat: dashboard storage tab — per-.vxb file stats"
```

---

### Task 9: Wire dashboard into main.go

**Files:**
- Modify: `cmd/vexilbot/main.go`

- [ ] **Step 1: Write the failing test (compile check)**

The wiring change is in `main.go` and isn't directly unit-testable, but we can verify the binary compiles:

```bash
go build ./...
```

If this fails at any point in this task, treat it as the "failing" condition and fix before proceeding.

- [ ] **Step 2: Add list() to installationStore**

In `cmd/vexilbot/main.go`, add a `list()` method to `installationStore`:

```go
func (s *installationStore) list() []string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]string, 0, len(s.entries))
	for k := range s.entries {
		out = append(out, k)
	}
	return out
}
```

- [ ] **Step 3: Open scheduledRelStore**

In `main()`, after the `eventStore` block, add:

```go
scheduledRelStore, err := vexstore.OpenAppendStore(cfg.Server.DataDir+"/scheduled_releases.vxb", scheduledrelease.SchemaHash)
if err != nil {
    fmt.Fprintf(os.Stderr, "open scheduled release store: %v\n", err)
    os.Exit(1)
}
defer scheduledRelStore.Close()
```

Add import `"github.com/vexil-lang/vexilbot/internal/vexstore/gen/scheduledrelease"` to the import block.

- [ ] **Step 4: Build the RunRelease callback**

In `main()`, after `configCache` is defined, add:

```go
runRelease := func(ctx context.Context, owner, repo, pkg string) (int, error) {
    id, ok := store.get(owner, repo)
    if !ok {
        return 0, fmt.Errorf("no installation ID known for %s/%s", owner, repo)
    }
    adapter := &ghAdapter{client: app.InstallationClient(id)}
    repoCfg, err := configCache.Get(ctx, owner, repo)
    if err != nil {
        return 0, fmt.Errorf("get repo config: %w", err)
    }
    return release.RunReleaseNow(ctx, adapter, owner, repo, pkg, repoCfg.Release)
}

fetchRepoConfig := func(ctx context.Context, owner, repo string) ([]byte, error) {
    id, ok := store.get(owner, repo)
    if !ok {
        return nil, fmt.Errorf("no installation ID known for %s/%s", owner, repo)
    }
    client := app.InstallationClient(id)
    return app.FetchRepoConfig(ctx, client, owner, repo)
}
```

- [ ] **Step 5: Create and start the dashboard server**

In `main()`, after the `mux` and `handler` setup, before `http.ListenAndServe`, add:

```go
if cfg.Server.DashboardPort != 0 {
    dashSrv := dashboard.New(dashboard.Deps{
        LogStore:        logStore,
        EventStore:      eventStore,
        ReleaseStore:    scheduledRelStore,
        DataDir:         cfg.Server.DataDir,
        ServerConfig:    cfg,
        KnownRepos:      store.list,
        RunRelease:      runRelease,
        FetchRepoConfig: fetchRepoConfig,
    })
    dashAddr := fmt.Sprintf("127.0.0.1:%d", cfg.Server.DashboardPort)
    go func() {
        slog.Info("dashboard starting", "listen", dashAddr)
        if err := http.ListenAndServe(dashAddr, dashSrv); err != nil {
            slog.Error("dashboard error", "error", err)
        }
    }()
}
```

Add import `"github.com/vexil-lang/vexilbot/internal/dashboard"` to the import block.

- [ ] **Step 6: Verify it compiles and tests pass**

```bash
go build ./...
go test ./...
```

Expected: build succeeds, all tests pass.

- [ ] **Step 7: Commit**

```bash
git add cmd/vexilbot/main.go
git commit -m "feat: wire dashboard server into main — second listener on 127.0.0.1:8081"
```

---

### Task 10: Docker volume for persistent data

**Files:**
- Modify: `docker-compose.yml`
- Modify: `docker-compose.override.yml`

- [ ] **Step 1: Update docker-compose.yml**

Add `vexilbot-data` volume at the top-level `volumes:` key and mount it in the `vexilbot` service:

```yaml
services:
  vexilbot:
    image: ghcr.io/vexil-lang/vexilbot:latest
    restart: unless-stopped
    expose:
      - "8080"
    volumes:
      - ./config.toml:/etc/vexilbot/config.toml:ro
      - ./secrets/github_app_key.pem:/run/secrets/github_app_key:ro
      - vexilbot-data:/data
    healthcheck:
      test: ["CMD", "/busybox/wget", "-qO-", "http://localhost:8080/healthz"]
      interval: 30s
      timeout: 5s
      retries: 3
      start_period: 5s
    logging:
      driver: json-file
      options:
        max-size: "10m"
        max-file: "5"

  caddy:
    image: caddy:2-alpine
    restart: unless-stopped
    ports:
      - "80:80"
      - "443:443"
      - "443:443/udp"
    volumes:
      - ./deploy/caddy/Caddyfile:/etc/caddy/Caddyfile:ro
      - caddy_data:/data
      - caddy_config:/config
    depends_on:
      vexilbot:
        condition: service_healthy

volumes:
  caddy_data:
  caddy_config:
  vexilbot-data:
```

Note: `config.toml` in production must set `data_dir = "/data"`.

- [ ] **Step 2: Update docker-compose.override.yml**

Add the same volume mount for local dev:

```yaml
# Local dev: use local image, expose port directly, no TLS
services:
  vexilbot:
    image: vexilbot:local
    ports:
      - "8080:8080"
    volumes:
      - ./deploy/config.example.toml:/etc/vexilbot/config.toml:ro
      - /dev/null:/run/secrets/github_app_key:ro
      - vexilbot-data:/data

  caddy:
    profiles: ["prod"]

volumes:
  vexilbot-data:
```

- [ ] **Step 3: Add data_dir note to config.example.toml**

Update the `data_dir` comment in `deploy/config.example.toml` to make the Docker path explicit:

```toml
# data_dir = "/data"  # Use "/data" when running in Docker (matches volume mount); default: "data"
```

- [ ] **Step 4: Commit**

```bash
git add docker-compose.yml docker-compose.override.yml deploy/config.example.toml
git commit -m "feat: add vexilbot-data named volume for persistent .vxb store files"
```

---

## Self-Review Checklist

### Spec coverage

| Spec requirement | Task |
|---|---|
| Second listener on `127.0.0.1:8081` | Task 9 |
| `dashboard_port` config field | Task 2 |
| Go templates + HTMX embedded | Task 3 |
| Top-tabs nav | Tasks 3–8 (base.html nav template) |
| Logs tab with level/owner/repo filter + 3s poll | Task 4 |
| Events tab with stat cards + 24h bar chart | Task 5 |
| Releases tab with schedule/cancel/confirm/run-now | Task 6 |
| `RunReleaseNow` (no issue comment) | Task 6 |
| Config tab with redacted server config + repo TOML | Task 7 |
| Storage tab with per-.vxb file stats | Task 8 |
| `scheduledrelease` schema + codegen | Task 1 |
| `vexilbot-data` Docker named volume | Task 10 |
| `docker-compose.override.yml` volume | Task 10 |
