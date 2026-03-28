// internal/dashboard/handlers_releases.go
package dashboard

import (
	"context"
	"crypto/rand"
	"fmt"
	"net/http"
	"sort"
	"time"

	vexil "github.com/vexil-lang/vexil/packages/runtime-go"
	"github.com/vexil-lang/vexilbot/internal/vexstore/gen/scheduledrelease"
)

type releaseRow struct {
	ID      string
	Package string
	Bump    string
	Owner   string
	Repo    string
	RunAt   string
	AutoRun bool
	Status  string
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
	ctx := context.Background()
	go func() {
		_, runErr := s.deps.RunRelease(ctx, target.Owner, target.Repo, target.Package)
		status := scheduledrelease.ReleaseStatusDone
		if runErr != nil {
			status = scheduledrelease.ReleaseStatusCancelled
		}
		_ = s.updateReleaseStatus(id, status)
	}()
	http.Redirect(w, r, "/releases", http.StatusFound)
}

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
