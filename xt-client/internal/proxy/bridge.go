package proxy

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"xiaotao/xt-client/internal/providers"
	"xiaotao/xt-client/internal/providers/antigravity"
	"xiaotao/xt-client/internal/providers/codex"
	"xiaotao/xt-client/internal/providers/qoder"
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
	switch provider {
	case "qoder":
		return qoder.Refresh(ctx, cred)
	case "antigravity":
		return antigravity.Refresh(ctx, cred)
	case "codex":
		return codex.Refresh(ctx, cred)
	case "workbuddy":
		return workbuddy.Refresh(ctx, "", cred)
	case "trae":
		return traeRefresh(cred.RefreshToken, cred.Region)
	}
	if p, err := providers.Get(provider); err == nil {
		if refresher, ok := p.(providers.Refresher); ok {
			return refresher.Refresh(ctx, cred)
		}
	}
	return nil, fmt.Errorf("unknown provider %q", provider)
}

func providerQuota(ctx context.Context, provider string, cred *providers.Credential) (*storage.QuotaSnapshot, error) {
	switch provider {
	case "qoder":
		return qoder.FetchQuota(ctx, cred)
	case "antigravity":
		return antigravity.FetchQuota(ctx, cred)
	case "codex":
		return codex.FetchQuota(ctx, cred)
	case "workbuddy":
		return workbuddy.FetchQuota(ctx, "", cred)
	case "trae":
		return traeFetchQuota(cred, cred.MachineID, cred.DeviceID)
	}
	return nil, fmt.Errorf("unknown provider %q", provider)
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
