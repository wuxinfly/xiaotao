package trae

import (
	"context"

	"xiaotao/xt-client/internal/providers"
)

func Exchange(region, refreshToken string) (*providers.Credential, error) {
	return exchangeToken(refreshToken, region)
}

func UserInfo(cred *providers.Credential) (string, string, error) {
	return userInfo(cred.AccessToken, cred.Region)
}

func JWTUserID(token string) string { return decodeJWTUserID(token) }

func CollectChat(ctx context.Context, cred *providers.Credential, machineID, deviceID, model string, body map[string]any) (*ChatResult, error) {
	return collectChat(ctx, cred, machineID, deviceID, model, body)
}
