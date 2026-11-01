package proxy

import (
	"net/http"
	"strings"
	"time"

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

func (s *Service) accountCandidates(model string) ([]storage.Account, string) {
	providerID, modelID := splitModelID(model)
	accounts := []storage.Account{}
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
			continue
		}
		accounts = append(accounts, *acct)
	}
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
		for _, m := range s.AccountModels(acct) {
			if m.ID == modelID {
				return true
			}
		}
		return false
	}
	return supportsModel(acct, modelID)
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
