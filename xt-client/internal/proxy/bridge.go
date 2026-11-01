package proxy

import (
	"context"
	"encoding/json"
	"strings"

	"xiaotao/xt-client/internal/providers"
	"xiaotao/xt-client/internal/providers/trae"
	"xiaotao/xt-client/internal/providers/workbuddy"
	"xiaotao/xt-client/internal/storage"
)

func traeFetchQuota(cred *providers.Credential, machineID, deviceID string) (*storage.QuotaSnapshot, error) {
	return trae.FetchQuota(cred, machineID, deviceID)
}

func traeRefresh(refresh, region string) (*providers.Credential, error) {
	return trae.Exchange(region, refresh)
}

func providerRefresh(ctx context.Context, provider string, cred *providers.Credential) (*providers.Credential, error) {
	if provider == "workbuddy" {
		return workbuddy.Refresh(ctx, "", cred)
	}
	return traeRefresh(cred.RefreshToken, cred.Region)
}

func providerQuota(ctx context.Context, provider string, cred *providers.Credential) (*storage.QuotaSnapshot, error) {
	if provider == "workbuddy" {
		return workbuddy.FetchQuota(ctx, "", cred)
	}
	return traeFetchQuota(cred, cred.MachineID, cred.DeviceID)
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
