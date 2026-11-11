package sourceplugin

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"sync"
	"time"

	"xiaotao/xt-client/internal/modelsource"
)

type Saved struct {
	ID       string   `json:"id"`
	Manifest Manifest `json:"manifest"`
	Enabled  bool     `json:"enabled"`
	Models   []Model  `json:"models"`
	Secret   []byte   `json:"secret,omitempty"`
}
type Manager struct {
	mu               sync.Mutex
	dir              string
	encrypt, decrypt func([]byte) ([]byte, error)
	entries          []Saved
	gates            map[string]chan struct{}
	active           map[string]context.CancelFunc
	closed           bool
}

func Open(dir string, encrypt, decrypt func([]byte) ([]byte, error)) (*Manager, error) {
	m := &Manager{dir: filepath.Join(dir, "source-plugins"), encrypt: encrypt, decrypt: decrypt, entries: []Saved{}, gates: map[string]chan struct{}{}, active: map[string]context.CancelFunc{}}
	raw, err := os.ReadFile(filepath.Join(m.dir, "state.json"))
	if os.IsNotExist(err) {
		return m, nil
	}
	if err != nil {
		return nil, err
	}
	if json.Unmarshal(raw, &m.entries) != nil {
		return nil, errors.New("invalid plugin state")
	}
	seen := map[string]bool{}
	for _, e := range m.entries {
		if !identifier.MatchString(e.ID) || e.Manifest.ID != e.ID || seen[e.ID] {
			return nil, errors.New("invalid plugin state identity")
		}
		seen[e.ID] = true
	}
	return m, nil
}
func (m *Manager) persist() error {
	if err := os.MkdirAll(m.dir, 0700); err != nil {
		return err
	}
	raw, err := json.MarshalIndent(m.entries, "", "  ")
	if err != nil {
		return err
	}
	f, err := os.CreateTemp(m.dir, ".state-*")
	if err != nil {
		return err
	}
	defer os.Remove(f.Name())
	if _, err = f.Write(raw); err != nil {
		f.Close()
		return err
	}
	if err = f.Sync(); err != nil {
		f.Close()
		return err
	}
	if err = f.Close(); err != nil {
		return err
	}
	return os.Rename(f.Name(), filepath.Join(m.dir, "state.json"))
}
func (m *Manager) List() []Saved {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make([]Saved, 0, len(m.entries))
	for _, e := range m.entries {
		e.Secret = nil
		e.Models = append([]Model(nil), e.Models...)
		out = append(out, e)
	}
	return out
}
func route(id, key string) string {
	sum := sha256.Sum256([]byte(id + "\x00" + key))
	return "plugin_model_" + hex.EncodeToString(sum[:16])
}
func (m *Manager) Sources() []modelsource.Source {
	out := []modelsource.Source{}
	for _, e := range m.List() {
		src := modelsource.Source{ID: e.ID, Label: e.Manifest.Name, Kind: "plugin", Enabled: e.Enabled, HasAPIKey: true}
		for _, v := range e.Models {
			src.Models = append(src.Models, modelsource.Model{ID: route(e.ID, v.Key), Label: v.Label, UpstreamModelID: v.UpstreamID, ContextWindow: v.ContextWindow, SupportsImages: v.SupportsImages, SupportsReasoning: v.SupportsReasoning, ReasoningEfforts: v.ReasoningEfforts})
		}
		out = append(out, src)
	}
	return out
}
func (m *Manager) Resolve(id string) (modelsource.Source, modelsource.Model, error) {
	for _, src := range m.Sources() {
		if !src.Enabled {
			continue
		}
		for _, v := range src.Models {
			if v.ID == id {
				return src, v, nil
			}
		}
	}
	return modelsource.Source{}, modelsource.Model{}, modelsource.ErrNotFound
}

// Operations serialize credential refresh and re-login with state writes. The
// mutex is acquired only for state snapshots and compare-and-swap, never network IO.
func (m *Manager) Run(ctx context.Context, id, method string, input json.RawMessage, body map[string]any, model string, notify func(Message)) (Result, error) {
	m.mu.Lock()
	gate := m.gates[id]
	if gate == nil {
		gate = make(chan struct{}, 1)
		m.gates[id] = gate
	}
	m.mu.Unlock()
	select {
	case gate <- struct{}{}:
	case <-ctx.Done():
		return Result{}, ctx.Err()
	}
	defer func() { <-gate }()
	ctx, cancel := context.WithTimeout(ctx, 30*time.Minute)
	defer cancel()
	m.mu.Lock()
	if m.closed {
		m.mu.Unlock()
		return Result{}, errors.New("plugin manager closed")
	}
	m.active[id] = cancel
	defer func() { m.mu.Lock(); delete(m.active, id); m.mu.Unlock() }()
	var saved Saved
	found := false
	for _, e := range m.entries {
		if e.ID == id {
			saved = e
			found = true
			break
		}
	}
	m.mu.Unlock()
	if !found || !saved.Enabled {
		return Result{}, errors.New("plugin unavailable")
	}
	var credential []byte
	var err error
	if len(saved.Secret) > 0 {
		credential, err = m.decrypt(saved.Secret)
		if err != nil {
			return Result{}, errors.New("cannot decrypt plugin credential")
		}
	}
	p, err := Start(ctx, filepath.Join(m.dir, id), notify)
	if err != nil {
		return Result{}, err
	}
	defer p.Close()
	var result Result
	err = p.Call(ctx, method, Operation{Credential: credential, Input: input, Body: body, Model: model}, &result)
	if err != nil {
		return Result{}, err
	}
	if ctx.Err() != nil {
		return Result{}, ctx.Err()
	}
	seen := map[string]bool{}
	for _, v := range result.Models {
		if v.Key == "" || v.UpstreamID == "" || seen[v.Key] {
			return Result{}, errors.New("invalid plugin model catalog")
		}
		seen[v.Key] = true
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	for i, e := range m.entries {
		if e.ID != id {
			continue
		}
		if !e.Enabled || string(e.Secret) != string(saved.Secret) {
			return Result{}, errors.New("plugin credential changed; retry request")
		}
		next := e
		if len(result.Credential) > 0 && !bytes.Equal(result.Credential, credential) {
			next.Secret, err = m.encrypt(result.Credential)
			if err != nil {
				return Result{}, err
			}
		}
		if result.Models != nil {
			next.Models = result.Models
		}
		if result.Models == nil && bytes.Equal(next.Secret, e.Secret) {
			return result, nil
		}
		m.entries[i] = next
		if err = m.persist(); err != nil {
			m.entries[i] = e
			return Result{}, err
		}
		return result, nil
	}
	return Result{}, errors.New("plugin removed")
}

// Close cancels operations owned by the application during shutdown.
func (m *Manager) Close() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.closed = true
	for _, cancel := range m.active {
		cancel()
	}
}
