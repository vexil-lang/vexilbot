package dashboard_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/vexil-lang/vexilbot/internal/dashboard"
	"github.com/vexil-lang/vexilbot/internal/serverconfig"
	"github.com/vexil-lang/vexilbot/internal/vexstore"
	"github.com/vexil-lang/vexilbot/internal/vexstore/gen/logentry"
	"github.com/vexil-lang/vexilbot/internal/vexstore/gen/scheduledrelease"
	"github.com/vexil-lang/vexilbot/internal/vexstore/gen/webhookevent"
	vexilruntime "github.com/vexil-lang/vexil/packages/runtime-go"
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
	cfg := &serverconfig.Config{}
	cfg.Server.WebhookSecret = "secret"
	cfg.GitHub.PrivateKeyPath = "/tmp/key"
	cfg.Credentials.CargoRegistryToken = "token"
	cfg.LLM.AnthropicAPIKey = "key"
	return dashboard.New(dashboard.Deps{
		LogStore:        openTestStore(t, logentry.SchemaHash),
		EventStore:      openTestStore(t, webhookevent.SchemaHash),
		ReleaseStore:    openTestStore(t, scheduledrelease.SchemaHash),
		DataDir:         t.TempDir(),
		ServerConfig:    cfg,
		KnownRepos:      func() []string { return nil },
		RunRelease:      func(_ context.Context, _, _, _ string) (int, error) { return 0, nil },
		FetchRepoConfig: func(_ context.Context, _, _ string) ([]byte, error) { return nil, nil },
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

func TestLogsFilterLevel(t *testing.T) {
	store := openTestStore(t, logentry.SchemaHash)
	// Append one INFO and one ERROR record
	writeLogRecord(t, store, logentry.LogEntry{Ts: 1000, Level: logentry.LogLevelInfo, Msg: "hello", Owner: "o", Repo: "r"})
	writeLogRecord(t, store, logentry.LogEntry{Ts: 2000, Level: logentry.LogLevelError, Msg: "boom", Owner: "o", Repo: "r"})

	srv := dashboard.New(dashboard.Deps{
		LogStore:        store,
		EventStore:      openTestStore(t, webhookevent.SchemaHash),
		ReleaseStore:    openTestStore(t, scheduledrelease.SchemaHash),
		DataDir:         t.TempDir(),
		ServerConfig:    &serverconfig.Config{},
		KnownRepos:      func() []string { return nil },
		RunRelease:      func(_ context.Context, _, _, _ string) (int, error) { return 0, nil },
		FetchRepoConfig: func(_ context.Context, _, _ string) ([]byte, error) { return nil, nil },
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

func TestEventsOK(t *testing.T) {
	evStore := openTestStore(t, webhookevent.SchemaHash)
	writeEventRecord(t, evStore, webhookevent.WebhookEvent{
		Ts: uint64(time.Now().UnixNano()), Kind: webhookevent.EventKindPush, Owner: "o", Repo: "r",
	})

	srv := dashboard.New(dashboard.Deps{
		LogStore:        openTestStore(t, logentry.SchemaHash),
		EventStore:      evStore,
		ReleaseStore:    openTestStore(t, scheduledrelease.SchemaHash),
		DataDir:         t.TempDir(),
		ServerConfig:    &serverconfig.Config{},
		KnownRepos:      func() []string { return nil },
		RunRelease:      func(_ context.Context, _, _, _ string) (int, error) { return 0, nil },
		FetchRepoConfig: func(_ context.Context, _, _ string) ([]byte, error) { return nil, nil },
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

func TestReleasesOK(t *testing.T) {
	srv := newTestServer(t)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/releases", nil)
	srv.ServeHTTP(rec, req)
	if rec.Code != 200 {
		t.Fatalf("want 200, got %d: %s", rec.Code, rec.Body.String())
	}
}

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

func TestReleasesCreate(t *testing.T) {
	srv := newTestServer(t)
	form := "package=mylib&bump=patch&run_at=2026-04-01T10%3A00&auto_run=on&owner=o&repo=r"
	rec := httptest.NewRecorder()
	req := httptest.NewRequest("POST", "/releases", strings.NewReader(form))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	srv.ServeHTTP(rec, req)
	if rec.Code != http.StatusFound {
		t.Errorf("want 302, got %d: %s", rec.Code, rec.Body.String())
	}
}

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
		LogStore:        openTestStore(t, logentry.SchemaHash),
		EventStore:      openTestStore(t, webhookevent.SchemaHash),
		ReleaseStore:    openTestStore(t, scheduledrelease.SchemaHash),
		DataDir:         dir,
		ServerConfig:    &serverconfig.Config{},
		KnownRepos:      func() []string { return nil },
		RunRelease:      func(_ context.Context, _, _, _ string) (int, error) { return 0, nil },
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

func TestBasePage_RepoFromQuery(t *testing.T) {
	srv := newTestServer(t)
	req := httptest.NewRequest("GET", "/?repo=owner/repo", nil)
	bp := srv.Base(req, "logs")
	if bp.Repo != "owner/repo" {
		t.Fatalf("got %q want owner/repo", bp.Repo)
	}
	if bp.Tab != "logs" {
		t.Fatalf("got %q want logs", bp.Tab)
	}
}
