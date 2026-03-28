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
