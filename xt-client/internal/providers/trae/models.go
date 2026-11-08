package trae

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"time"

	"xiaotao/xt-client/internal/effort"
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
// Successful catalogs replace the static fallback; hidden and custom entries are excluded.
func FetchUpstreamModels(cred *providers.Credential, machineID, deviceID string) ([]providers.ModelInfo, error) {
	r := RegionOf(cred.Region)
	body, _ := json.Marshal(map[string]any{
		"function": "chat_v3", "need_prompt": false, "poly_prompt": false,
	})
	head := headers(cred, machineID, deviceID)
	head["Accept"] = "application/json"
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, r.ChatBase+"/api/ide/v1/get_detail_param", bytes.NewReader(body))
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
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, &HTTPError{Code: resp.StatusCode}
	}
	return parseModelCatalog(raw)
}

func parseModelCatalog(raw []byte) ([]providers.ModelInfo, error) {
	var parsed struct {
		ConfigInfoList []struct {
			Name      string                `json:"config_name"`
			Switch    *bool                 `json:"config_switch"`
			Invisible *bool                 `json:"is_invisible_to_user"`
			Context   catalogContext        `json:"context_window_tokens"`
			Details   []modelDetail         `json:"model_detail_list"`
			Display   catalogDisplay        `json:"display_config"`
			Contact   json.RawMessage       `json:"display_contact_config"`
			Efforts   reasoningEffortConfig `json:"reasoning_effort_config"`
		} `json:"config_info_list"`
	}
	if err := json.Unmarshal(raw, &parsed); err != nil {
		return nil, err
	}
	out := []providers.ModelInfo{}
	seen := map[string]bool{}
	for _, e := range parsed.ConfigInfoList {
		id := lower(e.Name)
		internal := strings.HasPrefix(id, "custom_model") || id == "summary" || id == "fast_apply" || id == "fast_apply_new" || id == "title_generation" || id == "input_optimization"
		if e.Name == "" || internal || seen[id] || e.Switch != nil && !*e.Switch || e.Invisible != nil && *e.Invisible || e.Display.IsCustom || e.Display.Builder {
			continue
		}
		seen[id] = true
		info := providers.ModelInfo{ID: id, UpstreamID: e.Name, DisplayName: orDefault(e.Display.Name, e.Name), Multimodal: e.Display.Multi, PriceFactor: catalogPriceFactor(e.Contact)}
		info.MaxInputTokens = int(e.Context.Dev)
		for _, raw := range e.Efforts.Options {
			if level, ok := effort.Parse(raw); ok && !containsLevel(info.Efforts, string(level)) {
				info.Efforts = append(info.Efforts, string(level))
				info.Reasoning = true
			}
		}
		out = append(out, info)
		for _, detail := range e.Details {
			if detail.Name == "" || !strings.HasSuffix(detail.Name, detailMaxSuffix) {
				continue
			}
			if e.Context.Max <= e.Context.Dev {
				continue
			}
			var price *float64
			if info.PriceFactor != nil {
				value := *info.PriceFactor
				price = &value
			}
			maxInfo := providers.ModelInfo{
				ID:             id + "-max",
				UpstreamID:     detail.Name,
				DisplayName:    orDefault(e.Display.Name, e.Name) + " (Max)",
				Multimodal:     e.Display.Multi,
				PriceFactor:    price,
				MaxInputTokens: int(e.Context.Max),
				Reasoning:      info.Reasoning,
				Efforts:        info.Efforts,
			}
			out = append(out, maxInfo)
			break
		}
	}
	if len(out) == 0 {
		return nil, ErrEmptyCatalog
	}
	return out, nil
}

type reasoningEffortConfig struct {
	Options []string `json:"options"`
}

type catalogContext struct {
	Dev int64 `json:"dev"`
	Max int64 `json:"max"`
}

type modelDetail struct {
	Name      string `json:"model_name"`
	MaxTokens int64  `json:"max_tokens"`
}

func containsLevel(list []string, value string) bool {
	for _, item := range list {
		if item == value {
			return true
		}
	}
	return false
}

type catalogDisplay struct {
	IsCustom bool   `json:"is_custom_model"`
	Builder  bool   `json:"builder"`
	Name     string `json:"display_name"`
	Multi    bool   `json:"multimodal"`
}

func (d *catalogDisplay) UnmarshalJSON(raw []byte) error {
	if len(raw) > 0 && raw[0] == '"' {
		var value string
		if err := json.Unmarshal(raw, &value); err != nil {
			return err
		}
		raw = []byte(value)
	}
	type plain catalogDisplay
	// Malformed optional display metadata must not discard the entire catalog.
	_ = json.Unmarshal(raw, (*plain)(d))
	return nil
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

// UpstreamModel preserves curated casing when the live catalog is unavailable.
func UpstreamModel(id string) string {
	aliases := map[string]string{
		"deepseek-v4-pro":       "DeepSeek-V4-Pro",
		"doubao-seed-evolving":  "Doubao-Seed-Evolving",
		"doubao-seed-2.1-pro":   "Doubao-Seed-2.1-Pro",
		"doubao-seed-2.1-turbo": "Doubao-Seed-2.1-Turbo",
		"doubao-seed-code":      "Doubao-Seed-Code",
	}
	if name := aliases[id]; name != "" {
		return name
	}
	return id
}

// NativeEffort translates a normalized level into Trae's spelling. Models
// without a matching entry reject the field, so callers only send levels the
// catalog declared.
func NativeEffort(level string) string {
	switch effort.Level(level) {
	case effort.Low:
		return "light"
	case effort.XHigh:
		return "extra_high"
	default:
		return level
	}
}

func catalogPriceFactor(raw json.RawMessage) *float64 {
	if len(raw) > 0 && raw[0] == '"' {
		var text string
		if json.Unmarshal(raw, &text) != nil {
			return nil
		}
		raw = []byte(text)
	}
	var contact struct {
		ConsumptionRate struct {
			Data struct {
				Rate any `json:"rate"`
			} `json:"data"`
		} `json:"consumption_rate"`
	}
	if json.Unmarshal(raw, &contact) != nil {
		return nil
	}
	return providers.ParsePriceFactor(contact.ConsumptionRate.Data.Rate)
}
