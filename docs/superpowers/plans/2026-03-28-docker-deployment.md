# Docker Deployment Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Package vexilbot as a minimal Docker image and provide a production-ready docker-compose stack with TLS termination via Caddy, secret injection via env/mounted files, and a GitHub Actions workflow that builds and pushes the image on every push to `main`.

**Architecture:** Multi-stage Docker build (builder → distroless/scratch final image). Secrets (webhook secret, GitHub App private key, cargo token) are mounted as files or passed as environment variables at runtime — never baked into the image. Caddy runs as a sidecar in docker-compose, terminates TLS, and reverse-proxies to vexilbot on port 8080.

**Tech Stack:** Docker (multi-stage build), docker-compose v2, Caddy 2, GitHub Actions (`docker/build-push-action`), Go 1.25 (existing).

---

## File Structure

```
Dockerfile                          Multi-stage build: builder + minimal runtime image
docker-compose.yml                  Production stack: vexilbot + Caddy
docker-compose.override.yml         Local dev overrides (no TLS, bind-mount config)
deploy/caddy/Caddyfile              Caddy TLS config (replaces deploy/Caddyfile)
deploy/config.example.toml         Example server-side config (no secrets)
.dockerignore                       Exclude test files, worktrees, docs from build context
.github/workflows/docker.yml        Build + push image to ghcr.io on push to main
```

---

## Task 1: Dockerfile (Multi-Stage Build)

**Files:**
- Create: `Dockerfile`
- Create: `.dockerignore`

- [ ] **Step 1: Create `.dockerignore`**

```
.worktrees/
bin/
docs/
*.md
*_test.go
.git/
.github/
deploy/
```

- [ ] **Step 2: Create `Dockerfile`**

```dockerfile
# ---- builder ----
FROM golang:1.25-alpine AS builder

WORKDIR /src

# Cache dependency downloads separately from source
COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags="-s -w" -o /vexilbot ./cmd/vexilbot

# ---- runtime ----
FROM gcr.io/distroless/static-debian12:nonroot

COPY --from=builder /vexilbot /vexilbot

# Config is mounted at runtime — not baked in
ENTRYPOINT ["/vexilbot", "/etc/vexilbot/config.toml"]
```

Key points:
- `CGO_ENABLED=0` — pure Go binary, no libc needed
- `-trimpath -ldflags="-s -w"` — strip debug info, smaller binary
- `distroless/static-debian12:nonroot` — no shell, runs as UID 65532
- Config file is injected at `/etc/vexilbot/config.toml` via Docker volume/secret at runtime

- [ ] **Step 3: Build locally to verify**

```bash
docker build -t vexilbot:local .
```

Expected: build completes, final image is ~10-15 MB.

```bash
docker image inspect vexilbot:local --format '{{.Size}}'
```

Expected: a number under 20000000 (20 MB).

- [ ] **Step 4: Smoke-test the image fails gracefully without config**

```bash
docker run --rm vexilbot:local 2>&1
```

Expected: exits with a non-zero code and prints an error about reading config (the file doesn't exist — that's correct behavior).

- [ ] **Step 5: Commit**

```bash
git add Dockerfile .dockerignore
git commit -m "feat: multi-stage Docker build producing minimal distroless image"
```

---

## Task 2: Example Config File

**Files:**
- Create: `deploy/config.example.toml`

The image requires a config file at runtime. Operators copy this file, fill in their secrets, and mount it.

- [ ] **Step 1: Create `deploy/config.example.toml`**

```toml
[server]
# Address the HTTP server listens on inside the container.
# Caddy reverse-proxies to this — do not expose port 8080 publicly.
listen = "0.0.0.0:8080"

# GitHub sends this secret with every webhook. Generate with:
#   openssl rand -hex 32
webhook_secret = "REPLACE_WITH_WEBHOOK_SECRET"

[github]
# Your GitHub App ID (visible on the App settings page)
app_id = 0

# Path to the GitHub App private key PEM file inside the container.
# Mount it as a Docker secret or volume — never bake it into the image.
private_key_path = "/run/secrets/github_app_key"

[credentials]
# crates.io registry token for cargo publish (optional)
cargo_registry_token = ""

[llm]
# Anthropic API key for future LLM features (optional)
anthropic_api_key = ""
```

- [ ] **Step 2: Commit**

```bash
git add deploy/config.example.toml
git commit -m "docs: example server config with all fields documented"
```

---

## Task 3: docker-compose Production Stack

