package policy

import (
	"context"
	"fmt"

	"github.com/vexil-lang/vexilbot/internal/labeler"
)

type RFCResult int

const (
	RFCNotApplicable RFCResult = iota
	RFCSatisfied
	RFCRequired
)

func (r RFCResult) String() string {
	switch r {
	case RFCNotApplicable:
		return "not_applicable"
	case RFCSatisfied:
		return "satisfied"
	case RFCRequired:
		return "required"
	default:
		return "unknown"
	}
}

type PolicyAPI interface {
	GetLabels(ctx context.Context, owner, repo string, number int) ([]string, error)
	SetCommitStatus(ctx context.Context, owner, repo, sha, state, statusContext, description string) error
	CreateComment(ctx context.Context, owner, repo string, number int, body string) error
}

const rfcStatusContext = "vexilbot/policy"

// CheckRFCGate checks if a PR touching RFC-required paths has the rfc label.
func CheckRFCGate(
	ctx context.Context,
	api PolicyAPI,
	owner, repo string,
	number int,
	sha string,
	rfcRequiredPaths []string,
	changedFiles []string,
) (RFCResult, error) {
	touchesRFCPaths := false
	for _, file := range changedFiles {
		for _, pattern := range rfcRequiredPaths {
			if labeler.MatchGlob(pattern, file) {
				touchesRFCPaths = true
				break
			}
		}
		if touchesRFCPaths {
			break
		}
	}

	if !touchesRFCPaths {
		return RFCNotApplicable, nil
	}

	labels, err := api.GetLabels(ctx, owner, repo, number)
	if err != nil {
		return RFCNotApplicable, fmt.Errorf("get labels: %w", err)
	}

	hasRFC := false
	for _, l := range labels {
		if l == "rfc" {
			hasRFC = true
			break
		}
	}

	if hasRFC {
		if err := api.SetCommitStatus(ctx, owner, repo, sha, "success", rfcStatusContext, "RFC label present"); err != nil {
			return RFCSatisfied, fmt.Errorf("set commit status: %w", err)
		}
		return RFCSatisfied, nil
	}

	if err := api.SetCommitStatus(ctx, owner, repo, sha, "pending", rfcStatusContext, "RFC label required"); err != nil {
		return RFCRequired, fmt.Errorf("set commit status: %w", err)
	}

	comment := "This PR modifies files that require an RFC per [GOVERNANCE.md](../GOVERNANCE.md). " +
		"Please open an RFC issue first, or add the `rfc` label if one already exists."
	if err := api.CreateComment(ctx, owner, repo, number, comment); err != nil {
		return RFCRequired, fmt.Errorf("create comment: %w", err)
	}

	return RFCRequired, nil
}
