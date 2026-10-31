package proxy

import (
	"encoding/json"
	"strings"

	"xiaotao/xt-client/internal/providers"
	"xiaotao/xt-client/internal/providers/trae"
	"xiaotao/xt-client/internal/storage"
)

func traeFetchQuota(cred *providers.Credential, machineID, deviceID string) (*storage.QuotaSnapshot, error) {
	return trae.FetchQuota(cred, machineID, deviceID)
}

func traeRefresh(refresh, region string) (*providers.Credential, error) {
	return trae.Exchange(region, refresh)
}

func jsonUnmarshal(b []byte, v any) error { return json.Unmarshal(b, v) }

func jsonMarshal(v any) ([]byte, error) { return json.Marshal(v) }

func encryptFn(plain []byte) ([]byte, error) {
	return defaultEncrypt(plain)
}

func redactText(s string) string {
	low := strings.ToLower(s)
	for _, key := range []string{"token", "credential", "userjwt", "authorization"} {
		if i := strings.Index(low, key); i >= 0 {
			return s[:i] + key + "=<redacted>"
		}
	}
	return s
}
