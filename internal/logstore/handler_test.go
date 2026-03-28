package logstore_test

import (
	"bytes"
	"context"
	"crypto/sha256"
	"log/slog"
	"path/filepath"
	"testing"

	vexil "github.com/vexil-lang/vexil/packages/runtime-go"
	"github.com/vexil-lang/vexilbot/internal/logstore"
	"github.com/vexil-lang/vexilbot/internal/vexstore"
	"github.com/vexil-lang/vexilbot/internal/vexstore/gen/logentry"
)

var anyID = sha256.Sum256([]byte("logstore-test"))

func openStore(t *testing.T) *vexstore.AppendStore {
	t.Helper()
	s, err := vexstore.OpenAppendStore(filepath.Join(t.TempDir(), "l.vxb"), anyID)
	if err != nil {
		t.Fatalf("OpenAppendStore: %v", err)
	}
	t.Cleanup(func() { s.Close() })
	return s
}

func decodeEntry(t *testing.T, b []byte) logentry.LogEntry {
	t.Helper()
	r := vexil.NewBitReader(b)
	var e logentry.LogEntry
	if err := e.Unpack(r); err != nil {
		t.Fatalf("Unpack LogEntry: %v", err)
	}
	return e
}

func TestHandlerWritesRecord(t *testing.T) {
	store := openStore(t)
	var buf bytes.Buffer
	h := logstore.NewHandler(store, slog.NewJSONHandler(&buf, nil))
	slog.New(h).Info("test message", "owner", "vexil-lang", "repo", "vexilbot")

	records, err := store.ReadAll()
	if err != nil {
		t.Fatalf("ReadAll: %v", err)
	}
	if len(records) != 1 {
		t.Fatalf("want 1 record, got %d", len(records))
	}
	entry := decodeEntry(t, records[0])
	if entry.Msg != "test message" {
		t.Errorf("Msg: want %q, got %q", "test message", entry.Msg)
	}
	if entry.Owner != "vexil-lang" {
		t.Errorf("Owner: want %q, got %q", "vexil-lang", entry.Owner)
	}
	if entry.Repo != "vexilbot" {
		t.Errorf("Repo: want %q, got %q", "vexilbot", entry.Repo)
	}
	if entry.Level != logentry.LogLevelInfo {
		t.Errorf("Level: want Info, got %d", entry.Level)
	}
	if entry.Ts == 0 {
		t.Error("Ts is zero")
	}
}

func TestHandlerForwardsToNext(t *testing.T) {
	var buf bytes.Buffer
	h := logstore.NewHandler(openStore(t), slog.NewJSONHandler(&buf, nil))
	slog.New(h).Warn("forwarded")
	if !bytes.Contains(buf.Bytes(), []byte("forwarded")) {
		t.Fatalf("stdout missing message: %s", buf.String())
	}
}

func TestHandlerLevelMapping(t *testing.T) {
	cases := []struct {
		in   slog.Level
		want logentry.LogLevel
	}{
		{slog.LevelDebug, logentry.LogLevelDebug},
		{slog.LevelInfo, logentry.LogLevelInfo},
		{slog.LevelWarn, logentry.LogLevelWarn},
		{slog.LevelError, logentry.LogLevelError},
	}
	for _, tc := range cases {
		store := openStore(t)
		var buf bytes.Buffer
		h := logstore.NewHandler(store, slog.NewJSONHandler(&buf, &slog.HandlerOptions{Level: slog.LevelDebug}))
		slog.New(h).Log(context.Background(), tc.in, "msg")
		records, _ := store.ReadAll()
		if len(records) == 0 {
			t.Fatalf("level %v: no records", tc.in)
		}
		entry := decodeEntry(t, records[0])
		if entry.Level != tc.want {
			t.Errorf("level %v: want %d, got %d", tc.in, tc.want, entry.Level)
		}
	}
}

func TestHandlerMissingOwnerRepo(t *testing.T) {
	store := openStore(t)
	var buf bytes.Buffer
	slog.New(logstore.NewHandler(store, slog.NewJSONHandler(&buf, nil))).Info("no attrs")
	records, _ := store.ReadAll()
	if len(records) != 1 {
		t.Fatalf("want 1 record, got %d", len(records))
	}
	entry := decodeEntry(t, records[0])
	if entry.Owner != "" || entry.Repo != "" {
		t.Errorf("want empty owner/repo, got %q/%q", entry.Owner, entry.Repo)
	}
}
