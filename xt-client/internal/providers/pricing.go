package providers

import (
	"encoding/json"
	"math"
	"strconv"
	"strings"
)

// ParsePriceFactor preserves explicit zero and leaves malformed or unknown prices absent.
func ParsePriceFactor(value any) *float64 {
	var n float64
	switch v := value.(type) {
	case float64:
		n = v
	case json.Number:
		parsed, err := v.Float64()
		if err != nil {
			return nil
		}
		n = parsed
	case string:
		parsed, err := strconv.ParseFloat(strings.TrimSpace(v), 64)
		if err != nil {
			return nil
		}
		n = parsed
	default:
		return nil
	}
	if n < 0 || math.IsNaN(n) || math.IsInf(n, 0) {
		return nil
	}
	return &n
}
func ParseFreeFlag(value any) *bool {
	if v, ok := value.(bool); ok {
		return &v
	}
	return nil
}
