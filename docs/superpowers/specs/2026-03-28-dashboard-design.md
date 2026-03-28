# Vexilbot Dashboard Design Spec

**Date:** 2026-03-28

## Goal

Add a read/write web dashboard to vexilbot for operational visibility and release management. Accessible via SSH tunnel only (no auth). Runs as a second HTTP listener in the same binary.

## Architecture

Single binary, two listeners:

- `0.0.0.0:<webhook_port>` — existing webhook server (unchanged)
- `127.0.0.1:<dashboard_port>` — dashboard HTTP server (default `8081`)

`dashboard_port = 8081` added to `config.toml`. Setting it to `0` disables the dashboard. Both servers start in `main.go`; the dashboard server runs in a goroutine.

Access control is handled at the infrastructure level: the dashboard port is bound to localhost only, and operators expose it via SSH tunnel. No application-level auth.

## Frontend

Go `html/template` + HTMX. No build step, no npm. Templates and static assets embedded via `embed.FS` in `internal/dashboard/`. HTMX loaded from CDN (inline in base template). Top-tabs navigation layout.

## Package Layout

```
internal/dashboard/
  server.go              — http.ServeMux, route registration, shared template helpers
  handlers_logs.go       — GET /logs — reads logstore, filter params
  handlers_events.go     — GET /events — reads eventstore, computes stats + chart data
  handlers_releases.go   — GET /releases, POST /releases, DELETE /releases/:id
  handlers_config.go     — GET /config — repo selector, fetches .vexilbot.toml from GitHub
  handlers_storage.go    — GET /storage — walks data_dir, per-.vxb file stats
  templates/
    base.html            — page shell: top-tabs nav, HTMX script tag
    logs.html
    events.html
    releases.html
    config.html
    storage.html
```

## Tabs

### Logs

- Reads `logs.vxb` via `vexstore.ReadAll()`, decodes each record with `logentry.Unpack`.
- Filters by `level`, `owner`, `repo` query params (server-side).
- Renders as a table with colored level badges.
- HTMX polling every 3 s on the table element only (not full page reload).

### Events

- Reads `events.vxb`, decodes with `webhookevent.Unpack`.
- Computes: total count today, count by kind, hourly bucket counts for last 24 h.
- Renders summary stat cards + a bar chart built from plain HTML `<div>` elements sized by inline `style` (no JS charting library).

### Releases

- Reads and writes `scheduled_releases.vxb` (new store, new schema `scheduledrelease`).
- Lists pending releases with: crate/package name, bump level, scheduled time, auto/manual badge.
- Actions:
  - **Run now** — calls `release.Orchestrator.Run(ctx, pkg, bumpLevel)` directly from the handler, bypassing the scheduler. Runs in a goroutine; response redirects back to the releases tab.
  - **Confirm** — for manual-confirmation releases: marks ready to run at scheduled time.
  - **Cancel** — soft-delete via status field in the record.
- **Schedule release** button opens an inline form: package name, bump level (patch/minor/major), scheduled datetime, auto vs. manual toggle.
- POST `/releases` creates a new record; DELETE `/releases/:id` cancels.

### Config

- Repo selector `<select>` populated from server config (known repos).
- On change (`hx-get`), fetches `.vexilbot.toml` for the selected repo from GitHub API directly (not through the TTL cache — always live).
- Renders content as read-only `<pre>` block.
- Also shows rendered server `config.toml` (secrets redacted: webhook secret, private key path, tokens shown as `[redacted]`).

### Storage

- Walks `data_dir`, finds all `*.vxb` files.
- For each file: filename, record count, file size on disk, oldest record timestamp, newest record timestamp, schema hash (hex, truncated to 8 chars).
- Rendered as a simple table. No editing.

## New Schema: `scheduledrelease`

```
namespace scheduledrelease

enum BumpLevel : u8 {
  Patch @0
  Minor @1
  Major @2
}

enum ReleaseStatus : u8 {
  Pending   @0
  Confirmed @1
  Running   @2
  Done      @3
  Cancelled @4
}

message ScheduledRelease {
  id         @0 : string   // UUID v4, generated at creation time
  package    @1 : string
  bump       @2 : BumpLevel
  run_at     @3 : u64
  auto_run   @4 : bool
  status     @5 : ReleaseStatus
  created_at @6 : u64
}
```

Generated into `internal/vexstore/gen/scheduledrelease/` via `vexilc --target go`.

Store opened at startup: `scheduled_releases.vxb`.

## Config Changes

`serverconfig.Server` gains:

```toml
dashboard_port = 8081   # set to 0 to disable
```

`deploy/config.example.toml` updated accordingly.

## Docker

Named volume `vexilbot-data` mounted at `/data` in the container. `config.toml` sets `data_dir = "/data"`.

`docker-compose.yml` (production):
```yaml
volumes:
  vexilbot-data:

services:
  vexilbot:
    volumes:
      - vexilbot-data:/data
```

`docker-compose.override.yml` (dev) gets the same volume mount.

## What This Does Not Include

- Authentication or session management (SSH tunnel is the access control).
- Log streaming via WebSocket (HTMX polling is sufficient for the use case).
- Editing config files (Config tab is read-only).
- Metrics export (Prometheus/OpenTelemetry) — separate concern.
- Release history view (can be added later from the `logs.vxb` data).
