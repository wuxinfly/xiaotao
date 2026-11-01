package app

import (
	"encoding/json"
	"os"
	"path/filepath"
	"time"

	"xiaotao/xt-client/internal/runtimeenv"
	"xiaotao/xt-client/internal/secrets"
	"xiaotao/xt-client/internal/storage"
)

const modelAccountImportVersion = 1

type modelAccountImportState struct {
	Version     int    `json:"version"`
	CompletedAt int64  `json:"completedAt"`
	Source      string `json:"source"`
	Accounts    int    `json:"accounts"`
}

// importModelAccounts copies production model accounts into UAT once. UAT
// owns its copy afterwards, so test account changes never write production.
func importModelAccounts(dst *storage.Store, dstKeyring *secrets.Keyring, dataDir string) error {
	if runtimeenv.Current().Name != runtimeenv.UAT {
		return nil
	}
	statePath := filepath.Join(dataDir, "accounts", "import.json")
	if _, err := os.Stat(statePath); err == nil {
		return nil
	} else if !os.IsNotExist(err) {
		return err
	}
	base := os.Getenv("LOCALAPPDATA")
	if base == "" {
		return nil
	}
	sourceDir := runtimeenv.DefaultModelSourceDir(base)
	if sourceDir == "" {
		return nil
	}
	source, err := storage.OpenReadOnly(filepath.Join(sourceDir, "accounts"))
	if err != nil {
		return nil
	}
	sourceKeyring, err := secrets.OpenReadOnly(filepath.Join(sourceDir, "keys"))
	if err != nil {
		return nil
	}
	accounts, plainSecrets := source.Export(sourceKeyring.Decrypt)
	if len(accounts) == 0 {
		return nil
	}
	for _, acct := range accounts {
		acct.UsageTokens = 0
		acct.UsageCalls = 0
		acct.Quota = nil
	}
	if err := dst.Bootstrap(accounts, plainSecrets, dstKeyring.Encrypt); err != nil {
		return err
	}
	if len(dst.Accounts()) == 0 {
		return nil
	}
	state := modelAccountImportState{
		Version:     modelAccountImportVersion,
		CompletedAt: time.Now().Unix(),
		Source:      sourceDir,
		Accounts:    len(accounts),
	}
	raw, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(statePath), ".import-*")
	if err != nil {
		return err
	}
	name := tmp.Name()
	defer os.Remove(name)
	if _, err = tmp.Write(raw); err != nil {
		tmp.Close()
		return err
	}
	if err = tmp.Close(); err != nil {
		return err
	}
	return os.Rename(name, statePath)
}
