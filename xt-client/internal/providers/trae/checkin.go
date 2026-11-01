package trae

import (
	"bytes"
	"crypto/sha256"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"xiaotao/xt-client/internal/providers"
)

type CheckinStatus struct {
	Supported   bool    `json:"supported"`
	Active      bool    `json:"active"`
	CheckedIn   bool    `json:"checkedIn"`
	DailyCredit float64 `json:"dailyCredit"`
	Message     string  `json:"message"`
}

type CheckinClaimResult struct {
	OK           bool    `json:"ok"`
	CheckedIn    bool    `json:"checkedIn"`
	ExtraCredits float64 `json:"extraCredits"`
	Message      string  `json:"message"`
}

var devicePool = []struct {
	brand string
	dtype string
}{
	{"MacBookPro18,3", "macos"},
	{"Mac14,6", "macos"},
	{"ThinkPad X1 Carbon Gen 11", "windows"},
	{"Dell XPS 15 9530", "windows"},
	{"HP Spectre x360 14", "windows"},
	{"MateBook X Pro 2023", "windows"},
}

func deriveIdentity(cred *providers.Credential, accountID string) string {
	if cred == nil {
		return accountID
	}
	if cred.UID != "" {
		return cred.UID
	}
	if jwtUID := decodeJWTUserID(cred.AccessToken); jwtUID != "" {
		return jwtUID
	}
	if accountID != "" {
		return accountID
	}
	return cred.AccessToken
}

func deriveDevice(identity string, alt int) (string, string, string) {
	suffix := ""
	if alt > 0 {
		suffix = fmt.Sprintf("/alt%d", alt)
	}
	h1 := sha256.Sum256([]byte(identity + suffix))
	val1 := binary.BigEndian.Uint64(h1[:8])
	deviceID := fmt.Sprintf("%016d", val1%10000000000000000)

	h2 := sha256.Sum256([]byte(identity + "/fp" + suffix))
	val2 := binary.BigEndian.Uint64(h2[:8])
	dev := devicePool[int(val2%uint64(len(devicePool)))]

	return deviceID, dev.brand, dev.dtype
}

func postCheckin(target string, token string, devID, brand, dtype string, out any) error {
	req, err := http.NewRequest(http.MethodPost, target, bytes.NewReader([]byte("{}")))
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Cloud-IDE-JWT "+token)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", "Trae/"+appVersion)
	req.Header.Set("x-device-id", devID)
	req.Header.Set("x-device-brand", brand)
	req.Header.Set("x-device-type", dtype)
	req.Header.Set("package-type", "stable_cn")

	client := &http.Client{Timeout: 15 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return err
	}
	if resp.StatusCode >= 400 {
		return fmt.Errorf("trae checkin http %d: %s", resp.StatusCode, redact(string(body)))
	}
	if len(bytes.TrimSpace(body)) == 0 {
		return nil
	}
	return json.Unmarshal(body, out)
}

func FetchCheckinStatus(cred *providers.Credential, accountID string) (*CheckinStatus, error) {
	if cred == nil || cred.AccessToken == "" {
		return &CheckinStatus{Supported: false, Message: "未登录或缺少凭据"}, nil
	}
	reg := strings.ToLower(cred.Region)
	if reg != "" && reg != "cn" {
		return &CheckinStatus{Supported: false, CheckedIn: true, Message: "海外版无每日签到"}, nil
	}

	identity := deriveIdentity(cred, accountID)
	devID, brand, dtype := deriveDevice(identity, 0)

	target := "https://api.trae.cn/trae/api/v2/ug/checkin_credits/status"
	var out struct {
		Code         int     `json:"code"`
		Message      string  `json:"message"`
		CheckedIn    bool    `json:"checked_in"`
		Enable       *bool   `json:"enable"`
		Credits      float64 `json:"credits"`
		ExtraCredits float64 `json:"extra_credits"`
		Data         struct {
			CheckedIn    bool    `json:"checked_in"`
			Enable       *bool   `json:"enable"`
			Credits      float64 `json:"credits"`
			ExtraCredits float64 `json:"extra_credits"`
		} `json:"data"`
	}

	if err := postCheckin(target, cred.AccessToken, devID, brand, dtype, &out); err != nil {
		return nil, err
	}

	checkedIn := out.CheckedIn || out.Data.CheckedIn
	enabled := true
	if out.Enable != nil {
		enabled = *out.Enable
	} else if out.Data.Enable != nil {
		enabled = *out.Data.Enable
	}

	daily := out.Credits + out.ExtraCredits
	if daily == 0 {
		daily = out.Data.Credits + out.Data.ExtraCredits
	}

	msg := out.Message
	if msg == "" && !enabled {
		msg = "当前暂无签到活动"
	}

	return &CheckinStatus{
		Supported:   true,
		Active:      enabled,
		CheckedIn:   checkedIn,
		DailyCredit: daily,
		Message:     msg,
	}, nil
}

func ClaimCheckin(cred *providers.Credential, accountID string) (*CheckinClaimResult, error) {
	if cred == nil || cred.AccessToken == "" {
		return nil, fmt.Errorf("未登录或缺少凭据")
	}
	reg := strings.ToLower(cred.Region)
	if reg != "" && reg != "cn" {
		return &CheckinClaimResult{OK: false, CheckedIn: true, Message: "海外版无每日签到"}, nil
	}

	identity := deriveIdentity(cred, accountID)
	target := "https://api.trae.cn/trae/api/v2/ug/checkin_credits/claim"

	type claimResp struct {
		Code           int     `json:"code"`
		Message        string  `json:"message"`
		CreditsGranted float64 `json:"credits_granted"`
		ExtraCredits   float64 `json:"extra_credits"`
		Data           struct {
			CreditsGranted float64 `json:"credits_granted"`
			ExtraCredits   float64 `json:"extra_credits"`
		} `json:"data"`
	}

	var last claimResp
	var lastErr error

	// 尝试默认设备，遇到 9074（指纹风控）时换备用机型重试
	for _, alt := range []int{0, 1, 2, 3} {
		devID, brand, dtype := deriveDevice(identity, alt)
		var resp claimResp
		err := postCheckin(target, cred.AccessToken, devID, brand, dtype, &resp)
		if err != nil {
			lastErr = err
			break
		}
		last = resp
		if resp.Code == 0 {
			credits := resp.CreditsGranted
			if credits == 0 {
				credits = resp.ExtraCredits
			}
			if credits == 0 {
				credits = resp.Data.CreditsGranted
			}
			if credits == 0 {
				credits = resp.Data.ExtraCredits
			}
			return &CheckinClaimResult{
				OK:           true,
				CheckedIn:    true,
				ExtraCredits: credits,
				Message:      resp.Message,
			}, nil
		}
		// 9095 表示设备今日已签到，按成功处理
		if resp.Code == 9095 {
			return &CheckinClaimResult{
				OK:        true,
				CheckedIn: true,
				Message:   fmt.Sprintf("今日已签到：%s", resp.Message),
			}, nil
		}
		// 9074 是设备风控，重试下一个设备
		if resp.Code != 9074 {
			break
		}
		time.Sleep(500 * time.Millisecond)
	}

	if lastErr != nil {
		return nil, lastErr
	}
	return &CheckinClaimResult{
		OK:      false,
		Message: last.Message,
	}, nil
}
