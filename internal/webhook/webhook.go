package webhook

import (
	"io"
	"log/slog"
	"net/http"
)

type Router interface {
	Route(eventType string, payload []byte)
}

type RouterFunc func(eventType string, payload []byte)

func (f RouterFunc) Route(eventType string, payload []byte) {
	f(eventType, payload)
}

type Handler struct {
	secret string
	router Router
}

func NewHandler(secret string, router Router) *Handler {
	return &Handler{secret: secret, router: router}
}

func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	body, err := io.ReadAll(r.Body)
	if err != nil {
		slog.Error("read body", "error", err)
		http.Error(w, "failed to read body", http.StatusInternalServerError)
		return
	}

	sig := r.Header.Get("X-Hub-Signature-256")
	if err := VerifySignature(body, sig, h.secret); err != nil {
		slog.Warn("signature verification failed", "error", err)
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	eventType := r.Header.Get("X-GitHub-Event")
	if eventType == "" {
		http.Error(w, "missing X-GitHub-Event header", http.StatusBadRequest)
		return
	}

	slog.Info("webhook received", "event", eventType, "bytes", len(body))
	h.router.Route(eventType, body)

	w.WriteHeader(http.StatusOK)
}
