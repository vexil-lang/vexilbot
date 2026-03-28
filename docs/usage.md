# vexilbot Usage

vexilbot is a GitHub App that automates labeling, triage, welcome messages, and policy enforcement for vexil-lang repositories. It runs as a stateless webhook server — all behavior is driven by GitHub events, and all state is derived from the GitHub API.

## Security

**Webhook verification.** Every incoming request is verified with HMAC-SHA256 against the webhook secret configured at deploy time. Requests with missing or invalid signatures are rejected with HTTP 401 before any code runs.

**Command permissions.** `@vexilbot` commands only execute for:
- Members of org teams listed in `[triage].allowed_teams`
- Repository collaborators with **write** or **admin** permission, if `allow_collaborators = true`

Read-level and triage-level collaborators cannot trigger commands. Access is controlled entirely through GitHub org and repo settings — there is no in-bot permission management.

**Blast radius.** The bot can add/remove labels, assign users, and open/close issues and PRs. It cannot push code, modify branches, or read secrets.

---

## Features

### Automatic labeling

Labels are applied automatically on PR open and issue creation — no commands needed.

**Path-based** (PRs only): labels are applied based on which files changed, matched against glob patterns configured in `[labels].paths`. `*` matches within a single directory segment; `**` matches across directories.

**Keyword-based** (issues and PRs): labels are applied when the title or body contains any of the configured substrings. Matching is case-insensitive.

### Welcome messages

When a contributor opens their first PR or files their first issue in a repository (zero prior PRs *and* zero prior issues), the bot posts a configurable message. The PR and issue messages are independent — configure one, both, or neither.

### Commands

Write `@vexilbot <command> [args]` on any line of an issue or PR comment. Multiple commands can appear in a single comment, one per line. On success the bot reacts with 👍 to the comment.

| Command | Args | Effect |
|---------|------|--------|
| `label` | one or more label names | Adds labels to the issue or PR |
| `unlabel` | one or more label names | Removes labels |
| `assign` | one or more usernames | Adds assignees |
| `prioritize` | `p0`, `p1`, `p2`, or `p3` | Sets exactly one priority label, removing any others |
| `close` | — | Closes the issue or PR |
| `reopen` | — | Reopens the issue or PR |

Example:
```
@vexilbot label bug good-first-issue
@vexilbot assign alice
@vexilbot prioritize p1
```

### Policy enforcement

**RFC gate.** When a PR touches paths listed in `[policy].rfc_required_paths`, the bot sets a pending commit status (`vexilbot/policy`) and posts a comment noting the requirement. The status clears to success once the `rfc` label is added. To make this a hard gate, configure branch protection to require the `vexilbot/policy` status check.

**Wire format warning.** When a PR touches paths listed in `[policy].wire_format_warning_paths`, the bot posts an advisory comment referencing the 14-day RFC comment period. This is informational only — it does not set a commit status or block merging.

### Release management

The release subsystem — conventional commits → semver bump → `Cargo.toml` update → `git-cliff` changelog → `cargo publish` — is implemented but not yet activated via commands. It will be wired in a future update.

---

## Configuration

Drop a `.vexilbot.toml` file in your repository root. All sections are optional; omitting a section disables the corresponding feature.

```toml
[labels]

# Path-based labels (PRs only).
# Maps a label name to a list of glob patterns matched against changed file paths.
paths = { "spec" = ["spec/**", "*.md"], "core" = ["src/core/**"] }

# Keyword-based labels (issues and PRs).
# Maps a label name to a list of substrings matched case-insensitively in title + body.
keywords = { "bug" = ["crash", "panic", "regression"], "docs" = ["documentation", "readme"] }


[triage]

# Org team slugs whose members may run @vexilbot commands.
# Use the slug from https://github.com/orgs/<org>/teams/<slug>.
allowed_teams = ["maintainers", "core-team"]

# If true, repo collaborators with write or admin permission may also run commands.
allow_collaborators = true


[welcome]

# Posted when a contributor opens their first PR. Empty string or omitted = disabled.
pr_message = """
Welcome, and thanks for your first PR!
Please make sure CI passes before requesting review.
"""

# Posted when a contributor files their first issue. Empty string or omitted = disabled.
issue_message = """
Thanks for reporting! Please include reproduction steps and the output of `vexil --version`.
"""


[policy]

# PRs touching these paths require the "rfc" label (sets a pending commit status).
rfc_required_paths = ["spec/**", "corpus/**/*.vx"]

# PRs touching these paths receive an advisory comment about the RFC comment period.
wire_format_warning_paths = ["src/wire/**", "encoding/**"]


[release]

changelog_tool   = "git-cliff"
tag_format       = "{{ crate }}-v{{ version }}"
auto_release     = false  # not yet active
require_ci       = true

[release.crates.my-crate]
path             = "crates/my-crate"
publish          = "crates.io"   # or false to skip publishing
suggest_threshold = 5            # suggest a release after N unreleased commits
depends_on       = ["my-base"]   # published after my-base; circular deps are rejected

[[release.crates.my-crate.post_publish]]
run = "cargo doc --no-deps"


[llm]
enabled = false  # LLM features are stubbed; not active
```

### Configuration caching

The bot fetches `.vexilbot.toml` from the GitHub API on first use and caches it per repository for five minutes. Changes to the file take effect within one cache TTL — no restart or manual invalidation needed.
