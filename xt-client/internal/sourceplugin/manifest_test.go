package sourceplugin

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestManifestBoundary(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "provider"), []byte("executable"), 0700); err != nil {
		t.Fatal(err)
	}
	valid := Manifest{ID: "example.platform", Version: "0.1.0", ContractVersion: 1, Name: "Example", Entrypoint: "provider", Capabilities: []string{"chat"}}
	tests := []struct {
		name   string
		mutate func(*Manifest)
		valid  bool
	}{
		{"valid", func(*Manifest) {}, true},
		{"version", func(m *Manifest) { m.ContractVersion = 2 }, false},
		{"identity", func(m *Manifest) { m.ID = "../../escape" }, false},
		{"escape", func(m *Manifest) { m.Entrypoint = "../provider" }, false},
		{"missing", func(m *Manifest) { m.Entrypoint = "absent" }, false},
		{"no chat", func(m *Manifest) { m.Capabilities = []string{"login"} }, false},
		{"unknown", func(m *Manifest) { m.Capabilities = []string{"chat", "execute"} }, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := valid
			tt.mutate(&m)
			raw, _ := json.Marshal(m)
			if err := os.WriteFile(filepath.Join(dir, "manifest.json"), raw, 0600); err != nil {
				t.Fatal(err)
			}
			_, _, err := ReadManifest(dir)
			if (err == nil) != tt.valid {
				t.Fatalf("valid=%v err=%v", tt.valid, err)
			}
		})
	}
}
func TestManifestSymlinkEscape(t *testing.T) {
	dir := t.TempDir()
	outside := filepath.Join(t.TempDir(), "provider")
	if err := os.WriteFile(outside, []byte("code"), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(dir, "provider")); err != nil {
		t.Skip(err)
	}
	raw := []byte(`{"id":"example.platform","version":"1","contractVersion":1,"name":"Example","entrypoint":"provider","capabilities":["chat"]}`)
	if err := os.WriteFile(filepath.Join(dir, "manifest.json"), raw, 0600); err != nil {
		t.Fatal(err)
	}
	if _, _, err := ReadManifest(dir); err == nil {
		t.Fatal("accepted escaped symlink")
	}
}
