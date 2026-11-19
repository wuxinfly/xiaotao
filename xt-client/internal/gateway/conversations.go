package gateway

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"strconv"
	"strings"

	"xiaotao/xt-client/internal/agent"
	"xiaotao/xt-client/internal/usage"
	"xiaotao/xt-client/internal/wireprotocol"
)

type incomingProtocolKey struct{}

func (s *Service) conversationInput(r *http.Request, body map[string]any, window int64) usage.ConversationInput {
	messages := objects(body["messages"])
	tools := objects(body["tools"])
	parts := agent.EstimateRequestContext(messages, tools)
	input := usage.ConversationInput{
		Context: usage.RequestContext{Items: contextItems(messages, tools), Window: window, Estimated: parts.TotalTokens, Parts: usage.ContextParts{
			System: parts.SystemTokens, Tools: parts.ToolsTokens, Messages: parts.MessagesTokens,
			ToolCalls: parts.ToolResultsTokens, References: parts.ReferenceTokens, Images: parts.ImageTokens,
		}},
	}
	if id := identifier(r.Header.Get("X-Xiaotao-Session")); id != "" {
		input.ID = dashboardID("agent", id)
		input.Source = "小涛"
		if encoded := r.Header.Get("X-Xiaotao-Title"); len(encoded) <= 1024 {
			if data, err := base64.RawURLEncoding.DecodeString(encoded); err == nil {
				input.Title = string(data)
			}
		}
		if override, err := strconv.ParseInt(r.Header.Get("X-Xiaotao-Context-Window"), 10, 64); err == nil && override > 0 {
			input.Context.Window = override
		}
		return input
	}
	incoming, _ := r.Context().Value(incomingProtocolKey{}).(incomingContext)
	protocol := incoming.protocol
	input.Source = "外部 API"
	if id := identifier(r.Header.Get("X-Xiaotao-Session-Id")); id != "" {
		input.ID = dashboardID("external", id)
		return input
	}
	if incoming.codexID != "" {
		input.ID = dashboardID("responses", incoming.codexID)
		input.Source = "Codex"
		input.Auxiliary = incoming.auxiliary
		input.TitleLookup = func() string { return s.codexTitles.lookup(incoming.codexID) }
		if !incoming.auxiliary {
			input.Title = firstUserTitle(messages)
		}
		return input
	}
	if protocol == wireprotocol.Messages {
		metadata, _ := body["metadata"].(map[string]any)
		var client struct {
			SessionID string `json:"session_id"`
		}
		raw, _ := metadata["user_id"].(string)
		if len(raw) < 2048 && json.Unmarshal([]byte(raw), &client) == nil {
			if id := identifier(client.SessionID); id != "" {
				input.ID = dashboardID("messages", id)
				input.Source = "Messages"
				return input
			}
		}
	}
	input.Source = "归属待确认"
	input.InferenceScope = protocol
	input.Prefixes = historyPrefixes(messages)
	return input
}

func withIncomingContext(r *http.Request, incoming incomingContext) *http.Request {
	return r.WithContext(context.WithValue(r.Context(), incomingProtocolKey{}, incoming))
}

func identifier(value string) string {
	value = strings.TrimSpace(value)
	if len(value) > 256 {
		return ""
	}
	return value
}

func dashboardID(namespace, id string) string {
	hash := sha256.Sum256([]byte(namespace + "\x00" + id))
	return hex.EncodeToString(hash[:])
}

func objects(value any) []map[string]any {
	list, _ := value.([]any)
	result := make([]map[string]any, 0, len(list))
	for _, value := range list {
		if item, ok := value.(map[string]any); ok {
			result = append(result, item)
		}
	}
	return result
}

// Only retain digests. A session can be inferred when an earlier completed
// request is a strict prefix of the new dialogue and matches uniquely.
func historyPrefixes(messages []map[string]any) [][32]byte {
	if len(messages) > 256 {
		return nil
	}
	h := sha256.New()
	prefixes := make([][32]byte, 0, len(messages))
	for _, message := range messages {
		role, _ := message["role"].(string)
		if role == "system" || role == "developer" {
			continue
		}
		data, err := json.Marshal(message)
		if err != nil {
			return nil
		}
		digest := sha256.Sum256(data)
		h.Write(digest[:])
		var prefix [32]byte
		h.Sum(prefix[:0])
		prefixes = append(prefixes, prefix)
	}
	return prefixes
}
