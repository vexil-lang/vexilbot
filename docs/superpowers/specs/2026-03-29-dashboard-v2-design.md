# Dashboard v2 — Full Repo Management & Editable Config

**Date:** 2026-03-29
**Status:** Approved

## Overview

Overhaul the vexilbot dashboard from a read-only monitoring tool into a full management interface. Adds repo-scoped issue/PR management with triage actions, editable config with local overrides, and a sidebar navigation layout that replaces the current top-tab bar.

## Architecture

### Navigation: Sidebar Layout

Replace the current 5-tab top bar with a fixed left sidebar (200px wide). The sidebar has two sections:

**Repo-scoped** (requires a selected repo):
- Issues
- Pull Requests
- Releases
- Repo Config

**Global** (not repo-scoped):
- Logs
- Events
- Server Config
- Storage

A repo selector dropdown sits at the top of the sidebar. Repos are populated from the `installationStore` (known repos from incoming webhooks). The selected repo is persisted via a query parameter (`?repo=vexil-lang/vexil`) so it survives page navigation.

### Identity

All GitHub actions taken from the dashboard go through vexilbot's GitHub App identity. When the dashboard labels an issue, it's the bot labeling it — same as if someone typed `@vexil-bot label bug` in a comment. This keeps a consistent audit trail on GitHub.

## New Pages

### Issues Page (`/issues`)

**List view (left pane):**
- Fetches all open issues for the selected repo via GitHub API (`GET /repos/{owner}/{repo}/issues?state=open`)
- Paginated (30 per page, load-more or pagination controls)
- Filters: label, assignee, priority (p0-p3), state (open/closed)
- Each row shows: number, title, labels (as colored badges), assignees, age
- HTMX-driven filtering — form changes trigger partial page updates

**Detail panel (right pane):**
- Appears when an issue is clicked in the list
- Shows: title, number, state badge, author, opened date, description (markdown rendered as plain text or basic HTML)
- Current labels displayed as removable badges (click X to unlabel)
- Current assignees displayed similarly

**Actions (in detail panel):**
- **Add label:** Text input with submit, calls GitHub API to add label
- **Remove label:** Click X on existing label badge
- **Assign:** Text input for GitHub username, calls API to add assignee
- **Prioritize:** Dropdown (p0/p1/p2/p3), removes existing priority labels and adds selected one
- **Close / Reopen:** Button toggles issue state
- All actions POST to dashboard endpoints, which use the bot's installation client to call GitHub API

### Pull Requests Page (`/pulls`)

Same list+detail layout as Issues. Additional features:

- Shows: merge status, CI status, head branch name, changed file count
- **Changed files list:** Expandable section showing file paths touched by the PR
- **RFC gate indicator:** Shows whether the PR is blocked by missing RFC label (reads commit status `vexilbot/policy`)
- **Merge button:** For release PRs or any PR — calls `MergePR` via the bot
- Same triage actions as issues: label, unlabel, assign, prioritize, close/reopen

### Repo Config Page (`/config/repo`)

**Two-pane layout:**

*Left pane — Base config (read-only):*
- Fetched `.vexilbot.toml` from the repo via GitHub API
- Displayed as formatted TOML in a `<pre>` block
- Refreshed on page load (uses existing config cache with TTL)

*Right pane — Local overrides (editable):*
- Form-based editor for each config section
- Sections mirror the `.vexilbot.toml` schema:
  - **Labels > Paths:** Add/remove glob patterns per label
  - **Labels > Keywords:** Add/remove keywords per label
  - **Welcome:** Edit PR and issue welcome messages
  - **Triage:** Edit allowed teams, toggle allow_collaborators
  - **Policy:** Edit RFC-required paths, wire format warning paths
  - **Release:** Edit crate/package definitions, tag format
- Each field shows its base value (from repo) with an override toggle
- Overrides are stored locally (see Config Override System below)
- A "diff" indicator shows which fields have local overrides

### Server Config Page (`/config/server`)

Split into two sections:

**Read-only fields** (require restart to change):
- `listen` — server listen address
- `app_id` — GitHub App ID
- `private_key_path` — path to PEM file
- `webhook_secret` — displayed as `[redacted]`
- `dashboard_port` — dashboard listen port
- `data_dir` — data directory path
- `cargo_registry_token` — displayed as `[redacted]`
- `anthropic_api_key` — displayed as `[redacted]`

**Hot-editable fields** (take effect immediately):
- `bot_name` — the @mention name the bot responds to
- Saved to a server override file, merged at runtime
- No restart needed — the config value is read fresh on each webhook

## Config Override System

### Storage

Per-repo overrides stored as TOML files in the data directory:
```
<data_dir>/overrides/<owner>-<repo>.toml
```

Example: `/data/overrides/vexil-lang-vexil.toml`

Server-level hot-editable overrides stored at:
```
<data_dir>/overrides/server.toml
```

### Schema

Override files use the same schema as `.vexilbot.toml` (for repo overrides) or `config.toml` (for server overrides). Only fields that differ from the base are present.

### Merge Behavior

