package proxy

import (
	"xiaotao/xt-client/internal/effort"
	"xiaotao/xt-client/internal/providers/trae"
)

// providerEffort translates a normalized level into the provider's native
// spelling. Unknown values pass through so provider-specific clients keep
// working; providers without a mapping receive the value untouched.
func providerEffort(provider, value string) string {
	if level, ok := effort.Parse(value); ok {
		switch provider {
		case "trae":
			return trae.NativeEffort(string(level))
		default:
			return string(level)
		}
	}
	return value
}
