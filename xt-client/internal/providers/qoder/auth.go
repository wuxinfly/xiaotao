package qoder

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
	"xiaotao/xt-client/internal/providers"
)

// DeviceFlow keeps the verifier exclusively in the backend.
type DeviceFlow struct {
	AuthURL                    string
	verifier, nonce, machineID string
	region                     Region
}

type AuthClient struct {
	HTTP         *http.Client
	PollInterval time.Duration
}

func StartDeviceFlow(region, mid string) (DeviceFlow, error) {
	reg, ok := Regions[region]
	if !ok {
		return DeviceFlow{}, fmt.Errorf("unsupported Qoder region")
	}
	verifier, challenge, err := PKCEPair()
	if err != nil {
		return DeviceFlow{}, err
	}
	if mid == "" {
		mid, err = machineID()
		if err != nil {
			return DeviceFlow{}, err
		}
	}
	nonce := newUUIDString()
	q := url.Values{"challenge": {challenge}, "challenge_method": {"S256"}, "nonce": {nonce}, "machine_id": {mid}, "client_id": {ClientID}}
	return DeviceFlow{AuthURL: reg.AuthURL() + "?" + q.Encode(), verifier: verifier, nonce: nonce, machineID: mid, region: reg}, nil
}

func (c AuthClient) request(ctx context.Context, method, endpoint, token string, body any) (map[string]any, int, error) {
	var reader io.Reader
	if body != nil {
		raw, err := json.Marshal(body)
		if err != nil {
			return nil, 0, err
		}
		reader = bytes.NewReader(raw)
	}
	req, err := http.NewRequestWithContext(ctx, method, endpoint, reader)
	if err != nil {
		return nil, 0, fmt.Errorf("cannot create Qoder request")
	}
	req.Header.Set("Accept", "application/json")
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	client := c.HTTP
	if client == nil {
		client = &http.Client{Timeout: 30 * time.Second}
	}
	resp, err := client.Do(req)
	if err != nil {
		if ctx.Err() != nil {
			return nil, 0, ctx.Err()
		}
		// URL errors can contain the private verifier query string.
		return nil, 0, fmt.Errorf("Qoder request failed")
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, resp.StatusCode, nil
	}
	var result map[string]any
	if err := json.NewDecoder(io.LimitReader(resp.Body, 2<<20)).Decode(&result); err != nil {
		return nil, resp.StatusCode, fmt.Errorf("invalid Qoder JSON response")
	}
	return result, resp.StatusCode, nil
}

func (c AuthClient) Wait(ctx context.Context, flow DeviceFlow) (*providers.Credential, error) {
	ctx, cancel := context.WithTimeout(ctx, 10*time.Minute)
	defer cancel()
	interval := c.PollInterval
	if interval <= 0 {
		interval = 2 * time.Second
	}
	endpoint := flow.region.DevicePollURL() + "?" + url.Values{"nonce": {flow.nonce}, "verifier": {flow.verifier}, "challenge_method": {"S256"}}.Encode()
	for {
		data, status, err := c.request(ctx, http.MethodGet, endpoint, "", nil)
		if err != nil {
			return nil, err
		}
		if status == http.StatusOK {
			cred, err := parseToken(data, &providers.Credential{Region: flow.region.Key, MachineID: flow.machineID})
			if err != nil {
				return nil, err
			}
			info, code, err := c.request(ctx, http.MethodGet, flow.region.UserInfoURL(), cred.AccessToken, nil)
			if err != nil {
				return nil, err
			}
			if code != http.StatusOK {
				return nil, &providers.UpstreamError{Status: code, Message: "Qoder userinfo failed"}
			}
			if uid := stringField(info, "user_id", "uid", "id", "sub"); uid != "" {
				cred.UID = uid
			}
			cred.Nickname = stringField(info, "name", "username", "email")
			if cred.UID == "" {
				return nil, fmt.Errorf("Qoder login response missing account identity")
			}
			return cred, nil
		}
		if status != http.StatusNotFound {
			return nil, &providers.UpstreamError{Status: status, Message: "Qoder authorization failed"}
		}
		timer := time.NewTimer(interval)
		select {
		case <-ctx.Done():
			timer.Stop()
			return nil, ctx.Err()
		case <-timer.C:
		}
	}
}

func Refresh(ctx context.Context, cred *providers.Credential) (*providers.Credential, error) {
	reg, ok := Regions[cred.Region]
	if !ok {
		return nil, fmt.Errorf("unsupported Qoder region")
	}
	return (AuthClient{}).refresh(ctx, reg, cred)
}

func (c AuthClient) refresh(ctx context.Context, reg Region, cred *providers.Credential) (*providers.Credential, error) {
	if cred.RefreshToken == "" {
		return nil, fmt.Errorf("Qoder login must be renewed")
	}
	data, status, err := c.request(ctx, http.MethodPost, reg.DeviceRefreshURL(), "", map[string]string{"refresh_token": cred.RefreshToken})
	if err != nil {
		return nil, err
	}
	if status != http.StatusOK {
		return nil, &providers.UpstreamError{Status: status, Message: "Qoder credential refresh failed"}
	}
	return parseToken(data, cred)
}

func parseToken(data map[string]any, base *providers.Credential) (*providers.Credential, error) {
	next := *base
	next.AccessToken = stringField(data, "token", "device_token", "access_token")
	if next.AccessToken == "" {
		return nil, fmt.Errorf("Qoder response missing access credential")
	}
	if value := stringField(data, "refresh_token", "refreshToken"); value != "" {
		next.RefreshToken = value
	}
	if value := stringField(data, "user_id", "uid"); value != "" {
		next.UID = value
	}
	if value := stringField(data, "machine_id"); value != "" {
		next.MachineID = value
	}
	if value := stringField(data, "name"); value != "" {
		next.Nickname = value
	}
	next.ExpiresAt = parseExpiry(data["expires_at"])
	if next.ExpiresAt == 0 {
		next.ExpiresAt = parseExpiry(data["expiresAt"])
	}
	if next.ExpiresAt == 0 {
		if seconds, ok := data["expires_in"].(float64); ok && seconds > 0 {
			next.ExpiresAt = time.Now().Unix() + int64(seconds)
		}
	}
	if next.ExpiresAt == 0 {
		next.ExpiresAt = time.Now().Add(30 * 24 * time.Hour).Unix()
	}
	return &next, nil
}

func parseExpiry(value any) int64 {
	var n float64
	switch v := value.(type) {
	case float64:
		n = v
	case string:
		if stamp, err := time.Parse(time.RFC3339Nano, v); err == nil {
			return stamp.Unix()
		}
		n, _ = strconv.ParseFloat(strings.TrimSpace(v), 64)
	}
	if n >= 1e11 {
		n /= 1000
	}
	if n <= 0 {
		return 0
	}
	return int64(n)
}

func stringField(data map[string]any, keys ...string) string {
	for _, key := range keys {
		switch value := data[key].(type) {
		case string:
			if strings.TrimSpace(value) != "" {
				return value
			}
		case float64:
			return strconv.FormatFloat(value, 'f', -1, 64)
		}
	}
	return ""
}
