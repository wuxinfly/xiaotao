package sourceplugin

import (
	"os"
	"path/filepath"
	"testing"
)

func TestInstallBrowserLoginCapability(t *testing.T) {
	pkg := t.TempDir()
	if err := os.WriteFile(filepath.Join(pkg, "provider.exe"), []byte("fixture"), 0600); err != nil {
		t.Fatal(err)
	}
	manifest := `{"id":"example.browser","version":"0.2.0","contractVersion":1,"name":"Browser","entrypoint":"provider.exe","capabilities":["chat","browser-login"]}`
	if err := os.WriteFile(filepath.Join(pkg, "manifest.json"), []byte(manifest), 0600); err != nil {
		t.Fatal(err)
	}
	identity := func(raw []byte) ([]byte, error) { return raw, nil }
	manager, err := Open(t.TempDir(), identity, identity)
	if err != nil {
		t.Fatal(err)
	}
	entry, err := manager.Install(pkg)
	if err != nil {
		t.Fatal(err)
	}
	if !manager.Supports(entry.ID, "browser-login") {
		t.Fatal("browser-login capability was not preserved")
	}
}
