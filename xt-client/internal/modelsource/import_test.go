package modelsource

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestImportReencryptsAndPreservesIDsWithoutTouchingSource(t *testing.T) {
	sourceDir := t.TempDir()
	enc, dec := testCrypto(t)
	source, _ := Open(sourceDir, enc, dec)
	src, err := source.Save("", input())
	if err != nil {
		t.Fatal(err)
	}
	before, _ := os.ReadFile(source.path)
	target := openTest(t)
	count, err := target.ImportFrom(sourceDir, dec)
	if err != nil || count != 1 {
		t.Fatal(count, err)
	}
	got := target.List()[0]
	if got.ID != src.ID || got.Models[0].ID != src.Models[0].ID {
		t.Fatal("import changed IDs")
	}
	_, _, key, err := target.Resolve(got.Models[0].ID)
	if err != nil || key != *input().APIKey {
		t.Fatal("target key not readable", err)
	}
	after, _ := os.ReadFile(source.path)
	if !bytes.Equal(before, after) {
		t.Fatal("source modified")
	}
	if bytes.Equal(source.entries[0].Secret, target.entries[0].Secret) {
		t.Fatal("ciphertext copied without reencryption")
	}
	if _, err := dec(target.entries[0].Secret); err == nil {
		t.Fatal("target still uses source encryption key")
	}
	count, err = target.ImportFrom(sourceDir, func([]byte) ([]byte, error) { t.Fatal("nonempty target read source"); return nil, nil })
	if err != nil || count != 0 {
		t.Fatal(count, err)
	}
	// Only the model-source file is created in the target environment.
	entries, _ := os.ReadDir(filepath.Dir(filepath.Dir(target.path)))
	if len(entries) != 1 || entries[0].Name() != "model-sources" {
		t.Fatal("unrelated runtime files imported")
	}
}

func TestImportMissingAndFailuresLeaveTargetEmpty(t *testing.T) {
	target := openTest(t)
	_, dec := testCrypto(t)
	if count, err := target.ImportFrom(t.TempDir(), dec); err != nil || count != 0 {
		t.Fatal(count, err)
	}
	sourceDir := t.TempDir()
	enc, sourceDec := testCrypto(t)
	source, _ := Open(sourceDir, enc, sourceDec)
	_, _ = source.Save("", input())
	if _, err := target.ImportFrom(sourceDir, dec); err == nil {
		t.Fatal("bad source decryption accepted")
	}
	original := target.encrypt
	target.encrypt = func([]byte) ([]byte, error) { return nil, errors.New("encryption failure") }
	if _, err := target.ImportFrom(sourceDir, sourceDec); err == nil {
		t.Fatal("encryption failure accepted")
	}
	target.encrypt = original
	if len(target.List()) != 0 {
		t.Fatal("failed import changed target memory")
	}
	if _, err := os.Stat(target.path); !os.IsNotExist(err) {
		t.Fatal("failed import created target file")
	}
	_ = os.WriteFile(source.path, []byte("damaged"), 0600)
	if _, err := target.ImportFrom(sourceDir, sourceDec); err == nil {
		t.Fatal("corrupt source accepted")
	}
}
