package proxy

import (
	"net/http"
	"strings"
	"time"

	"xiaotao/xt-client/internal/logging"
	"xiaotao/xt-client/internal/providers"
	"xiaotao/xt-client/internal/storage"
)

const (
	quotaCooldown   = time.Minute
	accountCooldown = 15 * time.Second
)

// SelectAccount routes by the model prefix: the model chooses the provider,
// then accounts are tried in store order (added time) and accounts with a
// known exhausted quota are skipped. Missing quota data never blocks a call.
func (s *Service) SelectAccount(model string) (storage.Account, string, bool) {
	accounts, modelID := s.accountCandidates(model)
	if len(accounts) == 0 {
		return storage.Account{}, "", false
	}
	return accounts[0], modelID, true
}

func (s *Service) accountCandidatesWithReasons(model string) ([]storage.Account, []SkipReason) {
	providerID, modelID := splitModelID(model)
	accounts := []storage.Account{}
	var skips []SkipReason
	for _, acct := range s.store.Accounts() {
		if acct.Provider != providerID {
			continue
		}
		if !acct.Enabled {
			skips = append(skips, SkipReason{Account: acct.ID, Provider: acct.Provider, Model: model, Reason: "disabled"})
			continue
		}
		if !s.supportsAccountModel(*acct, modelID) {
			skips = append(skips, SkipReason{Account: acct.ID, Provider: acct.Provider, Model: model, Reason: "model_unsupported"})
			continue
		}
		if acct.Provider != "qoder" && quotaExhausted(acct.Quota) {
			skips = append(skips, SkipReason{Account: acct.ID, Provider: acct.Provider, Model: model, Reason: "quota_exhausted"})
			continue
		}
		if s.accountCooling(acct.ID) {
			skips = append(skips, SkipReason{Account: acct.ID, Provider: acct.Provider, Model: model, Reason: "cooling"})
			continue
		}
		accounts = append(accounts, *acct)
	}
	for _, skip := range skips {
		logProxyInfo("account skipped",
			logging.F("model", skip.Model),
			logging.F("provider", skip.Provider),
			logging.F("account", skip.Account),
			logging.F("reason", skip.Reason),
		)
	}
	return accounts, skips
}

func (s *Service) accountCandidates(model string) ([]storage.Account, string) {
	_, modelID := splitModelID(model)
	accounts, _ := s.accountCandidatesWithReasons(model)
	return accounts, modelID
}

func (s *Service) hasCoolingAccount(model string) bool {
	providerID, modelID := splitModelID(model)
	for _, acct := range s.store.Accounts() {
		if !acct.Enabled || acct.Provider != providerID {
			continue
		}
		if !s.supportsAccountModel(*acct, modelID) {
			continue
		}
		if acct.Provider != "qoder" && quotaExhausted(acct.Quota) {
			continue
		}
		if s.accountCooling(acct.ID) {
			return true
		}
	}
	return false
}

func (s *Service) markAccountCooling(id string, quota bool) {
	until := time.Now().Add(accountCooldown)
	if quota {
		until = time.Now().Add(quotaCooldown)
	}
	s.mu.Lock()
	if s.cooldowns == nil {
		s.cooldowns = map[string]time.Time{}
	}
	s.cooldowns[id] = until
	s.mu.Unlock()
	logProxyInfo("account cooling set",
		logging.F("account", id),
		logging.F("until", until.Format(time.RFC3339)),
		logging.F("quota", quota),
	)
}

func (s *Service) accountCooling(id string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	until, ok := s.cooldowns[id]
	if !ok {
		return false
	}
	if time.Now().Before(until) {
		return true
	}
	delete(s.cooldowns, id)
	logProxyInfo("account cooling expired", logging.F("account", id))
	return false
}

// RetryAccountError reports whether an upstream failure is account-scoped and
// safe to retry before any response has been sent. A 429 is quota exhaustion;
// a 403 can be account-level verification or permission rejection.
func RetryAccountError(status int) bool {
	return status == http.StatusForbidden || status == http.StatusTooManyRequests
}

func (s *Service) supportsAccountModel(acct storage.Account, modelID string) bool {
	p, err := providers.Get(acct.Provider)
	if err != nil {
		return false
	}
	if _, ok := p.(providers.ModelFetcher); ok {
		return modelInCatalog(s.AccountModels(acct), modelID)
	}
	// Trae and Antigravity build their catalogs through a wrapper rather than
	// the ModelFetcher interface. Routing must honour the cached catalog, or
	// live-only variants such as the "-max" context models are rejected as
	// unsupported even though the UI lists them.
	if models, ok := s.cachedAccountModels(acct); ok {
		return modelInCatalog(models, modelID)
	}
	return supportsModel(acct, modelID)
}

func modelInCatalog(models []providers.ModelInfo, modelID string) bool {
	for _, m := range models {
		if m.ID == modelID {
			return true
		}
	}
	return false
}

func splitModelID(model string) (string, string) {
	if prefix, rest, ok := strings.Cut(model, "/"); ok {
		return strings.ToLower(prefix), rest
	}
	return "trae", model
}

func supportsModel(acct storage.Account, modelID string) bool {
	p, err := providers.Get(acct.Provider)
	if err != nil {
		return false
	}
	for _, m := range p.ListModels(acct.Region) {
		if m.ID == modelID {
			return true
		}
	}
	return false
}

// quotaExhausted skips accounts whose remaining quota is explicitly zero.
// Qoder is excluded by the caller: its free models do not consume quota, so a
// zero snapshot cannot prove that the account is unusable.
// Snapshots without remaining values (Trae reports used/total only) and
// error snapshots are treated as unknown and stay eligible.
func quotaExhausted(snapshot *storage.QuotaSnapshot) bool {
	if snapshot == nil || snapshot.Status != "ok" {
		return false
	}
	anyCounted := false
	for _, item := range snapshot.Items {
		if item.Remaining == nil {
			continue
		}
		if item.Total != nil && *item.Total <= 0 {
			continue
		}
		anyCounted = true
		if *item.Remaining > 0 {
			return false
		}
	}
	return anyCounted
}
