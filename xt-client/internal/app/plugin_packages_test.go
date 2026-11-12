package app

import (
	"os"
	"path/filepath"
	"testing"

	"xiaotao/xt-client/internal/sourceplugin"
)

func TestScanPluginPackages(t *testing.T) {
	root := t.TempDir()
	writePackage(t, filepath.Join(root, "trae"), `{"id":"xiaotao.trae","version":"0.2.0","contractVersion":1,"name":"Trae 国内版","entrypoint":"provider.exe","capabilities":["chat","browser-login"]}`)
	writePackage(t, filepath.Join(root, "other"), `{"id":"example.other","version":"1.0.0","contractVersion":1,"name":"Other","entrypoint":"provider.exe","capabilities":["chat"]}`)
	writePackage(t, filepath.Join(root, "broken"), `{"id":"example.broken","version":"1.0.0","contractVersion":1,"name":"Broken","entrypoint":"missing.exe","capabilities":["chat"]}`)
	installed := map[string]sourceplugin.Saved{
		"xiaotao.trae": {ID: "xiaotao.trae", Manifest: sourceplugin.Manifest{Version: "0.1.0"}},
	}
	packages := scanPluginPackages([]string{root}, installed)
	if len(packages) != 2 {
		t.Fatalf("expected two valid packages, got %d", len(packages))
	}
	if packages[0].Manifest.ID != "example.other" || packages[0].UpdateAvailable {
		t.Fatalf("unexpected first package: %+v", packages[0])
	}
	if packages[1].Manifest.ID != "xiaotao.trae" || !packages[1].UpdateAvailable || packages[1].InstalledVersion != "0.1.0" {
		t.Fatalf("unexpected Trae package: %+v", packages[1])
	}
	if len(packages[0].Hash) != 64 || len(packages[1].Hash) != 64 {
		t.Fatalf("package hash missing: %+v", packages)
	}
}

func TestCompareVersion(t *testing.T) {
	tests := []struct {
		left, right string
		want        int
	}{
		{"0.1.0", "0.2.0", -1},
		{"0.2.0", "0.2.0", 0},
		{"1.0", "1.0.0", -1},
		{"1.10.0", "1.9.0", 1},
	}
	for _, tt := range tests {
		if got := compareVersion(tt.left, tt.right); got != tt.want {
			t.Fatalf("compareVersion(%q, %q) = %d, want %d", tt.left, tt.right, got, tt.want)
		}
	}
}

func writePackage(t *testing.T, dir, manifest string) {
	t.Helper()
	if err := os.MkdirAll(dir, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "provider.exe"), []byte("fixture"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "manifest.json"), []byte(manifest), 0600); err != nil {
		t.Fatal(err)
	}
}
