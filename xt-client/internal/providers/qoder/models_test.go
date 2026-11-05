package qoder

import "testing"

func TestParseModels(t *testing.T) {
	models, err := parseModels([]byte(`{"chat":[{"key":"qmodel_38max","display_name":"Qwen3.8-Max","is_vl":true,"is_reasoning":true},{"key":"off","enable":false},{"key":"custom","source":"byok"},{"key":"new1","display_name":"New"},{"key":"new2","display_name":"New"}],"assistant":[{"key":"qmodel_38max"}]}`))
	if err != nil {
		t.Fatal(err)
	}
	if len(models) != 3 || models[0].ID != "qwen3.8-max" || models[0].UpstreamID != "qmodel_38max" || !models[0].Multimodal || !models[0].Reasoning || models[1].ID == models[2].ID {
		t.Fatalf("bad directory: %#v", models)
	}
	if _, err := parseModels([]byte(`{"error":"denied"}`)); err == nil {
		t.Fatal("accepted invalid catalog")
	}
}

func TestModelPricing(t *testing.T) {
	models, err := parseModels([]byte(`{"chat":[{"key":"paid","price_factor":0.5,"is_free":false},{"key":"free","price_factor":0.1,"is_free":true},{"key":"zero","price_factor":0},{"key":"unknown"},{"key":"bad","price_factor":"NaN","is_free":"yes"}]}`))
	if err != nil || len(models) != 5 {
		t.Fatalf("models=%v err=%v", models, err)
	}
	if models[0].PriceFactor == nil || *models[0].PriceFactor != 0.5 || models[0].IsFree == nil || *models[0].IsFree {
		t.Fatal("lost paid pricing")
	}
	if models[1].IsFree == nil || !*models[1].IsFree || *models[1].PriceFactor != 0.1 {
		t.Fatal("lost free promotion")
	}
	if models[2].PriceFactor == nil || *models[2].PriceFactor != 0 {
		t.Fatal("lost explicit zero")
	}
	for _, i := range []int{3, 4} {
		if models[i].PriceFactor != nil || models[i].IsFree != nil {
			t.Fatal("invented pricing")
		}
	}
}
