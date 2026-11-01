package storage

import (
	"bytes"
	"crypto/rand"
	"os"
	"path/filepath"
	"testing"
)

func TestExportDoesNotAliasSource(t *testing.T) {
	s, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	identity := func(b []byte) ([]byte, error) { return b, nil }
	acct := &Account{ID: "trae:1", Provider: "trae", Nickname: "source", Enabled: true}
	if err := s.UpsertAccount(acct); err != nil {
		t.Fatal(err)
	}
	if err := s.SaveSecret("trae:1", []byte("secret"), identity); err != nil {
		t.Fatal(err)
	}
	if err := s.SaveSecret("proxy", []byte("proxy-key"), identity); err != nil {
		t.Fatal(err)
	}
	accounts, secrets := s.Export(identity)
	accounts[0].Nickname = "changed"
	secrets["trae:1"] = []byte("changed")
	got, ok := s.Account("trae:1")
	if !ok || got.Nickname != "source" {
		t.Fatal("export aliases source account")
	}
	raw, err := s.LoadSecret("trae:1", identity)
	if err != nil || string(raw) != "secret" {
		t.Fatal("export aliases source secret")
	}
	if _, ok := secrets["proxy"]; ok {
		t.Fatal("export includes the internal proxy key")
	}
}

func TestBootstrapIsOneTime(t *testing.T) {
	dst, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	identity := func(b []byte) ([]byte, error) { return b, nil }
	accounts := []*Account{{ID: "trae:1", Provider: "trae"}}
	if err := dst.Bootstrap(accounts, map[string][]byte{"trae:1": []byte("source")}, identity); err != nil {
		t.Fatal(err)
	}
	if err := dst.Bootstrap(
		[]*Account{{ID: "trae:2", Provider: "trae"}},
		map[string][]byte{"trae:2": []byte("later")},
		identity,
	); err != nil {
		t.Fatal(err)
	}
	got, ok := dst.Account("trae:1")
	if !ok || got.ID != "trae:1" {
		t.Fatal("bootstrap overwrote existing account data")
	}
	if _, ok := dst.Account("trae:2"); ok {
		t.Fatal("bootstrap merged later account data")
	}
}

func TestOpenReadOnlyDoesNotCreateFiles(t *testing.T) {
	dir := t.TempDir()
	if _, err := OpenReadOnly(dir); err == nil {
		t.Fatal("expected missing accounts file error")
	}
	if _, err := os.Stat(filepath.Join(dir, "accounts.json")); !os.IsNotExist(err) {
		t.Fatal("read-only open created accounts.json")
	}
}

func TestReorderAccountsPersistsRequestOrder(t *testing.T) {
	dir := t.TempDir()
	s, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	for _, acct := range []*Account{
		{ID: "trae:first", Provider: "trae", AddedAt: 1},
		{ID: "workbuddy:middle", Provider: "workbuddy", AddedAt: 2},
		{ID: "trae:second", Provider: "trae", AddedAt: 3},
	} {
		if err := s.UpsertAccount(acct); err != nil {
			t.Fatal(err)
		}
	}
	if err := s.ReorderAccounts("trae", []string{"trae:second", "trae:first"}); err != nil {
		t.Fatal(err)
	}
	got := s.Accounts()
	if got[0].ID != "trae:second" || got[1].ID != "workbuddy:middle" || got[2].ID != "trae:first" {
		t.Fatalf("unexpected global order: %s, %s, %s", got[0].ID, got[1].ID, got[2].ID)
	}

	reopened, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	got = reopened.Accounts()
	if got[0].ID != "trae:second" || got[2].ID != "trae:first" {
		t.Fatal("request order did not survive restart")
	}
}

func TestReorderAccountsRejectsIncompleteOrder(t *testing.T) {
	s, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	for _, acct := range []*Account{
		{ID: "trae:first", Provider: "trae"},
		{ID: "trae:second", Provider: "trae"},
	} {
		if err := s.UpsertAccount(acct); err != nil {
			t.Fatal(err)
		}
	}
	if err := s.ReorderAccounts("trae", []string{"trae:second"}); err == nil {
		t.Fatal("expected incomplete order to be rejected")
	}
	if err := s.ReorderAccounts("trae", []string{"trae:second", "trae:second"}); err == nil {
		t.Fatal("expected duplicate order to be rejected")
	}
	if got := s.Accounts(); got[0].ID != "trae:first" || got[1].ID != "trae:second" {
		t.Fatal("invalid reorder changed account order")
	}
}

// Regression: encrypted blobs are binary and must survive the JSON
// round-trip. Go's JSON encoder replaces invalid UTF-8 bytes with U+FFFD,
// which previously corrupted secrets saved with string(blob).
func TestSecretRoundTripBinary(t *testing.T) {
	dir := t.TempDir()
	s, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	plain := []byte("credential-json-payload")
	blob := make([]byte, 64)
	if _, err := rand.Read(blob); err != nil {
		t.Fatal(err)
	}
	if err := s.SaveSecret("acct:1", plain, func(p []byte) ([]byte, error) {
		return blob, nil
	}); err != nil {
		t.Fatal(err)
	}

	// Simulate a restart: reload from disk.
	s2, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	gotBlob, err := s2.LoadSecret("acct:1", func(b []byte) ([]byte, error) { return b, nil })
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(gotBlob, blob) {
		t.Fatalf("blob corrupted across save/load: got %d bytes, want %d", len(gotBlob), len(blob))
	}
	got, err := s2.LoadSecret("acct:1", func(b []byte) ([]byte, error) {
		if !bytes.Equal(b, blob) {
			t.Fatalf("decoder received corrupted blob")
		}
		return plain, nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != string(plain) {
		t.Fatalf("plaintext mismatch")
	}
	_ = os.Remove(filepath.Join(dir, "accounts.json"))
}
