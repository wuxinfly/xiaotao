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
	PriceFactor       *float64 `json:"priceFactor,omitempty"`
	IsFree            *bool    `json:"isFree,omitempty"`
}
type AccountInfo struct {
	ID   string `json:"id"`
	Name string `json:"name,omitempty"`
}
type Result struct {
	Account    *AccountInfo    `json:"account,omitempty"`
	Credential json.RawMessage `json:"credential,omitempty"`
	Models     []Model         `json:"models,omitempty"`
	Value      json.RawMessage `json:"value,omitempty"`
}
