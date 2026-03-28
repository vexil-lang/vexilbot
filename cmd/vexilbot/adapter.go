package main

import (
	"context"
	"fmt"

	"github.com/google/go-github/v68/github"
)

// ghAdapter wraps a GitHub installation client and implements all feature interfaces.
type ghAdapter struct {
	client *github.Client
}

// --- welcome.ContribAPI ---

func (a *ghAdapter) CountUserPRs(ctx context.Context, owner, repo, user string) (int, error) {
	query := fmt.Sprintf("type:pr repo:%s/%s author:%s", owner, repo, user)
	result, _, err := a.client.Search.Issues(ctx, query, &github.SearchOptions{
		ListOptions: github.ListOptions{PerPage: 1},
	})
	if err != nil {
		return 0, err
	}
	return result.GetTotal(), nil
}

func (a *ghAdapter) CountUserIssues(ctx context.Context, owner, repo, user string) (int, error) {
	query := fmt.Sprintf("type:issue repo:%s/%s author:%s", owner, repo, user)
	result, _, err := a.client.Search.Issues(ctx, query, &github.SearchOptions{
		ListOptions: github.ListOptions{PerPage: 1},
	})
	if err != nil {
		return 0, err
	}
	return result.GetTotal(), nil
}

// --- triage.GitHubPermissions ---

func (a *ghAdapter) IsTeamMember(ctx context.Context, org, teamSlug, user string) (bool, error) {
	membership, _, err := a.client.Teams.GetTeamMembershipBySlug(ctx, org, teamSlug, user)
	if err != nil {
		return false, nil // 404 means not a member
	}
	return membership.GetState() == "active", nil
}

func (a *ghAdapter) GetCollaboratorPermission(ctx context.Context, owner, repo, user string) (string, error) {
	level, _, err := a.client.Repositories.GetPermissionLevel(ctx, owner, repo, user)
	if err != nil {
		return "", err
	}
	return level.GetPermission(), nil
}

// --- triage.IssueAPI + welcome.ContribAPI + policy.PolicyAPI (shared) ---

func (a *ghAdapter) CreateComment(ctx context.Context, owner, repo string, number int, body string) error {
	_, _, err := a.client.Issues.CreateComment(ctx, owner, repo, number, &github.IssueComment{
		Body: github.Ptr(body),
	})
	return err
}

func (a *ghAdapter) AddLabels(ctx context.Context, owner, repo string, number int, labels []string) error {
	_, _, err := a.client.Issues.AddLabelsToIssue(ctx, owner, repo, number, labels)
	return err
}

func (a *ghAdapter) RemoveLabel(ctx context.Context, owner, repo string, number int, label string) error {
	_, err := a.client.Issues.RemoveLabelForIssue(ctx, owner, repo, number, label)
	return err
}

func (a *ghAdapter) AddAssignees(ctx context.Context, owner, repo string, number int, assignees []string) error {
	_, _, err := a.client.Issues.AddAssignees(ctx, owner, repo, number, assignees)
	return err
}

func (a *ghAdapter) CloseIssue(ctx context.Context, owner, repo string, number int) error {
	_, _, err := a.client.Issues.Edit(ctx, owner, repo, number, &github.IssueRequest{
		State: github.Ptr("closed"),
	})
	return err
}

func (a *ghAdapter) ReopenIssue(ctx context.Context, owner, repo string, number int) error {
	_, _, err := a.client.Issues.Edit(ctx, owner, repo, number, &github.IssueRequest{
		State: github.Ptr("open"),
	})
	return err
}

func (a *ghAdapter) AddReaction(ctx context.Context, owner, repo string, commentID int64, reaction string) error {
	_, _, err := a.client.Reactions.CreateCommentReaction(ctx, owner, repo, commentID, reaction)
	return err
}

// --- policy.PolicyAPI ---

func (a *ghAdapter) GetLabels(ctx context.Context, owner, repo string, number int) ([]string, error) {
	labels, _, err := a.client.Issues.ListLabelsByIssue(ctx, owner, repo, number, nil)
	if err != nil {
		return nil, err
	}
	names := make([]string, len(labels))
	for i, l := range labels {
		names[i] = l.GetName()
	}
	return names, nil
}

func (a *ghAdapter) SetCommitStatus(ctx context.Context, owner, repo, sha, state, statusContext, description string) error {
	_, _, err := a.client.Repositories.CreateStatus(ctx, owner, repo, sha, &github.RepoStatus{
		State:       github.Ptr(state),
		Context:     github.Ptr(statusContext),
		Description: github.Ptr(description),
	})
	return err
}

// --- PR files ---

func (a *ghAdapter) ListPRFiles(ctx context.Context, owner, repo string, number int) ([]string, error) {
	var files []string
	opts := &github.ListOptions{PerPage: 100}
	for {
		page, resp, err := a.client.PullRequests.ListFiles(ctx, owner, repo, number, opts)
		if err != nil {
			return nil, err
		}
		for _, f := range page {
			files = append(files, f.GetFilename())
		}
		if resp.NextPage == 0 {
			break
		}
		opts.Page = resp.NextPage
	}
	return files, nil
}
