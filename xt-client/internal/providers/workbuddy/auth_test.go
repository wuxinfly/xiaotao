package workbuddy

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"xiaotao/xt-client/internal/providers"
)

func TestStartAuthUnwrapsNestedPayload(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != authStatePath || r.URL.Query().Get("platform") != "VSCode" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"data":{"data":{"authUrl":"https://example.test/login","state":"st-1"}}}`))
	}))
	defer server.Close()
	restore := SetBaseURL(server.URL)
	defer restore()

	loginURL, state, err := StartAuth(context.Background(), "")
	if err != nil {
		t.Fatalf("StartAuth failed: %v", err)
	}
	if loginURL != "https://example.test/login" || state != "st-1" {
		t.Fatalf("unexpected login response: %q %q", loginURL, state)
	}
}

func TestWaitForTokenConvertsMillisecondsAndKeepsDomain(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != authTokenPath {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"data":{"accessToken":"at-1","refreshToken":"rt-1","expiresAt":1893456000000,"domain":"copilot.tencent.com"}}`))
	}))
	defer server.Close()
	restore := SetBaseURL(server.URL)
	defer restore()

	cred, err := WaitForToken(context.Background(), "", "st-1")
	if err != nil {
		t.Fatalf("WaitForToken failed: %v", err)
	}
	if cred.AccessToken != "at-1" || cred.RefreshToken != "rt-1" || cred.ExpiresAt != 1893456000 || cred.Domain != "copilot.tencent.com" {
		t.Fatalf("unexpected credential: %+v", cred)
	}
}

func TestWaitForAccountRequiresUID(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == authTokenPath {
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"data":{"accessToken":"at-1","expiresAt":1893456000000,"domain":"copilot.tencent.com"}}`))
			return
		}
		if r.URL.Path == loginAccPath {
			if r.Header.Get("Authorization") != "Bearer at-1" || r.Header.Get("X-Domain") != "copilot.tencent.com" ||
				r.Header.Get("X-Product-Code") != "codebuddy" {
				http.Error(w, "unauthorized", http.StatusUnauthorized)
				return
			}
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"data":{"uid":"42","nickname":"小涛","enterpriseId":"ent-1"}}`))
			return
		}
		http.NotFound(w, r)
	}))
	defer server.Close()
	restore := SetBaseURL(server.URL)
	defer restore()

	uid, nickname, enterprise, department, err := WaitForAccount(context.Background(), "", "st-1", "at-1")
	if err != nil {
		t.Fatalf("WaitForAccount failed: %v", err)
	}
	if uid != "42" || nickname != "小涛" || enterprise != "ent-1" || department != "" {
		t.Fatalf("unexpected account: %q %q %q %q", uid, nickname, enterprise, department)
	}
}

func TestWaitForAccountTimesOutWithoutUID(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"data":{"nickname":"小涛","enterpriseId":"ent-1"}}`))
	}))
	defer server.Close()
	restore := SetBaseURL(server.URL)
	defer restore()

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Millisecond)
	defer cancel()
	_, _, _, _, err := WaitForAccount(ctx, "", "st-1", "at-1")
	if err == nil || !strings.Contains(err.Error(), "context deadline exceeded") {
		t.Fatalf("expected timeout, got %v", err)
	}
}

func TestRefreshKeepsIdentityAndFallsBackToOldRefreshToken(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("X-Refresh-Token") != "rt-old" || r.Header.Get("X-Auth-Refresh-Source") != "plugin" {
			http.Error(w, "bad refresh headers", http.StatusBadRequest)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"data":{"accessToken":"at-2","expiresAt":1893456000000,"domain":"new.example"}}`))
	}))
	defer server.Close()
	restore := SetBaseURL(server.URL)
	defer restore()

	cred := &providers.Credential{
		UID:          "42",
		RefreshToken: "rt-old",
		MachineID:    "machine",
		EnterpriseID: "ent-1",
	}
	next, err := Refresh(context.Background(), "", cred)
	if err != nil {
		t.Fatalf("Refresh failed: %v", err)
	}
	if next.AccessToken != "at-2" || next.RefreshToken != "rt-old" || next.UID != "42" || next.Domain != "new.example" || next.EnterpriseID != "ent-1" {
		t.Fatalf("unexpected refreshed credential: %+v", next)
	}
}
