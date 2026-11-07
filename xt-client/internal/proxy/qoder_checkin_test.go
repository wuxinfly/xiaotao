package proxy

import (
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"xiaotao/xt-client/internal/storage"
)

func TestQoderCheckinServiceAndProviderScope(t *testing.T) {
	store, err := storage.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	for _, a := range []storage.Account{{ID: "q", Provider: "qoder", Region: "global", Enabled: true}, {ID: "off", Provider: "qoder", Region: "cn", Enabled: false}, {ID: "wb", Provider: "workbuddy", Enabled: true}} {
		if err := store.UpsertAccount(&a); err != nil {
			t.Fatal(err)
		}
	}
	old := http.DefaultTransport
	defer func() { http.DefaultTransport = old }()
	calls := 0
	http.DefaultTransport = catalogTransport(func(r *http.Request) (*http.Response, error) {
		calls++
		if r.URL.Host != "openapi.qoder.sh" || r.URL.Path != "/sash/api/v1/me/campaigns" || r.Method != "GET" {
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL)
		}
		return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(`{"campaigns":[]}`)), Header: http.Header{}}, nil
	})
	svc := NewAccountService(store, func(id string) ([]byte, error) {
		if id != "q" {
			t.Fatalf("accessed unrelated credential: %s", id)
		}
		return []byte(`{"AccessToken":"fixture"}`), nil
	})
	status, err := svc.FetchCheckin("q")
	if err != nil || !status.Supported || status.Active {
		t.Fatalf("wrong status: %+v %v", status, err)
	}
	a, _ := store.Account("q")
	if a.Checkin == nil || a.Checkin.Message != "暂无可领取活动" {
		t.Fatalf("status not persisted: %+v", a)
	}
	results := svc.ClaimSupportedProvider("qoder")
	if len(results) != 1 || results["q"].OK || results["q"].CheckedIn || calls != 2 {
		t.Fatalf("wrong filtered result: %+v calls=%d", results, calls)
	}
}

func TestQoderCheckinRefreshesOnceOnUnauthorized(t *testing.T) {
	store, _ := storage.Open(t.TempDir())
	acct := storage.Account{ID: "q", Provider: "qoder", Region: "cn", Enabled: true}
	store.UpsertAccount(&acct)
	identity := func(b []byte) ([]byte, error) { return b, nil }
	if err := store.SaveSecret("q", []byte(`{"AccessToken":"old","RefreshToken":"refresh"}`), identity); err != nil {
		t.Fatal(err)
	}
	oldEncrypt := defaultEncrypt
	defaultEncrypt = identity
	defer func() { defaultEncrypt = oldEncrypt }()
	old := http.DefaultTransport
	defer func() { http.DefaultTransport = old }()
	lists, refreshes := 0, 0
	http.DefaultTransport = catalogTransport(func(r *http.Request) (*http.Response, error) {
		status := 200
		body := `{"campaigns":[]}`
		switch r.URL.Path {
		case "/sash/api/v1/me/campaigns":
			lists++
			if r.Header.Get("Authorization") == "Bearer old" {
				status = 401
			} else if r.Header.Get("Authorization") != "Bearer new" {
				t.Fatal("lost refreshed token")
			}
		case "/api/v1/deviceToken/refresh":
			refreshes++
			body = `{"token":"new","refresh_token":"rotated","expires_in":3600}`
		default:
			t.Fatalf("unexpected path %s", r.URL.Path)
		}
		return &http.Response{StatusCode: status, Body: io.NopCloser(strings.NewReader(body)), Header: http.Header{}}, nil
	})
	svc := NewAccountService(store, func(id string) ([]byte, error) { return store.LoadSecret(id, identity) })
	if _, err := svc.FetchCheckin("q"); err != nil {
		t.Fatal(err)
	}
	if lists != 2 || refreshes != 1 {
		t.Fatalf("wrong retry counts: lists=%d refreshes=%d", lists, refreshes)
	}
	raw, _ := store.LoadSecret("q", identity)
	if !strings.Contains(string(raw), "rotated") {
		t.Fatal("refresh rotation not persisted")
	}
}

func TestQoderCheckinDelaysQuotaRefresh(t *testing.T) {
	store, err := storage.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	acct := storage.Account{ID: "q", Provider: "qoder", Region: "cn", Enabled: true}
	if err := store.UpsertAccount(&acct); err != nil {
		t.Fatal(err)
	}
	old := http.DefaultTransport
	defer func() { http.DefaultTransport = old }()
	quotaRequests := 0
	http.DefaultTransport = catalogTransport(func(r *http.Request) (*http.Response, error) {
		body := `{"campaigns":[]}`
		switch r.URL.Path {
		case "/sash/api/v1/me/campaigns":
			if r.Method == http.MethodGet {
				body = `{"campaigns":[{"campaignId":"daily","actionType":"CLAIM_BENEFIT","claimStatus":"CLAIMABLE","benefit":{"kind":"CREDITS","amount":37}}]}`
			}
		case "/sash/api/v1/me/campaigns/daily/claim":
			body = `{"status":"CLAIMED","benefit":{"kind":"CREDITS","amount":37}}`
		case "/api/v2/quota/usage":
			quotaRequests++
			body = `{"userQuota":{"used":0,"total":100,"remaining":100}}`
		default:
			t.Fatalf("unexpected path %s", r.URL.Path)
		}
		return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader(body)), Header: http.Header{}}, nil
	})
	svc := NewAccountService(store, func(string) ([]byte, error) {
		return []byte(`{"AccessToken":"fixture"}`), nil
	})
	svc.checkinQuotaDelay = 50 * time.Millisecond
	res, err := svc.ClaimCheckin("q")
	if err != nil || !res.OK || res.ExtraCredits != 37 {
		t.Fatalf("unexpected claim result: %+v %v", res, err)
	}
	if quotaRequests != 0 {
		t.Fatalf("quota refreshed before delay: %d", quotaRequests)
	}
	time.Sleep(100 * time.Millisecond)
	if quotaRequests != 1 {
		t.Fatalf("expected one delayed quota refresh, got %d", quotaRequests)
	}
}
