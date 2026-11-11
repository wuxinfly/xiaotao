package main

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"xiaotao/xt-client/internal/providers"
	"xiaotao/xt-client/internal/sourceplugin"
)

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }
func jwtWithUID(uid string) string {
	return "x." + base64.RawURLEncoding.EncodeToString([]byte(`{"data":{"id":"`+uid+`"}}`)) + ".x"
}
func TestRefreshRestoresAndPreservesUID(t *testing.T) {
	old := http.DefaultClient
	t.Cleanup(func() { http.DefaultClient = old })
	http.DefaultClient = &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		if !strings.Contains(r.URL.Path, "ExchangeToken") {
			t.Fatalf("unexpected request: %s", r.URL.Path)
		}
		body := `{"Result":{"Token":"new-token","RefreshToken":"rotated","TokenExpireDuration":3600}}`
		return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(body)), Header: make(http.Header)}, nil
	})}
	for _, tc := range []struct {
		name string
		cred providers.Credential
		want string
	}{
		{"existing UID survives rotation", providers.Credential{RefreshToken: "old", UID: "account-id", MachineID: "machine", DeviceID: "device", Region: "cn"}, "account-id"},
		{"existing plugin recovers UID", providers.Credential{RefreshToken: "old", AccessToken: jwtWithUID("jwt-id"), ExpiresAt: time.Now().Unix() + 3600, Region: "cn"}, "jwt-id"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			raw, _ := json.Marshal(tc.cred)
			result, err := operate(context.Background(), "credentials/refresh", sourceplugin.Operation{Credential: raw}, nil)
			if err != nil {
				t.Fatal(err)
			}
			var got providers.Credential
			if err := json.Unmarshal(result.Credential, &got); err != nil {
				t.Fatal(err)
			}
			if got.UID != tc.want {
				t.Fatalf("UID = %q, want %q", got.UID, tc.want)
			}
			if tc.name == "existing UID survives rotation" && (got.RefreshToken != "rotated" || got.MachineID != "machine" || got.DeviceID != "device") {
				t.Fatal("rotation lost credentials or device identity")
			}
		})
	}
}
