package sourceplugin

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"time"

	"xiaotao/xt-client/internal/modelsource"
)

// PackageRef keeps the previously installed package for a single-step rollback.
type PackageRef struct {
	Manifest    Manifest `json:"manifest"`
	PackageDir  string   `json:"packageDir"`
	PackageHash string   `json:"packageHash,omitempty"`
	Models      []Model  `json:"models,omitempty"`
}

type Saved struct {
	ID              string            `json:"id"`
	Manifest        Manifest          `json:"manifest"`
	Enabled         bool              `json:"enabled"`
	Models          []Model           `json:"models"`
	ModelSettings   *ModelSettings    `json:"modelSettings,omitempty"`
	SettingValues   map[string]string `json:"settingValues,omitempty"`
	PackageDir      string            `json:"packageDir,omitempty"`
	PackageHash     string            `json:"packageHash,omitempty"`
	Previous        *PackageRef       `json:"previous,omitempty"`
	Diagnostics     bool              `json:"diagnostics,omitempty"`
	DiagnosticsPath string            `json:"diagnosticsPath,omitempty"`
	Accounts        []PluginAccount   `json:"accounts,omitempty"`
	Secret          []byte            `json:"secret,omitempty"`
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
	for i, e := range m.entries {
		if !identifier.MatchString(e.ID) || e.Manifest.ID != e.ID || seen[e.ID] {
			return nil, errors.New("invalid plugin state identity")
		}
		seen[e.ID] = true
		if (e.PackageDir != "" && !validPackageDir(e.PackageDir)) || (e.Previous != nil && !validPackageDir(e.Previous.PackageDir)) {
			return nil, errors.New("invalid plugin package directory")
		}
		if len(e.Secret) > 0 && len(e.Accounts) == 0 {
			m.entries[i].Accounts = []PluginAccount{{ID: "original", Name: "已导入账号", Secret: e.Secret}}
			m.entries[i].Secret = nil
		}
	}
	return m, nil
}
func validPackageDir(name string) bool {
	return name != "" && strings.HasPrefix(name, ".install-") && filepath.Base(name) == name && filepath.VolumeName(name) == ""
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
func (m *Manager) diagnosticsWriter(e Saved) io.Writer {
	if !e.Diagnostics {
		return nil
	}
	dir := filepath.Join(m.dir, "diagnostics")
	if os.MkdirAll(dir, 0700) != nil {
		return nil
	}
	path := filepath.Join(dir, e.ID+".log")
	if info, err := os.Stat(path); err == nil && info.Size() > 1<<20 {
		_ = os.Truncate(path, 0)
	}
	file, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0600)
	if err != nil {
		return nil
	}
	return file
}

func (m *Manager) publicEntry(e Saved) Saved {
	e.Accounts = publicAccounts(e.Accounts)
	e.Secret = nil
	e.Models = append([]Model(nil), e.Models...)
	e.SettingValues = e.Settings()
	if e.ModelSettings != nil {
		settings := *e.ModelSettings
		settings.SelectedKeys = slices.Clone(settings.SelectedKeys)
		settings.CustomIDs = slices.Clone(settings.CustomIDs)
		e.ModelSettings = &settings
	}
	if e.Previous != nil {
		previous := *e.Previous
		previous.Models = append([]Model(nil), previous.Models...)
		e.Previous = &previous
	}
	e.DiagnosticsPath = filepath.Join(m.dir, "diagnostics", e.ID+".log")
	return e
}
func (m *Manager) List() []Saved {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make([]Saved, 0, len(m.entries))
	for _, e := range m.entries {
		out = append(out, m.publicEntry(e))
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
		for _, v := range e.selectedModels() {
			src.Models = append(src.Models, modelsource.Model{ID: route(e.ID, v.Key), CallID: modelsource.CallID(e.ID, v.Key), Label: v.Label, UpstreamModelID: v.UpstreamID, ContextWindow: v.ContextWindow, SupportsImages: v.SupportsImages, SupportsReasoning: v.SupportsReasoning, ReasoningEfforts: v.ReasoningEfforts, PriceFactor: v.PriceFactor, IsFree: v.IsFree})
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
			if v.ID == id || v.CallID == modelsource.ReadableCallID(id) {
				return src, v, nil
			}
		}
	}
	return modelsource.Source{}, modelsource.Model{}, modelsource.ErrNotFound
}

// Operations serialize credential refresh and re-login with state writes. The
// mutex is acquired only for state snapshots and compare-and-swap, never network IO.
func (m *Manager) Run(ctx context.Context, id, method string, input json.RawMessage, body map[string]any, model string, notify func(Message)) (Result, error) {
	return m.RunForAccount(ctx, id, "", method, input, body, model, notify)
}

func (m *Manager) RunForAccount(ctx context.Context, id, accountID, method string, input json.RawMessage, body map[string]any, model string, notify func(Message)) (Result, error) {
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
	adding := method == "login/browser" || method == "login/import" || method == "login/complete" || method == "credentials/import"
	account, err := saved.accountFor(accountID, adding)
	if err != nil {
		return Result{}, err
	}
	var credential []byte
	if len(account.Secret) > 0 {
		credential, err = m.decrypt(account.Secret)
		if err != nil {
			return Result{}, errors.New("cannot decrypt plugin credential")
		}
	}
	packageDir := saved.PackageDir
	if packageDir == "" {
		packageDir = id
	}
	stderr := m.diagnosticsWriter(saved)
	if stderr != nil {
		fmt.Fprintf(stderr, "\n[%s] %s v%s %s\n", time.Now().Format(time.RFC3339), saved.ID, saved.Manifest.Version, method)
		if closer, ok := stderr.(io.Closer); ok {
			defer closer.Close()
		}
	}
	p, err := Start(ctx, filepath.Join(m.dir, packageDir), notify, stderr)
	if err != nil {
		return Result{}, err
	}
	defer p.Close()
	var result Result
	err = p.Call(ctx, method, Operation{Credential: credential, Input: input, Body: body, Model: model, Settings: saved.Settings()}, &result)
	if err != nil {
		if stderr != nil {
			fmt.Fprintf(stderr, "[host] %s failed: %s (%s)\n", method, err.Error(), ErrorCode(err))
		}
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
		if !e.Enabled {
			return Result{}, errors.New("plugin disabled")
		}
		next := e
		changed, err := m.applyAccountResult(&next, account, result, credential, adding)
		if err != nil {
			return Result{}, err
		}
		if result.Models != nil && !adding {
			next.Models = result.Models
		}
		if (result.Models == nil || adding) && !changed {
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
