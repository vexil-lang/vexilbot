package logstore

import (
	"context"
	"log/slog"
	"time"

	vexil "github.com/vexil-lang/vexil/packages/runtime-go"
	"github.com/vexil-lang/vexilbot/internal/vexstore"
	"github.com/vexil-lang/vexilbot/internal/vexstore/gen/logentry"
)

// Handler is a slog.Handler that persists each log record as a logentry.LogEntry
// and forwards every record to a wrapped next handler.
type Handler struct {
	store *vexstore.AppendStore
	next  slog.Handler
}

func NewHandler(store *vexstore.AppendStore, next slog.Handler) *Handler {
	return &Handler{store: store, next: next}
}

func (h *Handler) Enabled(ctx context.Context, level slog.Level) bool {
	return h.next.Enabled(ctx, level)
}

func (h *Handler) Handle(ctx context.Context, r slog.Record) error {
	entry := logentry.LogEntry{
		Ts:    uint64(r.Time.UnixNano()),
		Level: slogLevelToVex(r.Level),
		Msg:   r.Message,
	}
	if r.Time.IsZero() {
		entry.Ts = uint64(time.Now().UnixNano())
	}
	r.Attrs(func(a slog.Attr) bool {
		switch a.Key {
		case "owner":
			entry.Owner = a.Value.String()
		case "repo":
			entry.Repo = a.Value.String()
		}
		return true
	})
	w := vexil.NewBitWriter()
	if entry.Pack(w) == nil {
		_ = h.store.Append(w.Finish()) // best-effort; never block stdout logging
	}
	return h.next.Handle(ctx, r)
}

func (h *Handler) WithAttrs(attrs []slog.Attr) slog.Handler {
	return &Handler{store: h.store, next: h.next.WithAttrs(attrs)}
}

func (h *Handler) WithGroup(name string) slog.Handler {
	return &Handler{store: h.store, next: h.next.WithGroup(name)}
}

func slogLevelToVex(level slog.Level) logentry.LogLevel {
	switch {
	case level >= slog.LevelError:
		return logentry.LogLevelError
	case level >= slog.LevelWarn:
		return logentry.LogLevelWarn
	case level >= slog.LevelInfo:
		return logentry.LogLevelInfo
	default:
		return logentry.LogLevelDebug
	}
}
