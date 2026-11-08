package trae

import "testing"

func TestCatalogStringMetadataAndFiltering(t *testing.T) {
	models, err := parseModelCatalog([]byte(`{"config_info_list":[{"config_name":"Doubao-Seed-Code","display_config":"{\"display_name\":\"Seed Code\",\"multimodal\":true}"},{"config_name":"summary"},{"config_name":"fast_apply"},{"config_name":"fast_apply_new"},{"config_name":"title_generation"},{"config_name":"input_optimization"},{"config_name":"custom_model_1"},{"config_name":"private","display_config":{"builder":true}},{"config_name":"off","config_switch":false},{"config_name":"hidden","is_invisible_to_user":true}]}`))
	if err != nil {
		t.Fatal(err)
	}
	if len(models) != 1 || models[0].ID != "doubao-seed-code" || models[0].UpstreamID != "Doubao-Seed-Code" || models[0].DisplayName != "Seed Code" || !models[0].Multimodal {
		t.Fatalf("invalid catalog: %+v", models)
	}
}

func TestCatalogWindowsAndMaxVariant(t *testing.T) {
	models, err := parseModelCatalog([]byte(`{"config_info_list":[{"config_name":"DeepSeek-V4-Flash","context_window_tokens":{"dev":262144,"max":1048576},"model_detail_list":[{"model_name":"DeepSeek-V4-Flash__dev","max_tokens":16384},{"model_name":"DeepSeek-V4-Flash__max","max_tokens":16384}],"display_config":{"display_name":"DeepSeek V4.1 Flash"}},{"config_name":"Plain","context_window_tokens":{"dev":131072}}]}`))
	if err != nil {
		t.Fatal(err)
	}
	if len(models) != 3 {
		t.Fatalf("expected base, max, and plain models, got %d: %+v", len(models), models)
	}
	base := models[0]
	if base.ID != "deepseek-v4-flash" || base.MaxInputTokens != 262144 {
		t.Fatalf("base window lost: %+v", base)
	}
	max := models[1]
	if max.ID != "deepseek-v4-flash-max" || max.UpstreamID != "DeepSeek-V4-Flash__max" || max.MaxInputTokens != 1048576 {
		t.Fatalf("max variant wrong: %+v", max)
	}
	if max.DisplayName != "DeepSeek V4.1 Flash (Max)" {
		t.Fatalf("max display name wrong: %q", max.DisplayName)
	}
	if models[2].ID != "plain" || models[2].MaxInputTokens != 131072 {
		t.Fatalf("plain window wrong: %+v", models[2])
	}
}

func TestCatalogSkipsMaxWhenWindowNotLarger(t *testing.T) {
	models, err := parseModelCatalog([]byte(`{"config_info_list":[{"config_name":"Model","context_window_tokens":{"dev":262144,"max":262144},"model_detail_list":[{"model_name":"Model__max","max_tokens":16384}]}]}`))
	if err != nil || len(models) != 1 || models[0].ID != "model" {
		t.Fatalf("max variant should not exist without a bigger window: %v %+v", err, models)
	}
}
