package gateway

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"

	"xiaotao/xt-client/internal/modelsource"
	"xiaotao/xt-client/internal/usage"
)

func (s *Service) HandleChat(w http.ResponseWriter, r *http.Request) {
	if !s.Authorize(r) {
		writeError(w, 401, "Invalid gateway API key")
		return
	}
	if r.Method != http.MethodPost {
		writeError(w, 405, "POST is required")
		return
	}
	var body map[string]any
	decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxBody))
	decoder.UseNumber()
	if decoder.Decode(&body) != nil || body == nil {
		writeError(w, 400, "Invalid or oversized chat request")
		return
	}
	var trailing any
	if decoder.Decode(&trailing) != io.EOF {
		writeError(w, 400, "Invalid chat request")
		return
	}
	modelID, _ := body["model"].(string)
	var src modelsource.Source
	var m modelsource.Model
	var key string
	var err error
	if s.store == nil {
		err = modelsource.ErrNotFound
	} else {
		src, m, key, err = s.store.Resolve(modelID)
	}
	if err != nil && s.plugins != nil {
		src, m, err = s.plugins.Resolve(modelID)
	}
	id := requestID()
	s.mu.RLock()
	u := s.usage
	s.mu.RUnlock()
	var tracker *usage.ActiveRequest
	if u != nil {
		tracker = u.StartRequest(id, src.Kind, modelID, src.ID, "用户", "模型请求")
	}
	if err != nil {
		if tracker != nil {
			tracker.OnError("模型路由不可用")
		}
		writeError(w, 400, "Model route is unavailable")
		return
	}
	if err = prepareRequest(body, m); err != nil {
		if tracker != nil {
			tracker.OnError("模型请求参数无效")
		}
		writeError(w, 400, err.Error())
		return
	}
	if src.Kind == "plugin" {
		body["_plugin_context_window"] = m.ContextWindow
	}
	stream, _ := body["stream"].(bool)
	includeUsage := false
	if stream {
		options, _ := body["stream_options"].(map[string]any)
		if options == nil {
			options = map[string]any{}
		}
		includeUsage, _ = options["include_usage"].(bool)
		options["include_usage"] = true
		body["stream_options"] = options
	}
	resp, cancel, err := s.send(r.Context(), src, key, body)
	if err != nil {
		finishFailure(tracker, r, false)
		writeError(w, 502, "Upstream connection failed")
		return
	}
	defer cancel()
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		if tracker != nil {
			tracker.OnError("上游拒绝请求")
		}
		writeError(w, 502, "Upstream rejected chat request")
		return
	}
	if stream {
		s.stream(w, r, resp.Body, modelID, id, includeUsage, tracker)
		return
	}
	result, err := decodeCompletion(resp.Body)
	if err != nil {
		finishFailure(tracker, r, false)
		writeError(w, 502, "Invalid upstream chat completion")
		return
	}
	result["model"] = modelID
	result["id"] = id
	if tracker != nil {
		tracker.OnFirstToken()
		recordUsage(tracker, result)
		tracker.OnCompleted()
	}
	writeJSON(w, 200, result)
}

func prepareRequest(body map[string]any, m modelsource.Model) error {
	messages, ok := body["messages"].([]any)
	if !ok || len(messages) == 0 {
		return errors.New("messages must be a nonempty array")
	}
	if v, exists := body["stream"]; exists {
		if _, ok := v.(bool); !ok {
			return errors.New("stream must be boolean")
		}
	}
	for _, name := range []string{"max_tokens", "max_completion_tokens"} {
		if v, ok := body[name]; ok {
			n, valid := v.(json.Number)
			if !valid {
				return errors.New("output token limit must be an integer")
			}
			i, err := n.Int64()
			if err != nil || i < 1 || (m.ContextWindow > 0 && i >= int64(m.ContextWindow)) {
				return errors.New("output token limit is outside model context")
			}
		}
	}
	if body["max_tokens"] == nil && body["max_completion_tokens"] == nil && m.MaxOutputTokens > 0 {
		body["max_tokens"] = m.MaxOutputTokens
	}
	if effort, ok := body["reasoning_effort"]; ok {
		v, valid := effort.(string)
		if !valid || !m.SupportsReasoning {
			return errors.New("reasoning effort is not supported")
		}
		if len(m.ReasoningEfforts) > 0 {
			found := false
			for _, allowed := range m.ReasoningEfforts {
				if v == allowed {
					found = true
				}
			}
			if !found {
				return errors.New("reasoning effort is not configured")
			}
		}
	}
	body["model"] = m.UpstreamModelID
	return nil
}

func decodeCompletion(reader io.Reader) (map[string]any, error) {
	raw, err := io.ReadAll(io.LimitReader(reader, maxBody+1))
	if err != nil || len(raw) > maxBody {
		return nil, errors.New("invalid response")
	}
	var result map[string]any
	if json.Unmarshal(raw, &result) != nil || result == nil || result["error"] != nil {
		return nil, errors.New("invalid response")
	}
	choices, ok := result["choices"].([]any)
	if !ok || len(choices) == 0 {
		return nil, errors.New("missing completion")
	}
	for _, choice := range choices {
		c, ok := choice.(map[string]any)
		if !ok {
			return nil, errors.New("invalid completion choice")
		}
		message, messageOK := c["message"].(map[string]any)
		reason, reasonOK := c["finish_reason"].(string)
		if !messageOK || message == nil || !reasonOK || reason == "" || reason == "error" {
			return nil, errors.New("incomplete completion")
		}
	}
	return result, nil
}

func finishFailure(tracker *usage.ActiveRequest, r *http.Request, started bool) {
	if tracker == nil {
		return
	}
	if r.Context().Err() != nil {
		tracker.OnCanceled()
	} else if started {
		tracker.OnInterrupted("上游响应中断")
	} else {
		tracker.OnError("上游请求失败")
	}
}
func token(v any) *int64 {
	var n int64
	switch value := v.(type) {
	case float64:
		if value < 0 || value != float64(int64(value)) {
			return nil
		}
		n = int64(value)
	case json.Number:
		var err error
		n, err = value.Int64()
		if err != nil || n < 0 {
			return nil
		}
	default:
		return nil
	}
	return &n
}
func recordUsage(tracker *usage.ActiveRequest, payload map[string]any) {
	if tracker == nil {
		return
	}
	u, ok := payload["usage"].(map[string]any)
	if !ok {
		return
	}
	var cached *int64
	if details, ok := u["prompt_tokens_details"].(map[string]any); ok {
		cached = token(details["cached_tokens"])
	}
	tracker.OnUsage(token(u["prompt_tokens"]), token(u["completion_tokens"]), cached)
}
