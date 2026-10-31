package trae

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"xiaotao/xt-client/internal/providers"
)

const (
	clientID      = "en1oxy7wnw8j9n"
	appVersion    = "0.2.0"
	appVerCode    = "20260901"
	ideVerCode    = "20260906"
	xAppID        = "6eefa01c-1036-4c7e-9ca5-d891f63bfcd8"
	chatEndpoint  = "/api/agent/v3/llm_utils_chat"
	defaultRegion = "cn"
)

type Region struct {
	Key         string
	ChatBase    string
	UGBase      string
	UGVersion   string
	OAuthAPI    string
	AuthHost    string
	PackageType string
	Billing     string
}

var regions = map[string]Region{
	defaultRegion: {
		Key:         defaultRegion,
		ChatBase:    "https://trae-api-cn.mchost.guru",
		UGBase:      "https://api.trae.cn",
		UGVersion:   "v2",
		OAuthAPI:    "https://api.trae.com.cn",
		AuthHost:    "www.trae.cn",
		PackageType: "stable_cn",
		Billing:     "credit",
	},
}

func RegionOf(key string) Region {
	if r, ok := regions[strings.ToLower(key)]; ok {
		return r
	}
	return regions[defaultRegion]
}

func LoginURL(region string, port int, nonce string) string {
	r := RegionOf(region)
	q := url.Values{
		"login_version":     {"1"},
		"auth_from":         {"solo"},
		"login_channel":     {"native_ide"},
		"plugin_version":    {"trae-handoff-1.0"},
		"auth_type":         {"local"},
		"client_id":         {clientID},
		"redirect":          {"0"},
		"login_trace_id":    {nonce},
		"auth_callback_url": {fmt.Sprintf("http://127.0.0.1:%d/authorize", port)},
	}
	return "https://" + r.AuthHost + "/authorization?" + q.Encode()
}

func ExtractRefreshToken(callbackURL string) string {
	u, err := url.Parse(callbackURL)
	if err != nil {
		return ""
	}
	q := u.Query()
	if cred := q.Get("credential"); cred != "" {
		if token := tokenFromBlob(cred); token != "" {
			return token
		}
	}
	if jwt := q.Get("userJwt"); jwt != "" {
		var v struct {
			RefreshToken string `json:"RefreshToken"`
		}
		if json.Unmarshal([]byte(jwt), &v) == nil && v.RefreshToken != "" {
			return v.RefreshToken
		}
	}
	return q.Get("refreshToken")
}

func tokenFromBlob(blob string) string {
	if decoded, err := url.QueryUnescape(blob); err == nil {
		blob = decoded
	}
	var obj map[string]any
	if json.Unmarshal([]byte(blob), &obj) == nil {
		if jwt, ok := obj["userJwt"].(map[string]any); ok {
			if rt, _ := jwt["RefreshToken"].(string); rt != "" {
				return rt
			}
		}
		if rt, _ := obj["refreshToken"].(string); rt != "" {
			return rt
		}
	}
	if vals, err := url.ParseQuery(blob); err == nil {
		if rt := vals.Get("refreshToken"); rt != "" {
			return rt
		}
		if jwt := vals.Get("userJwt"); jwt != "" {
			var v struct {
				RefreshToken string `json:"RefreshToken"`
			}
			if json.Unmarshal([]byte(jwt), &v) == nil {
				return v.RefreshToken
			}
		}
	}
	return ""
}

type tokenResult struct {
	Result struct {
		Token          string `json:"Token"`
		RefreshToken   string `json:"RefreshToken"`
		TokenExpireAt  int64  `json:"TokenExpireAt"`
		TokenExpireDur int64  `json:"TokenExpireDuration"`
	} `json:"Result"`
}

