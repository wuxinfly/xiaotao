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

type credentialLockKey struct {
	store *storage.Store
	id    string
}

var credentialLocks sync.Map

// Both chat and quota share this refresh lock and reload before rotating.
func refreshStoredCredential(ctx context.Context, store *storage.Store, secret func(string) ([]byte, error), acct storage.Account, cred *providers.Credential) error {
	value, _ := credentialLocks.LoadOrStore(credentialLockKey{store, acct.ID}, &sync.Mutex{})
	lock := value.(*sync.Mutex)
	lock.Lock()
	defer lock.Unlock()
	currentAccount, ok := store.Account(acct.ID)
	if !ok {
		return fmt.Errorf("account deleted")
	}
	acct = currentAccount
	revision, err := store.SecretRevision(acct.ID)
	if err != nil {
		return err
	}
	raw, err := secret(acct.ID)
	if err != nil {
		return err
	}
	var latest providers.Credential
	if err := json.Unmarshal(raw, &latest); err != nil {
		return fmt.Errorf("invalid stored credential")
	}
	latest.UID, latest.Region = acct.UID, acct.Region
	latest.MachineID, latest.DeviceID = acct.MachineID, acct.DeviceID
	if latest.AccessToken != cred.AccessToken && (latest.ExpiresAt == 0 || latest.ExpiresAt-time.Now().Unix() >= 300) {
		*cred = latest
		return nil
	}
	next, err := providerRefresh(ctx, acct.Provider, &latest)
	if err != nil {
		return err
	}
	if next.UID == "" {
		next.UID = latest.UID
	}
	if next.Nickname == "" {
		next.Nickname = latest.Nickname
	}
	if next.Region == "" {
		next.Region = latest.Region
	}
	if next.MachineID == "" {
		next.MachineID = latest.MachineID
	}
	if next.DeviceID == "" {
		next.DeviceID = latest.DeviceID
	}
	if next.RefreshToken == "" {
		next.RefreshToken = latest.RefreshToken
	}
	data, err := json.Marshal(next)
	if err != nil {
		return err
	}
	if err := store.ReplaceSecret(acct.ID, revision, data, encryptFn); err != nil {
		return err
	}
	*cred = *next
	return nil
}
