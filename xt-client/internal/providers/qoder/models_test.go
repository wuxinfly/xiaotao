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
