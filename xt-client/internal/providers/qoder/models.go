package qoder

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"time"
	"xiaotao/xt-client/internal/providers"
)

func (Provider) FetchModels(ctx context.Context, cred *providers.Credential) ([]providers.ModelInfo, error) {
	reg, ok := Regions[cred.Region]
	if !ok {
		return nil, fmt.Errorf("unsupported Qoder region")
	}
	ctx, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()
	resp, err := (HTTPClient{}).SignedGet(ctx, reg, "/algo/api/v2/model/list?Encode=1", cred.UID, cred.AccessToken, cred.MachineID, SignOptions{})
	if err != nil {
		return nil, fmt.Errorf("Qoder model directory request failed")
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return nil, &providers.UpstreamError{Status: resp.StatusCode, Message: "Qoder model directory unavailable"}
	}
	raw, err := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
	if err != nil {
		return nil, err
	}
	return parseModels(raw)
}

var publicIDs = map[string]string{
	"qmodel_38max": "qwen3.8-max", "qfmodel": "qwen3.8-flash", "qmodel_latest": "qwen3.7-max", "qmodel": "qwen3.7-plus", "q37fmodel": "qwen3.7-flash",
	"dmodel": "deepseek-v4-pro", "dfmodel": "deepseek-v4.1-flash", "gmodel": "glm-5.3", "gfmodel": "glm-5.3-flash", "gm51model": "glm-5.2",
	"kmodel_latest": "kimi-k3.1-pro", "kmodel": "kimi-k2.8-preview", "mmodel": "minimax-m3.1-flash", "cmodel": "cantus", "smodel": "sonus",
}

func parseModels(raw []byte) ([]providers.ModelInfo, error) {
	var data map[string]json.RawMessage
	if err := json.Unmarshal(raw, &data); err != nil {
		return nil, fmt.Errorf("invalid Qoder model directory")
	}
	out := []providers.ModelInfo{}
	seen := map[string]bool{}
	valid := false
	for _, scene := range []string{"chat", "developer", "assistant", "app"} {
		encoded, ok := data[scene]
		if !ok {
			continue
		}
		var entries []struct {
			Key       string `json:"key"`
			Name      string `json:"display_name"`
			Enable    *bool  `json:"enable"`
			Source    string `json:"source"`
			Vision    bool   `json:"is_vl"`
			Reasoning bool   `json:"is_reasoning"`
			MaxInput  int    `json:"max_input_tokens"`
		}
		if json.Unmarshal(encoded, &entries) != nil {
			return nil, fmt.Errorf("invalid Qoder model scene")
		}
		valid = true
		for _, e := range entries {
			if e.Key == "" || seen[e.Key] || (e.Enable != nil && !*e.Enable) || (e.Source != "" && e.Source != "system") {
				continue
			}
			seen[e.Key] = true
			id := publicIDs[e.Key]
			if id == "" {
				name := strings.ToLower(strings.TrimSpace(e.Name))
				if name == "" {
					name = e.Key
				}
				name = strings.NewReplacer(" ", "-", "_", "-", "/", "-").Replace(name)
				hash := sha256.Sum256([]byte(e.Key))
				id = fmt.Sprintf("%s-%x", name, hash[:4])
			}
			out = append(out, providers.ModelInfo{ID: id, DisplayName: e.Name, UpstreamID: e.Key, Multimodal: e.Vision, Reasoning: e.Reasoning, MaxInputTokens: e.MaxInput})
		}
	}
	if !valid {
		return nil, fmt.Errorf("Qoder model directory unavailable")
	}
	return out, nil
}
