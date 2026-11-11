// Package sourceplugin manages the model-source process boundary.
package sourceplugin

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"regexp"
)

const ContractVersion = 1
const MaxMessage = 32 << 20

// Manifest describes executable code. Reading it never starts the entrypoint.
type Manifest struct {
	ID              string   `json:"id"`
	Version         string   `json:"version"`
	ContractVersion int      `json:"contractVersion"`
	Name            string   `json:"name"`
	Entrypoint      string   `json:"entrypoint"`
	Capabilities    []string `json:"capabilities"`
}

var identifier = regexp.MustCompile(`^[a-z][a-z0-9]*(\.[a-z0-9][a-z0-9-]*)+$`)

// ReadManifest validates the physical path, including symlinks, before execution.
func ReadManifest(dir string) (Manifest, string, error) {
	var m Manifest
	raw, err := os.ReadFile(filepath.Join(dir, "manifest.json"))
	if err != nil {
		return m, "", err
	}
	if len(raw) > 64<<10 || json.Unmarshal(raw, &m) != nil {
		return m, "", errors.New("invalid plugin manifest")
	}
	if !identifier.MatchString(m.ID) || m.Version == "" || m.Name == "" || m.ContractVersion != ContractVersion {
		return m, "", errors.New("incompatible plugin identity or contract")
	}
	if m.Entrypoint == "" || filepath.IsAbs(m.Entrypoint) || filepath.VolumeName(m.Entrypoint) != "" {
		return m, "", errors.New("invalid plugin entrypoint")
	}
	root, err := filepath.EvalSymlinks(dir)
	if err != nil {
		return m, "", err
	}
	root, err = filepath.Abs(root)
	if err != nil {
		return m, "", err
	}
	entry, err := filepath.EvalSymlinks(filepath.Join(root, m.Entrypoint))
	if err != nil {
		return m, "", err
	}
	rel, err := filepath.Rel(root, entry)
	if err != nil || rel == ".." || (len(rel) > 3 && rel[:3] == ".."+string(filepath.Separator)) {
		return m, "", errors.New("plugin entrypoint escapes package")
	}
	info, err := os.Stat(entry)
	if err != nil {
		return m, "", err
	}
	if !info.Mode().IsRegular() {
		return m, "", errors.New("entrypoint must be a regular file")
	}
	chat := false
	seen := map[string]bool{}
	for _, c := range m.Capabilities {
		if seen[c] {
			return m, "", errors.New("duplicate capability")
		}
		seen[c] = true
		switch c {
		case "chat":
			chat = true
		case "login", "refresh", "models", "quota", "checkin":
		default:
			return m, "", errors.New("unknown capability")
		}
	}
	if !chat {
		return m, "", errors.New("chat capability is required")
	}
	return m, entry, nil
}
