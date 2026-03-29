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

type ghLabel struct {
	Name  string
	Color string
}

type issueRow struct {
	Number    int
	Title     string
	State     string
	Labels    []ghLabel
	Assignees []string
	Age       string
}

type issuesPageData struct {
	basePage
	Issues         []issueRow
	FilterLabel    string
	FilterAssignee string
	FilterPriority string
	ErrorMsg       string
}

func (s *Server) handleIssues(w http.ResponseWriter, r *http.Request) {
	bp := s.base(r, "issues")
	owner, repo, ok := splitRepo(bp.Repo)
	if !ok {
		s.render(w, "issues.html", issuesPageData{basePage: bp, ErrorMsg: "select a repo first"})
		return
	}
	client, err := s.deps.GetInstallationClient(owner, repo)
	if err != nil {
		s.render(w, "issues.html", issuesPageData{basePage: bp, ErrorMsg: err.Error()})
		return
	}
	q := r.URL.Query()
	opts := &github.IssueListByRepoOptions{
		State:       "open",
		ListOptions: github.ListOptions{PerPage: 30},
	}
	if p := q.Get("page"); p != "" {
		if n, err := strconv.Atoi(p); err == nil {
			opts.Page = n
		}
	}
	if lbl := q.Get("label"); lbl != "" {
		opts.Labels = []string{lbl}
	}
	if a := q.Get("assignee"); a != "" {
		opts.Assignee = a
	}
	ctx, cancel := context.WithTimeout(r.Context(), 15*time.Second)
	defer cancel()
	ghIssues, _, err := client.Issues.ListByRepo(ctx, owner, repo, opts)
	if err != nil {
		s.render(w, "issues.html", issuesPageData{basePage: bp, ErrorMsg: "GitHub API: " + err.Error()})
		return
	}
	rows := make([]issueRow, 0, len(ghIssues))
	for _, gi := range ghIssues {
		if gi.PullRequestLinks != nil {
			continue // skip PRs from issues endpoint
		}
		row := issueRow{
			Number: gi.GetNumber(),
			Title:  gi.GetTitle(),
			State:  gi.GetState(),
			Age:    issueAge(gi.GetCreatedAt().Time),
		}
		for _, l := range gi.Labels {
			row.Labels = append(row.Labels, ghLabel{Name: l.GetName(), Color: l.GetColor()})
		}
		for _, a := range gi.Assignees {
			row.Assignees = append(row.Assignees, a.GetLogin())
		}
		rows = append(rows, row)
	}
	s.render(w, "issues.html", issuesPageData{
		basePage:       bp,
		Issues:         rows,
		FilterLabel:    q.Get("label"),
		FilterAssignee: q.Get("assignee"),
		FilterPriority: q.Get("priority"),
	})
}

type issueDetailData struct {
	basePage
	Number    int
	Title     string
	State     string
	Author    string
	Age       string
	Body      string
	Labels    []ghLabel
	Assignees []string
	ErrorMsg  string
}

func (s *Server) handleIssueDetail(w http.ResponseWriter, r *http.Request) {
	bp := s.base(r, "issues")
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
		s.render(w, "issue_detail.html", issueDetailData{basePage: bp, ErrorMsg: err.Error()})
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()
	gi, _, err := client.Issues.Get(ctx, owner, repo, num)
	if err != nil {
		s.render(w, "issue_detail.html", issueDetailData{basePage: bp, ErrorMsg: err.Error()})
		return
	}
	d := issueDetailData{
		basePage: bp,
		Number:   gi.GetNumber(),
		Title:    gi.GetTitle(),
		State:    gi.GetState(),
		Author:   gi.GetUser().GetLogin(),
		Age:      issueAge(gi.GetCreatedAt().Time),
		Body:     gi.GetBody(),
	}
	for _, l := range gi.Labels {
		d.Labels = append(d.Labels, ghLabel{Name: l.GetName(), Color: l.GetColor()})
	}
	for _, a := range gi.Assignees {
		d.Assignees = append(d.Assignees, a.GetLogin())
	}
	s.render(w, "issue_detail.html", d)
}

