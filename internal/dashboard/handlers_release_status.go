package dashboard

import (
	"context"
	"net/http"
	"strings"
	"time"

	"github.com/google/go-github/v68/github"
	"github.com/vexil-lang/vexilbot/internal/release"
	"github.com/vexil-lang/vexilbot/internal/repoconfig"
)

type releaseStatusRow struct {
	Name    string
	Version string
	Bump    string
	Commits int
}

type releaseStatusPageData struct {
	basePage
	StatusRows []releaseStatusRow
	ErrorMsg   string
}

func (s *Server) handleReleaseStatus(w http.ResponseWriter, r *http.Request) {
	bp := s.base(r, "releases")
	owner, repo, ok := splitRepo(bp.Repo)
	if !ok {
		s.render(w, "release_status.html", releaseStatusPageData{basePage: bp, ErrorMsg: "select a repo first"})
		return
	}
	cfg, err := s.repoConfig(r.Context(), owner, repo)
	if err != nil {
		s.render(w, "release_status.html", releaseStatusPageData{basePage: bp, ErrorMsg: "fetch config: " + err.Error()})
		return
	}
	client, err := s.deps.GetInstallationClient(owner, repo)
	if err != nil {
		s.render(w, "release_status.html", releaseStatusPageData{basePage: bp, ErrorMsg: err.Error()})
		return
	}
	api := &ghReleaseAPI{client: client, owner: owner, repo: repo}
	ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
	defer cancel()
	statuses, err := release.GetStatus(ctx, api, owner, repo, cfg.Release)
	if err != nil {
		s.render(w, "release_status.html", releaseStatusPageData{basePage: bp, ErrorMsg: err.Error()})
		return
	}
	rows := make([]releaseStatusRow, 0, len(statuses))
	for _, st := range statuses {
		rows = append(rows, releaseStatusRow{
			Name:    st.Name,
			Version: st.Version,
			Bump:    st.Bump,
			Commits: st.Commits,
		})
	}
	s.render(w, "release_status.html", releaseStatusPageData{basePage: bp, StatusRows: rows})
}

func (s *Server) handleWorkspaceRelease(w http.ResponseWriter, r *http.Request) {
	bp := s.base(r, "releases")
	_, _, ok := splitRepo(bp.Repo)
	if !ok {
		http.Error(w, "repo required", http.StatusBadRequest)
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Minute)
	defer cancel()
	if _, err := s.deps.RunWorkspaceRelease(ctx, strings.Split(bp.Repo, "/")[0], strings.Split(bp.Repo, "/")[1]); err != nil {
		http.Error(w, "release: "+err.Error(), http.StatusInternalServerError)
		return
	}
	http.Redirect(w, r, "/releases/status?repo="+bp.Repo, http.StatusSeeOther)
}

func (s *Server) handleSingleRelease(w http.ResponseWriter, r *http.Request) {
	bp := s.base(r, "releases")
	_, _, ok := splitRepo(bp.Repo)
	if !ok {
		http.Error(w, "repo required", http.StatusBadRequest)
		return
	}
	pkg := strings.TrimSpace(r.FormValue("package"))
	if pkg == "" {
		http.Error(w, "package required", http.StatusBadRequest)
		return
	}
	parts := strings.SplitN(bp.Repo, "/", 2)
	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Minute)
	defer cancel()
	if _, err := s.deps.RunRelease(ctx, parts[0], parts[1], pkg); err != nil {
		http.Error(w, "release: "+err.Error(), http.StatusInternalServerError)
		return
	}
	http.Redirect(w, r, "/releases/status?repo="+bp.Repo, http.StatusSeeOther)
}

// repoConfig fetches and parses the repo config.
func (s *Server) repoConfig(ctx context.Context, owner, repo string) (*repoconfig.Config, error) {
	raw, err := s.deps.FetchRepoConfig(ctx, owner, repo)
	if err != nil {
		return nil, err
	}
	return repoconfig.Parse(raw)
}

// ghReleaseAPI adapts a *github.Client to the release.GitAPI interface.
type ghReleaseAPI struct {
	client *github.Client
	owner  string
	repo   string
}

func (a *ghReleaseAPI) ListTags(ctx context.Context, owner, repo string) ([]string, error) {
	tags, _, err := a.client.Repositories.ListTags(ctx, owner, repo, &github.ListOptions{PerPage: 100})
	if err != nil {
		return nil, err
	}
	names := make([]string, len(tags))
	for i, t := range tags {
		names[i] = t.GetName()
	}
	return names, nil
}

func (a *ghReleaseAPI) CommitsSinceTag(ctx context.Context, owner, repo, tag, path string) ([]release.Commit, error) {
	if tag == "" {
		// Never released — list all commits on HEAD.
		ghCommits, _, err := a.client.Repositories.ListCommits(ctx, owner, repo, &github.CommitsListOptions{
			ListOptions: github.ListOptions{PerPage: 100},
		})
		if err != nil {
			return nil, err
		}
		var commits []release.Commit
		for _, c := range ghCommits {
			if path != "" {
				full, _, err := a.client.Repositories.GetCommit(ctx, owner, repo, c.GetSHA(), &github.ListOptions{})
				if err != nil {
					continue
				}
				touches := false
				for _, f := range full.Files {
					if strings.HasPrefix(f.GetFilename(), path) {
						touches = true
						break
					}
				}
				if !touches {
					continue
				}
			}
			commits = append(commits, release.Commit{
				SHA:     c.GetSHA(),
				Message: c.GetCommit().GetMessage(),
			})
		}
		return commits, nil
	}
	cmp, _, err := a.client.Repositories.CompareCommits(ctx, owner, repo, tag, "HEAD", &github.ListOptions{PerPage: 100})
	if err != nil {
		return nil, err
	}
	var commits []release.Commit
	for _, c := range cmp.Commits {
		if path != "" {
			fullCommit, _, err := a.client.Repositories.GetCommit(ctx, owner, repo, c.GetSHA(), &github.ListOptions{})
			if err != nil {
				continue
			}
			touches := false
			for _, f := range fullCommit.Files {
				if strings.HasPrefix(f.GetFilename(), path) {
					touches = true
					break
				}
			}
			if !touches {
				continue
			}
		}
		commits = append(commits, release.Commit{
			SHA:     c.GetSHA(),
			Message: c.GetCommit().GetMessage(),
		})
	}
	return commits, nil
}
