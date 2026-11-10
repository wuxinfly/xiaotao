package modelsource

import (
	"bytes"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func testCrypto(t *testing.T) (func([]byte) ([]byte, error), func([]byte) ([]byte, error)) {
	t.Helper()
	key := make([]byte, 32)
	_, _ = rand.Read(key)
	block, _ := aes.NewCipher(key)
	gcm, _ := cipher.NewGCM(block)
	return func(b []byte) ([]byte, error) {
			nonce := make([]byte, gcm.NonceSize())
			_, _ = rand.Read(nonce)
			return gcm.Seal(nonce, nonce, b, nil), nil
		}, func(b []byte) ([]byte, error) {
			if len(b) < gcm.NonceSize() {
				return nil, errors.New("invalid ciphertext")
			}
			return gcm.Open(nil, b[:gcm.NonceSize()], b[gcm.NonceSize():], nil)
		}
}
func input() Input {
	key := "secret-test-never-expose"
	return Input{Label: "Local", Protocol: "openai-chat", BaseURL: "http://localhost:1234/v1/chat/completions/", APIKey: &key, Models: []Model{{UpstreamModelID: "same-upstream", Label: "Example", ContextWindow: 4096, MaxOutputTokens: 128, SupportsReasoning: true, ReasoningEfforts: []string{"low", "high"}}}}
}
func openTest(t *testing.T) *Store {
	t.Helper()
	enc, dec := testCrypto(t)
	s, err := Open(t.TempDir(), enc, dec)
	if err != nil {
		t.Fatal(err)
	}
	return s
}

func TestPersistenceStableIDsAndSecrecy(t *testing.T) {
	enc, dec := testCrypto(t)
	dir := t.TempDir()
	s, err := Open(dir, enc, dec)
	if err != nil {
		t.Fatal(err)
	}
	in := input()
	src, err := s.Save("", in)
	if err != nil {
		t.Fatal(err)
	}
	if src.ID == "" || src.Models[0].ID == "" || !src.HasAPIKey || !src.Enabled || src.BaseURL != "http://localhost:1234/v1" {
		t.Fatalf("unexpected source: %+v", src)
	}
	disk, err := os.ReadFile(s.path)
	if err != nil {
		t.Fatal(err)
	}
	api, _ := json.Marshal(src)
	if bytes.Contains(disk, []byte(*in.APIKey)) || bytes.Contains(api, []byte(*in.APIKey)) || bytes.Contains(api, []byte("encryptedAPIKey")) {
		t.Fatal("key leaked")
	}
	var persisted diskFile
	if err := json.Unmarshal(disk, &persisted); err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(api, persisted.Sources[0].Secret) {
		t.Fatal("ciphertext leaked")
	}
	s2, err := Open(dir, enc, dec)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(s.List(), s2.List()) {
		t.Fatal("reload changed model/source metadata")
	}
	in.Models = src.Models
	in.APIKey = nil
	in.Label = "Edited"
	updated, err := s2.Save(src.ID, in)
	if err != nil {
		t.Fatal(err)
	}
	if updated.Models[0].ID != src.Models[0].ID || updated.ID != src.ID {
		t.Fatal("IDs changed")
	}
	_, _, key, err := s2.Resolve(updated.Models[0].ID)
	if err != nil || key != "secret-test-never-expose" {
		t.Fatal("edit lost key", err)
	}
	// Omitting the ID on an unchanged upstream model also preserves its route.
	omitted := input()
	omitted.APIKey = nil
	again, err := s2.Save(src.ID, omitted)
	if err != nil || again.Models[0].ID != src.Models[0].ID {
		t.Fatal("omitted ID rotated an existing route", err)
	}
	_, resolvedModel, _, err := s2.Resolve(src.Models[0].ID)
	if err != nil {
		t.Fatal(err)
	}
	resolvedModel.ReasoningEfforts[0] = "must not mutate store"
	returned := s2.List()
	returned[0].Models[0].ReasoningEfforts[0] = "mutated"
	if s2.List()[0].Models[0].ReasoningEfforts[0] != "low" {
		t.Fatal("public snapshot aliases store")
	}
}

func TestRoutesOwnershipDisableDelete(t *testing.T) {
	s := openTest(t)
	a, err := s.Save("", input())
	if err != nil {
		t.Fatal(err)
	}
	b, err := s.Save("", input())
	if err != nil {
		t.Fatal(err)
	}
	if a.Models[0].ID == b.Models[0].ID {
		t.Fatal("global IDs collide")
	}
	for _, src := range []Source{a, b} {
		resolved, _, _, err := s.Resolve(src.Models[0].ID)
		if err != nil || resolved.ID != src.ID {
			t.Fatal("wrong route", err)
		}
	}
	bad := input()
	bad.Models = b.Models
	if _, err := s.Save(a.ID, bad); !errors.Is(err, ErrInvalid) {
		t.Fatal("foreign model ID accepted", err)
	}
	bad.Models = a.Models
	if _, err := s.Save("", bad); !errors.Is(err, ErrInvalid) {
		t.Fatal("client-created ID accepted")
	}
	off := false
	in := input()
	in.Models = a.Models
	in.Enabled = &off
	in.APIKey = nil
	if _, err := s.Save(a.ID, in); err != nil {
		t.Fatal(err)
	}
	for _, id := range []string{a.Models[0].ID, "same-upstream", "trae/same-upstream"} {
		if _, _, _, err := s.Resolve(id); !errors.Is(err, ErrNotFound) {
			t.Fatal("unavailable route accepted", id)
		}
	}
	if err := s.Delete(b.ID); err != nil {
		t.Fatal(err)
	}
	if _, _, _, err := s.Resolve(b.Models[0].ID); !errors.Is(err, ErrNotFound) {
		t.Fatal("deleted route remains")
	}
	if err := s.Delete(b.ID); !errors.Is(err, ErrNotFound) {
		t.Fatal("missing deletion not classified")
	}
}

func TestValidationAndFailedWritesAreAtomic(t *testing.T) {
	s := openTest(t)
	src, err := s.Save("", input())
	if err != nil {
		t.Fatal(err)
	}
	before, _ := os.ReadFile(s.path)
	cases := []func(*Input){
		func(i *Input) { i.BaseURL = "http://user:password@localhost/v1" }, func(i *Input) { i.BaseURL = "ftp://localhost" }, func(i *Input) { i.BaseURL = "https://example.com/v1?q=x" }, func(i *Input) { i.BaseURL = "https://example.com/#fragment" },
		func(i *Input) { i.Models[0].ContextWindow = 1023 }, func(i *Input) { i.Models[0].MaxOutputTokens = 15 }, func(i *Input) { i.Models[0].MaxOutputTokens = 4096 }, func(i *Input) { i.Models = append(i.Models, i.Models[0]) }, func(i *Input) { empty := ""; i.APIKey = &empty },
	}
	for n, change := range cases {
		in := input()
		in.Models = src.Models
		in.Models = append([]Model(nil), in.Models...)
		change(&in)
		if _, err := s.Save(src.ID, in); !errors.Is(err, ErrInvalid) {
			t.Fatalf("case %d accepted: %v", n, err)
		}
		after, _ := os.ReadFile(s.path)
		if !bytes.Equal(before, after) {
			t.Fatal("failed validation changed disk")
		}
	}
	originalEncrypt := s.encrypt
	s.encrypt = func([]byte) ([]byte, error) { return nil, errors.New("secret-test-never-expose") }
	if _, err := s.Save(src.ID, input()); err == nil || bytes.Contains([]byte(err.Error()), []byte("secret-test-never-expose")) {
		t.Fatal("encryption failure leaked key or succeeded")
	}
	s.encrypt = originalEncrypt
	after, _ := os.ReadFile(s.path)
	if !bytes.Equal(before, after) {
		t.Fatal("failed encryption changed disk")
	}
	// A directory at the destination forces the final rename to fail on all OSes.
	backup := s.path + ".original"
	if err := os.Rename(s.path, backup); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(s.path, 0700); err != nil {
		t.Fatal(err)
	}
	in := input()
	in.Models = src.Models
	in.Label = "must not commit"
	if _, err := s.Save(src.ID, in); err == nil {
		t.Fatal("write unexpectedly succeeded")
	}
	if s.List()[0].Label != src.Label {
		t.Fatal("failed write changed memory")
	}
	if err := s.Delete(src.ID); err == nil {
		t.Fatal("delete unexpectedly succeeded")
	}
	if len(s.List()) != 1 {
		t.Fatal("failed delete changed memory")
	}
	_ = os.Remove(s.path)
	_ = os.Rename(backup, s.path)
	after, _ = os.ReadFile(s.path)
	if !bytes.Equal(before, after) {
		t.Fatal("failed commit changed original file")
	}
}

func TestCorruptFileIsNeverReplaced(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "model-sources", "sources.json")
	_ = os.MkdirAll(filepath.Dir(path), 0700)
	bad := []byte(`{"version":1,"sources":[broken`)
	_ = os.WriteFile(path, bad, 0600)
	enc, dec := testCrypto(t)
	if _, err := Open(dir, enc, dec); err == nil {
		t.Fatal("corrupt file accepted")
	}
	after, _ := os.ReadFile(path)
	if !bytes.Equal(bad, after) {
		t.Fatal("bad file overwritten")
	}
	s := openTest(t)
	_, _ = s.Save("", input())
	raw, _ := os.ReadFile(s.path)
	var disk diskFile
	_ = json.Unmarshal(raw, &disk)
	disk.Sources[0].Secret = []byte("broken ciphertext")
	raw, _ = json.Marshal(disk)
	_ = os.WriteFile(s.path, raw, 0600)
	if _, err := Open(filepath.Dir(filepath.Dir(s.path)), s.encrypt, s.decrypt); err == nil {
		t.Fatal("bad ciphertext accepted")
	}
}

func TestURLNormalization(t *testing.T) {
	for raw, want := range map[string]string{"https://api.example/v1": "https://api.example/v1/chat/completions", "https://api.example/v1/": "https://api.example/v1/chat/completions", "http://localhost:8080/": "http://localhost:8080/chat/completions", "https://api.example/v1/chat/completions": "https://api.example/v1/chat/completions"} {
		base, err := NormalizeURL(raw)
		if err != nil || ChatURL(base) != want {
			t.Fatalf("%q -> %q (%v)", raw, ChatURL(base), err)
		}
	}
}
