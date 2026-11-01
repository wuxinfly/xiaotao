package workbuddy

import "xiaotao/xt-client/internal/providers"

// Curated CodeBuddy CN catalog, synced from buddy-proxy on 2026-11-01.
var Models = []providers.ModelInfo{
	{ID: "auto", DisplayName: "Auto", Multimodal: true},
	{ID: "kimi-k3", DisplayName: "Kimi-K3", Multimodal: false},
	{ID: "kimi-k3.1-pro", DisplayName: "Kimi-K3.1-Pro", Multimodal: false},
	{ID: "mimo-v2.6-pro", DisplayName: "Xiaomi MiMo-V2.6-Pro", Multimodal: true},
	{ID: "hy4-preview", DisplayName: "Hy4 preview", Multimodal: false},
	{ID: "glm-5.3", DisplayName: "GLM-5.3", Multimodal: false},
	{ID: "glm-5.3-flash", DisplayName: "GLM-5.3-Flash", Multimodal: true},
	{ID: "glm-5.3-flashx", DisplayName: "GLM-5.3-FlashX", Multimodal: false},
	{ID: "glm-5.2", DisplayName: "GLM-5.2", Multimodal: false},
	{ID: "glm-5.1", DisplayName: "GLM-5.1", Multimodal: false},
	{ID: "glm-5v-turbo", DisplayName: "GLM-5v-Turbo", Multimodal: true},
	{ID: "hy3", DisplayName: "Hy3", Multimodal: true},
	{ID: "default", DisplayName: "Default", Multimodal: false},
	{ID: "minimax-m3", DisplayName: "MiniMax-M3", Multimodal: true},
	{ID: "minimax-m3.1-flash", DisplayName: "MiniMax-M3.1-Flash", Multimodal: true},
	{ID: "kimi-k2.7", DisplayName: "Kimi-K2.7-Code", Multimodal: true},
	{ID: "kimi-k2.8-preview", DisplayName: "Kimi-K2.8-Preview", Multimodal: true},
	{ID: "kimi-k2.6", DisplayName: "Kimi-K2.6", Multimodal: true},
	{ID: "deepseek-v4.1-flash", DisplayName: "Deepseek-V4.1-Flash", Multimodal: true},
	{ID: "space-bunny", DisplayName: "Space-Bunny", Multimodal: true},
	{ID: "step-5-preview", DisplayName: "Step-5-Preview", Multimodal: false},
	{ID: "deepseek-v4-flash", DisplayName: "Deepseek-V4-Flash", Multimodal: false},
	{ID: "deepseek-v4-pro", DisplayName: "Deepseek-V4-Pro", Multimodal: false},
}
