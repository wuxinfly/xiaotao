package workbuddy

import (
	"xiaotao/xt-client/internal/providers"
)

type Provider struct{}

func (Provider) Info() providers.Info {
	return providers.Info{
		ID:          "workbuddy",
		DisplayName: "WorkBuddy",
		LoginType:   "browser",
		Regions:     []string{"cn"},
		Billing:     "credit",
	}
}

func (Provider) ListModels(region string) []providers.ModelInfo {
	_ = region
	return Models
}
