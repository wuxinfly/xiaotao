// Package effort defines the shared reasoning-effort vocabulary used across
// providers and the agent client. Levels are normalized before mapping to a
// provider's native spelling so UI and persistence stay provider-agnostic.
package effort

import "strings"

// Level is a normalized reasoning-effort level.
type Level string

const (
	Low    Level = "low"
	Medium Level = "medium"
	High   Level = "high"
	XHigh  Level = "xhigh"
	Max    Level = "max"
)

var all = []Level{Low, Medium, High, XHigh, Max}

// Parse maps aliases to a canonical level. The boolean is false when the
// value is empty or unknown; callers treat that as "follow provider default".
func Parse(value string) (Level, bool) {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case string(Low), "minimal", "none", "light":
		return Low, true
	case string(Medium):
		return Medium, true
	case string(High):
		return High, true
	case string(XHigh), "ultra", "extra_high":
		return XHigh, true
	case string(Max):
		return Max, true
	default:
		return "", false
	}
}

// Available reports whether the normalized level is offered by a model.
func Available(level Level, offered []string) bool {
	for _, item := range offered {
		if strings.EqualFold(item, string(level)) {
			return true
		}
	}
	return false
}

// Nearest picks the offered level closest to the requested one. Empty input
// or no overlap falls back to the first offered level, mirroring magpie's
// "a usable tier is better than a hard 400" rule.
func Nearest(level Level, offered []string) Level {
	if len(offered) == 0 {
		return ""
	}
	target := indexOf(level)
	best := 0
	bestDistance := -1
	for i, item := range offered {
		parsed, ok := Parse(item)
		if !ok {
			continue
		}
		distance := indexOf(parsed) - target
		if distance < 0 {
			distance = -distance
		}
		if bestDistance < 0 || distance < bestDistance || distance == bestDistance && i > best {
			best, bestDistance = i, distance
		}
	}
	if bestDistance < 0 {
		return ""
	}
	return Level(offered[best])
}

func indexOf(level Level) int {
	for i, item := range all {
		if item == level {
			return i
		}
	}
	return 0
}
