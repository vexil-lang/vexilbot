# Deploying vexilbot

## Prerequisites

- VPS with Ubuntu 22.04+
- Go 1.22+ or pre-built binary
- Caddy (for TLS)
- git-cliff (for changelog generation)
- cargo (for crate publishing)

## Setup

1. Create service user:
   ```bash
   sudo useradd -r -s /usr/sbin/nologin vexilbot
   ```

2. Create config directory:
   ```bash
   sudo mkdir -p /etc/vexilbot
   sudo cp config.toml /etc/vexilbot/config.toml
   sudo cp app.pem /etc/vexilbot/app.pem
   sudo chown -R vexilbot:vexilbot /etc/vexilbot
   sudo chmod 600 /etc/vexilbot/config.toml /etc/vexilbot/app.pem
   ```

3. Install binary:
   ```bash
   sudo cp vexilbot /usr/local/bin/vexilbot
   ```

4. Install systemd unit:
   ```bash
   sudo cp vexilbot.service /etc/systemd/system/
   sudo systemctl daemon-reload
   sudo systemctl enable --now vexilbot
   ```

5. Configure Caddy:
   ```bash
   sudo cp Caddyfile /etc/caddy/Caddyfile
   sudo systemctl reload caddy
   ```

6. Configure GitHub App webhook URL to `https://bot.vexil-lang.dev/webhook`

## Updating

```bash
sudo cp vexilbot /usr/local/bin/vexilbot
sudo systemctl restart vexilbot
```

## Logs

```bash
journalctl -u vexilbot -f
```
