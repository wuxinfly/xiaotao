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
	calls := 0
	http.DefaultClient = &http.Client{Transport: catalogTransport(func(r *http.Request) (*http.Response, error) {
		calls++
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
	if calls != 1 {
		t.Fatalf("viewing Trae fetched %d times", calls)
	}
	models, source, refreshErr := svc.RefreshAccountModelCatalog(acct)
	if refreshErr == nil || source != "stale" || len(models) != 1 || calls != 2 {
		t.Fatalf("Trae refresh failed to retain cache: %s %+v %v calls=%d", source, models, refreshErr, calls)
	}
	other := acct
	other.ID = "other-account"
	otherModels, otherSource := svc.AccountModelCatalog(other)
	if otherSource != "builtin" || len(otherModels) < 2 {
		t.Fatalf("cross-account cache leak: %s %+v", otherSource, otherModels)
	}
	if got := (workbuddy.Provider{}).ListModels("cn"); len(got) != 0 {
		t.Fatal("WorkBuddy must not advertise the legacy CodeBuddy catalog")
	}

}

// metadataProvider supplies deterministic account-specific catalogs without network calls.
type metadataProvider struct{}

func (metadataProvider) Info() providers.Info { return providers.Info{ID: "metadata-test"} }
func (metadataProvider) ListModels(region string) []providers.ModelInfo {
	if region == "first" {
		return []providers.ModelInfo{{ID: "shared", Multimodal: true, MaxInputTokens: 180000, PriceFactor: providers.ParsePriceFactor(0.5), IsFree: providers.ParseFreeFlag(false)}, {ID: "unknown"}}
	}
	return []providers.ModelInfo{{ID: "shared", Reasoning: true, MaxInputTokens: 600000, PriceFactor: providers.ParsePriceFactor(2.0)}, {ID: "other", Reasoning: true}}
}

func TestModelsCapabilityMetadata(t *testing.T) {
	providers.Register(metadataProvider{})
	store, err := storage.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	for _, acct := range []*storage.Account{
		{ID: "first", Provider: "metadata-test", Region: "first", Enabled: true},
		{ID: "second", Provider: "metadata-test", Region: "second", Enabled: true},
		{ID: "disabled", Provider: "workbuddy", Enabled: false},
	} {
		if err := store.UpsertAccount(acct); err != nil {
			t.Fatal(err)
		}
	}
	svc := New(store, nil, "")
	models, err := svc.Models()
	if err != nil {
		t.Fatal(err)
	}
	if len(models) != 3 {
		t.Fatalf("expected 3 unique enabled models: %+v", models)
	}
	byID := map[string]map[string]any{}
	for _, model := range models {
		byID[model["id"].(string)] = model
	}
	shared := byID["metadata-test/shared"]
	if shared["supports_images"] != true || shared["supports_reasoning"] != false || shared["max_input_tokens"] != 180000 {
		t.Fatalf("metadata must follow first account, not union: %+v", shared)
	}
	if shared["price_factor"] != 0.5 || shared["is_free"] != false {
		t.Fatalf("wrong account pricing: %+v", shared)
	}
	unknown := byID["metadata-test/unknown"]
	if _, ok := unknown["price_factor"]; ok {
		t.Fatal("unknown price must be omitted")
	}
	if _, ok := unknown["is_free"]; ok {
		t.Fatal("unknown free flag must be omitted")
	}
	if _, ok := unknown["max_input_tokens"]; ok {
		t.Fatalf("unknown window must be omitted: %+v", unknown)
	}
	if unknown["supports_images"] != false || unknown["supports_reasoning"] != false {
		t.Fatalf("unexpected unknown capabilities: %+v", unknown)
	}
	if byID["metadata-test/other"]["supports_reasoning"] != true {
		t.Fatal("lost reasoning metadata")
	}
}
