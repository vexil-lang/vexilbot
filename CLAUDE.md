# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project Overview

vexilbot is a stateless Go GitHub App bot that automates labeling, triage, welcome messages, policy enforcement, and release management for vexil-lang repositories. Single binary HTTP webhook server with no database — all state derived from GitHub API.

**GitHub App:** ID `<your-app-id>`, installed on `vexil-lang` org
**Production URL:** `https://<your-domain>/webhook`
**VPS:** `<user>@<vps-host>`, deployed at `<deploy-path>/vexilbot/`
**Container registry:** `ghcr.io/vexil-lang/vexilbot` (requires `gh auth token` login on VPS)

## Tech Stack

Go 1.25, google/go-github/v68, bradleyfalzon/ghinstallation/v2, BurntSushi/toml, log/slog (stdlib), net/http (stdlib), crypto/hmac for webhook verification. External tools: git-cliff (changelog), cargo (publishing).

## Commands

```bash
# Build
make build                    # or: go build -o bin/vexilbot ./cmd/vexilbot

# Test
make test                     # or: go test ./...
go test ./internal/labeler/   # single package

# Lint
make lint                     # or: go vet ./... && golangci-lint run

# Run
./bin/vexilbot <config-path>
```

## Architecture

Packages organized by feature under `internal/`:

- **serverconfig/** — Server-side TOML config (secrets, GitHub App credentials, listen address)
- **repoconfig/** — Repo-side `.vexilbot.toml` config (behavior rules per repo) with TTL cache
- **webhook/** — HTTP handler with HMAC-SHA256 signature verification and event routing
- **ghclient/** — GitHub App auth wrapper (JWT + installation tokens)
- **labeler/** — PR path-based labeling (glob matching) + issue keyword labeling. Uses `path.Match` (not `filepath.Match`) for cross-platform glob support
- **welcome/** — First-time contributor detection + welcome messages
- **triage/** — `@vexilbot` command parser, handlers (label/assign/prioritize/close/reopen), RBAC permissions
- **policy/** — RFC gate (label requirement for spec/corpus PRs), wire format deprecation warnings, RFC 14-day timer
- **release/** — Change detection, semver bumping via conventional commits, Cargo.toml updates, git-cliff changelog, release orchestration (branch → PR → publish), cargo publish + hooks
- **llm/** — Client interface stub for future Claude API integration

Entry point: `cmd/vexilbot/main.go`

## Two-Tier Configuration

1. **Server-side** (`config.toml`): listen address, webhook secret, GitHub App ID + private key path, cargo registry token, Anthropic API key
2. **Repo-side** (`.vexilbot.toml` in each repo root): label path/keyword rules, triage permissions, welcome messages, policy settings, release crate definitions, LLM feature flags

See `deploy/config.example.toml` for all fields with documentation.

## Docker Deployment

Multi-stage build: `golang:1.25-alpine` builder → `gcr.io/distroless/static-debian12:debug-nonroot` runtime (~4 MB image).

**Key notes:**
- Health check uses `CMD /busybox/wget` (not `CMD-SHELL`) — distroless has no `/bin/sh`
- Runtime config at `/etc/vexilbot/config.toml`, private key at `/run/secrets/github_app_key`
- On VPS: uses Traefik (not Caddy) on the `traefik-public` network with Let's Encrypt TLS
- `docker-compose.yml` is the Caddy-based reference; `<deploy-path>/vexilbot/docker-compose.yml` on VPS uses Traefik labels
- `docker-compose.override.yml` for local dev: `vexilbot:local` image, port 8080 exposed directly, Caddy skipped via profile

**Updating production:**
```bash
ssh <user>@<vps-host> "cd <deploy-path>/vexilbot && docker compose pull && docker compose up -d"
```

## CI/CD

- `.github/workflows/ci.yml` — test (Go 1.25), lint (golangci-lint), docker-build (verify image builds on every PR)
- `.github/workflows/docker.yml` — builds and pushes `ghcr.io/vexil-lang/vexilbot:latest` on push to master; builds only (no push) on PRs
- Tags: `latest` (default branch), `master` (branch), `sha-<commit>` (immutable)

## Known Issues / Gotchas

- `<your-domain>` root cert renewal failing in Traefik (pre-existing, unrelated to vexilbot) — root domain proxied via Cloudflare blocking ACME HTTP challenge
- Cloudflare DNS records for vexilbot must be **DNS only (grey cloud)**, not proxied, so Traefik can complete ACME challenges
- `CrateEntry.Publish` is `interface{}` — can be string (`"crates.io"`) or bool (`false`) in TOML
