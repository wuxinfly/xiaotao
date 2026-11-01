package codex

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sort"
	"strings"
	"time"

	"xiaotao/xt-client/internal/providers"
)

const (
	CatalogClientVersion = "0.159.0"
)

var defaultModels = []providers.ModelInfo{
	{
		ID:             "gpt-6.1-sol",
		DisplayName:    "GPT-6.1 Sol — Codex 主力旗舰",
		Reasoning:      true,
		Multimodal:     true,
		MaxInputTokens: 500000,
		UpstreamID:     "gpt-6.1-sol",
	},
	{
		ID:             "gpt-6-astra",
		DisplayName:    "GPT-6 Astra — 深度推理设计",
		Reasoning:      true,
		Multimodal:     true,
		MaxInputTokens: 272000,
		UpstreamID:     "gpt-6-astra",
	},
	{
		ID:             "gpt-6-sol",
		DisplayName:    "GPT-6 Sol — 上一代主力",
		Reasoning:      true,
		Multimodal:     true,
		MaxInputTokens: 272000,
		UpstreamID:     "gpt-6-sol",
	},
	{
		ID:             "gpt-6-luna",
		DisplayName:    "GPT-6 Luna — 轻量快速",
		Reasoning:      false,
		Multimodal:     true,
		MaxInputTokens: 272000,
		UpstreamID:     "gpt-6-luna",
	},
	{
		ID:             "gpt-5.6-sol",
		DisplayName:    "GPT-5.6 Sol — 前沿编码研究",
		Reasoning:      true,
		Multimodal:     true,
		MaxInputTokens: 272000,
		UpstreamID:     "gpt-5.6-sol",
	},
	{
		ID:             "gpt-5.6-terra",
		DisplayName:    "GPT-5.6 Terra — 均衡编码",
		Reasoning:      false,
		Multimodal:     true,
		MaxInputTokens: 272000,
		UpstreamID:     "gpt-5.6-terra",
	},
	{
		ID:             "gpt-5.6-luna",
		DisplayName:    "GPT-5.6 Luna — 极速响应",
		Reasoning:      false,
		Multimodal:     true,
		MaxInputTokens: 272000,
		UpstreamID:     "gpt-5.6-luna",
	},
}

func DefaultModels() []providers.ModelInfo {
	out := make([]providers.ModelInfo, len(defaultModels))
	copy(out, defaultModels)
	return out
}

var localNotes = map[string]string{
	"gpt-6.1-sol":   "GPT-6.1 Sol — Codex 主力旗舰",
	"gpt-6-astra":   "GPT-6 Astra — 深度推理设计",
	"gpt-6-sol":     "GPT-6 Sol — 上一代主力",
	"gpt-6-luna":    "GPT-6 Luna — 轻量快速",
	"gpt-5.6-sol":   "GPT-5.6 Sol — 前沿编码研究",
	"gpt-5.6-terra": "GPT-5.6 Terra — 均衡编码",
	"gpt-5.6-luna":  "GPT-5.6 Luna — 极速响应",
}

type upstreamModelItem struct {
	Slug            string   `json:"slug"`
	ID              string   `json:"id"`
	Visibility      string   `json:"visibility"`
	SupportedInAPI  *bool    `json:"supported_in_api"`
	DisplayName     string   `json:"display_name"`
	Description     string   `json:"description"`
	ContextWindow   int      `json:"context_window"`
	InputModalities []string `json:"input_modalities"`
}

func ParseCatalogPayload(raw []byte) ([]providers.ModelInfo, error) {
	var payload struct {
		Models []upstreamModelItem `json:"models"`
	}
	if err := json.Unmarshal(raw, &payload); err != nil {
		return nil, fmt.Errorf("解析模型目录失败: %w", err)
	}

	var out []providers.ModelInfo
	seen := map[string]bool{}
	for _, item := range payload.Models {
		slug := strings.TrimSpace(item.Slug)
		if slug == "" {
			slug = strings.TrimSpace(item.ID)
		}
		if slug == "" || seen[slug] {
			continue
		}
		vis := item.Visibility
		if vis == "" {
			vis = "list"
		}
		if vis != "list" {
			continue
		}
		if item.SupportedInAPI != nil && !*item.SupportedInAPI {
			continue
		}
		seen[slug] = true

		displayName := localNotes[slug]
		if displayName == "" {
			if item.DisplayName != "" {
				displayName = item.DisplayName
				if item.Description != "" {
					displayName = displayName + " — " + item.Description
				}
			} else {
				displayName = slug
			}
		}

		multimodal := false
		for _, m := range item.InputModalities {
			if m == "image" {
				multimodal = true
				break
			}
		}

		reasoning := strings.Contains(slug, "astra") || strings.Contains(slug, "sol")

		ctxWin := item.ContextWindow
		if ctxWin <= 0 {
			ctxWin = 272000
		}

		out = append(out, providers.ModelInfo{
			ID:             slug,
			DisplayName:    displayName,
			Reasoning:      reasoning,
			Multimodal:     multimodal,
			MaxInputTokens: ctxWin,
			UpstreamID:     slug,
		})
	}

	if len(out) == 0 {
		return nil, fmt.Errorf("上游未返回可用模型")
	}

	sort.SliceStable(out, func(i, j int) bool {
		if out[i].ID == "gpt-6.1-sol" {
			return true
		}
		if out[j].ID == "gpt-6.1-sol" {
			return false
		}
		return i < j
	})

	return out, nil
}

func FetchCatalog(ctx context.Context, cred *providers.Credential) ([]providers.ModelInfo, error) {
	if cred == nil || cred.AccessToken == "" {
		return nil, fmt.Errorf("missing access token")
	}
	url := fmt.Sprintf("%s/models?client_version=%s", normalizeBaseURL(""), CatalogClientVersion)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	accountID := cred.EnterpriseID
	for k, v := range buildHeaders(cred.AccessToken, accountID, "", false) {
		req.Header.Set(k, v)
	}

	client := &http.Client{Timeout: 15 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("请求上游模型目录失败: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("上游模型目录接口返回 HTTP %d", resp.StatusCode)
	}

	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return nil, err
	}
	return ParseCatalogPayload(body)
}