func (s *Server) handleIssueLabel(w http.ResponseWriter, r *http.Request) {
	s.issueAction(w, r, func(ctx context.Context, client *github.Client, owner, repo string, num int) error {
		label := strings.TrimSpace(r.FormValue("label"))
		if label == "" {
			return fmt.Errorf("label required")
		}
		_, _, err := client.Issues.AddLabelsToIssue(ctx, owner, repo, num, []string{label})
		return err
	})
}

func (s *Server) handleIssueUnlabel(w http.ResponseWriter, r *http.Request) {
	s.issueAction(w, r, func(ctx context.Context, client *github.Client, owner, repo string, num int) error {
		label := strings.TrimSpace(r.FormValue("label"))
		if label == "" {
			return fmt.Errorf("label required")
		}
		_, err := client.Issues.RemoveLabelForIssue(ctx, owner, repo, num, label)
		return err
	})
}

func (s *Server) handleIssueAssign(w http.ResponseWriter, r *http.Request) {
	s.issueAction(w, r, func(ctx context.Context, client *github.Client, owner, repo string, num int) error {
		login := strings.TrimSpace(r.FormValue("assignee"))
		if login == "" {
			return fmt.Errorf("assignee required")
		}
		_, _, err := client.Issues.AddAssignees(ctx, owner, repo, num, []string{login})
		return err
	})
}

func (s *Server) handleIssuePrioritize(w http.ResponseWriter, r *http.Request) {
	s.issueAction(w, r, func(ctx context.Context, client *github.Client, owner, repo string, num int) error {
		priority := strings.TrimSpace(r.FormValue("priority"))
		if priority == "" {
			return fmt.Errorf("priority required")
		}
		for _, p := range []string{"p0", "p1", "p2", "p3"} {
			client.Issues.RemoveLabelForIssue(ctx, owner, repo, num, p) //nolint:errcheck
		}
		_, _, err := client.Issues.AddLabelsToIssue(ctx, owner, repo, num, []string{priority})
		return err
	})
}

func (s *Server) handleIssueClose(w http.ResponseWriter, r *http.Request) {
	s.issueAction(w, r, func(ctx context.Context, client *github.Client, owner, repo string, num int) error {
		closed := "closed"
		_, _, err := client.Issues.Edit(ctx, owner, repo, num, &github.IssueRequest{State: &closed})
		return err
	})
}

func (s *Server) handleIssueReopen(w http.ResponseWriter, r *http.Request) {
	s.issueAction(w, r, func(ctx context.Context, client *github.Client, owner, repo string, num int) error {
		open := "open"
		_, _, err := client.Issues.Edit(ctx, owner, repo, num, &github.IssueRequest{State: &open})
		return err
	})
}

// issueAction parses the issue number, runs fn, then re-renders the detail panel.
func (s *Server) issueAction(w http.ResponseWriter, r *http.Request, fn func(ctx context.Context, client *github.Client, owner, repo string, num int) error) {
	bp := s.base(r, "issues")
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
	// Re-fetch and re-render detail panel
	gi, _, err := client.Issues.Get(ctx, owner, repo, num)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	d := issueDetailData{
		basePage: bp,
		Number:   gi.GetNumber(),
		Title:    gi.GetTitle(),
		State:    gi.GetState(),
		Author:   gi.GetUser().GetLogin(),
		Age:      issueAge(gi.GetCreatedAt().Time),
		Body:     gi.GetBody(),
	}
	for _, l := range gi.Labels {
		d.Labels = append(d.Labels, ghLabel{Name: l.GetName(), Color: l.GetColor()})
	}
	for _, a := range gi.Assignees {
		d.Assignees = append(d.Assignees, a.GetLogin())
	}
	s.render(w, "issue_detail.html", d)
}

// issueAge returns a human-readable age string like "3d", "2h", "5m".
func issueAge(t time.Time) string {
	d := time.Since(t)
	switch {
	case d >= 24*time.Hour:
		return fmt.Sprintf("%dd", int(d.Hours()/24))
	case d >= time.Hour:
		return fmt.Sprintf("%dh", int(d.Hours()))
	default:
		return fmt.Sprintf("%dm", int(d.Minutes()))
	}
}
