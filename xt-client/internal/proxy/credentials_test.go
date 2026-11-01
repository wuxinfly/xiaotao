package proxy

import (
	"context"
	"encoding/json"
	"sync"
	"sync/atomic"
	"testing"
	"time"
	"xiaotao/xt-client/internal/providers"
	"xiaotao/xt-client/internal/storage"
)

type rotationProvider struct {
	calls atomic.Int32
	hook  func()
}

func (*rotationProvider) Info() providers.Info                    { return providers.Info{ID: "rotation-test"} }
func (*rotationProvider) ListModels(string) []providers.ModelInfo { return nil }
func (p *rotationProvider) Refresh(_ context.Context, c *providers.Credential) (*providers.Credential, error) {
	p.calls.Add(1)
	if p.hook != nil {
		p.hook()
	}
	next := *c
	next.AccessToken = "new"
	next.RefreshToken = "rotated"
	next.ExpiresAt = time.Now().Add(time.Hour).Unix()
	return &next, nil
}

func TestSharedCredentialRotation(t *testing.T) {
	originalEncrypt := defaultEncrypt
	SetEncryptor(func(b []byte) ([]byte, error) { return b, nil })
	defer SetEncryptor(originalEncrypt)
	p := &rotationProvider{}
	providers.Register(p)
	store, _ := storage.Open(t.TempDir())
	acct := storage.Account{ID: "a", Provider: "rotation-test", UID: "u", Region: "cn"}
	store.UpsertAccount(&acct)
	initial := providers.Credential{AccessToken: "old", RefreshToken: "old-refresh", ExpiresAt: 1}
	raw, _ := json.Marshal(initial)
	store.SaveSecret("a", raw, encryptFn)
	secret := func(id string) ([]byte, error) {
		return store.LoadSecret(id, func(b []byte) ([]byte, error) { return b, nil })
	}
	chat := New(store, secret, "")
	quota := NewAccountService(store, secret)
	var wg sync.WaitGroup
	errs := make(chan error, 2)
	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			cred := initial
			if i == 0 {
				errs <- chat.refreshCredential(&acct, &cred)
			} else {
				errs <- quota.refreshToken("a", &cred)
			}
			if cred.AccessToken != "new" {
				t.Error("caller did not receive refreshed token")
			}
		}(i)
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatal(err)
		}
	}
	if p.calls.Load() != 1 {
		t.Fatalf("refresh called %d times", p.calls.Load())
	}
	p.hook = func() { store.SaveSecret("a", []byte(`{"AccessToken":"relogin"}`), encryptFn) }
	cred := providers.Credential{AccessToken: "new"}
	if err := chat.refreshCredential(&acct, &cred); err == nil {
		t.Fatal("stale refresh replaced login")
	}
	saved, _ := secret("a")
	var current providers.Credential
	json.Unmarshal(saved, &current)
	if current.AccessToken != "relogin" {
		t.Fatal("relogin overwritten")
	}
	p.hook = func() { store.DeleteAccount("a") }
	cred.AccessToken = "relogin"
	if err := chat.refreshCredential(&acct, &cred); err == nil {
		t.Fatal("deleted account revived")
	}
}
