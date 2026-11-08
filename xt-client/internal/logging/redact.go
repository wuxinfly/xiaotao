package logging

import (
	"strings"
)

var sensitiveKeys = []string{
	"token",
	"jwt",
	"key",
	"password",
	"secret",
	"bearer",
	"authorization",
	"cookie",
}

// Redact masks suspected credentials in s. When any sensitive keyword is found
// (case-insensitive), s is truncated to the earliest match position and suffixed
// with "<key>=<redacted>". If the result exceeds maxLen (when maxLen > 0),
// it is truncated to maxLen with "...".
func Redact(s string, maxLen int) string {
	low := strings.ToLower(s)
	earliest := -1
	var matchedKey string

	for _, key := range sensitiveKeys {
		if i := strings.Index(low, key); i >= 0 {
			if earliest == -1 || i < earliest {
				earliest = i
				matchedKey = key
			}
		}
	}

	if earliest >= 0 {
		s = s[:earliest] + matchedKey + "=<redacted>"
	}

	if maxLen > 0 && len(s) > maxLen {
		s = s[:maxLen] + "..."
	}
	return s
}
