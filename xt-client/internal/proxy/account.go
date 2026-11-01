package proxy

import (
	"context"
	"fmt"
	"sync"
	"time"

	"xiaotao/xt-client/internal/providers"
	"xiaotao/xt-client/internal/storage"
)

type AccountService struct {
	Store   *storage.Store
	secret  func(id string) ([]byte, error)
	mu      sync.Mutex
	refresh map[string]chan struct{}
	quotaAt map[string]int64
}

func NewAccountService(store *storage.Store, secret func(id string) ([]byte, error)) *AccountService {
	return &AccountService{Store: store, secret: secret, refresh: map[string]chan struct{}{}, quotaAt: map[string]int64{}}
}

func (a *AccountService) decodeCred(id string) (*providers.Credential, error) {
	b, err := a.secret(id)
	if err != nil {
		return nil, err
	}
	var cred providers.Credential
	if err := jsonUnmarshal(b, &cred); err != nil {
		return nil, fmt.Errorf("invalid credential: %w", err)
	}
	if acct, ok := a.Store.Account(id); ok {
		cred.UID = acct.UID
		cred.Region = acct.Region
		cred.MachineID = acct.MachineID
		cred.DeviceID = acct.DeviceID
	}
	return &cred, nil
}

func (a *AccountService) RefreshQuota(id string) (storage.QuotaSnapshot, error) {
	a.mu.Lock()
	if ch, ok := a.refresh[id]; ok {
		a.mu.Unlock()
		<-ch
		if acct, ok := a.Store.Account(id); ok && acct.Quota != nil {
			return *acct.Quota, nil
		}
		return storage.QuotaSnapshot{Status: "error", Error: "quota refresh unavailable"}, fmt.Errorf("quota refresh unavailable")
	}
	ch := make(chan struct{})
	a.refresh[id] = ch
	a.mu.Unlock()
	defer func() {
		a.mu.Lock()
		delete(a.refresh, id)
		a.mu.Unlock()
		close(ch)
	}()
	cred, err := a.decodeCred(id)
	if err != nil {
		return storage.QuotaSnapshot{}, err
	}
	if cred.ExpiresAt > 0 && cred.ExpiresAt-time.Now().Unix() < 300 {
		if err := a.refreshToken(id, cred); err != nil {
			a.Store.UpdateAccount(id, func(acct *storage.Account) {
				acct.AuthState = "expired"
			})
			return storage.QuotaSnapshot{}, err
		}
	}
	snap, err := providerQuota(context.Background(), a.providerID(id), cred)
	if err != nil {
		a.Store.UpdateAccount(id, func(acct *storage.Account) {
			if acct.Quota == nil {
				acct.Quota = &storage.QuotaSnapshot{Status: "error", Items: []storage.QuotaItem{}, FetchedAt: time.Now().Unix()}
			}
			acct.Quota.Status = "error"
			acct.Quota.Error = redactText(err.Error())
			acct.Quota.FetchedAt = time.Now().Unix()
		})
		return storage.QuotaSnapshot{}, err
	}
	_ = a.Store.UpdateAccount(id, func(acct *storage.Account) {
		acct.Quota = snap
		acct.AuthState = "valid"
	})
	a.mu.Lock()
	a.quotaAt[id] = time.Now().Unix()
	a.mu.Unlock()
	return *snap, nil
}

func (a *AccountService) providerID(id string) string {
	if acct, ok := a.Store.Account(id); ok {
		return acct.Provider
	}
	return ""
}

func (a *AccountService) refreshToken(id string, cred *providers.Credential) error {
	acct, ok := a.Store.Account(id)
	if !ok {
		return fmt.Errorf("account deleted")
	}
	return refreshStoredCredential(context.Background(), a.Store, a.secret, acct, cred)
}
func (a *AccountService) CacheFresh(id string, maxAge int64) bool {
	a.mu.Lock()
	defer a.mu.Unlock()
	at, ok := a.quotaAt[id]
	return ok && time.Now().Unix()-at < maxAge
}
