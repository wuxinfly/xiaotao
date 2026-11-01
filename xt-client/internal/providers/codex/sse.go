package codex

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"time"
)

const (
	gateHeartbeatOnlyTimeout = 90 * time.Second
	gateBufferMaxLines       = 256
)

var heartbeatEvents = map[string]bool{
	"keepalive": true,
	"ping":      true,
	"heartbeat": true,
}

type SSEEvent struct {
	Name string
	Raw  []byte
	Data map[string]any
}

// SSEReader parses lines into SSE events according to the W3C spec.
type SSEReader struct {
	scanner *bufio.Scanner
}

func NewSSEReader(r io.Reader) *SSEReader {
	scanner := bufio.NewScanner(r)
	// Set larger buffer capacity for big SSE frames
	buf := make([]byte, 64*1024)
	scanner.Buffer(buf, 4*1024*1024)
	return &SSEReader{scanner: scanner}
}

func (s *SSEReader) ReadEvent() (*SSEEvent, error) {
	var eventName string
	var dataLines []string

	for s.scanner.Scan() {
		line := s.scanner.Text()
		if line == "" || line == "\r" {
			if len(dataLines) == 0 {
				eventName = ""
				continue
			}
			raw := strings.Join(dataLines, "\n")
			if strings.TrimSpace(raw) == "[DONE]" {
				return &SSEEvent{Name: "[DONE]", Raw: []byte(raw)}, nil
			}
			var payload map[string]any
			if err := json.Unmarshal([]byte(raw), &payload); err != nil {
				// Skip malformed data frame
				eventName = ""
				dataLines = nil
				continue
			}
			name := eventName
			if name == "" {
				if t, ok := payload["type"].(string); ok {
					name = t
				}
			}
			return &SSEEvent{
				Name: name,
				Raw:  []byte(raw),
				Data: payload,
			}, nil
		}

		if strings.HasPrefix(line, ":") {
			continue // Comment / keepalive
		}
		if strings.HasPrefix(line, "event:") {
			eventName = strings.TrimSpace(line[len("event:"):])
			continue
		}
		if strings.HasPrefix(line, "data:") {
			dataLines = append(dataLines, strings.TrimPrefix(line[len("data:"):], " "))
			continue
		}
	}

	if err := s.scanner.Err(); err != nil {
		return nil, err
	}

	// Flush any trailing event at EOF
	if len(dataLines) > 0 {
		raw := strings.Join(dataLines, "\n")
		if strings.TrimSpace(raw) == "[DONE]" {
			return &SSEEvent{Name: "[DONE]", Raw: []byte(raw)}, nil
		}
		var payload map[string]any
		if err := json.Unmarshal([]byte(raw), &payload); err == nil {
			name := eventName
			if name == "" {
				if t, ok := payload["type"].(string); ok {
					name = t
				}
			}
			return &SSEEvent{Name: name, Raw: []byte(raw), Data: payload}, nil
		}
	}

	return nil, io.EOF
}

type GateResult struct {
	Committed    bool
	AccountError bool
	StatusCode   int
	ErrorMessage string
	EOF          bool
	TimedOut     bool
	Buffered     []*SSEEvent
}

// GateFirstEvent reads events until the first semantic event arrives.
// Heartbeats are absorbed and discarded from semantic commitment.
func GateFirstEvent(ctx context.Context, reader *SSEReader) (*GateResult, error) {
	start := time.Now()
	var buffered []*SSEEvent

	for {
		select {
		case <-ctx.Done():
			return &GateResult{EOF: true, TimedOut: true}, ctx.Err()
		default:
		}

		ev, err := reader.ReadEvent()
		if err != nil {
			if err == io.EOF {
				return &GateResult{EOF: true}, nil
			}
			return &GateResult{EOF: true, TimedOut: strings.Contains(err.Error(), "timeout")}, err
		}

		if ev.Name == "[DONE]" {
			return &GateResult{EOF: true}, nil
		}

		if heartbeatEvents[ev.Name] {
			if time.Since(start) > gateHeartbeatOnlyTimeout {
				return &GateResult{EOF: true, TimedOut: true}, fmt.Errorf("keepalive timeout without semantic event")
			}
			continue
		}

		status, msg := isErrorEvent(ev.Name, ev.Data)
		if status == 401 || status == 403 || status == 429 {
			return &GateResult{
				AccountError: true,
				StatusCode:   status,
				ErrorMessage: msg,
			}, nil
		}

		buffered = append(buffered, ev)
		return &GateResult{
			Committed:    true,
			StatusCode:   status,
			ErrorMessage: msg,
			Buffered:     buffered,
		}, nil
	}
}

// EventStream combines buffered events with the remaining SSEReader events.
type EventStream struct {
	buffered []*SSEEvent
	reader   *SSEReader
}

func NewEventStream(buffered []*SSEEvent, reader *SSEReader) *EventStream {
	return &EventStream{
		buffered: buffered,
		reader:   reader,
	}
}

func (s *EventStream) Next() (*SSEEvent, error) {
	if len(s.buffered) > 0 {
		ev := s.buffered[0]
		s.buffered = s.buffered[1:]
		return ev, nil
	}
	return s.reader.ReadEvent()
}
