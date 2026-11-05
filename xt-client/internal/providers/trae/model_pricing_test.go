package trae

import "testing"

func TestCatalogPricing(t *testing.T) {
	raw := []byte(`{"config_info_list":[{"config_name":"paid","display_contact_config":{"consumption_rate":{"data":{"rate":0.31}}}},{"config_name":"zero","display_contact_config":"{\"consumption_rate\":{\"data\":{\"rate\":\"0\"}}}"},{"config_name":"unknown"},{"config_name":"bad","display_contact_config":{"consumption_rate":{"data":{"rate":-1}}}}]}`)
	models, err := parseModelCatalog(raw)
	if err != nil || len(models) != 4 {
		t.Fatalf("models=%v err=%v", models, err)
	}
	if models[0].PriceFactor == nil || *models[0].PriceFactor != 0.31 {
		t.Fatal("lost object rate")
	}
	if models[1].PriceFactor == nil || *models[1].PriceFactor != 0 {
		t.Fatal("lost string/zero rate")
	}
	if models[2].PriceFactor != nil || models[3].PriceFactor != nil {
		t.Fatal("invented pricing")
	}
}
