package webhook

import (
	"encoding/json"
	"log/slog"
)

type PullRequestEvent struct {
	Action         string
	Number         int
	HeadSHA        string
	HeadRef        string // branch name
	Merged         bool
	UserLogin      string
	Owner          string
	Repo           string
	InstallationID int64
}

type IssueCommentEvent struct {
	Action         string
	CommentID      int64
	CommentBody    string
	CommentUser    string
	IssueNumber    int
	IsPR           bool // true if the comment is on a pull request
	Owner          string
	Repo           string
	InstallationID int64
}

type IssuesEvent struct {
	Action         string
	Number         int
	Title          string
	Body           string
	UserLogin      string
	Labels         []string
	Owner          string
	Repo           string
	InstallationID int64
}

type PushEvent struct {
	Ref            string
	Owner          string
	Repo           string
	InstallationID int64
}

type Dispatcher struct {
	pullRequestHandlers  []func(PullRequestEvent)
	issueCommentHandlers []func(IssueCommentEvent)
	issuesHandlers       []func(IssuesEvent)
	pushHandlers         []func(PushEvent)
}

func NewDispatcher() *Dispatcher {
	return &Dispatcher{}
}

func (d *Dispatcher) OnPullRequest(h func(PullRequestEvent)) {
	d.pullRequestHandlers = append(d.pullRequestHandlers, h)
}

func (d *Dispatcher) OnIssueComment(h func(IssueCommentEvent)) {
	d.issueCommentHandlers = append(d.issueCommentHandlers, h)
}

func (d *Dispatcher) OnIssues(h func(IssuesEvent)) {
	d.issuesHandlers = append(d.issuesHandlers, h)
}

func (d *Dispatcher) OnPush(h func(PushEvent)) {
	d.pushHandlers = append(d.pushHandlers, h)
}

func (d *Dispatcher) Route(eventType string, payload []byte) {
	switch eventType {
	case "pull_request":
		d.dispatchPullRequest(payload)
	case "issue_comment":
		d.dispatchIssueComment(payload)
	case "issues":
		d.dispatchIssues(payload)
	case "push":
		d.dispatchPush(payload)
	default:
		slog.Debug("unhandled event type", "type", eventType)
	}
}

func (d *Dispatcher) dispatchPullRequest(payload []byte) {
	var raw struct {
		Action      string `json:"action"`
		Number      int    `json:"number"`
		PullRequest struct {
			Head struct {
				SHA string `json:"sha"`
				Ref string `json:"ref"`
			} `json:"head"`
			User struct {
				Login string `json:"login"`
			} `json:"user"`
			Merged bool `json:"merged"`
		} `json:"pull_request"`
		Repository   repoInfo     `json:"repository"`
		Installation installation `json:"installation"`
	}
	if err := json.Unmarshal(payload, &raw); err != nil {
		slog.Error("parse pull_request event", "error", err)
		return
	}

	event := PullRequestEvent{
		Action:         raw.Action,
		Number:         raw.Number,
		HeadSHA:        raw.PullRequest.Head.SHA,
		HeadRef:        raw.PullRequest.Head.Ref,
		Merged:         raw.PullRequest.Merged,
		UserLogin:      raw.PullRequest.User.Login,
		Owner:          raw.Repository.Owner.Login,
		Repo:           raw.Repository.Name,
		InstallationID: raw.Installation.ID,
	}

	for _, h := range d.pullRequestHandlers {
		h(event)
	}
}

func (d *Dispatcher) dispatchIssueComment(payload []byte) {
	var raw struct {
		Action  string `json:"action"`
		Comment struct {
			ID   int64  `json:"id"`
			Body string `json:"body"`
			User struct {
				Login string `json:"login"`
			} `json:"user"`
		} `json:"comment"`
		Issue struct {
			Number      int              `json:"number"`
			PullRequest *json.RawMessage `json:"pull_request"` // non-nil if comment is on a PR
		} `json:"issue"`
		Repository   repoInfo     `json:"repository"`
		Installation installation `json:"installation"`
	}
	if err := json.Unmarshal(payload, &raw); err != nil {
		slog.Error("parse issue_comment event", "error", err)
		return
	}

	event := IssueCommentEvent{
		Action:         raw.Action,
		CommentID:      raw.Comment.ID,
		CommentBody:    raw.Comment.Body,
		CommentUser:    raw.Comment.User.Login,
		IssueNumber:    raw.Issue.Number,
		IsPR:           raw.Issue.PullRequest != nil,
		Owner:          raw.Repository.Owner.Login,
		Repo:           raw.Repository.Name,
		InstallationID: raw.Installation.ID,
	}

	for _, h := range d.issueCommentHandlers {
		h(event)
	}
}

func (d *Dispatcher) dispatchIssues(payload []byte) {
	var raw struct {
		Action string `json:"action"`
		Issue  struct {
			Number int    `json:"number"`
			Title  string `json:"title"`
			Body   string `json:"body"`
			User   struct {
				Login string `json:"login"`
			} `json:"user"`
			Labels []struct {
				Name string `json:"name"`
			} `json:"labels"`
		} `json:"issue"`
		Repository   repoInfo     `json:"repository"`
		Installation installation `json:"installation"`
	}
	if err := json.Unmarshal(payload, &raw); err != nil {
		slog.Error("parse issues event", "error", err)
		return
	}

	labels := make([]string, len(raw.Issue.Labels))
	for i, l := range raw.Issue.Labels {
		labels[i] = l.Name
	}

	event := IssuesEvent{
		Action:         raw.Action,
		Number:         raw.Issue.Number,
		Title:          raw.Issue.Title,
		Body:           raw.Issue.Body,
		UserLogin:      raw.Issue.User.Login,
		Labels:         labels,
		Owner:          raw.Repository.Owner.Login,
		Repo:           raw.Repository.Name,
		InstallationID: raw.Installation.ID,
	}

	for _, h := range d.issuesHandlers {
		h(event)
	}
}

func (d *Dispatcher) dispatchPush(payload []byte) {
	var raw struct {
		Ref          string       `json:"ref"`
		Repository   repoInfo     `json:"repository"`
		Installation installation `json:"installation"`
	}
	if err := json.Unmarshal(payload, &raw); err != nil {
		slog.Error("parse push event", "error", err)
		return
	}

	event := PushEvent{
		Ref:            raw.Ref,
		Owner:          raw.Repository.Owner.Login,
		Repo:           raw.Repository.Name,
		InstallationID: raw.Installation.ID,
	}

	for _, h := range d.pushHandlers {
		h(event)
	}
}

type repoInfo struct {
	Owner struct {
		Login string `json:"login"`
	} `json:"owner"`
	Name string `json:"name"`
}

type installation struct {
	ID int64 `json:"id"`
}
