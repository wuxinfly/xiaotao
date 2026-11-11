package sourceplugin

import "encoding/json"

// Credential is opaque to the host and encrypted before persistence.
type Operation struct {
	Credential json.RawMessage `json:"credential,omitempty"`
	Input      json.RawMessage `json:"input,omitempty"`
	Body       map[string]any  `json:"body,omitempty"`
	Model      string          `json:"model,omitempty"`
}
type Model struct {
	Key               string   `json:"key"`
	UpstreamID        string   `json:"upstreamId"`
	Label             string   `json:"label"`
	ContextWindow     int      `json:"contextWindow,omitempty"`
	SupportsImages    bool     `json:"supportsImages"`
	SupportsReasoning bool     `json:"supportsReasoning"`
	ReasoningEfforts  []string `json:"reasoningEfforts,omitempty"`
}
type Result struct {
	Credential json.RawMessage `json:"credential,omitempty"`
	Models     []Model         `json:"models,omitempty"`
	Value      json.RawMessage `json:"value,omitempty"`
}
