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
