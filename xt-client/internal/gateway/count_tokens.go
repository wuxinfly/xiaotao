package gateway

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"

	"xiaotao/xt-client/internal/modelsource"
	"xiaotao/xt-client/internal/wireprotocol"
)

func (s *Service) HandleCountTokens(w http.ResponseWriter, r *http.Request) {
	if !s.Authorize(r) {
		nativeError(w, wireprotocol.Messages, http.StatusUnauthorized, "Invalid gateway API key")
		return
	}
	if r.Method != http.MethodPost {
		nativeError(w, wireprotocol.Messages, http.StatusMethodNotAllowed, "POST is required")
		return
	}

	var body map[string]any
	decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxBody))
	decoder.UseNumber()
	if decoder.Decode(&body) != nil || body == nil {
		nativeError(w, wireprotocol.Messages, http.StatusBadRequest, "Invalid or oversized request")
		return
	}
	var trailing any
	if decoder.Decode(&trailing) != io.EOF {
		nativeError(w, wireprotocol.Messages, http.StatusBadRequest, "Invalid request")
		return
	}
	if body["stream"] == true {
		nativeError(w, wireprotocol.Messages, http.StatusBadRequest, "stream is not supported by count_tokens")
		return
	}

	modelID, _ := body["model"].(string)
	src, model, key, err := s.resolveCall(modelID)
	if err != nil || src.Protocol != wireprotocol.Messages || src.Kind != "manual" {
		nativeError(w, wireprotocol.Messages, http.StatusBadRequest, "count_tokens requires an Anthropic Messages manual upstream")
		return
	}
	delete(body, "stream")
	body["model"] = model.UpstreamModelID
	raw, err := json.Marshal(body)
	if err != nil {
		nativeError(w, wireprotocol.Messages, http.StatusBadRequest, "Invalid request")
		return
	}
	upstream, err := http.NewRequestWithContext(r.Context(), http.MethodPost, modelsource.AnthropicCountTokensURL(src.BaseURL), bytes.NewReader(raw))
	if err != nil {
		nativeError(w, wireprotocol.Messages, http.StatusBadGateway, "Upstream count_tokens request failed")
		return
	}
	setUpstreamAuth(upstream, src.Protocol, key)
	upstream.Header.Set("Content-Type", "application/json")
	resp, err := s.client.Do(upstream)
	if err != nil {
		nativeError(w, wireprotocol.Messages, http.StatusBadGateway, "Upstream count_tokens request failed")
		return
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		nativeError(w, wireprotocol.Messages, http.StatusBadGateway, "Upstream rejected count_tokens request")
		return
	}
	data, err := io.ReadAll(io.LimitReader(resp.Body, maxBody+1))
	if err != nil || len(data) > maxBody {
		nativeError(w, wireprotocol.Messages, http.StatusBadGateway, "Invalid count_tokens response")
		return
	}
	var result struct {
		InputTokens *int64 `json:"input_tokens"`
	}
	if json.Unmarshal(data, &result) != nil || result.InputTokens == nil || *result.InputTokens < 0 {
		nativeError(w, wireprotocol.Messages, http.StatusBadGateway, "Invalid count_tokens response")
		return
	}
	writeJSON(w, http.StatusOK, result)
}
