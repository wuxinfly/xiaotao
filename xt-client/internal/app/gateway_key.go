package app

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"xiaotao/xt-client/internal/secrets"
	"xiaotao/xt-client/internal/storage"
)

// Local gateway credentials stay separate from upstream API keys. The old
// gateway key can be read once without mutating any frozen platform account.
func localGatewayKey(data string, keyring *secrets.Keyring) (string, func(string) error, error) {
	path := filepath.Join(data, "keys", "gateway.json")
	save := func(key string) error {
		encrypted, err := keyring.Encrypt([]byte(key))
		if err != nil {
			return err
		}
		return Save(path, encrypted)
	}
	if raw, err := os.ReadFile(path); err == nil {
		var encrypted []byte
		if err = json.Unmarshal(raw, &encrypted); err != nil {
			return "", nil, fmt.Errorf("invalid gateway key file: %w", err)
		}
		key, err := keyring.Decrypt(encrypted)
		if err != nil {
			return "", nil, err
		}
		decoded, decodeErr := hex.DecodeString(strings.TrimPrefix(string(key), "xt_"))
		if decodeErr != nil || len(decoded) != 24 {
			return "", nil, fmt.Errorf("invalid gateway key format")
		}
		return string(key), save, nil
	} else if !os.IsNotExist(err) {
		return "", nil, err
	}
	key := legacyLocalGatewayKey(data, keyring)
	if key == "" {
		raw := make([]byte, 24)
		if _, err := rand.Read(raw); err != nil {
			return "", nil, err
		}
		key = hex.EncodeToString(raw)
	}
	if err := save(key); err != nil {
		return "", nil, err
	}
	return key, save, nil
}

func legacyLocalGatewayKey(data string, keyring *secrets.Keyring) string {
	store, err := storage.OpenReadOnly(filepath.Join(data, "accounts"))
	if err != nil {
		return ""
	}
	raw, err := store.LoadSecret("proxy", keyring.Decrypt)
	if err != nil {
		return ""
	}
	// Early versions stored random bytes; later versions stored their hex form.
	if len(raw) == 24 {
		return hex.EncodeToString(raw)
	}
	if len(raw) == 48 {
		if _, err := hex.DecodeString(string(raw)); err == nil {
			return string(raw)
		}
	}
	return ""
}
