package qoder

import "xiaotao/xt-client/internal/providers"

type Provider struct{}

func init() { providers.Register(Provider{}) }
func (Provider) Info() providers.Info {
	return providers.Info{ID: "qoder", DisplayName: "Qoder", LoginType: "web", Regions: []string{"cn", "global"}, Billing: "credits"}
}
func (Provider) ListModels(string) []providers.ModelInfo { return []providers.ModelInfo{} }