**Files:**
- Create: `docker-compose.yml`
- Create: `deploy/caddy/Caddyfile` (replaces the root-level `deploy/Caddyfile`)

- [ ] **Step 1: Create `deploy/caddy/Caddyfile`**

```
{
    # Caddy global options
    email admin@vexil-lang.dev
}

bot.vexil-lang.dev {
    reverse_proxy vexilbot:8080
}
```

- [ ] **Step 2: Create `docker-compose.yml`**

```yaml
services:
  vexilbot:
    image: ghcr.io/vexil-lang/vexilbot:latest
    restart: unless-stopped
    # No published ports — Caddy reaches vexilbot on the internal network
    expose:
      - "8080"
    volumes:
      - ./config.toml:/etc/vexilbot/config.toml:ro
      - ./secrets/github_app_key.pem:/run/secrets/github_app_key:ro
    healthcheck:
      test: ["CMD-SHELL", "wget -qO- http://localhost:8080/healthz || exit 1"]
      interval: 30s
      timeout: 5s
      retries: 3
      start_period: 5s
    logging:
      driver: json-file
      options:
        max-size: "10m"
        max-file: "5"

  caddy:
    image: caddy:2-alpine
    restart: unless-stopped
    ports:
      - "80:80"
      - "443:443"
      - "443:443/udp"    # HTTP/3
    volumes:
      - ./deploy/caddy/Caddyfile:/etc/caddy/Caddyfile:ro
      - caddy_data:/data
      - caddy_config:/config
    depends_on:
      vexilbot:
        condition: service_healthy

volumes:
  caddy_data:
  caddy_config:
```

Notes for operators:
- Place a filled-in `config.toml` (copied from `deploy/config.example.toml`) next to `docker-compose.yml`
- Place the GitHub App private key at `secrets/github_app_key.pem`
- Both files are read-only mounted — not copied into the image

- [ ] **Step 3: Create `docker-compose.override.yml` for local dev**

```yaml
# Local dev: use local image, expose port directly, no TLS
services:
  vexilbot:
    image: vexilbot:local
    ports:
      - "8080:8080"
    volumes:
      - ./deploy/config.example.toml:/etc/vexilbot/config.toml:ro
      - /dev/null:/run/secrets/github_app_key:ro

  caddy:
    profiles: ["prod"]   # skip Caddy in dev
```

- [ ] **Step 4: Validate compose file syntax**

```bash
docker compose config --quiet
```

Expected: exits 0 with no errors.

- [ ] **Step 5: Commit**

```bash
git add docker-compose.yml docker-compose.override.yml deploy/caddy/Caddyfile
git commit -m "feat: docker-compose production stack with Caddy TLS and health checks"
```

---

## Task 4: GitHub Actions Docker Build + Push Workflow

**Files:**
- Create: `.github/workflows/docker.yml`

