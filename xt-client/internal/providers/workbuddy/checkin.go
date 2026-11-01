package workbuddy

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"xiaotao/xt-client/internal/providers"
)

type CheckinStatus struct {
	Supported   bool    `json:"supported"`
	Active      bool    `json:"active"`
	CheckedIn   bool    `json:"checkedIn"`
	StreakDays  int     `json:"streakDays"`
	DailyCredit float64 `json:"dailyCredit"`
	Message     string  `json:"message"`
}

type CheckinClaimResult struct {
	OK           bool    `json:"ok"`
	CheckedIn    bool    `json:"checkedIn"`
	ExtraCredits float64 `json:"extraCredits"`
	StreakDays   int     `json:"streakDays"`
	Message      string  `json:"message"`
}

func checkinHeaders(cred *providers.Credential) map[string]string {
	h := headersFor(cred)
	h["Accept-Language"] = "zh"
	return h
}

func FetchCheckinStatus(ctx context.Context, baseURL string, cred *providers.Credential) (*CheckinStatus, error) {
	if testBaseURL != "" {
		baseURL = testBaseURL
	}
	if baseURL == "" {
		baseURL = DefaultBaseURL
	}
	if cred == nil || cred.AccessToken == "" {
		return &CheckinStatus{Supported: false, Message: "未登录或缺少凭据"}, nil
	}

	raw, err := post(ctx, baseURL+"/v2/billing/meter/checkin-activity-status", map[string]any{}, checkinHeaders(cred))
	if err != nil {
		return nil, err
	}

	var envelope struct {
		Code any    `json:"code"`
		Msg  string `json:"msg"`
		Data struct {
			Active         bool     `json:"active"`
			TodayCheckedIn bool     `json:"today_checked_in"`
			StreakDays     int      `json:"streak_days"`
			TodayCredit    float64  `json:"today_credit"`
			DailyCredit    float64  `json:"daily_credit"`
			CheckinDates   []string `json:"checkin_dates"`
			ActivityName   string   `json:"activity_name"`
		} `json:"data"`
	}

	if err := json.Unmarshal(raw, &envelope); err != nil {
		return nil, fmt.Errorf("invalid checkin status response: %w", err)
	}

	if code := number(envelope.Code); code != 0 {
		return nil, fmt.Errorf("checkin status code %d: %s", code, envelope.Msg)
	}

	daily := envelope.Data.TodayCredit
	if daily == 0 {
		daily = envelope.Data.DailyCredit
	}

	msg := envelope.Msg
	if !envelope.Data.Active {
		msg = "当前暂无签到活动"
	} else if envelope.Data.TodayCheckedIn {
		msg = "今日已签到"
	}

	return &CheckinStatus{
		Supported:   true,
		Active:      envelope.Data.Active,
		CheckedIn:   envelope.Data.TodayCheckedIn,
		StreakDays:  envelope.Data.StreakDays,
		DailyCredit: daily,
		Message:     msg,
	}, nil
}

func ClaimCheckin(ctx context.Context, baseURL string, cred *providers.Credential) (*CheckinClaimResult, error) {
	if testBaseURL != "" {
		baseURL = testBaseURL
	}
	if baseURL == "" {
		baseURL = DefaultBaseURL
	}
	if cred == nil || cred.AccessToken == "" {
		return nil, fmt.Errorf("未登录或缺少凭据")
	}

	raw, err := post(ctx, baseURL+"/v2/billing/meter/daily-checkin", map[string]any{}, checkinHeaders(cred))
	if err != nil {
		return nil, err
	}

	var envelope struct {
		Code any    `json:"code"`
		Msg  string `json:"msg"`
		Data struct {
			Credit      float64 `json:"credit"`
			TodayCredit float64 `json:"today_credit"`
			DailyCredit float64 `json:"daily_credit"`
			StreakDays  int     `json:"streak_days"`
		} `json:"data"`
	}

	if err := json.Unmarshal(raw, &envelope); err != nil {
		return nil, fmt.Errorf("invalid daily-checkin response: %w", err)
	}

	if code := number(envelope.Code); code != 0 {
		msg := envelope.Msg
		if strings.Contains(msg, "已经签到") || strings.Contains(msg, "已签到") {
			return &CheckinClaimResult{
				OK:        true,
				CheckedIn: true,
				Message:   "今日已签到",
			}, nil
		}
		return &CheckinClaimResult{
			OK:      false,
			Message: msg,
		}, nil
	}

	credits := envelope.Data.Credit
	if credits == 0 {
		credits = envelope.Data.TodayCredit
	}
	if credits == 0 {
		credits = envelope.Data.DailyCredit
	}

	return &CheckinClaimResult{
		OK:           true,
		CheckedIn:    true,
		ExtraCredits: credits,
		StreakDays:   envelope.Data.StreakDays,
		Message:      envelope.Msg,
	}, nil
}
