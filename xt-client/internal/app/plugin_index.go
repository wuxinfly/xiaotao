package app

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"

	"xiaotao/xt-client/internal/sourceplugin"
)

// pluginIndexEntry is one locally curated plugin. The index is a plain file the
// user controls; nothing is downloaded automatically.
type pluginIndexEntry struct {
	ID           string   `json:"id"`
	Name         string   `json:"name"`
	Version      string   `json:"version"`
	Path         string   `json:"path"`
	Hash         string   `json:"hash,omitempty"`
	Description  string   `json:"description,omitempty"`
	Capabilities []string `json:"capabilities,omitempty"`
}

type pluginIndex struct {
	Version int                `json:"version"`
	Plugins []pluginIndexEntry `json:"plugins"`
}

type indexItem struct {
	pluginIndexEntry
	ResolvedPath     string `json:"resolvedPath"`
	HashMatches      bool   `json:"hashMatches"`
	Installed        bool   `json:"installed"`
	InstalledVersion string `json:"installedVersion,omitempty"`
	Enabled          bool   `json:"enabled"`
	UpdateAvailable  bool   `json:"updateAvailable"`
	Error            string `json:"error,omitempty"`
}

// localPluginIndex reads plugins/index.json beside the executable and under
// dist/plugins. Entries must point at a package inside the same root.
func localPluginIndex(manager *sourceplugin.Manager) []indexItem {
	installed := map[string]sourceplugin.Saved{}
	for _, entry := range manager.List() {
		installed[entry.ID] = entry
	}
	return scanPluginIndex(pluginPackageRoots(), installed)
}

func scanPluginIndex(roots []string, installed map[string]sourceplugin.Saved) []indexItem {
	out := []indexItem{}
	seen := map[string]bool{}
	for _, root := range roots {
		raw, err := os.ReadFile(filepath.Join(root, "index.json"))
		if err != nil {
			continue
		}
		var index pluginIndex
		if json.Unmarshal(raw, &index) != nil || index.Version != 1 {
			continue
		}
		for _, entry := range index.Plugins {
			if entry.ID == "" || seen[entry.ID] || len(index.Plugins) > 100 {
				continue
			}
			seen[entry.ID] = true
			item := indexItem{pluginIndexEntry: entry}
			path := entry.Path
			if !filepath.IsAbs(path) {
				path = filepath.Join(root, path)
			}
			resolved, err := filepath.Abs(path)
			rootAbs, rootErr := filepath.Abs(root)
			if err != nil || rootErr != nil || !insideDir(rootAbs, resolved) {
				item.Error = "插件路径无效"
				out = append(out, item)
				continue
			}
			item.ResolvedPath = resolved
			manifest, _, err := sourceplugin.ReadManifest(resolved)
			if err != nil {
				item.Error = "插件包无效"
				out = append(out, item)
				continue
			}
			if manifest.ID != entry.ID {
				item.Error = "插件 ID 与目录不一致"
				out = append(out, item)
				continue
			}
			item.Name, item.Version, item.Capabilities = manifest.Name, manifest.Version, manifest.Capabilities
			if digest, err := sourceplugin.PackageDigest(resolved); err == nil {
				item.HashMatches = entry.Hash == "" || strings.EqualFold(entry.Hash, digest)
				if entry.Hash == "" {
					item.Hash = digest
				}
			} else {
				item.Error = "无法校验插件包"
			}
			if current, ok := installed[entry.ID]; ok {
				item.Installed = true
				item.InstalledVersion = current.Manifest.Version
				item.Enabled = current.Enabled
				item.UpdateAvailable = compareVersion(current.Manifest.Version, manifest.Version) < 0
			}
			out = append(out, item)
		}
	}
	return out
}

func insideDir(root, path string) bool {
	rel, err := filepath.Rel(root, path)
	if err != nil {
		return false
	}
	return rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))
}