This builds the image and pushes it to `ghcr.io/vexil-lang/vexilbot` on every push to `main`. It also builds (but doesn't push) on pull requests to catch Dockerfile regressions early.

- [ ] **Step 1: Create `.github/workflows/docker.yml`**

```yaml
name: Docker

on:
  push:
    branches: [main, master]
  pull_request:
    branches: [main, master]

concurrency:
  group: ${{ github.workflow }}-${{ github.ref }}
  cancel-in-progress: true

env:
  REGISTRY: ghcr.io
  IMAGE_NAME: ${{ github.repository }}

jobs:
  build:
    runs-on: ubuntu-latest
    permissions:
      contents: read
      packages: write

    steps:
      - uses: actions/checkout@v4

      - name: Set up Docker Buildx
        uses: docker/setup-buildx-action@v3

      - name: Log in to GHCR
        if: github.event_name == 'push'
        uses: docker/login-action@v3
        with:
          registry: ${{ env.REGISTRY }}
          username: ${{ github.actor }}
          password: ${{ secrets.GITHUB_TOKEN }}

      - name: Extract metadata
        id: meta
        uses: docker/metadata-action@v5
        with:
          images: ${{ env.REGISTRY }}/${{ env.IMAGE_NAME }}
          tags: |
            type=ref,event=branch
            type=sha,prefix=sha-
            type=raw,value=latest,enable={{is_default_branch}}

      - name: Build and push
        uses: docker/build-push-action@v6
        with:
          context: .
          push: ${{ github.event_name == 'push' }}
          tags: ${{ steps.meta.outputs.tags }}
          labels: ${{ steps.meta.outputs.labels }}
          cache-from: type=gha
          cache-to: type=gha,mode=max
```

On push to `main`/`master` this produces three tags:
- `ghcr.io/vexil-lang/vexilbot:latest`
- `ghcr.io/vexil-lang/vexilbot:master` (or `main`)
- `ghcr.io/vexil-lang/vexilbot:sha-<short-sha>`

On pull requests it builds but does not push (validates Dockerfile only).

- [ ] **Step 2: Commit**

```bash
git add .github/workflows/docker.yml
git commit -m "ci: build and push Docker image to ghcr.io on push to main"
```

---

## Task 5: Deployment Runbook (deploy/README.md update)

**Files:**
- Modify: `deploy/README.md`

- [ ] **Step 1: Replace `deploy/README.md` with Docker-focused runbook**

```markdown
# Deploying vexilbot

vexilbot runs as a Docker container. Caddy handles TLS automatically.

## Prerequisites

- Docker Engine 24+ and Docker Compose v2
- A domain pointing to your server (for TLS)
- A registered GitHub App with a private key
- Port 80 and 443 open on the server firewall

## Initial Setup

### 1. Clone the repo (or copy the deploy files)

```bash
git clone https://github.com/vexil-lang/vexilbot.git
cd vexilbot
```

### 2. Create your config file

```bash
cp deploy/config.example.toml config.toml
# Edit config.toml — fill in webhook_secret and app_id
$EDITOR config.toml
```

### 3. Add your GitHub App private key

```bash
mkdir -p secrets
cp /path/to/your/app.private-key.pem secrets/github_app_key.pem
chmod 600 secrets/github_app_key.pem
```

### 4. Update the Caddyfile with your domain

Edit `deploy/caddy/Caddyfile` and replace `bot.vexil-lang.dev` with your actual domain.

### 5. Start the stack

```bash
docker compose up -d
```

Caddy will automatically obtain a TLS certificate from Let's Encrypt on first start.

### 6. Configure your GitHub App

Set the webhook URL to: `https://your-domain.com/webhook`

### 7. Verify

```bash
curl https://your-domain.com/healthz
# Expected: {"status":"ok"}
```

## Updating

```bash
docker compose pull
docker compose up -d
```

## Logs

```bash
docker compose logs -f vexilbot
docker compose logs -f caddy
```

## Rotating Secrets

1. Update `config.toml` or replace `secrets/github_app_key.pem`
2. `docker compose restart vexilbot`
```

- [ ] **Step 2: Commit**

```bash
git add deploy/README.md
git commit -m "docs: Docker-focused deployment runbook"
```

---

## Task 6: Add `.github/workflows/docker.yml` Branch Protection Note + CI Integration

**Files:**
- Modify: `.github/workflows/ci.yml`

The existing CI workflow runs `go test`. Add a job that verifies the Docker build succeeds on every PR, so both test and build must pass before merge.

- [ ] **Step 1: Add `docker-build` job to `.github/workflows/ci.yml`**

Open `.github/workflows/ci.yml` and append this job at the end:

```yaml
  docker-build:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4

      - name: Set up Docker Buildx
        uses: docker/setup-buildx-action@v3

      - name: Build image (no push)
        uses: docker/build-push-action@v6
        with:
          context: .
          push: false
          tags: vexilbot:ci
          cache-from: type=gha
          cache-to: type=gha,mode=max
```

This means CI now has 3 jobs: `test`, `lint`, `docker-build` — all must pass.

- [ ] **Step 2: Run locally to verify Dockerfile still builds**

```bash
docker build -t vexilbot:ci-verify . && echo "BUILD OK"
```

Expected: `BUILD OK`

- [ ] **Step 3: Commit**

```bash
git add .github/workflows/ci.yml
git commit -m "ci: verify Docker build passes on every PR"
```

---

## Task 7: Push All Changes to GitHub

- [ ] **Step 1: Push to remote**

```bash
git push origin master
```

Expected: push succeeds, GitHub Actions triggers CI + Docker workflows.

- [ ] **Step 2: Verify CI passes**

```bash
gh run list --limit 5
gh run watch
```

Expected: all jobs green within a few minutes.

- [ ] **Step 3: Verify image is published**

After the push workflow completes:

```bash
gh api /orgs/vexil-lang/packages/container/vexilbot/versions --jq '.[0].metadata.container.tags'
```

Expected: `["latest", "master", "sha-<shortsha>"]`
