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
