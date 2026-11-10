package app

import (
	"os"
	"path/filepath"
	"time"

	"xiaotao/xt-client/internal/modelsource"
	"xiaotao/xt-client/internal/runtimeenv"
	"xiaotao/xt-client/internal/secrets"
)

// UAT imports manually configured sources once, re-encrypting keys with its own
// keyring. Legacy platform accounts, gateway keys and runtime data are excluded.
func importManualModelSources(dst *modelsource.Store, dstKeyring *secrets.Keyring, dataDir string) error {
	if runtimeenv.Current().Name != runtimeenv.UAT {
		return nil
	}
	marker := filepath.Join(dataDir, "model-sources", "import.json")
	if _, err := os.Stat(marker); err == nil {
		return nil
	} else if !os.IsNotExist(err) {
		return err
	}
	if len(dst.List()) > 0 {
		return Save(marker, map[string]any{"version": 1, "completedAt": time.Now().Unix(), "sources": 0, "reason": "existing configuration"})
	}
	base := os.Getenv("LOCALAPPDATA")
	if base == "" {
		return nil
	}
	sourceDir := runtimeenv.DefaultModelSourceDir(base)
	if sourceDir == "" || filepath.Clean(sourceDir) == filepath.Clean(dataDir) {
		return nil
	}
	if _, err := os.Stat(filepath.Join(sourceDir, "model-sources", "sources.json")); os.IsNotExist(err) {
		return nil
	} else if err != nil {
		return err
	}
	sourceKeyring, err := secrets.OpenReadOnly(filepath.Join(sourceDir, "keys"))
	if err != nil {
		return err
	}
	count, err := dst.ImportFrom(sourceDir, sourceKeyring.Decrypt)
	if err != nil {
		return err
	}
	// An existing empty production configuration is still a completed first import.
	if err := os.MkdirAll(filepath.Dir(marker), 0700); err != nil {
		return err
	}
	return Save(marker, map[string]any{"version": 1, "completedAt": time.Now().Unix(), "source": sourceDir, "sources": count})
}
