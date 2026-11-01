package qoder

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
	"time"
	"xiaotao/xt-client/internal/providers"
)

func TestDeviceAuthorization(t *testing.T) {
	flow, err := StartDeviceFlow("global", "stable-machine")
	if err != nil {
		t.Fatal(err)
	}
	u, _ := url.Parse(flow.AuthURL)
	if u.Host != "qoder.com" || u.Query().Get("machine_id") != "stable-machine" || u.Query().Get("challenge_method") != "S256" || u.Query().Get("verifier") != "" {
		t.Fatal("invalid authorization URL")
	}
	calls := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v1/deviceToken/poll":
			if r.URL.Query().Get("verifier") != flow.verifier || r.URL.Query().Get("nonce") != flow.nonce {
				t.Error("missing PKCE parameters")
			}
			calls++
			if calls == 1 {
				w.WriteHeader(404)
				return
			}
			w.Write([]byte(`{"token":"secret","refresh_token":"rotate","expires_at":1800000000000,"user_id":"uid"}`))
		case "/api/v1/userinfo":
			if r.Header.Get("Authorization") != "Bearer secret" {
				t.Error("missing bearer")
			}
			w.Write([]byte(`{"name":"Alice"}`))
		}
	}))
	defer server.Close()
	flow.region.OpenAPIBase = server.URL
	client := AuthClient{HTTP: server.Client(), PollInterval: time.Millisecond}
	cred, err := client.Wait(context.Background(), flow)
	if err != nil {
		t.Fatal(err)
	}
	if cred.UID != "uid" || cred.Region != "global" || cred.MachineID != "stable-machine" || cred.Nickname != "Alice" || cred.ExpiresAt != 1800000000 {
		t.Fatalf("wrong identity: %#v", cred)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := client.Wait(ctx, flow); err == nil {
		t.Fatal("cancelled authorization succeeded")
	}
}

func TestRefreshPreservesIdentityAndRejectsInvalidToken(t *testing.T) {
	bad := false
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body map[string]string
		json.NewDecoder(r.Body).Decode(&body)
		if body["refresh_token"] != "old" {
			t.Error("wrong refresh token")
		}
		if bad {
			w.Write([]byte(`{}`))
			return
		}
		w.Write([]byte(`{"access_token":"new","refreshToken":"rotated","expires_at":"2027-01-15T08:00:00Z"}`))
	}))
	defer server.Close()
	client := AuthClient{HTTP: server.Client()}
	base := &providers.Credential{AccessToken: "old-access", RefreshToken: "old", UID: "uid", Region: "cn", MachineID: "machine", Nickname: "name"}
	reg := Regions["cn"]
	reg.OpenAPIBase = server.URL
	next, err := client.refresh(context.Background(), reg, base)
	if err != nil {
		t.Fatal(err)
	}
	if next.RefreshToken != "rotated" || next.AccessToken != "new" || next.UID != base.UID || next.MachineID != base.MachineID || next.Nickname != base.Nickname || next.Region != "cn" {
		t.Fatal("refresh lost identity")
	}
	bad = true
	if _, err := client.refresh(context.Background(), reg, base); err == nil {
		t.Fatal("accepted missing access token")
	}
}

func TestDeviceErrorsAndTimeout(t *testing.T) {
	for _, status := range []int{200, 403, 404} {
		t.Run(http.StatusText(status), func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(status); w.Write([]byte(`{"token":"x"}`)) }))
			defer server.Close()
			flow, _ := StartDeviceFlow("cn", "m")
			flow.region.OpenAPIBase = server.URL
			ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
			defer cancel()
			if _, err := (AuthClient{HTTP: server.Client(), PollInterval: time.Millisecond}).Wait(ctx, flow); err == nil {
				t.Fatal("expected missing identity, HTTP error, or timeout")
			}
		})
	}
	if _, err := StartDeviceFlow("invalid", "m"); err == nil {
		t.Fatal("invalid region accepted")
	}
}
