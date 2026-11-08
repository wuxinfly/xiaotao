package proxy

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"xiaotao/xt-client/internal/logging"
	"xiaotao/xt-client/internal/providers"
	"xiaotao/xt-client/internal/providers/qoder"
	"xiaotao/xt-client/internal/providers/trae"
	"xiaotao/xt-client/internal/providers/workbuddy"
	"xiaotao/xt-client/internal/storage"
)

type CheckinClaimResult struct {
	Error        string  `json:"error,omitempty"`
	OK           bool    `json:"ok"`
	CheckedIn    bool    `json:"checkedIn"`
	ExtraCredits float64 `json:"extraCredits"`
	StreakDays   int     `json:"streakDays"`
	Message      string  `json:"message"`
}

type AccountService struct {
	Store             *storage.Store
	secret            func(id string) ([]byte, error)
	mu                sync.Mutex
	refresh           map[string]chan struct{}
	quotaAt           map[string]int64
	checkinQuotaDelay time.Duration
}

func NewAccountService(store *storage.Store, secret func(id string) ([]byte, error)) *AccountService {
	return &AccountService{
		Store:             store,
		secret:            secret,
		refresh:           map[string]chan struct{}{},
		quotaAt:           map[string]int64{},
		checkinQuotaDelay: 5 * time.Second,
	}
}

func (a *AccountService) decodeCred(id string) (*providers.Credential, error) {
	b, err := a.secret(id)
	if err != nil {
		logging.Default().For(logging.ModuleAuth).Warn("credential decode failed", logging.F("account", id), logging.F("error", logging.Redact(err.Error(), 200)))
		return nil, err
	}
	var cred providers.Credential
	if err := jsonUnmarshal(b, &cred); err != nil {
		logging.Default().For(logging.ModuleAuth).Warn("credential decode failed", logging.F("account", id), logging.F("error", logging.Redact(err.Error(), 200)))
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
	start := time.Now()
	provider := a.providerID(id)
	logging.Default().For(logging.ModuleAuth).Info("refresh quota started", logging.F("account", id), logging.F("provider", provider))
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
		logging.Default().For(logging.ModuleAuth).Warn("refresh quota failed", logging.F("account", id), logging.F("provider", provider), logging.F("duration_ms", time.Since(start).Milliseconds()), logging.F("error", logging.Redact(err.Error(), 200)))
		return storage.QuotaSnapshot{}, err
	}
	if cred.ExpiresAt > 0 && cred.ExpiresAt-time.Now().Unix() < 300 {
		if err := a.refreshToken(id, cred); err != nil {
			a.Store.UpdateAccount(id, func(acct *storage.Account) {
				acct.AuthState = "expired"
			})
			logging.Default().For(logging.ModuleAuth).Warn("refresh quota failed", logging.F("account", id), logging.F("provider", provider), logging.F("duration_ms", time.Since(start).Milliseconds()), logging.F("error", logging.Redact(err.Error(), 200)))
			return storage.QuotaSnapshot{}, err
		}
	}
	snap, err := providerQuota(context.Background(), provider, cred)
	if err != nil {
		a.Store.UpdateAccount(id, func(acct *storage.Account) {
			if acct.Quota == nil {
				acct.Quota = &storage.QuotaSnapshot{Status: "error", Items: []storage.QuotaItem{}, FetchedAt: time.Now().Unix()}
			}
			acct.Quota.Status = "error"
			acct.Quota.Error = redactText(err.Error())
			acct.Quota.FetchedAt = time.Now().Unix()
		})
		logging.Default().For(logging.ModuleAuth).Warn("refresh quota failed", logging.F("account", id), logging.F("provider", provider), logging.F("duration_ms", time.Since(start).Milliseconds()), logging.F("error", logging.Redact(err.Error(), 200)))
		return storage.QuotaSnapshot{}, err
	}
	_ = a.Store.UpdateAccount(id, func(acct *storage.Account) {
		acct.Quota = snap
		acct.AuthState = "valid"
	})
	a.mu.Lock()
	a.quotaAt[id] = time.Now().Unix()
	a.mu.Unlock()
	if provider == "trae" || provider == "workbuddy" || provider == "qoder" {
		go func() {
			_, _ = a.FetchCheckin(id)
		}()
	}
	logging.Default().For(logging.ModuleAuth).Info("refresh quota finished", logging.F("account", id), logging.F("provider", provider), logging.F("status", snap.Status), logging.F("duration_ms", time.Since(start).Milliseconds()))
	return *snap, nil
}

func (a *AccountService) FetchCheckin(id string) (storage.CheckinStatus, error) {
	cred, err := a.decodeCred(id)
	if err != nil {
		return storage.CheckinStatus{}, err
	}
	if cred.ExpiresAt > 0 && cred.ExpiresAt-time.Now().Unix() < 300 {
		if err := a.refreshToken(id, cred); err != nil {
			return storage.CheckinStatus{}, err
		}
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
	case "qoder":
		res, err := (qoder.CheckinClient{}).Status(context.Background(), cred)
		var upstream *providers.UpstreamError
		if errors.As(err, &upstream) && upstream.Status == 401 {
			if err = a.refreshToken(id, cred); err == nil {
				res, err = (qoder.CheckinClient{}).Status(context.Background(), cred)
			}
		}
		if err != nil {
			return storage.CheckinStatus{}, err
		}
		st = res
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
	logging.Default().For(logging.ModuleAuth).Info("fetch checkin finished",
		logging.F("account", id),
		logging.F("provider", provider),
		logging.F("checked", st.CheckedIn),
		logging.F("streak", st.StreakDays),
	)
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
	case "qoder":
		res, err := (qoder.CheckinClient{}).Claim(context.Background(), cred)
		var upstream *providers.UpstreamError
		if errors.As(err, &upstream) && upstream.Status == 401 {
			if err = a.refreshToken(id, cred); err == nil {
				res, err = (qoder.CheckinClient{}).Claim(context.Background(), cred)
			}
		}
		if err != nil {
			return CheckinClaimResult{Message: err.Error()}, err
		}
		result = CheckinClaimResult{OK: res.OK, CheckedIn: res.CheckedIn, ExtraCredits: res.ExtraCredits, Message: res.Message}
	default:
		return CheckinClaimResult{OK: false, Message: "该平台不支持签到"}, fmt.Errorf("provider %s does not support checkin", provider)
	}

	if result.OK || result.CheckedIn {
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
			time.Sleep(a.checkinQuotaDelay)
			_, _ = a.RefreshQuota(id)
		}()
	}
	logging.Default().For(logging.ModuleAuth).Info("claim checkin finished",
		logging.F("account", id),
		logging.F("provider", provider),
		logging.F("ok", result.OK),
		logging.F("checked", result.CheckedIn),
		logging.F("streak", result.StreakDays),
	)
	return result, nil
}

func (a *AccountService) ClaimAllSupported() map[string]CheckinClaimResult {
	return a.ClaimSupportedProvider("")
}

func (a *AccountService) ClaimSupportedProvider(provider string) map[string]CheckinClaimResult {
	results := make(map[string]CheckinClaimResult)
	accounts := a.Store.Accounts()
	first := true
	for _, acct := range accounts {
		if !acct.Enabled || (provider != "" && acct.Provider != provider) {
			continue
		}
		if acct.Provider != "trae" && acct.Provider != "workbuddy" && acct.Provider != "qoder" {
			continue
		}
		if !first {
			time.Sleep(3 * time.Second)
		}
		first = false
		res, err := a.ClaimCheckin(acct.ID)
		if err != nil {
			res.Error = redactText(err.Error())
		}
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
