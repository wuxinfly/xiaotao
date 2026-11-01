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
