package triage

import (
	"context"
	"fmt"
)

type IssueAPI interface {
	AddLabels(ctx context.Context, owner, repo string, number int, labels []string) error
	RemoveLabel(ctx context.Context, owner, repo string, number int, label string) error
	AddAssignees(ctx context.Context, owner, repo string, number int, assignees []string) error
	CloseIssue(ctx context.Context, owner, repo string, number int) error
	ReopenIssue(ctx context.Context, owner, repo string, number int) error
	AddReaction(ctx context.Context, owner, repo string, commentID int64, reaction string) error
	CreateComment(ctx context.Context, owner, repo string, number int, body string) error
}

var priorities = []string{"p0", "p1", "p2", "p3"}

// Execute runs a parsed command against the GitHub API.
func Execute(ctx context.Context, api IssueAPI, cmd Command, owner, repo string, number int, commentID int64) error {
	switch cmd.Name {
	case "label":
		if len(cmd.Args) == 0 {
			return fmt.Errorf("label command requires at least one label")
		}
		if err := api.AddLabels(ctx, owner, repo, number, cmd.Args); err != nil {
			return err
		}
		return api.AddReaction(ctx, owner, repo, commentID, "+1")

	case "unlabel":
		if len(cmd.Args) == 0 {
			return fmt.Errorf("unlabel command requires a label")
		}
		for _, label := range cmd.Args {
			if err := api.RemoveLabel(ctx, owner, repo, number, label); err != nil {
				return err
			}
		}
		return api.AddReaction(ctx, owner, repo, commentID, "+1")

	case "assign":
		if len(cmd.Args) == 0 {
			return fmt.Errorf("assign command requires a username")
		}
		if err := api.AddAssignees(ctx, owner, repo, number, cmd.Args); err != nil {
			return err
		}
		return api.AddReaction(ctx, owner, repo, commentID, "+1")

	case "prioritize":
		if len(cmd.Args) != 1 {
			return fmt.Errorf("prioritize command requires exactly one priority (p0-p3)")
		}
		target := cmd.Args[0]
		valid := false
		for _, p := range priorities {
			if p == target {
				valid = true
			}
		}
		if !valid {
			return fmt.Errorf("invalid priority %q, must be one of: p0, p1, p2, p3", target)
		}
		for _, p := range priorities {
			if p != target {
				_ = api.RemoveLabel(ctx, owner, repo, number, p)
			}
		}
		if err := api.AddLabels(ctx, owner, repo, number, []string{target}); err != nil {
			return err
		}
		return api.AddReaction(ctx, owner, repo, commentID, "+1")

	case "close":
		if err := api.CloseIssue(ctx, owner, repo, number); err != nil {
			return err
		}
		return api.AddReaction(ctx, owner, repo, commentID, "+1")

	case "reopen":
		if err := api.ReopenIssue(ctx, owner, repo, number); err != nil {
			return err
		}
		return api.AddReaction(ctx, owner, repo, commentID, "+1")

	case "release", "rfc":
		return nil

	default:
		return fmt.Errorf("unknown command: %q", cmd.Name)
	}
}
