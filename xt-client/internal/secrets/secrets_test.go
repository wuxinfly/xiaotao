package secrets

import (
	"os"
	"path/filepath"
	"testing"
)

func TestKeyringAcrossInstances(t *testing.T) {
	dir := t.TempDir()
	k1, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	enc, err := k1.Encrypt([]byte("payload"))
	if err != nil {
		t.Fatal(err)
	}
	k2, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	dec, err := k2.Decrypt(enc)
	if err != nil {
		t.Fatal(err)
	}
	if string(dec) != "payload" {
		t.Fatalf("unexpected payload %q", dec)
	}
}

func TestOpenReadOnlyDoesNotCreateKey(t *testing.T) {
	dir := t.TempDir()
	if _, err := OpenReadOnly(dir); err == nil {
		t.Fatal("expected missing keyring error")
	}
	if _, err := os.Stat(filepath.Join(dir, "secret.key")); !os.IsNotExist(err) {
		t.Fatal("read-only open created secret.key")
	}
}
