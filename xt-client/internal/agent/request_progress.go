package agent

import "time"

// withModelProgress copies the HTTP client so callbacks remain local to one
// request. Other clients retain the visible-delta fallback and their interface.
func withModelProgress(client modelClient, progress func(int, time.Time)) (modelClient, bool) {
	httpClient, ok := client.(*httpModelClient)
	if !ok {
		return client, false
	}
	copy := *httpClient
	copy.onProgress = progress
	return &copy, true
}

// newRequestProgress counts output independently of visible message deltas.
func newRequestProgress(stats *RunStats, completedTokens int64, sessionID string, emit func(Event)) func(int, time.Time) {
	var requestChars int
	var firstOutputAt, lastEmit time.Time
	var emittedSpeed bool
	return func(chars int, at time.Time) {
		if firstOutputAt.IsZero() {
			firstOutputAt = at
			stats.FirstTokenMs = max(int64(1), at.Sub(stats.firstRequestAt).Milliseconds())
			emit(Event{Type: "run_stats", SessionID: sessionID, RunStats: stats})
		}
		requestChars += chars
		updateLiveOutput(stats, completedTokens, requestChars, firstOutputAt, at)
		if at.Sub(lastEmit) >= liveStatsInterval || (!emittedSpeed && stats.LiveSpeed > 0) {
			lastEmit = at
			emittedSpeed = stats.LiveSpeed > 0
			emit(Event{Type: "run_stats", SessionID: sessionID, RunStats: stats})
		}
	}
}
