package modelsource

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"maps"
	"os"
	"path/filepath"
	"strings"
	"sync"
)

type cryptFunc func([]byte) ([]byte, error)
type entry struct {
	Source Source `json:"source"`
	Secret []byte `json:"encryptedAPIKey"`
}
type diskFile struct {
	Version int               `json:"version"`
	Sources []entry           `json:"sources"`
	Routes  map[string]string `json:"routes,omitempty"`
}
type Store struct {
	mu               sync.RWMutex
	path             string
	encrypt, decrypt cryptFunc
	entries          []entry
	routes           map[string]string
}

// Open loads data/model-sources/sources.json. Corrupt files fail closed and are never replaced.
func Open(dir string, encrypt, decrypt func([]byte) ([]byte, error)) (*Store, error) {
	if encrypt == nil || decrypt == nil {
		return nil, errors.New("model source encryption is unavailable")
	}
	return open(dir, encrypt, decrypt, true)
}

func open(dir string, encrypt, decrypt cryptFunc, persistRoutes bool) (*Store, error) {
	s := &Store{path: filepath.Join(dir, "model-sources", "sources.json"), encrypt: encrypt, decrypt: decrypt, entries: []entry{}}
	b, err := os.ReadFile(s.path)
	if os.IsNotExist(err) {
		return s, nil
	}
	if err != nil {
		return nil, errors.New("cannot read model sources")
	}
	var disk diskFile
	if json.Unmarshal(b, &disk) != nil || disk.Version != 1 || disk.Sources == nil {
		return nil, errors.New("model source file is damaged")
	}
	seen := map[string]bool{}
	routes := map[string]bool{}
	for _, e := range disk.Sources {
		src := e.Source
		if !strings.HasPrefix(src.ID, "source_") || seen[src.ID] || src.Kind != "manual" || src.Protocol != "openai-chat" || strings.TrimSpace(src.Label) == "" || len(e.Secret) == 0 {
			return nil, errors.New("model source file is damaged")
		}
		normalized, err := NormalizeURL(src.BaseURL)
		if err != nil || normalized != src.BaseURL {
			return nil, errors.New("model source file is damaged")
		}
		owned := map[string]bool{}
		for _, m := range src.Models {
			if !strings.HasPrefix(m.ID, "model_") || routes[m.ID] {
				return nil, errors.New("model source file is damaged")
			}
			routes[m.ID] = true
			owned[m.ID] = true
		}
		if _, err := validateModels(src.Models, owned); err != nil {
			return nil, errors.New("model source file is damaged")
		}
		key, err := decrypt(e.Secret)
		if err != nil || len(key) == 0 {
			return nil, errors.New("cannot decrypt model source key")
		}
		seen[src.ID] = true
	}
	s.entries = disk.Sources
	if err := validateRoutes(disk.Routes, disk.Sources); err != nil {
		return nil, err
	}
	s.routes = allocateRoutes(disk.Routes, disk.Sources)
	if persistRoutes && !maps.Equal(s.routes, disk.Routes) {
		if err := s.commit(s.entries); err != nil {
			return nil, err
		}
	}
	return s, nil
}

func newID(prefix string) (string, error) {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return "", errors.New("cannot generate identifier")
	}
	return prefix + hex.EncodeToString(b), nil
}
func (s *Store) public(e entry) Source {
	src := e.Source
	src.HasAPIKey = len(e.Secret) > 0
	src.Models = append([]Model{}, src.Models...)
	for i := range src.Models {
		src.Models[i].CallID = s.routes[src.Models[i].ID]
		src.Models[i].ReasoningEfforts = append([]string(nil), src.Models[i].ReasoningEfforts...)
	}
	return src
}
func (s *Store) List() []Source {
	s.mu.RLock()
	defer s.mu.RUnlock()
	result := make([]Source, 0, len(s.entries))
	for _, e := range s.entries {
		result = append(result, s.public(e))
	}
	return result
}
func (s *Store) Get(id string) (Source, error) {
	for _, src := range s.List() {
		if src.ID == id {
			return src, nil
		}
	}
	return Source{}, ErrNotFound
}

