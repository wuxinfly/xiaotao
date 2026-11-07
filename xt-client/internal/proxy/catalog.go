package proxy

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"time"
	"xiaotao/xt-client/internal/providers"
	"xiaotao/xt-client/internal/providers/antigravity"
	"xiaotao/xt-client/internal/providers/trae"
	"xiaotao/xt-client/internal/storage"
)

type catalogKey struct {
	store      *storage.Store
	id, region string
}

var catalogLocks sync.Map

const builtinRetryInterval = time.Minute

// Internal routes are excluded from public ModelInfo JSON, but must survive a
// restart in the shared cache (including case-sensitive upstream IDs).
type persistedModel struct {
	providers.ModelInfo
	UpstreamID string            `json:"upstreamId,omitempty"`
	Routes     map[string]string `json:"routes,omitempty"`
}

func encodeCatalog(models []providers.ModelInfo) ([]byte, error) {
	stored := make([]persistedModel, 0, len(models))
	for _, model := range models {
		stored = append(stored, persistedModel{ModelInfo: model, UpstreamID: model.UpstreamID, Routes: model.Routes})
	}
	return json.Marshal(stored)
}

func decodeCatalog(raw []byte) ([]providers.ModelInfo, error) {
	var stored []persistedModel
	if err := json.Unmarshal(raw, &stored); err != nil {
		return nil, err
	}
	models := make([]providers.ModelInfo, 0, len(stored))
	for _, model := range stored {
		model.ModelInfo.UpstreamID, model.ModelInfo.Routes = model.UpstreamID, model.Routes
		models = append(models, model.ModelInfo)
	}
	return models, nil
}

type catalogFetcher func(context.Context, *providers.Credential) ([]providers.ModelInfo, error)

func (f catalogFetcher) FetchModels(ctx context.Context, cred *providers.Credential) ([]providers.ModelInfo, error) {
	return f(ctx, cred)
}

func (s *Service) accountModelCatalog(acct storage.Account, force bool) ([]providers.ModelInfo, string, error) {
	p, err := providers.Get(acct.Provider)
	if err != nil {
		return []providers.ModelInfo{}, "error", err
	}
	if fetcher, ok := p.(providers.ModelFetcher); ok {
		return s.dynamicCatalog(acct, fetcher, force)
	}
	switch acct.Provider {
	case "trae":
		return s.dynamicCatalog(acct, catalogFetcher(func(_ context.Context, cred *providers.Credential) ([]providers.ModelInfo, error) {
			return trae.FetchUpstreamModels(cred, acct.MachineID, acct.DeviceID)
		}), force)
	case "antigravity":
		models, source, err := s.dynamicCatalog(acct, catalogFetcher(antigravity.FetchModels), force)
		if source == "live" {
			source = "merged"
		}
		return models, source, err
	default:
		return p.ListModels(acct.Region), "builtin", nil
	}
}

// No TTL: fetch only for a missing catalog or an explicit refresh.
func (s *Service) dynamicCatalog(acct storage.Account, fetcher providers.ModelFetcher, force bool) ([]providers.ModelInfo, string, error) {
	value, _ := catalogLocks.LoadOrStore(catalogKey{s.store, acct.ID, acct.Region}, &sync.Mutex{})
	lock := value.(*sync.Mutex)
	lock.Lock()
	defer lock.Unlock()
	revision := ""
	var err error
	if s.store != nil {
		revision, err = s.store.SecretRevision(acct.ID)
		if err != nil {
			return []providers.ModelInfo{}, "error", err
		}
	}
	s.mu.Lock()
	previous, ok := s.catalogs[acct.ID]
	s.mu.Unlock()
	ok = ok && previous.region == acct.Region && previous.revision == revision
	if s.store != nil {
		cache, exists := s.store.ModelCatalog(acct.ID)
		if exists {
			ok = cache.Provider == acct.Provider && cache.Region == acct.Region && cache.UID == acct.UID && cache.Revision == revision
			if ok {
				previous.models, err = decodeCatalog(cache.Models)
				ok = err == nil
				previous.source = cache.Source
			}
		}
	}
	if ok && !force && (previous.source != "builtin" || (!previous.at.IsZero() && time.Since(previous.at) < builtinRetryInterval)) {
		if previous.source == "builtin" {
			return previous.models, "builtin", nil
		}
		return previous.models, "cached", nil
	}
	save := func(models []providers.ModelInfo, source string) error {
		if models == nil {
			models = []providers.ModelInfo{}
		}
		if s.store != nil {
			raw, err := encodeCatalog(models)
			if err != nil {
				return err
			}
			if err := s.store.SaveModelCatalog(acct.ID, storage.ModelCatalogCache{Provider: acct.Provider, Region: acct.Region, UID: acct.UID, Revision: revision, Models: raw, Source: source}); err != nil {
				return err
			}
		}
		s.mu.Lock()
		defer s.mu.Unlock()
		if s.catalogs == nil {
			s.catalogs = map[string]catalogSnapshot{}
		}
		s.catalogs[acct.ID] = catalogSnapshot{models: models, at: time.Now(), region: acct.Region, revision: revision, source: source}
		return nil
	}
	fail := func(err error) ([]providers.ModelInfo, string, error) {
		if ok && previous.source != "builtin" {
			return previous.models, "stale", err
		}
		if acct.Provider == "codex" || acct.Provider == "trae" || acct.Provider == "antigravity" {
			fallback := listProviderModels(acct)
			if len(fallback) > 0 {
				if saveErr := save(fallback, "builtin"); saveErr != nil {
					return []providers.ModelInfo{}, "error", saveErr
				}
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
	cred.Region, cred.UID, cred.MachineID, cred.DeviceID = acct.Region, acct.UID, acct.MachineID, acct.DeviceID
	if cred.ExpiresAt > 0 && cred.ExpiresAt-time.Now().Unix() < 300 {
		if s.store == nil {
			return fail(fmt.Errorf("account store unavailable"))
		}
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
	if models == nil {
		models = []providers.ModelInfo{}
	}
	if err := save(models, "live"); err != nil {
		return fail(err)
	}
	return models, "live", nil
}

func (s *Service) RefreshAccountModelCatalog(acct storage.Account) ([]providers.ModelInfo, string, error) {
	return s.accountModelCatalog(acct, true)
}
