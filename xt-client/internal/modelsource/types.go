// Package modelsource owns manually configured model routes and encrypted API keys.
package modelsource

import (
	"errors"
	"net/url"
	"strings"
)

var (
	ErrNotFound = errors.New("model source or route not found")
	ErrInvalid  = errors.New("invalid model source")
)

type Model struct {
	ID                string   `json:"id"`
	CallID            string   `json:"callId,omitempty"`
	UpstreamModelID   string   `json:"upstreamModelId"`
	Label             string   `json:"label"`
	ContextWindow     int      `json:"contextWindow,omitempty"`
	MaxOutputTokens   int      `json:"maxOutputTokens,omitempty"`
	SupportsImages    bool     `json:"supportsImages"`
	SupportsReasoning bool     `json:"supportsReasoning"`
	ReasoningEfforts  []string `json:"reasoningEfforts,omitempty"`
	// PriceFactor and IsFree come from upstream metadata and are display-only; they never change billing.
	PriceFactor *float64 `json:"priceFactor,omitempty"`
	IsFree      *bool    `json:"isFree,omitempty"`
}

type Source struct {
	ID        string  `json:"id"`
	Label     string  `json:"label"`
	Kind      string  `json:"kind"`
	Protocol  string  `json:"protocol"`
	BaseURL   string  `json:"baseURL"`
	Enabled   bool    `json:"enabled"`
	HasAPIKey bool    `json:"hasAPIKey"`
	Models    []Model `json:"models"`
}

type Input struct {
	Label    string  `json:"label"`
	Protocol string  `json:"protocol"`
	BaseURL  string  `json:"baseURL"`
	Enabled  *bool   `json:"enabled,omitempty"`
	APIKey   *string `json:"apiKey,omitempty"`
	Models   []Model `json:"models"`
}

// NormalizeURL accepts a server root, API prefix, or complete chat endpoint.
// The stored value is the API prefix; ChatURL appends the endpoint exactly once.
func NormalizeURL(raw string) (string, error) { return normalizeURL(raw, "openai-chat") }

func normalizeURL(raw, protocol string) (string, error) {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || u.Hostname() == "" || (u.Scheme != "http" && u.Scheme != "https") || u.User != nil || u.RawQuery != "" || u.ForceQuery || u.Fragment != "" || u.Opaque != "" {
		return "", ErrInvalid
	}
	u.Path = strings.TrimRight(u.Path, "/")
	u.Path = strings.TrimSuffix(u.Path, protocolPath(protocol))
	u.RawPath = ""
	return strings.TrimRight(u.String(), "/"), nil
}

func ValidProtocol(protocol string) bool {
	return protocol == "openai-chat" || protocol == "openai-responses" || protocol == "anthropic-messages"
}
func protocolPath(protocol string) string {
	switch protocol {
	case "openai-responses":
		return "/responses"
	case "anthropic-messages":
		return "/messages"
	default:
		return "/chat/completions"
	}
}
func ProtocolURL(base, protocol string) string {
	return strings.TrimRight(base, "/") + protocolPath(protocol)
}
func ChatURL(base string) string { return ProtocolURL(base, "openai-chat") }

func validateModels(models []Model, owned map[string]bool) ([]Model, error) {
	result := make([]Model, 0, len(models))
	ids := map[string]bool{}
	upstream := map[string]bool{}
	for _, m := range models {
		m.CallID = "" // Call aliases are allocated by the store, never accepted from input.
		m.UpstreamModelID = strings.TrimSpace(m.UpstreamModelID)
		m.Label = strings.TrimSpace(m.Label)
		if m.UpstreamModelID == "" || upstream[m.UpstreamModelID] || (m.ContextWindow != 0 && m.ContextWindow < 1024) || (m.MaxOutputTokens != 0 && m.MaxOutputTokens < 16) || (m.ContextWindow > 0 && m.MaxOutputTokens >= m.ContextWindow) {
			return nil, ErrInvalid
		}
		if m.ID != "" && (!owned[m.ID] || ids[m.ID]) {
			return nil, ErrInvalid
		}
		if m.ID == "" {
			var err error
			m.ID, err = newID("model_")
			if err != nil {
				return nil, err
			}
		}
		if m.Label == "" {
			m.Label = m.UpstreamModelID
		}
		seen := map[string]bool{}
		efforts := make([]string, 0, len(m.ReasoningEfforts))
		for _, effort := range m.ReasoningEfforts {
			effort = strings.TrimSpace(effort)
			if effort == "" || seen[effort] || !m.SupportsReasoning {
				return nil, ErrInvalid
			}
			seen[effort] = true
			efforts = append(efforts, effort)
		}
		m.ReasoningEfforts = efforts
		upstream[m.UpstreamModelID] = true
		ids[m.ID] = true
		result = append(result, m)
	}
	return result, nil
}
