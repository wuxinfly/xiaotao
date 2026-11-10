package gateway

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"

	"xiaotao/xt-client/internal/usage"
)

var errStreamDone = errors.New("upstream stream completed")

// streamState forwards complete SSE events without folding tool/reasoning deltas.
// A normal EOF is successful only after every observed choice has finished.
type streamState struct {
	w                         http.ResponseWriter
	tracker                   *usage.ActiveRequest
	model, id                 string
	includeUsage              bool
	started, firstToken, done bool
	choices                   map[string]bool
}

func (s *Service) stream(w http.ResponseWriter, r *http.Request, reader io.Reader, model, id string, includeUsage bool, tracker *usage.ActiveRequest) {
	state := &streamState{w: w, tracker: tracker, model: model, id: id, includeUsage: includeUsage, choices: map[string]bool{}}
	err := readEvents(reader, state.event)
	if err == nil && !state.done && !state.finished() {
		err = errors.New("incomplete upstream stream")
	}
	if err != nil {
		finishFailure(tracker, r, state.started)
		if r.Context().Err() != nil {
			return
		}
		if !state.started {
			writeError(w, 502, "Upstream stream failed")
			return
		}
		_ = state.write(map[string]any{"error": map[string]any{"message": "Upstream stream interrupted", "type": "gateway_error", "code": "upstream_interrupted"}})
		return
	}
	if r.Context().Err() != nil {
		if tracker != nil {
			tracker.OnCanceled()
		}
		return
	}
	// A provider may close after finish_reason instead of emitting [DONE]. Only
	// synthesize the delimiter after a verified finish, never after a broken read.
	if !state.done {
		if err = state.writeDone(); err != nil {
			finishFailure(tracker, r, true)
			return
		}
	}
	if tracker != nil {
		tracker.OnCompleted()
	}
}

func readEvents(reader io.Reader, handle func(string) error) error {
	scanner := bufio.NewScanner(reader)
	scanner.Buffer(make([]byte, 4096), maxBody)
	var data []string
	size := 0
	dispatch := func() error {
		if len(data) == 0 {
			return nil
		}
		payload := strings.Join(data, "\n")
		data = nil
		size = 0
		return handle(payload)
	}
	for scanner.Scan() {
		line := scanner.Text()
		if line == "" {
			if err := dispatch(); err != nil {
				if errors.Is(err, errStreamDone) {
					return nil
				}
				return err
			}
			continue
		}
		if strings.HasPrefix(line, "data:") {
			value := strings.TrimPrefix(line, "data:")
			value = strings.TrimPrefix(value, " ")
			size += len(value)
			if size > maxBody {
				return errors.New("oversized stream event")
			}
			data = append(data, value)
		}
	}
	if scanner.Err() != nil {
		return errors.New("upstream stream read failed")
	}
	// SSE requires a blank line dispatch boundary. An unfinished final frame is
	// evidence of truncation, even if a preceding frame carried finish_reason.
	if len(data) > 0 {
		return errors.New("truncated stream event")
	}
	return nil
}

func (s *streamState) event(data string) error {
	if data == "[DONE]" {
		if len(s.choices) == 0 {
			return errors.New("stream completed without any choices")
		}
		if err := s.writeDone(); err != nil {
			return err
		}
		s.done = true
		return errStreamDone
	}
	var payload map[string]any
	if json.Unmarshal([]byte(data), &payload) != nil || payload == nil || payload["error"] != nil {
		return errors.New("invalid upstream stream event")
	}
	choices, ok := payload["choices"].([]any)
	if !ok {
		return errors.New("missing stream choices")
	}
	for _, choice := range choices {
		c, ok := choice.(map[string]any)
		if !ok {
			return errors.New("invalid stream choice")
		}
		index, ok := c["index"].(float64)
		if !ok || index < 0 || index != float64(int(index)) {
			return errors.New("invalid stream index")
		}
		key := fmt.Sprint(index)
		finished := false
		if value := c["finish_reason"]; value != nil {
			reason, valid := value.(string)
			if !valid || reason == "error" {
				return errors.New("invalid upstream finish reason")
			}
			finished = reason != ""
		}
		s.choices[key] = s.choices[key] || finished
		if semanticDelta(c["delta"]) && !s.firstToken {
			s.firstToken = true
			if s.tracker != nil {
				s.tracker.OnFirstToken()
			}
		}
	}
	recordUsage(s.tracker, payload)
	if !s.includeUsage {
		delete(payload, "usage")
		if len(choices) == 0 {
			return nil
		}
	}
	payload["id"] = s.id
	payload["model"] = s.model
	return s.write(payload)
}
func semanticDelta(value any) bool {
	delta, ok := value.(map[string]any)
	if !ok {
		return false
	}
	for _, key := range []string{"content", "reasoning_content", "reasoning"} {
		if text, ok := delta[key].(string); ok && text != "" {
			return true
		}
	}
	if tools, ok := delta["tool_calls"].([]any); ok && len(tools) > 0 {
		return true
	}
	return delta["function_call"] != nil
}
func (s *streamState) finished() bool {
	if len(s.choices) == 0 {
		return false
	}
	for _, done := range s.choices {
		if !done {
			return false
		}
	}
	return true
}
func (s *streamState) headers() {
	if !s.started {
		s.w.Header().Set("Content-Type", "text/event-stream")
		s.w.Header().Set("Cache-Control", "no-cache")
		s.w.Header().Set("X-Accel-Buffering", "no")
		s.started = true
	}
}
func (s *streamState) write(payload map[string]any) error {
	raw, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	s.headers()
	if _, err = fmt.Fprintf(s.w, "data: %s\n\n", raw); err != nil {
		return err
	}
	return http.NewResponseController(s.w).Flush()
}
func (s *streamState) writeDone() error {
	s.headers()
	if _, err := io.WriteString(s.w, "data: [DONE]\n\n"); err != nil {
		return err
	}
	return http.NewResponseController(s.w).Flush()
}
