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
	Label string
	Count int
	Pct   int // 0–100 for bar height percentage
}

type eventsPageData struct {
	basePage
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
	hourCounts := make(map[int]int) // index 0=oldest hour, 23=most recent

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
			hoursAgo := int(now.Sub(ts).Hours())
			if hoursAgo >= 0 && hoursAgo < 24 {
				hourCounts[23-hoursAgo]++
			}
		}
	}

	kindOrder := []string{"Push", "PullRequest", "Issues", "IssueComment", "Unknown"}
	var byKind []eventKindCount
	for _, k := range kindOrder {
		if c := kindCounts[k]; c > 0 {
			byKind = append(byKind, eventKindCount{Kind: k, Count: c})
		}
	}

	maxH := 1
	for _, c := range hourCounts {
		if c > maxH {
			maxH = c
		}
	}

	var hourly []hourBucket
	for i := 0; i < 24; i++ {
		label := now.Add(time.Duration(i-23) * time.Hour).Format("15:00")
		c := hourCounts[i]
		hourly = append(hourly, hourBucket{Label: label, Count: c, Pct: c * 100 / maxH})
	}

	s.render(w, "events", eventsPageData{
		basePage:   s.base(r, "events"),
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
