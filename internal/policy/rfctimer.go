package policy

import (
	"fmt"
	"time"
)

const rfcPeriodDays = 14

// FormatRFCTimerStart returns a comment body for when an RFC comment period begins.
func FormatRFCTimerStart(start time.Time) string {
	end := start.AddDate(0, 0, rfcPeriodDays)
	return fmt.Sprintf(
		"RFC comment period started on %s. Earliest decision date: **%s** (14 days).",
		start.Format("2006-01-02"),
		end.Format("2006-01-02"),
	)
}

// RFCDaysRemaining returns the number of days remaining in the RFC comment period.
// Returns 0 if the period has expired.
func RFCDaysRemaining(start, now time.Time) int {
	end := start.AddDate(0, 0, rfcPeriodDays)
	remaining := int(end.Sub(now).Hours() / 24)
	if remaining < 0 {
		return 0
	}
	return remaining
}
