package qoder

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"
	"xiaotao/xt-client/internal/providers"
)

type checkinTransport func(*http.Request) (*http.Response, error)

func (f checkinTransport) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }
func checkinResponse(status int, body string) *http.Response {
	return &http.Response{StatusCode: status, Body: io.NopCloser(strings.NewReader(body)), Header: http.Header{}}
}
func TestCampaignEligibility(t *testing.T) {
	now := time.Now().Unix()
	mk := func(status string) campaign {
		a := campaign{ID: "id", Action: "CLAIM_BENEFIT", Status: status}
		a.Benefit.Kind = "CREDITS"
		a.Benefit.Amount = 42
		return a
	}
	ready := mk("CLAIMABLE")
	claimed := mk("CLAIMED")
	claimed.ID = "claimed-id"
	future := ready
	future.Start = now + 3600
	expired := ready
	expired.End = now - 1
	detail := ready
	detail.Action = "VIEW_DETAILS"
	for _, tc := range []struct {
		name            string
		rows            []campaign
		active, checked bool
		amount          float64
	}{
		{"empty", nil, false, false, 0}, {"ready", []campaign{ready}, true, false, 42}, {"claimed", []campaign{claimed}, true, true, 0},
		{"mixed", []campaign{claimed, ready}, true, false, 42}, {"outside-window", []campaign{future, expired, detail}, false, false, 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			st := campaignStatus(tc.rows)
			if st.Active != tc.active || st.CheckedIn != tc.checked || st.DailyCredit != tc.amount || !st.Supported {
				t.Fatalf("wrong status: %+v", st)
			}
		})
	}
}
func TestCampaignRegionHeadersAndReplay(t *testing.T) {
	for _, region := range []string{"cn", "global"} {
		for _, replayed := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s-%v", region, replayed), func(t *testing.T) {
				calls := 0
				c := CheckinClient{HTTP: &http.Client{Transport: checkinTransport(func(r *http.Request) (*http.Response, error) {
					calls++
					if r.URL.Host != strings.TrimPrefix(Regions[region].OpenAPIBase, "https://") || r.Header.Get("Cosy-ClientType") != "10" || r.Header.Get("Authorization") != "Bearer secret" || r.Header.Get("User-Agent") != "Qoder" {
						t.Fatalf("wrong request: %s", r.URL)
					}
					if r.Method == "GET" {
						if r.URL.Path != campaignsPath || r.URL.Query().Get("forceRefresh") != "true" {
							t.Fatal("wrong list path")
						}
						return checkinResponse(200, `{"campaigns":[{"campaignId":"abc","actionType":"CLAIM_BENEFIT","claimStatus":"CLAIMABLE","benefit":{"kind":"CREDITS","amount":37}},{"campaignId":"detail","actionType":"VIEW_DETAILS","claimStatus":"CLAIMABLE"}]}`), nil
					}
					if r.URL.Path != campaignsPath+"/abc/claim" {
						t.Fatal("wrong claim path")
					}
					b, _ := io.ReadAll(r.Body)
					if string(b) != "{}" {
						t.Fatal("wrong body")
					}
					return checkinResponse(200, fmt.Sprintf(`{"status":"CLAIMED","replayed":%v}`, replayed)), nil
				})}}
				res, err := c.Claim(context.Background(), &providers.Credential{Region: region, AccessToken: "secret"})
				if err != nil || !res.CheckedIn || res.OK == replayed || calls != 2 {
					t.Fatalf("wrong result: %+v %v calls=%d", res, err, calls)
				}
				expected := 37.0
				if replayed {
					expected = 0
				}
				if res.ExtraCredits != expected {
					t.Fatalf("wrong credits: %v", res.ExtraCredits)
				}
			})
		}
	}
}
func TestCampaignErrorsAndNoClaim(t *testing.T) {
	for _, tc := range []struct {
		body    string
		status  int
		wantErr bool
	}{
		{`{"campaigns":[]}`, 200, false}, {`{"campaigns":[{"campaignId":"a","actionType":"CLAIM_BENEFIT","claimStatus":"CLAIMED","benefit":{"kind":"CREDITS"}}]}`, 200, false},
		{`{}`, 200, true}, {`bad json`, 200, true}, {`{}`, 401, true}, {`{}`, 500, true},
	} {
		calls := 0
		c := CheckinClient{HTTP: &http.Client{Transport: checkinTransport(func(r *http.Request) (*http.Response, error) {
			calls++
			if r.Method != "GET" {
				t.Fatal("unexpected claim")
			}
			return checkinResponse(tc.status, tc.body), nil
		})}}
		_, err := c.Claim(context.Background(), &providers.Credential{Region: "cn", AccessToken: "secret"})
		if (err != nil) != tc.wantErr || calls != 1 {
			t.Fatalf("response %s: %v", tc.body, err)
		}
	}
	for _, cred := range []*providers.Credential{nil, {Region: "bad", AccessToken: "secret"}, {Region: "cn"}} {
		if _, err := (CheckinClient{}).Status(context.Background(), cred); err == nil {
			t.Fatal("accepted invalid credentials")
		}
	}
}
