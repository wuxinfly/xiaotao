package app

import (
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"xiaotao/xt-client/internal/sourceplugin"
)

type localPluginPackage struct {
	Path             string                `json:"path"`
	Manifest         sourceplugin.Manifest `json:"manifest"`
	InstalledVersion string                `json:"installedVersion,omitempty"`
	UpdateAvailable  bool                  `json:"updateAvailable"`
	Enabled          bool                  `json:"enabled"`
}

// localPluginPackages exposes plugin packages shipped beside the executable or
// produced under dist/plugins during local development. Reading a manifest
// never executes plugin code.
func localPluginPackages(manager *sourceplugin.Manager) []localPluginPackage {
	installed := map[string]sourceplugin.Saved{}
	for _, entry := range manager.List() {
		installed[entry.ID] = entry
	}
	return scanPluginPackages(pluginPackageRoots(), installed)
}

func scanPluginPackages(roots []string, installed map[string]sourceplugin.Saved) []localPluginPackage {
	out := []localPluginPackage{}
	seen := map[string]bool{}
	for _, root := range roots {
		dirs, err := os.ReadDir(root)
		if err != nil {
			continue
		}
		for _, dir := range dirs {
			if !dir.IsDir() || dir.Type()&os.ModeSymlink != 0 {
				continue
			}
			path := filepath.Join(root, dir.Name())
			key := strings.ToLower(filepath.Clean(path))
			if seen[key] {
				continue
			}
			seen[key] = true
			manifest, _, err := sourceplugin.ReadManifest(path)
			if err != nil {
				continue
			}
			item := localPluginPackage{Path: path, Manifest: manifest}
			if entry, ok := installed[manifest.ID]; ok {
				item.InstalledVersion = entry.Manifest.Version
				item.Enabled = entry.Enabled
				item.UpdateAvailable = compareVersion(entry.Manifest.Version, manifest.Version) < 0
			}
			out = append(out, item)
		}
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Manifest.Name != out[j].Manifest.Name {
			return out[i].Manifest.Name < out[j].Manifest.Name
		}
		return compareVersion(out[i].Manifest.Version, out[j].Manifest.Version) > 0
	})
	return out
}

func pluginPackageRoots() []string {
	roots := []string{}
	if exe, err := os.Executable(); err == nil {
		roots = append(roots, filepath.Join(filepath.Dir(exe), "plugins"))
	}
	if cwd, err := os.Getwd(); err == nil {
		roots = append(roots, filepath.Join(cwd, "dist", "plugins"))
	}
	return roots
}

func compareVersion(left, right string) int {
	a, b := strings.Split(left, "."), strings.Split(right, ".")
	for i := 0; i < len(a) || i < len(b); i++ {
		x, y := 0, 0
		if i < len(a) {
			x, _ = strconv.Atoi(a[i])
		}
		if i < len(b) {
			y, _ = strconv.Atoi(b[i])
		}
		if x != y {
			if x < y {
				return -1
			}
			return 1
		}
	}
	return strings.Compare(left, right)
}
