package trae

import (
	"strings"

	"xiaotao/xt-client/internal/providers"
)

// detailMaxSuffix marks a model_detail_list entry that serves Max mode, the
// larger context window variant of the base model.
const detailMaxSuffix = "__max"

// IsMaxModel reports whether a resolved model id selects the Max variant.
func IsMaxModel(id string) bool {
	return strings.HasSuffix(strings.ToLower(strings.TrimSpace(id)), "-max")
}

// BaseModel strips the Max suffix from a resolved id.
func BaseModel(id string) string {
	if IsMaxModel(id) {
		return strings.TrimSuffix(strings.ToLower(strings.TrimSpace(id)), "-max")
	}
	return strings.ToLower(strings.TrimSpace(id))
}

// MaxWindow looks up the Max variant's context window from a catalog. The
// catalog stores the max window only on the "-max" variant entry.
func MaxWindow(baseID string, models []providers.ModelInfo) int64 {
	target := strings.ToLower(baseID) + "-max"
	for _, model := range models {
		if strings.EqualFold(model.ID, target) {
			return int64(model.MaxInputTokens)
		}
	}
	return 0
}
