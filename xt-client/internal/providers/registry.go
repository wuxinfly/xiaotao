package providers

import (
	"context"
	"fmt"
	"sort"
	"sync"

	"xiaotao/xt-client/internal/storage"
)

type ModelInfo struct {
	Reasoning      bool              `json:"reasoning,omitempty"`
	MaxInputTokens int               `json:"maxInputTokens,omitempty"`
	UpstreamID     string            `json:"-"`
	Routes         map[string]string `json:"-"`
	ID             string            `json:"id"`
	DisplayName    string            `json:"displayName,omitempty"`
	Multimodal     bool              `json:"multimodal"`
}

type Info struct {
	ID          string   `json:"id"`
	DisplayName string   `json:"displayName"`
	LoginType   string   `json:"loginType"`
	Regions     []string `json:"regions"`
	Billing     string   `json:"billing"`
}

type Credential struct {
	ModelRoutes    map[string]string `json:"-"`
	RefreshToken   string
	AccessToken    string
	ExpiresAt      int64
	UID            string
	Nickname       string
	ProjectID      string
	AuthTier       string
	EnterpriseID   string
	DepartmentInfo string
	MachineID      string
	DeviceID       string
	Region         string
	Domain         string
}

type Provider interface {
	Info() Info
	ListModels(region string) []ModelInfo
}

type ModelFetcher interface {
	FetchModels(context.Context, *Credential) ([]ModelInfo, error)
}

// ChatEvents receives normalized provider output. Providers pre-merge tool
// fragments into the OpenAI tool-call shape before calling OnDelta.
type ChatEvents struct {
	OnFirstToken func()
	OnDelta      func(delta map[string]any)
	OnError      func(status int, message string)
	OnUsage      func(usage map[string]any)
	OnAction     func(tag, title string)
}

// ChatCollector aggregates a complete non-streaming provider response.
type ChatCollector interface {
	Collect(ctx context.Context, cred *Credential, model string, body map[string]any) (map[string]any, error)
}

// Streamer runs a provider streaming response and pushes normalized deltas.
type Streamer interface {
	Stream(ctx context.Context, cred *Credential, model string, body map[string]any, ev *ChatEvents) error
}

// Refresher exchanges a provider credential for a fresh access token.
type Refresher interface {
	Refresh(ctx context.Context, cred *Credential) (*Credential, error)
}

// Quoter fetches a provider account quota snapshot.
type Quoter interface {
	FetchQuota(ctx context.Context, cred *Credential) (*storage.QuotaSnapshot, error)
}

// UpstreamError carries an HTTP-like status from provider implementations.
type UpstreamError struct {
	Status  int
	Message string
}

func (e *UpstreamError) Error() string { return e.Message }

var (
	mu        sync.RWMutex
	providers = map[string]Provider{}
)

func Register(p Provider) {
	mu.Lock()
	defer mu.Unlock()
	providers[p.Info().ID] = p
}

func Get(id string) (Provider, error) {
	mu.RLock()
	defer mu.RUnlock()
	p, ok := providers[id]
	if !ok {
		return nil, fmt.Errorf("unknown provider %q", id)
	}
	return p, nil
}

func List() []Info {
	mu.RLock()
	defer mu.RUnlock()
	out := make([]Info, 0, len(providers))
	for _, p := range providers {
		out = append(out, p.Info())
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out
}
