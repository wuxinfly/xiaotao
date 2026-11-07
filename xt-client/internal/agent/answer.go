package agent

import "time"

func finishInterruptedRuns(history *Session) {
	end := time.Now()
	for i := range history.RunStats {
		if history.RunStats[i].EndedAt == nil {
			history.RunStats[i].Phase = "failed"
			history.RunStats[i].EndedAt = &end
		}
	}
}
