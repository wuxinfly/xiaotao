package trae

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"

	"xiaotao/xt-client/internal/providers"
)

type Provider struct{}

func (Provider) Info() providers.Info {
	return providers.Info{
		ID:          "trae",
		DisplayName: "Trae 国内版",
		LoginType:   "web",
		Regions:     []string{"cn"},
		Billing:     "credit",
	}
}

func (Provider) ListModels(region string) []providers.ModelInfo {
	return []providers.ModelInfo{
		{ID: "glm-5.3", DisplayName: "GLM-5.3"},
		{ID: "glm-5.3-flash", DisplayName: "GLM-5.3 Flash"},
		{ID: "glm-5.3-flashx", DisplayName: "GLM-5.3 FlashX"},
		{ID: "deepseek-v4.1-flash", DisplayName: "DeepSeek V4.1 Flash", Multimodal: true},
		{ID: "doubao-seed-evolving"},
		{ID: "kimi-k3"},
		{ID: "doubao-seed-2.1-pro"},
		{ID: "deepseek-v4-pro"},
		{ID: "qwen3.8-max"},
		{ID: "doubao-seed-2.1-turbo"},
		{ID: "minimax-m3"},
		{ID: "kimi-k2.6"},
		{ID: "glm-5.1"},
		{ID: "step-5-preview"},
		{ID: "doubao-seed-code"},
		{ID: "mimo-v2.6-flash"},
		{ID: "mimo-v2.6-pro"},
		{ID: "qwen-3.7-plus"},
		{ID: "kimi-k2.8-preview"},
		{ID: "qwen3.8-flash"},
		{ID: "kimi-k3.1-pro"},
		{ID: "minimax-m3.1-flash"},
		{ID: "glm-5.2"},
	}
}

// FetchUpstreamModels pulls the live CN catalog for one account credential.
// The proxy serves the union with static list; upstream names map through
// lowercase IDs, and only known non-custom entries are included.
func FetchUpstreamModels(cred *providers.Credential, machineID, deviceID string) ([]providers.ModelInfo, error) {
	r := RegionOf(cred.Region)
	body, _ := json.Marshal(map[string]any{
		"function": "chat_v3", "need_prompt": false, "poly_prompt": false,
	})
	head := headers(cred, machineID, deviceID)
	head["Accept"] = "application/json"
	req, err := http.NewRequest(http.MethodPost, r.ChatBase+"/api/ide/v1/get_detail_param", bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	for k, v := range head {
		req.Header.Set(k, v)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(resp.Body, 1<<21))
	if err != nil {
		return nil, err
	}
	if resp.StatusCode >= 400 {
		return nil, &HTTPError{Code: resp.StatusCode}
	}
	var parsed struct {
		ConfigInfoList []struct {
			Name      string `json:"config_name"`
			Switch    *bool  `json:"config_switch"`
			Invisible *bool  `json:"is_invisible_to_user"`
			Context   any    `json:"context_window_tokens"`
			Display   struct {
				IsCustom bool   `json:"is_custom_model"`
				Builder  bool   `json:"builder"`
				Name     string `json:"display_name"`
				Multi    bool   `json:"multimodal"`
			} `json:"display_config"`
		} `json:"config_info_list"`
	}
	if err := json.Unmarshal(raw, &parsed); err != nil {
		return nil, err
	}
	out := []providers.ModelInfo{}
	for _, e := range parsed.ConfigInfoList {
		if e.Name == "" || e.Switch != nil && !*e.Switch || e.Invisible != nil && *e.Invisible || e.Display.IsCustom || e.Display.Builder {
			continue
		}
		out = append(out, providers.ModelInfo{ID: lower(e.Name), DisplayName: orDefault(e.Display.Name, e.Name), Multimodal: e.Display.Multi})
	}
	if len(out) == 0 {
		return nil, ErrEmptyCatalog
	}
	return out, nil
}

func lower(s string) string {
	b := []byte(s)
	for i := range b {
		if b[i] >= 'A' && b[i] <= 'Z' {
			b[i] += 32
		}
	}
	return string(b)
}

func orDefault(a, b string) string {
	if a == "" {
		return b
	}
	return a
}
