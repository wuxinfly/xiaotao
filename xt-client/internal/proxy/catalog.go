package proxy

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"time"
	"xiaotao/xt-client/internal/providers"
	"xiaotao/xt-client/internal/storage"
)

type catalogKey struct {
	store      *storage.Store
	id, region string
}

var catalogLocks sync.Map

// A credential revision scopes the cache to the current login as well as region.
func (s *Service) dynamicCatalog(acct storage.Account, fetcher providers.ModelFetcher, force bool) ([]providers.ModelInfo, string, error) {
	value, _ := catalogLocks.LoadOrStore(catalogKey{s.store, acct.ID, acct.Region}, &sync.Mutex{})
	lock := value.(*sync.Mutex)
	lock.Lock()
	defer lock.Unlock()
	revision, err := s.store.SecretRevision(acct.ID)
	if err != nil {
		return []providers.ModelInfo{}, "error", err
	}
	s.mu.Lock()
	previous, ok := s.catalogs[acct.ID]
	s.mu.Unlock()
	ok = ok && previous.region == acct.Region && previous.revision == revision
	if ok && !force && time.Since(previous.at) < 5*time.Minute {
		return previous.models, "cached", nil
	}
	fail := func(err error) ([]providers.ModelInfo, string, error) {
		if ok {
			return previous.models, "stale", err
		}
		if acct.Provider == "codex" {
			fallback := listProviderModels(acct)
			if len(fallback) > 0 {
				return fallback, "builtin", err
			}
		}
		return []providers.ModelInfo{}, "error", err
	}
	raw, err := s.secret(acct.ID)
	if err != nil {
		return fail(err)
	}
	var cred providers.Credential
	if json.Unmarshal(raw, &cred) != nil {
		return fail(fmt.Errorf("account credential unavailable"))
	}
	cred.Region, cred.UID, cred.MachineID = acct.Region, acct.UID, acct.MachineID
	if cred.ExpiresAt > 0 && cred.ExpiresAt-time.Now().Unix() < 300 {
		if err = s.refreshCredential(&acct, &cred); err != nil {
			return fail(err)
		}
		revision, err = s.store.SecretRevision(acct.ID)
		if err != nil {
			return fail(err)
		}
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	models, err := fetcher.FetchModels(ctx, &cred)
	if err != nil {
		return fail(err)
	}
	current, err := s.store.SecretRevision(acct.ID)
	if _, exists := s.store.Account(acct.ID); !exists || err != nil || current != revision {
		return nil, "error", fmt.Errorf("account changed during model refresh")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.catalogs == nil {
		s.catalogs = map[string]catalogSnapshot{}
	}
	s.catalogs[acct.ID] = catalogSnapshot{models: models, at: time.Now(), region: acct.Region, revision: revision}
	return models, "live", nil
}

func (s *Service) RefreshAccountModelCatalog(acct storage.Account) ([]providers.ModelInfo, string, error) {
	p, err := providers.Get(acct.Provider)
	if err != nil {
		return nil, "error", err
	}
	if fetcher, ok := p.(providers.ModelFetcher); ok {
		return s.dynamicCatalog(acct, fetcher, true)
	}
	models, source := s.AccountModelCatalog(acct)
	return models, source, nil
}
