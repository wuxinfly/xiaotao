package storage

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
)

// ModelCatalogCache is environment-local and contains no credentials.
type ModelCatalogCache struct {
	Provider string          `json:"provider"`
	Region   string          `json:"region"`
	UID      string          `json:"uid"`
	Revision string          `json:"revision"`
	Source   string          `json:"source,omitempty"`
	Models   json.RawMessage `json:"models"`
}

func (s *Store) ModelCatalog(id string) (ModelCatalogCache, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	cache, ok := s.data.ModelCatalogs[id]
	cache.Models = append(json.RawMessage(nil), cache.Models...)
	return cache, ok
}

// SaveModelCatalog checks identity and login revision under the same lock as writes.
func (s *Store) SaveModelCatalog(id string, cache ModelCatalogCache) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	found := false
	for _, acct := range s.data.Accounts {
		if acct.ID == id && acct.Provider == cache.Provider && acct.Region == cache.Region && acct.UID == cache.UID {
			found = true
			break
		}
	}
	secret, ok := s.data.Secrets[id]
	if !found || !ok || fmt.Sprintf("%x", sha256.Sum256([]byte(secret))) != cache.Revision {
		return fmt.Errorf("account changed during model refresh")
	}
	if s.data.ModelCatalogs == nil {
		s.data.ModelCatalogs = map[string]ModelCatalogCache{}
	}
	previous, existed := s.data.ModelCatalogs[id]
	cache.Models = append(json.RawMessage(nil), cache.Models...)
	s.data.ModelCatalogs[id] = cache
	if err := s.save(); err != nil {
		if existed {
			s.data.ModelCatalogs[id] = previous
		} else {
			delete(s.data.ModelCatalogs, id)
		}
		return err
	}
	return nil
}
