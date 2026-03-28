package ghclient_test

import (
	"testing"

	"github.com/vexil-lang/vexilbot/internal/ghclient"
)

func TestNewApp_InvalidKeyPath(t *testing.T) {
	_, err := ghclient.NewApp(12345, "/nonexistent/key.pem")
	if err == nil {
		t.Fatal("expected error for invalid key path")
	}
}
