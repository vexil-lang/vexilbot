package main

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"os"

	"github.com/vexil-lang/vexilbot/internal/serverconfig"
	"github.com/vexil-lang/vexilbot/internal/webhook"
)

func main() {
	if len(os.Args) < 2 {
		fmt.Fprintf(os.Stderr, "usage: vexilbot <config-path>\n")
		os.Exit(1)
	}

	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	slog.SetDefault(logger)

	cfg, err := serverconfig.Load(os.Args[1])
	if err != nil {
		slog.Error("load config", "error", err)
		os.Exit(1)
	}

	router := webhook.RouterFunc(func(eventType string, payload []byte) {
		slog.Info("event received", "type", eventType, "bytes", len(payload))
	})

	handler := webhook.NewHandler(cfg.Server.WebhookSecret, router)

	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", healthHandler)
	mux.Handle("POST /webhook", handler)

	slog.Info("vexilbot starting", "listen", cfg.Server.Listen)
	if err := http.ListenAndServe(cfg.Server.Listen, mux); err != nil {
		slog.Error("server error", "error", err)
		os.Exit(1)
	}
}

func healthHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
}
