package secrets

import "testing"

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
