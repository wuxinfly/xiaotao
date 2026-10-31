package providers

import (
	"fmt"
	"sort"
	"sync"
)

type ModelInfo struct {
	ID          string `json:"id"`
	DisplayName string `json:"displayName,omitempty"`
	Multimodal  bool   `json:"multimodal"`
}

type Info struct {
	ID          string   `json:"id"`
	DisplayName string   `json:"displayName"`
	LoginType   string   `json:"loginType"`
	Regions     []string `json:"regions"`
	Billing     string   `json:"billing"`
}

type Credential struct {
	RefreshToken string
	AccessToken  string
	ExpiresAt    int64
	UID          string
	MachineID    string
	DeviceID     string
	Region       string
}

type Provider interface {
	Info() Info
	ListModels(region string) []ModelInfo
}

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
