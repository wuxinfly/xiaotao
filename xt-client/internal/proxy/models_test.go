package proxy

import (
	"io"
	"net/http"
	"strings"
	"testing"
	"xiaotao/xt-client/internal/providers"
	"xiaotao/xt-client/internal/providers/trae"
	"xiaotao/xt-client/internal/providers/workbuddy"
	"xiaotao/xt-client/internal/storage"
)

type catalogTransport func(*http.Request) (*http.Response, error)

func (f catalogTransport) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func TestAccountModelsCatalogAndFallback(t *testing.T) {
	providers.Register(trae.Provider{})
	providers.Register(workbuddy.Provider{})
	old := http.DefaultClient
	defer func() { http.DefaultClient = old }()
	status := 200
	http.DefaultClient = &http.Client{Transport: catalogTransport(func(r *http.Request) (*http.Response, error) {
		if r.URL.Path != "/api/ide/v1/get_detail_param" {
			t.Fatalf("unexpected path %s", r.URL.Path)
		}
		return &http.Response{StatusCode: status, Body: io.NopCloser(strings.NewReader(`{"config_info_list":[{"config_name":"NEW-MODEL"},{"config_name":"hidden","is_invisible_to_user":true},{"config_name":"disabled","config_switch":false},{"config_name":"custom","display_config":{"is_custom_model":true}}]}`)), Header: http.Header{}}, nil
	})}
	svc := New(nil, func(string) ([]byte, error) { return []byte(`{"AccessToken":"test"}`), nil }, "")
	acct := storage.Account{Provider: "trae", Region: "cn"}
	models, source := svc.AccountModelCatalog(acct)
	if source != "live" {
		t.Fatalf("expected live source, got %s", source)
	}
	if len(models) != 1 || models[0].ID != "new-model" {
		t.Fatalf("catalog replaced incorrectly: %+v", models)
	}
	if route := svc.chatModelRoutes(acct)["new-model"]; route != "NEW-MODEL" {
		t.Fatalf("lost upstream casing: %s", route)
	}
	status = 503
	models, source = svc.AccountModelCatalog(acct)
	if source != "cached" {
		t.Fatalf("expected cached catalog, got %s", source)
	}
	if len(models) != 1 || models[0].ID != "new-model" {
		t.Fatalf("lost previous catalog: %+v", models)
	}
	other := acct
	other.ID = "other-account"
	otherModels, otherSource := svc.AccountModelCatalog(other)
	if otherSource != "builtin" || len(otherModels) < 2 {
		t.Fatalf("cross-account cache leak: %s %+v", otherSource, otherModels)
	}
	acct.Provider = "workbuddy"
	models = svc.AccountModels(acct)
	if len(models) != 23 {
		t.Fatalf("incomplete WorkBuddy catalog: %d", len(models))
	}
	for _, m := range models {
		if m.ID == "mimo-v2.6-pro" {
			return
		}
	}
	t.Fatal("missing WorkBuddy MiMo")
}
