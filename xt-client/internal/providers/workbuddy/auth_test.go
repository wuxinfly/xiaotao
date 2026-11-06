package workbuddy

import (
	"context"
	"encoding/base64"
	"net/http"
	"net/http/httptest"
	"testing"

	"xiaotao/xt-client/internal/providers"
)

func TestStartAuthUnwrapsNestedPayload(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != authStatePath || r.URL.Query().Get("platform") != "workbuddy" {
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

func TestNativeAccountAndLegacyRejection(t *testing.T) {
	token := "header." + base64.RawURLEncoding.EncodeToString([]byte(`{"sub":"native-42","exp":1893456000}`)) + ".signature"
	c := &providers.Credential{AccessToken: token}
	if err := NativeAccount(c); err != nil {
		t.Fatal(err)
	}
	if c.UID != "native-42" || c.Domain != "www.workbuddy.cn" || c.ExpiresAt != 1893456000 {
		t.Fatalf("wrong metadata: %+v", c)
	}
	for _, domain := range []string{"copilot.tencent.com", "www.codebuddy.cn", "www.workbuddy.ai", ""} {
		if ValidateCredential(&providers.Credential{AccessToken: token, Domain: domain}) == nil {
			t.Fatalf("accepted wrong channel %q", domain)
		}
	}
	h := headersFor(c)
	if h["X-IDE-Type"] != "WorkBuddy" || h["X-Product-Code"] != "" || h["X-Product"] != "SaaS" {
		t.Fatalf("wrong native identity: %+v", h)
	}
}

func TestNativeModelCatalog(t *testing.T) {
	payload := `{"code":0,"data":{"models":[{"id":"native","name":"Native","supportsImages":true},{"id":"hidden"},{"id":"image","tags":["text-to-image"]}],"agents":[{"name":"other","models":["hidden"]},{"name":"cli","tags":["default"],"models":["native","native","image"]}]}}`
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v3/config" || r.Header.Get("X-IDE-Name") != "WorkBuddy" {
			t.Errorf("wrong catalog request: %s", r.URL.Path)
		}
		w.Write([]byte(payload))
	}))
	defer server.Close()
	restore := SetBaseURL(server.URL)
	defer restore()
	models, err := (Provider{}).FetchModels(context.Background(), &providers.Credential{AccessToken: "test"})
	if err != nil || len(models) != 1 || models[0].ID != "native" || !models[0].Multimodal {
		t.Fatalf("unexpected catalog: %+v %v", models, err)
	}
	payload = `{"code":0,"data":{"models":[{"id":"hidden"}]}}`
	if _, err = (Provider{}).FetchModels(context.Background(), &providers.Credential{AccessToken: "test"}); err == nil {
		t.Fatal("accepted missing client catalog")
	}
}
