package codex

import (
	"context"
	"fmt"
	"time"

	"xiaotao/xt-client/internal/providers"
)

func Refresh(ctx context.Context, cred *providers.Credential) (*providers.Credential, error) {
	if cred == nil || cred.RefreshToken == "" {
		return nil, fmt.Errorf("missing refresh token")
	}
	tokenResp, err := RefreshToken(ctx, cred.RefreshToken)
	if err != nil {
		return nil, fmt.Errorf("Codex token 刷新失败: %w", err)
	}

	next := *cred
	next.AccessToken = tokenResp.AccessToken
	if tokenResp.RefreshToken != "" {
		next.RefreshToken = tokenResp.RefreshToken
	}
	next.ExpiresAt = time.Now().Unix() + int64(tokenResp.ExpiresIn)

	claims := DecodeIDTokenClaims(tokenResp.IDToken)
	if accountID := AccountIDFromClaims(claims); accountID != "" {
		next.EnterpriseID = accountID
	}
	if email := EmailFromClaims(claims); email != "" {
		if next.Nickname == "" || next.Nickname == next.UID {
			next.Nickname = email
		}
	}
	if plan := PlanTypeFromClaims(claims); plan != "" {
		next.AuthTier = plan
	}

	return &next, nil
}
