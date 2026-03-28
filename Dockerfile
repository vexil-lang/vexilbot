# ---- builder ----
FROM golang:1.25-alpine AS builder

WORKDIR /src

COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -mod=vendor -trimpath -ldflags="-s -w" -o /vexilbot ./cmd/vexilbot

# ---- runtime ----
FROM gcr.io/distroless/static-debian12:debug-nonroot

COPY --from=builder /vexilbot /vexilbot

# Config is mounted at runtime — not baked in
ENTRYPOINT ["/vexilbot", "/etc/vexilbot/config.toml"]
