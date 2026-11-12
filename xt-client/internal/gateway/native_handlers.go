package gateway

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"

	"xiaotao/xt-client/internal/wireprotocol"
)

func (s *Service) HandleResponses(w http.ResponseWriter, r *http.Request) {
	s.handleNative(w, r, wireprotocol.Responses)
}
func (s *Service) HandleMessages(w http.ResponseWriter, r *http.Request) {
	s.handleNative(w, r, wireprotocol.Messages)
}
func (s *Service) handleNative(w http.ResponseWriter, r *http.Request, protocol string) {
	request := r.Clone(r.Context())
	request.Header = r.Header.Clone()
	if protocol == wireprotocol.Messages && request.Header.Get("Authorization") == "" {
		if key := request.Header.Get("x-api-key"); key != "" {
			request.Header.Set("Authorization", "Bearer "+key)
		}
	}
	if !s.Authorize(request) {
		nativeError(w, protocol, 401, "Invalid gateway API key")
		return
	}
	if r.Method != http.MethodPost {
		nativeError(w, protocol, 405, "POST is required")
		return
	}
	var body map[string]any
	decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxBody))
	decoder.UseNumber()
	if decoder.Decode(&body) != nil || body == nil {
		nativeError(w, protocol, 400, "Invalid or oversized request")
		return
	}
	var trailing any
	if decoder.Decode(&trailing) != io.EOF {
		nativeError(w, protocol, 400, "Invalid request")
		return
	}
	canonical, err := wireprotocol.ToChatRequest(protocol, body)
	if err != nil {
		nativeError(w, protocol, 400, err.Error())
		return
	}
	if canonical["stream"] == true {
		canonical["stream_options"] = map[string]any{"include_usage": true}
	}
	raw, err := json.Marshal(canonical)
	if err != nil {
		nativeError(w, protocol, 400, "Invalid request")
		return
	}
	request.Body = io.NopCloser(bytes.NewReader(raw))
	request.ContentLength = int64(len(raw))
	writer := &nativeWriter{dst: w, protocol: protocol, header: make(http.Header), status: 200, encoder: wireprotocol.Encoder{Protocol: protocol}}
	s.HandleChat(writer, request)
	if r.Context().Err() == nil {
		writer.finish()
	}
}
func nativeError(w http.ResponseWriter, protocol string, status int, message string) {
	errorType := "invalid_request_error"
	if status == 401 {
		errorType = "authentication_error"
	}
	if status >= 500 {
		errorType = "api_error"
	}
	body := map[string]any{"error": map[string]any{"type": errorType, "message": message}}
	if protocol == wireprotocol.Messages {
		body["type"] = "error"
	}
	writeJSON(w, status, body)
}
