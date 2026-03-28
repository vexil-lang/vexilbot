package webhook_test

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"testing"

	"github.com/vexil-lang/vexilbot/internal/webhook"
)

func TestVerifySignature_Valid(t *testing.T) {
	secret := "test-secret"
	body := []byte(`{"action":"opened"}`)

	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(body)
	sig := "sha256=" + hex.EncodeToString(mac.Sum(nil))

	if err := webhook.VerifySignature(body, sig, secret); err != nil {
		t.Fatalf("valid signature rejected: %v", err)
	}
}

func TestVerifySignature_Invalid(t *testing.T) {
	err := webhook.VerifySignature([]byte("body"), "sha256=deadbeef", "secret")
	if err == nil {
		t.Fatal("invalid signature accepted")
	}
}

func TestVerifySignature_MalformedHeader(t *testing.T) {
	err := webhook.VerifySignature([]byte("body"), "not-a-sig", "secret")
	if err == nil {
		t.Fatal("malformed signature header accepted")
	}
}

func TestVerifySignature_Empty(t *testing.T) {
	err := webhook.VerifySignature([]byte("body"), "", "secret")
	if err == nil {
		t.Fatal("empty signature accepted")
	}
}
