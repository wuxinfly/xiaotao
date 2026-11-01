package workbuddy

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"xiaotao/xt-client/internal/providers"
)

func TestWorkbuddyCheckinStatusAndClaim(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/v2/billing/meter/checkin-activity-status":
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{
				"code": 0,
				"msg": "success",
				"data": {
					"active": true,
					"today_checked_in": false,
					"streak_days": 2,
					"today_credit": 50,
					"activity_name": "打卡活动"
				}
			}`))
		case "/v2/billing/meter/daily-checkin":
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{
				"code": 0,
				"msg": "打卡成功",
				"data": {
					"credit": 50,
					"streak_days": 3
				}
			}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	cred := &providers.Credential{
		AccessToken: "test-token",
		UID:         "wb-user-1",
	}

	st, err := FetchCheckinStatus(context.Background(), server.URL, cred)
	if err != nil {
		t.Fatalf("fetch status failed: %v", err)
	}
	if !st.Supported || !st.Active || st.CheckedIn || st.DailyCredit != 50 || st.StreakDays != 2 {
		t.Fatalf("unexpected checkin status: %+v", st)
	}

	claim, err := ClaimCheckin(context.Background(), server.URL, cred)
	if err != nil {
		t.Fatalf("claim checkin failed: %v", err)
	}
	if !claim.OK || !claim.CheckedIn || claim.ExtraCredits != 50 || claim.StreakDays != 3 {
		t.Fatalf("unexpected claim result: %+v", claim)
	}
}