func (s *Store) Save(id string, in Input) (Source, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	index := -1
	var old entry
	for i, e := range s.entries {
		if e.Source.ID == id {
			index = i
			old = e
			break
		}
	}
	if id != "" && index < 0 {
		return Source{}, ErrNotFound
	}
	label := strings.TrimSpace(in.Label)
	base, err := NormalizeURL(in.BaseURL)
	if label == "" || in.Protocol != "openai-chat" || err != nil {
		return Source{}, ErrInvalid
	}
	owned := map[string]bool{}
	existing := map[string]string{}
	for _, m := range old.Source.Models {
		owned[m.ID] = true
		existing[m.UpstreamModelID] = m.ID
	}
	requested := append([]Model(nil), in.Models...)
	for i := range requested {
		if requested[i].ID == "" {
			requested[i].ID = existing[strings.TrimSpace(requested[i].UpstreamModelID)]
		}
	}
	models, err := validateModels(requested, owned)
	if err != nil {
		return Source{}, err
	}
	secret := old.Secret
	if in.APIKey != nil {
		key := strings.TrimSpace(*in.APIKey)
		if key == "" || strings.ContainsAny(key, "\r\n") {
			return Source{}, ErrInvalid
		}
		secret, err = s.encrypt([]byte(key))
		if err != nil || len(secret) == 0 {
			return Source{}, errors.New("cannot encrypt model source key")
		}
	}
	if len(secret) == 0 {
		return Source{}, fmt.Errorf("%w: API key is required", ErrInvalid)
	}
	if id == "" {
		id, err = newID("source_")
		if err != nil {
			return Source{}, err
		}
	}
	enabled := true
	if in.Enabled != nil {
		enabled = *in.Enabled
	}
	e := entry{Source: Source{ID: id, Label: label, Kind: "manual", Protocol: in.Protocol, BaseURL: base, Enabled: enabled, Models: models}, Secret: secret}
	next := append([]entry(nil), s.entries...)
	if index < 0 {
		next = append(next, e)
	} else {
		next[index] = e
	}
	if err = s.commit(next); err != nil {
		return Source{}, err
	}
	s.entries = next
	return s.public(e), nil
}

func (s *Store) Delete(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	next := make([]entry, 0, len(s.entries))
	found := false
	for _, e := range s.entries {
		if e.Source.ID == id {
			found = true
		} else {
			next = append(next, e)
		}
	}
	if !found {
		return ErrNotFound
	}
	if err := s.commit(next); err != nil {
		return err
	}
	s.entries = next
	return nil
}

func (s *Store) Resolve(modelID string) (Source, Model, string, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	for _, e := range s.entries {
		if !e.Source.Enabled || len(e.Secret) == 0 {
			continue
		}
		for _, m := range e.Source.Models {
			if m.ID == modelID || s.routes[m.ID] == modelID {
				key, err := s.decrypt(e.Secret)
				if err != nil || len(key) == 0 {
					return Source{}, Model{}, "", errors.New("cannot decrypt model source key")
				}
				m.CallID = s.routes[m.ID]
				m.ReasoningEfforts = append([]string(nil), m.ReasoningEfforts...)
				return s.public(e), m, string(key), nil
			}
		}
	}
	return Source{}, Model{}, "", ErrNotFound
}

func (s *Store) commit(entries []entry) error {
	routes := allocateRoutes(s.routes, entries)
	b, err := json.MarshalIndent(diskFile{Version: 1, Sources: entries, Routes: routes}, "", "  ")
	if err != nil {
		return errors.New("cannot encode model sources")
	}
	dir := filepath.Dir(s.path)
	if os.MkdirAll(dir, 0700) != nil {
		return errors.New("cannot prepare model source directory")
	}
	f, err := os.CreateTemp(dir, ".sources-*")
	if err != nil {
		return errors.New("cannot prepare model source file")
	}
	name := f.Name()
	defer os.Remove(name)
	if err = f.Chmod(0600); err == nil {
		_, err = f.Write(b)
	}
	if err == nil {
		err = f.Sync()
	}
	closeErr := f.Close()
	if err == nil {
		err = closeErr
	}
	if err != nil {
		return errors.New("cannot write model source file")
	}
	if os.Rename(name, s.path) != nil {
		return errors.New("cannot replace model source file")
	}
	s.routes = routes
	return nil
}
