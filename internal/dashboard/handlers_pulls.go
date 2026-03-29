package dashboard

import (
	"context"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/google/go-github/v68/github"
)

type pullRow struct {
	Number       int
	Title        string
	State        string
	HeadBranch   string
	Author       string
	Labels       []ghLabel
	Age          string
	Mergeable    string
	ChangedFiles int
}

type pullsPageData struct {
	basePage
	Pulls    []pullRow
	ErrorMsg string
}

func (s *Server) handlePulls(w http.ResponseWriter, r *http.Request) {
	bp := s.base(r, "pulls")
	owner, repo, ok := splitRepo(bp.Repo)
	if !ok {
		s.render(w, "pulls.html", pullsPageData{basePage: bp, ErrorMsg: "select a repo first"})
		return
	}
	client, err := s.deps.GetInstallationClient(owner, repo)
	if err != nil {
		s.render(w, "pulls.html", pullsPageData{basePage: bp, ErrorMsg: err.Error()})
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 15*time.Second)
	defer cancel()
	opts := &github.PullRequestListOptions{
		State:       "open",
		ListOptions: github.ListOptions{PerPage: 30},
	}
	prs, _, err := client.PullRequests.List(ctx, owner, repo, opts)
	if err != nil {
		s.render(w, "pulls.html", pullsPageData{basePage: bp, ErrorMsg: "GitHub API: " + err.Error()})
		return
	}
	rows := make([]pullRow, 0, len(prs))
	for _, pr := range prs {
		row := pullRow{
			Number:       pr.GetNumber(),
			Title:        pr.GetTitle(),
			State:        pr.GetState(),
			HeadBranch:   pr.GetHead().GetRef(),
			Author:       pr.GetUser().GetLogin(),
			Age:          issueAge(pr.GetCreatedAt().Time),
			ChangedFiles: pr.GetChangedFiles(),
		}
		if pr.Mergeable != nil {
			if *pr.Mergeable {
				row.Mergeable = "mergeable"
			} else {
				row.Mergeable = "conflict"
			}
		}
		for _, l := range pr.Labels {
			row.Labels = append(row.Labels, ghLabel{Name: l.GetName(), Color: l.GetColor()})
		}
		rows = append(rows, row)
	}
	s.render(w, "pulls.html", pullsPageData{basePage: bp, Pulls: rows})
}

type pullDetailData struct {
	basePage
	Number     int
	Title      string
	State      string
	Author     string
	Age        string
	Body       string
	HeadBranch string
	BaseBranch string
	Labels     []ghLabel
	Assignees  []string
	Mergeable  string
	ErrorMsg   string
}

func (s *Server) handlePullDetail(w http.ResponseWriter, r *http.Request) {
	bp := s.base(r, "pulls")
	owner, repo, ok := splitRepo(bp.Repo)
	if !ok {
		http.Error(w, "repo required", http.StatusBadRequest)
		return
	}
	num, err := strconv.Atoi(r.PathValue("number"))
	if err != nil {
		http.Error(w, "bad number", http.StatusBadRequest)
		return
	}
	client, err := s.deps.GetInstallationClient(owner, repo)
	if err != nil {
		s.render(w, "pull_detail.html", pullDetailData{basePage: bp, ErrorMsg: err.Error()})
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()
	pr, _, err := client.PullRequests.Get(ctx, owner, repo, num)
	if err != nil {
		s.render(w, "pull_detail.html", pullDetailData{basePage: bp, ErrorMsg: err.Error()})
		return
	}
	d := buildPullDetailData(bp, pr)
	s.render(w, "pull_detail.html", d)
}

func (s *Server) handlePullFiles(w http.ResponseWriter, r *http.Request) {
	bp := s.base(r, "pulls")
	owner, repo, ok := splitRepo(bp.Repo)
	if !ok {
		http.Error(w, "repo required", http.StatusBadRequest)
		return
	}
	num, err := strconv.Atoi(r.PathValue("number"))
	if err != nil {
		http.Error(w, "bad number", http.StatusBadRequest)
		return
	}
	client, err := s.deps.GetInstallationClient(owner, repo)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()
	files, _, err := client.PullRequests.ListFiles(ctx, owner, repo, num, &github.ListOptions{PerPage: 100})
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	type fileRow struct{ Filename, Status string }
	type filesData struct {
		basePage
		Files []fileRow
	}
	rows := make([]fileRow, 0, len(files))
	for _, f := range files {
		rows = append(rows, fileRow{Filename: f.GetFilename(), Status: f.GetStatus()})
	}
	s.render(w, "pull_files.html", filesData{basePage: bp, Files: rows})
}

func (s *Server) handlePullMerge(w http.ResponseWriter, r *http.Request) {
	bp := s.base(r, "pulls")
	owner, repo, ok := splitRepo(bp.Repo)
	if !ok {
		http.Error(w, "repo required", http.StatusBadRequest)
		return
	}
	num, err := strconv.Atoi(r.PathValue("number"))
	if err != nil {
		http.Error(w, "bad number", http.StatusBadRequest)
		return
	}
	client, err := s.deps.GetInstallationClient(owner, repo)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 15*time.Second)
	defer cancel()
	_, _, err = client.PullRequests.Merge(ctx, owner, repo, num, "", &github.PullRequestOptions{MergeMethod: "merge"})
	if err != nil {
		http.Error(w, "merge: "+err.Error(), http.StatusInternalServerError)
		return
	}
	pr, _, err := client.PullRequests.Get(ctx, owner, repo, num)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	s.render(w, "pull_detail.html", buildPullDetailData(bp, pr))
}

