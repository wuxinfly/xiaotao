package gateway

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"

	"xiaotao/xt-client/internal/wireprotocol"
)

type nativeWriter struct {
	converted               map[string]any
	dst                     http.ResponseWriter
	protocol                string
	header                  http.Header
	status                  int
	buffer                  bytes.Buffer
	encoder                 wireprotocol.Encoder
	streaming, done, failed bool
}

func (w *nativeWriter) Header() http.Header    { return w.header }
func (w *nativeWriter) WriteHeader(status int) { w.status = status }
func (w *nativeWriter) FlushError() error {
	if !w.streaming {
		return nil
	}
	return http.NewResponseController(w.dst).Flush()
}
func (w *nativeWriter) Write(raw []byte) (int, error) {
	if w.done || w.failed {
		return 0, errors.New("native stream is closed")
	}
	if w.buffer.Len()+len(raw) > maxBody {
		return 0, errors.New("protocol response exceeds size limit")
	}
	w.buffer.Write(raw)
	if !strings.HasPrefix(w.header.Get("Content-Type"), "text/event-stream") {
		return len(raw), nil
	}
	for {
		data := w.buffer.String()
		index := strings.Index(data, "\n\n")
		if index < 0 {
			break
		}
		event := data[:index]
		w.buffer.Next(index + 2)
		if !strings.HasPrefix(event, "data: ") {
			return 0, errors.New("invalid canonical event")
		}
		if err := w.convert(strings.TrimPrefix(event, "data: ")); err != nil {
			w.fail("Gateway protocol conversion failed")
			return 0, err
		}
	}
	return len(raw), nil
}
func (w *nativeWriter) convert(data string) error {
	var events []wireprotocol.Event
	var err error
	if data == "[DONE]" {
		events, err = w.encoder.Finish()
	} else {
		var body map[string]any
		if json.Unmarshal([]byte(data), &body) != nil {
			return errors.New("invalid canonical stream")
		}
		if body["error"] != nil {
			w.fail("Upstream stream interrupted")
			return errors.New("upstream error")
		}
		events, err = w.encoder.Push(body)
	}
	if err != nil {
		return err
	}
	for _, event := range events {
		if err = w.emit(event); err != nil {
			return err
		}
	}
	if data == "[DONE]" {
		w.done = true
	}
	return nil
}
func (w *nativeWriter) emit(event wireprotocol.Event) error {
	raw, err := json.Marshal(event.Data)
	if err != nil {
		return err
	}
	if !w.streaming {
		w.dst.Header().Set("Content-Type", "text/event-stream")
		w.dst.Header().Set("Cache-Control", "no-cache")
		w.dst.Header().Set("X-Accel-Buffering", "no")
		w.streaming = true
	}
	if _, err = fmt.Fprintf(w.dst, "event: %s\ndata: %s\n\n", event.Name, raw); err != nil {
		return err
	}
	return http.NewResponseController(w.dst).Flush()
}
func (w *nativeWriter) fail(message string) {
	if w.failed || w.done {
		return
	}
	w.failed = true
	if w.streaming {
		event := w.encoder.Error(message)
		_ = w.emit(event)
	} else {
		nativeError(w.dst, w.protocol, 502, message)
	}
}
func (w *nativeWriter) finish() {
	if w.done || w.failed {
		return
	}
	if w.streaming {
		w.fail("Upstream stream ended without completion")
		return
	}
	var body map[string]any
	if json.Unmarshal(w.buffer.Bytes(), &body) != nil {
		w.fail("Invalid gateway response")
		return
	}
	if w.status >= 400 {
		message := wireprotocol.Str(wireprotocol.Obj(body["error"])["message"])
		nativeError(w.dst, w.protocol, w.status, message)
		return
	}
	native := w.converted
	var err error
	if native == nil {
		native, err = wireprotocol.FromChatResponse(w.protocol, body)
	}
	if err != nil {
		w.fail("Invalid completion for requested protocol")
		return
	}
	writeJSON(w.dst, w.status, native)
}

func (w *nativeWriter) validateCompletion(body map[string]any) error {
	native, err := wireprotocol.FromChatResponse(w.protocol, body)
	if err == nil {
		w.converted = native
	}
	return err
}
