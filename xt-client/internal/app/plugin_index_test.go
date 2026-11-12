package app

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"xiaotao/xt-client/internal/sourceplugin"
)

func TestLocalPluginIndexValidatesEntries(t *testing.T) {
	root := t.TempDir()
	writePackage(t, filepath.Join(root, "trae"), `{"id":"xiaotao.trae","version":"0.4.0","contractVersion":2,"name":"Trae 国内版","entrypoint":"provider.exe","capabilities":["chat","host-login"]}`)
	digest, err := sourceplugin.PackageDigest(filepath.Join(root, "trae"))
	if err != nil {
		t.Fatal(err)
	}
	writePackage(t, filepath.Join(root, "other"), `{"id":"example.other","version":"1.0.0","contractVersion":1,"name":"Other","entrypoint":"provider.exe","capabilities":["chat"]}`)
	index := pluginIndex{Version: 1, Plugins: []pluginIndexEntry{
		{ID: "xiaotao.trae", Name: "Trae", Version: "0.4.0", Path: "trae", Hash: digest},
		{ID: "example.mismatch", Name: "Mismatch", Version: "1.0.0", Path: "other"},
		{ID: "example.escape", Name: "Escape", Version: "1.0.0", Path: "../outside"},
		{ID: "example.missing", Name: "Missing", Version: "1.0.0", Path: "absent"},
	}}
	raw, _ := json.Marshal(index)
	if err := os.WriteFile(filepath.Join(root, "index.json"), raw, 0600); err != nil {
		t.Fatal(err)
	}
	items := scanPluginIndex([]string{root}, map[string]sourceplugin.Saved{
		"xiaotao.trae": {ID: "xiaotao.trae", Manifest: sourceplugin.Manifest{Version: "0.3.0"}},
	})
	if len(items) != 4 {
		t.Fatalf("entries: %d", len(items))
	}
	trae := items[0]
	if trae.Error != "" || !trae.HashMatches || !trae.Installed || trae.InstalledVersion != "0.3.0" || !trae.UpdateAvailable {
		t.Fatalf("unexpected trae item: %+v", trae)
	}
	for _, item := range items[1:] {
		if item.Error == "" {
			t.Fatalf("invalid entry accepted: %+v", item)
		}
	}
}

func TestLocalPluginIndexHashMismatchIsReported(t *testing.T) {
	root := t.TempDir()
	writePackage(t, filepath.Join(root, "pkg"), `{"id":"example.pkg","version":"1.0.0","contractVersion":1,"name":"Pkg","entrypoint":"provider.exe","capabilities":["chat"]}`)
	raw, _ := json.Marshal(pluginIndex{Version: 1, Plugins: []pluginIndexEntry{{ID: "example.pkg", Name: "Pkg", Version: "1.0.0", Path: "pkg", Hash: "deadbeef"}}})
	if err := os.WriteFile(filepath.Join(root, "index.json"), raw, 0600); err != nil {
		t.Fatal(err)
	}
	items := scanPluginIndex([]string{root}, nil)
	if len(items) != 1 || items[0].HashMatches {
		t.Fatalf("hash mismatch not reported: %+v", items)
	}
}

func TestLocalPluginIndexIgnoresUnknownSchema(t *testing.T) {
	root := t.TempDir()
	raw, _ := json.Marshal(pluginIndex{Version: 2, Plugins: []pluginIndexEntry{{ID: "example.pkg", Path: "pkg"}}})
	if err := os.WriteFile(filepath.Join(root, "index.json"), raw, 0600); err != nil {
		t.Fatal(err)
	}
	if items := scanPluginIndex([]string{root}, nil); len(items) != 0 {
		t.Fatalf("unknown index version accepted: %+v", items)
	}
}
