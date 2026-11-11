package sourceplugin

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestAccountOrderSurvivesPluginUpdate(t *testing.T) {
	dir := t.TempDir()
	crypt := func(value []byte) ([]byte, error) { return append([]byte(nil), value...), nil }
	m, err := Open(dir, crypt, crypt)
	if err != nil {
		t.Fatal(err)
	}
	const id = "test.accounts"
	pkg := t.TempDir()
	if err := os.WriteFile(filepath.Join(pkg, "provider.exe"), []byte("fixture"), 0600); err != nil {
		t.Fatal(err)
	}
	manifest := Manifest{ID: id, Version: "1", ContractVersion: ContractVersion, Name: "Test", Entrypoint: "provider.exe", Capabilities: []string{"chat"}}
	write := func() {
		raw, _ := json.Marshal(manifest)
		if err := os.WriteFile(filepath.Join(pkg, "manifest.json"), raw, 0600); err != nil {
			t.Fatal(err)
		}
	}
	write()
	if _, err := m.Install(pkg); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"a", "b", "a"} {
		_, err := m.applyAccountResult(&m.entries[0], PluginAccount{}, Result{Account: &AccountInfo{ID: name, Name: name}, Credential: json.RawMessage(`{"fixture":true}`)}, nil, true)
		if err != nil {
			t.Fatal(err)
		}
	}
	before := m.List()[0].Accounts
	if len(before) != 2 || before[0].Secret != nil || before[1].Secret != nil {
		t.Fatal("duplicate account or exposed secret")
	}
	if err := m.ReorderAccounts(id, []string{before[1].ID, before[0].ID}); err != nil {
		t.Fatal(err)
	}
	manifest.Version = "2"
	write()
	if _, err := m.Update(pkg); err != nil {
		t.Fatal(err)
	}
	m, err = Open(dir, crypt, crypt)
	if err != nil {
		t.Fatal(err)
	}
	if m.PrimaryAccount(id) != before[1].ID || m.List()[0].Manifest.Version != "2" || len(m.entries[0].Accounts[0].Secret) == 0 {
		t.Fatal("update or restart lost preferred account or credential")
	}
}