func exchangeToken(refreshToken, region string) (*providers.Credential, error) {
	r := RegionOf(region)
	body, _ := json.Marshal(map[string]any{
		"ClientID":     clientID,
		"RefreshToken": refreshToken,
		"ClientSecret": "-",
		"UserID":       "",
	})
	var out tokenResult
	if err := postJSON(r.OAuthAPI+"/cloudide/api/v3/trae/oauth/ExchangeToken", body, map[string]string{
		"Content-Type": "application/json",
		"User-Agent":   "Trae/" + appVersion,
	}, &out); err != nil {
		return nil, err
	}
	if out.Result.Token == "" {
		return nil, fmt.Errorf("token exchange rejected")
	}
	refresh := out.Result.RefreshToken
	if refresh == "" {
		refresh = refreshToken
	}
	expires := out.Result.TokenExpireAt
	if expires > 1e12 {
		expires /= 1000
	}
	if expires <= time.Now().Unix() {
		expires = time.Now().Unix() + out.Result.TokenExpireDur
	}
	return &providers.Credential{
		RefreshToken: refresh,
		AccessToken:  out.Result.Token,
		ExpiresAt:    expires,
		Region:       r.Key,
	}, nil
}

func userInfo(token, region string) (uid, nickname string, err error) {
	r := RegionOf(region)
	body, _ := json.Marshal(map[string]any{"ReqSource": "IDE", "IDEVersion": appVersion})
	var out struct {
		Result struct {
			UserID     string `json:"UserID"`
			ScreenName string `json:"ScreenName"`
		} `json:"Result"`
	}
	if err = postJSON(r.OAuthAPI+"/cloudide/api/v3/trae/GetUserInfo", body, map[string]string{
		"Content-Type":     "application/json",
		"x-cloudide-token": token,
		"User-Agent":       "Trae/" + appVersion,
	}, &out); err != nil {
		return "", "", err
	}
	return out.Result.UserID, out.Result.ScreenName, nil
}

func postJSON(target string, body []byte, headers map[string]string, out any) error {
	req, err := http.NewRequest(http.MethodPost, target, bytes.NewReader(body))
	if err != nil {
		return err
	}
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	b, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return err
	}
	if resp.StatusCode >= 400 {
		return fmt.Errorf("trae http %d: %s", resp.StatusCode, redact(string(b)))
	}
	if len(bytes.TrimSpace(b)) == 0 {
		return nil
	}
	return json.Unmarshal(b, out)
}

func redact(s string) string {
	for _, key := range []string{"refreshToken", "userJwt", "credential", "token"} {
		if i := strings.Index(strings.ToLower(s), strings.ToLower(key)); i >= 0 {
			return s[:i] + key + "=<redacted>"
		}
	}
	return s
}

func headers(cred *providers.Credential, machineID, deviceID string) map[string]string {
	return map[string]string{
		"Content-Type":         "application/json",
		"Accept":               "text/event-stream",
		"User-Agent":           "Trae/" + appVersion,
		"Authorization":        "Cloud-IDE-JWT " + cred.AccessToken,
		"X-Cloudide-Token":     cred.AccessToken,
		"X-Ide-Token":          cred.AccessToken,
		"X-Uid":                cred.UID,
		"X-App-Id":             xAppID,
		"X-App-Version":        "default",
		"X-Ide-Version":        appVersion,
		"X-Ide-Version-Code":   ideVerCode,
		"X-App-Version-Code":   appVerCode,
		"X-Ide-Version-Type":   "stable",
		"X-Device-Type":        "windows",
		"X-OS-Version":         "Windows 11 Pro",
		"X-Device-Brand":       "83DG",
		"Request-Traffic-Type": "prod",
		"X-Machine-Id":         machineID,
		"X-Device-Id":          deviceID,
	}
}

func decodeJWTUserID(token string) string {
	parts := strings.Split(token, ".")
	if len(parts) < 2 {
		return ""
	}
	raw, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return ""
	}
	var payload struct {
		Data struct {
			ID string `json:"id"`
		} `json:"data"`
	}
	if json.Unmarshal(raw, &payload) == nil && payload.Data.ID != "" {
		return payload.Data.ID
	}
	return ""
}
