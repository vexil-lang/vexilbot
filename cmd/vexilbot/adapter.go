package main

import (
	"context"
	"fmt"

	"github.com/google/go-github/v68/github"
	"github.com/vexil-lang/vexilbot/internal/release"
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

// --- release.GitAPI ---

func (a *ghAdapter) ListTags(ctx context.Context, owner, repo string) ([]string, error) {
	var tags []string
	opts := &github.ListOptions{PerPage: 100}
	for {
		page, resp, err := a.client.Repositories.ListTags(ctx, owner, repo, opts)
		if err != nil {
			return nil, err
		}
		for _, t := range page {
			tags = append(tags, t.GetName())
		}
		if resp.NextPage == 0 {
			break
		}
		opts.Page = resp.NextPage
	}
	return tags, nil
}

func (a *ghAdapter) CommitsSinceTag(ctx context.Context, owner, repo, tag, path string) ([]release.Commit, error) {
	if tag == "" {
		// No tag exists — list all commits touching this path
		return a.listCommitsByPath(ctx, owner, repo, path)
	}

	// Use the Compare API: tag...HEAD gives exactly the commits between them.
	comparison, _, err := a.client.Repositories.CompareCommits(ctx, owner, repo, tag, "HEAD", nil)
	if err != nil {
		return nil, fmt.Errorf("compare %s...HEAD: %w", tag, err)
	}

	// Filter commits to only those touching the given path
	var commits []release.Commit
	for _, c := range comparison.Commits {
		// Check if any file in this commit matches the path prefix
		// The Compare API includes file lists per commit only at detail level,
		// so we filter by checking the full commit's files.
		// For efficiency, do a second pass: list commits by path since the tag.
		commits = append(commits, release.Commit{
			SHA:     c.GetSHA(),
			Message: c.Commit.GetMessage(),
		})
	}

	// The Compare API returns ALL commits between tag and HEAD, not filtered by path.
	// Filter by re-listing commits with path filter, but bounded by the tag's date.
	if path != "" {
		return a.listCommitsByPathSinceTag(ctx, owner, repo, tag, path)
	}

	return commits, nil
}

// listCommitsByPathSinceTag uses the Compare API to get the exact set of
// commits between a tag and HEAD, then filters to only those touching the
// given path prefix.
func (a *ghAdapter) listCommitsByPathSinceTag(ctx context.Context, owner, repo, tag, path string) ([]release.Commit, error) {
	comparison, _, err := a.client.Repositories.CompareCommits(ctx, owner, repo, tag, "HEAD", nil)
	if err != nil {
		return nil, fmt.Errorf("compare %s...HEAD: %w", tag, err)
	}

	// Build set of SHAs from the comparison (these are the actual commits between tag and HEAD)
	compareSHAs := make(map[string]string) // SHA → message
	for _, c := range comparison.Commits {
		compareSHAs[c.GetSHA()] = c.Commit.GetMessage()
	}

	if len(compareSHAs) == 0 {
		return nil, nil
	}

	// Now list commits by path on the default branch to get only those touching our path
	opts := &github.CommitsListOptions{
		Path:        path,
		ListOptions: github.ListOptions{PerPage: 100},
	}
	var commits []release.Commit
	for {
		page, resp, err := a.client.Repositories.ListCommits(ctx, owner, repo, opts)
		if err != nil {
			return nil, err
		}
		for _, c := range page {
			sha := c.GetSHA()
			if msg, ok := compareSHAs[sha]; ok {
				commits = append(commits, release.Commit{
					SHA:     sha,
					Message: msg,
				})
			}
		}
		// Stop early if we've found all comparison commits or gone past them
		if resp.NextPage == 0 || len(commits) >= len(compareSHAs) {
			break
		}
		opts.Page = resp.NextPage
	}
	return commits, nil
}

// listCommitsByPath lists all commits touching a path (no tag filter).
func (a *ghAdapter) listCommitsByPath(ctx context.Context, owner, repo, path string) ([]release.Commit, error) {
	opts := &github.CommitsListOptions{
		Path:        path,
		ListOptions: github.ListOptions{PerPage: 100},
	}
	var commits []release.Commit
	for {
		page, resp, err := a.client.Repositories.ListCommits(ctx, owner, repo, opts)
		if err != nil {
			return nil, err
		}
		for _, c := range page {
			commits = append(commits, release.Commit{
				SHA:     c.GetSHA(),
				Message: c.Commit.GetMessage(),
			})
		}
		if resp.NextPage == 0 {
			break
		}
		opts.Page = resp.NextPage
	}
	return commits, nil
}

// --- release.ReleaseAPI ---

func (a *ghAdapter) GetFileContent(ctx context.Context, owner, repo, path string) ([]byte, string, error) {
	content, _, _, err := a.client.Repositories.GetContents(ctx, owner, repo, path, nil)
	if err != nil {
		return nil, "", err
	}
	decoded, err := content.GetContent()
	if err != nil {
		return nil, "", err
	}
	return []byte(decoded), content.GetSHA(), nil
}

func (a *ghAdapter) GetFileContentRef(ctx context.Context, owner, repo, path, ref string) ([]byte, string, error) {
	opts := &github.RepositoryContentGetOptions{Ref: ref}
	content, _, _, err := a.client.Repositories.GetContents(ctx, owner, repo, path, opts)
	if err != nil {
		return nil, "", err
	}
	decoded, err := content.GetContent()
	if err != nil {
		return nil, "", err
	}
	return []byte(decoded), content.GetSHA(), nil
}

func (a *ghAdapter) GetDefaultBranch(ctx context.Context, owner, repo string) (string, error) {
	r, _, err := a.client.Repositories.Get(ctx, owner, repo)
	if err != nil {
		return "", err
	}
	return r.GetDefaultBranch(), nil
}

func (a *ghAdapter) GetBranchSHA(ctx context.Context, owner, repo, branch string) (string, error) {
	ref, _, err := a.client.Git.GetRef(ctx, owner, repo, "heads/"+branch)
	if err != nil {
		return "", err
	}
	return ref.Object.GetSHA(), nil
}

func (a *ghAdapter) CreateBranch(ctx context.Context, owner, repo, branch, sha string) error {
	_, _, err := a.client.Git.CreateRef(ctx, owner, repo, &github.Reference{
		Ref:    github.Ptr("refs/heads/" + branch),
		Object: &github.GitObject{SHA: github.Ptr(sha)},
	})
	return err
}

func (a *ghAdapter) UpdateFile(ctx context.Context, owner, repo, path, message string, content []byte, sha, branch string) error {
	opts := &github.RepositoryContentFileOptions{
		Message: github.Ptr(message),
		Content: content,
		Branch:  github.Ptr(branch),
	}
	if sha != "" {
		opts.SHA = github.Ptr(sha)
	}
	_, _, err := a.client.Repositories.UpdateFile(ctx, owner, repo, path, opts)
	return err
}

func (a *ghAdapter) CreatePR(ctx context.Context, owner, repo, title, body, head, base string) (int, error) {
	pr, _, err := a.client.PullRequests.Create(ctx, owner, repo, &github.NewPullRequest{
		Title: github.Ptr(title),
		Body:  github.Ptr(body),
		Head:  github.Ptr(head),
		Base:  github.Ptr(base),
	})
	if err != nil {
		return 0, err
	}
	return pr.GetNumber(), nil
}

func (a *ghAdapter) MergePR(ctx context.Context, owner, repo string, number int, method string) error {
	opts := &github.PullRequestOptions{MergeMethod: method}
	_, _, err := a.client.PullRequests.Merge(ctx, owner, repo, number, "", opts)
	return err
}

func (a *ghAdapter) CreateTag(ctx context.Context, owner, repo, tag, sha string) error {
	ref := "refs/tags/" + tag
	_, _, err := a.client.Git.CreateRef(ctx, owner, repo, &github.Reference{
		Ref:    github.Ptr(ref),
		Object: &github.GitObject{SHA: github.Ptr(sha)},
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
