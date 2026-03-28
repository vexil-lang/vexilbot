package webhook

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strings"
)

func VerifySignature(body []byte, signatureHeader, secret string) error {
	if signatureHeader == "" {
		return fmt.Errorf("missing signature header")
	}

	parts := strings.SplitN(signatureHeader, "=", 2)
	if len(parts) != 2 || parts[0] != "sha256" {
		return fmt.Errorf("malformed signature header: %q", signatureHeader)
	}

	gotSig, err := hex.DecodeString(parts[1])
	if err != nil {
		return fmt.Errorf("decode signature hex: %w", err)
	}

	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(body)
	expectedSig := mac.Sum(nil)

	if !hmac.Equal(gotSig, expectedSig) {
		return fmt.Errorf("signature mismatch")
	}

	return nil
}