At runtime, when `configCache.Get(ctx, owner, repo)` is called:
1. Fetch `.vexilbot.toml` from the repo (existing behavior)
2. Check for `<data_dir>/overrides/<owner>-<repo>.toml`
3. If override file exists, merge its values on top of the fetched config
4. Merge is per-field: override values replace base values. For map fields (like `labels.paths`), the override map is merged key-by-key (override keys replace base keys, base-only keys are preserved).

### Dashboard Endpoints

- `GET /config/repo/overrides?repo=owner/repo` — returns current override TOML
- `POST /config/repo/overrides?repo=owner/repo` — saves override TOML (form body)
- `DELETE /config/repo/overrides?repo=owner/repo` — removes all overrides for repo
- `POST /config/server/overrides` — saves server override (hot-editable fields only)

## Updated Existing Pages

### Releases Page (`/releases`)

Keep the current scheduled releases table and schedule form. Add:

- **Release status view:** Shows unreleased changes per crate/package for the selected repo (equivalent of `@bot release status`). Table with: package name, current version, unreleased commit count, suggested bump level.
- **Trigger workspace release:** Button that calls `RunWorkspaceRelease` for the selected repo
- **Trigger single release:** Per-package "Release" button in the status table

### Logs Page (`/logs`)

No changes — keep current implementation with level/owner/repo filters and HTMX polling.

### Events Page (`/events`)

No changes — keep current implementation with today's stats and hourly bar chart.

### Storage Page (`/storage`)

No changes — keep current `.vxb` file stats table.

## API Endpoints (New)

### Issue/PR Management
- `GET /issues?repo=owner/repo&label=X&assignee=Y&priority=Z&state=open` — list issues
- `GET /issues/{number}?repo=owner/repo` — issue detail (for HTMX partial)
- `POST /issues/{number}/label?repo=owner/repo` — add labels (form: `label=bug&label=p1`)
- `POST /issues/{number}/unlabel?repo=owner/repo` — remove labels
- `POST /issues/{number}/assign?repo=owner/repo` — assign users
- `POST /issues/{number}/prioritize?repo=owner/repo` — set priority
- `POST /issues/{number}/close?repo=owner/repo` — close issue/PR
- `POST /issues/{number}/reopen?repo=owner/repo` — reopen issue/PR
- `POST /issues/{number}/merge?repo=owner/repo` — merge PR
- `GET /pulls?repo=owner/repo&...` — list PRs (same filters as issues)
- `GET /pulls/{number}?repo=owner/repo` — PR detail
- `GET /pulls/{number}/files?repo=owner/repo` — PR changed files

### Config Overrides
- `GET /config/repo/overrides?repo=owner/repo`
- `POST /config/repo/overrides?repo=owner/repo`
- `DELETE /config/repo/overrides?repo=owner/repo`
- `POST /config/server/overrides`

### Release Status
- `GET /releases/status?repo=owner/repo` — unreleased changes per package
- `POST /releases/workspace?repo=owner/repo` — trigger workspace release
- `POST /releases/single?repo=owner/repo&package=name` — trigger single package release

## Technical Details

### GitHub API Usage

Issues and PRs are fetched live from the GitHub API on each page load. No local caching beyond the HTTP-level caching GitHub provides (ETags). The dashboard uses the bot's installation token for the selected repo.

For actions, the dashboard creates a `ghAdapter` using the installation client (same as the webhook handler does) and calls the appropriate GitHub API method.

### Template Changes

- `base.html` is rewritten: replaces the top-bar `{{define "nav"}}` with a sidebar template
- Each page template is updated to use the new sidebar layout
- New templates: `issues.html`, `issue-detail.html` (HTMX partial), `pulls.html`, `pull-detail.html`, `pull-files.html`, `repo-config.html`, `server-config.html`
- Existing templates (`logs.html`, `events.html`, `releases.html`, `storage.html`) updated for sidebar layout

### Handler Organization

New handler files:
- `handlers_issues.go` — issue list, detail, all triage actions
- `handlers_pulls.go` — PR list, detail, files, merge
- `handlers_config_overrides.go` — repo and server config override CRUD
- `handlers_release_status.go` — release status view and trigger endpoints

### Deps Changes

The `dashboard.Deps` struct needs additional fields:
- `GetInstallationClient func(owner, repo string) (*github.Client, error)` — to make API calls for the selected repo
- `MergeReleasePR func(ctx context.Context, owner, repo string, prNumber int) error` — for merge action
- `RunWorkspaceRelease func(ctx context.Context, owner, repo string) (int, error)` — for workspace release trigger

The existing `RunRelease`, `FetchRepoConfig`, and `KnownRepos` deps are kept.

## Docker / Deployment

No changes to Docker setup. Override files are written to `data_dir` which is already a persistent named volume (`vexilbot-data:/data`).

## Security

No authentication added. Access control is SSH-tunnel-only (dashboard binds to `0.0.0.0:<port>`, published on host as `127.0.0.1:<port>`). The SSH key + Hetzner 2FA provides the access boundary.