func (s *Server) handlePullLabel(w http.ResponseWriter, r *http.Request) {
	s.pullAction(w, r, func(ctx context.Context, client *github.Client, owner, repo string, num int) error {
		label := strings.TrimSpace(r.FormValue("label"))
		if label == "" {
			return fmt.Errorf("label required")
		}
		_, _, err := client.Issues.AddLabelsToIssue(ctx, owner, repo, num, []string{label})
		return err
	})
}

func (s *Server) handlePullUnlabel(w http.ResponseWriter, r *http.Request) {
	s.pullAction(w, r, func(ctx context.Context, client *github.Client, owner, repo string, num int) error {
		label := strings.TrimSpace(r.FormValue("label"))
		if label == "" {
			return fmt.Errorf("label required")
		}
		_, err := client.Issues.RemoveLabelForIssue(ctx, owner, repo, num, label)
		return err
	})
}

func (s *Server) handlePullAssign(w http.ResponseWriter, r *http.Request) {
	s.pullAction(w, r, func(ctx context.Context, client *github.Client, owner, repo string, num int) error {
		login := strings.TrimSpace(r.FormValue("assignee"))
		if login == "" {
			return fmt.Errorf("assignee required")
		}
		_, _, err := client.Issues.AddAssignees(ctx, owner, repo, num, []string{login})
		return err
	})
}

func (s *Server) handlePullClose(w http.ResponseWriter, r *http.Request) {
	s.pullAction(w, r, func(ctx context.Context, client *github.Client, owner, repo string, num int) error {
		closed := "closed"
		_, _, err := client.Issues.Edit(ctx, owner, repo, num, &github.IssueRequest{State: &closed})
		return err
	})
}

func (s *Server) handlePullReopen(w http.ResponseWriter, r *http.Request) {
	s.pullAction(w, r, func(ctx context.Context, client *github.Client, owner, repo string, num int) error {
		open := "open"
		_, _, err := client.Issues.Edit(ctx, owner, repo, num, &github.IssueRequest{State: &open})
		return err
	})
}

func (s *Server) pullAction(w http.ResponseWriter, r *http.Request, fn func(ctx context.Context, client *github.Client, owner, repo string, num int) error) {
	bp := s.base(r, "pulls")
	owner, repo, ok := splitRepo(bp.Repo)
	if !ok {
		http.Error(w, "repo required", http.StatusBadRequest)
		return
	}
	num, err := strconv.Atoi(r.PathValue("number"))
	if err != nil {
		http.Error(w, "bad number", http.StatusBadRequest)
		return
	}
	client, err := s.deps.GetInstallationClient(owner, repo)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()
	if err := fn(ctx, client, owner, repo, num); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	pr, _, err := client.PullRequests.Get(ctx, owner, repo, num)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	s.render(w, "pull_detail.html", buildPullDetailData(bp, pr))
}

func buildPullDetailData(bp basePage, pr *github.PullRequest) pullDetailData {
	d := pullDetailData{
		basePage:   bp,
		Number:     pr.GetNumber(),
		Title:      pr.GetTitle(),
		State:      pr.GetState(),
		Author:     pr.GetUser().GetLogin(),
		Age:        issueAge(pr.GetCreatedAt().Time),
		Body:       pr.GetBody(),
		HeadBranch: pr.GetHead().GetRef(),
		BaseBranch: pr.GetBase().GetRef(),
	}
	if pr.Mergeable != nil {
		if *pr.Mergeable {
			d.Mergeable = "mergeable"
		} else {
			d.Mergeable = "conflict"
		}
	}
	for _, l := range pr.Labels {
		d.Labels = append(d.Labels, ghLabel{Name: l.GetName(), Color: l.GetColor()})
	}
	for _, a := range pr.Assignees {
		d.Assignees = append(d.Assignees, a.GetLogin())
	}
	return d
}
