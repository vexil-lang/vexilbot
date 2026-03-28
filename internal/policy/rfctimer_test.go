package policy_test

import (
	"testing"
	"time"

	"github.com/vexil-lang/vexilbot/internal/policy"
)

func TestRFCTimerMessage(t *testing.T) {
	start := time.Date(2026, 3, 28, 12, 0, 0, 0, time.UTC)
	msg := policy.FormatRFCTimerStart(start)
	if msg == "" {
		t.Fatal("expected non-empty message")
	}
	if !containsSubstring(msg, "2026-04-11") {
		t.Errorf("message should contain end date 2026-04-11, got: %s", msg)
	}
}

func TestRFCTimerRemaining(t *testing.T) {
	start := time.Date(2026, 3, 28, 12, 0, 0, 0, time.UTC)

	now := start.Add(5 * 24 * time.Hour)
	remaining := policy.RFCDaysRemaining(start, now)
	if remaining != 9 {
		t.Errorf("remaining = %d, want 9", remaining)
	}

	now = start.Add(14 * 24 * time.Hour)
	remaining = policy.RFCDaysRemaining(start, now)
	if remaining != 0 {
		t.Errorf("remaining = %d, want 0", remaining)
	}

	now = start.Add(20 * 24 * time.Hour)
	remaining = policy.RFCDaysRemaining(start, now)
	if remaining != 0 {
		t.Errorf("remaining = %d, want 0", remaining)
	}
}

func containsSubstring(s, sub string) bool {
	for i := 0; i <= len(s)-len(sub); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
