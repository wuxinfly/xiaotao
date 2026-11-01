package storage

import (
	"bytes"
	"testing"
)

func TestSecretCompareAndSwap(t *testing.T) {
	s, _ := Open(t.TempDir())
	identity := func(b []byte) ([]byte, error) { return b, nil }
	s.UpsertAccount(&Account{ID: "a"})
	s.SaveSecret("a", []byte("old"), identity)
	revision, _ := s.SecretRevision("a")
	s.SaveSecret("a", []byte("login"), identity)
	if err := s.ReplaceSecret("a", revision, []byte("stale"), identity); err == nil {
		t.Fatal("overwrote re-login")
	}
	raw, _ := s.LoadSecret("a", identity)
	if !bytes.Equal(raw, []byte("login")) {
		t.Fatal("new login lost")
	}
	revision, _ = s.SecretRevision("a")
	s.DeleteAccount("a")
	if err := s.ReplaceSecret("a", revision, []byte("stale"), identity); err == nil {
		t.Fatal("revived deleted account")
	}
	s.UpsertAccount(&Account{ID: "a"})
	s.SaveSecret("a", []byte("current"), identity)
	revision, _ = s.SecretRevision("a")
	if err := s.ReplaceSecret("a", revision, []byte("next"), identity); err != nil {
		t.Fatal(err)
	}
	reopened, _ := Open(s.path[:len(s.path)-len("accounts.json")])
	raw, _ = reopened.LoadSecret("a", identity)
	if string(raw) != "next" {
		t.Fatal("rotation not persisted")
	}
}
