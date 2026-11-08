package usage

import (
	"encoding/json"
	"fmt"
	"net/http"
	"time"
)

// HandleSSE streams real-time DashboardSnapshot updates over Server-Sent Events.
func (s *Service) HandleSSE(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/event-stream; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Connection", "keep-alive")
	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming unsupported", http.StatusInternalServerError)
		return
	}

	// Initial snapshot
	snap, err := s.Snapshot()
	if err != nil {
		http.Error(w, "snapshot error", http.StatusInternalServerError)
		return
	}

	writeJSONEvent := func(eventType string, data any) error {
		b, err := json.Marshal(data)
		if err != nil {
			return err
		}
		if eventType != "" {
			fmt.Fprintf(w, "event: %s\n", eventType)
		}
		fmt.Fprintf(w, "data: %s\n\n", b)
		flusher.Flush()
		return nil
	}

	if err := writeJSONEvent("snapshot", snap); err != nil {
		return
	}

	// Register subscription
	ch := make(chan *DashboardSnapshot, 8)
	s.subMu.Lock()
	s.subs[ch] = struct{}{}
	s.subMu.Unlock()

	defer func() {
		s.subMu.Lock()
		delete(s.subs, ch)
		s.subMu.Unlock()
	}()

	pingTicker := time.NewTicker(15 * time.Second)
	defer pingTicker.Stop()

	ctx := r.Context()
	for {
		select {
		case <-ctx.Done():
			return
		case <-s.done:
			return
		case <-pingTicker.C:
			fmt.Fprint(w, ": ping\n\n")
			flusher.Flush()
		case update := <-ch:
			if err := writeJSONEvent("update", update); err != nil {
				return
			}
		}
	}
}
