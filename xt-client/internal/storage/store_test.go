package storage

import (
	"bytes"
	"crypto/rand"
	"os"
	"path/filepath"
	"testing"
)

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
