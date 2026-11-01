package proxy

import (
	"context"
	"fmt"
	"sync"
	"time"

	"xiaotao/xt-client/internal/providers"
	"xiaotao/xt-client/internal/providers/trae"
	"xiaotao/xt-client/internal/providers/workbuddy"
	"xiaotao/xt-client/internal/storage"
)

type CheckinClaimResult struct {
	OK           bool    `json:"ok"`
	CheckedIn    bool    `json:"checkedIn"`
	ExtraCredits float64 `json:"extraCredits"`
	StreakDays   int     `json:"streakDays"`
	Message      string  `json:"message"`
}

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
	if a.providerID(id) == "trae" || a.providerID(id) == "workbuddy" {
		go func() {
			_, _ = a.FetchCheckin(id)
		}()
	}
	return *snap, nil
}

func (a *AccountService) FetchCheckin(id string) (storage.CheckinStatus, error) {
	cred, err := a.decodeCred(id)
	if err != nil {
		return storage.CheckinStatus{}, err
	}
	if cred.ExpiresAt > 0 && cred.ExpiresAt-time.Now().Unix() < 300 {
		_ = a.refreshToken(id, cred)
	}
	provider := a.providerID(id)
	var st storage.CheckinStatus
	switch provider {
	case "trae":
		res, err := trae.FetchCheckinStatus(cred, id)
		if err != nil {
			return storage.CheckinStatus{}, err
		}
		st = storage.CheckinStatus{
			Supported:   res.Supported,
			Active:      res.Active,
			CheckedIn:   res.CheckedIn,
			DailyCredit: res.DailyCredit,
			Message:     res.Message,
			FetchedAt:   time.Now().Unix(),
		}
	case "workbuddy":
		res, err := workbuddy.FetchCheckinStatus(context.Background(), "", cred)
		if err != nil {
			return storage.CheckinStatus{}, err
		}
		st = storage.CheckinStatus{
			Supported:   res.Supported,
			Active:      res.Active,
			CheckedIn:   res.CheckedIn,
			StreakDays:  res.StreakDays,
			DailyCredit: res.DailyCredit,
			Message:     res.Message,
			FetchedAt:   time.Now().Unix(),
		}
	default:
		st = storage.CheckinStatus{
			Supported: false,
			Message:   "该平台不支持签到",
			FetchedAt: time.Now().Unix(),
		}
	}
	_ = a.Store.UpdateAccount(id, func(acct *storage.Account) {
		acct.Checkin = &st
	})
	return st, nil
}

func (a *AccountService) ClaimCheckin(id string) (CheckinClaimResult, error) {
	cred, err := a.decodeCred(id)
	if err != nil {
		return CheckinClaimResult{OK: false, Message: err.Error()}, err
	}
	if cred.ExpiresAt > 0 && cred.ExpiresAt-time.Now().Unix() < 300 {
		if err := a.refreshToken(id, cred); err != nil {
			return CheckinClaimResult{OK: false, Message: "Token 刷新失败: " + err.Error()}, err
		}
	}
	provider := a.providerID(id)
	var result CheckinClaimResult
	switch provider {
	case "trae":
		res, err := trae.ClaimCheckin(cred, id)
		if err != nil {
			return CheckinClaimResult{OK: false, Message: err.Error()}, err
		}
		result = CheckinClaimResult{
			OK:           res.OK,
			CheckedIn:    res.CheckedIn,
			ExtraCredits: res.ExtraCredits,
			Message:      res.Message,
		}
	case "workbuddy":
		res, err := workbuddy.ClaimCheckin(context.Background(), "", cred)
		if err != nil {
			return CheckinClaimResult{OK: false, Message: err.Error()}, err
		}
		result = CheckinClaimResult{
			OK:           res.OK,
			CheckedIn:    res.CheckedIn,
			ExtraCredits: res.ExtraCredits,
			StreakDays:   res.StreakDays,
			Message:      res.Message,
		}
	default:
		return CheckinClaimResult{OK: false, Message: "该平台不支持签到"}, fmt.Errorf("provider %s does not support checkin", provider)
	}

	if result.OK {
		_ = a.Store.UpdateAccount(id, func(acct *storage.Account) {
			if acct.Checkin == nil {
				acct.Checkin = &storage.CheckinStatus{Supported: true}
			}
			acct.Checkin.CheckedIn = true
			if result.StreakDays > 0 {
				acct.Checkin.StreakDays = result.StreakDays
			}
			acct.Checkin.FetchedAt = time.Now().Unix()
		})
		go func() {
			_, _ = a.RefreshQuota(id)
		}()
	}
	return result, nil
}

func (a *AccountService) ClaimAllSupported() map[string]CheckinClaimResult {
	results := make(map[string]CheckinClaimResult)
	accounts := a.Store.Accounts()
	first := true
	for _, acct := range accounts {
		if !acct.Enabled {
			continue
		}
		if acct.Provider != "trae" && acct.Provider != "workbuddy" {
			continue
		}
		if !first {
			time.Sleep(3 * time.Second)
		}
		first = false
		res, _ := a.ClaimCheckin(acct.ID)
		results[acct.ID] = res
	}
	return results
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
