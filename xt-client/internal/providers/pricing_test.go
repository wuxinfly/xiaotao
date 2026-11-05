package providers

import (
	"encoding/json"
	"math"
	"testing"
)

func TestParsePriceFactor(t *testing.T) {
	for _, v := range []any{nil, true, -1.0, "NaN", "Infinity", "", map[string]any{}, math.NaN(), math.Inf(1)} {
		if ParsePriceFactor(v) != nil {
			t.Fatalf("accepted %v", v)
		}
	}
	for _, v := range []any{0.0, "0", json.Number("0")} {
		if p := ParsePriceFactor(v); p == nil || *p != 0 {
			t.Fatalf("lost zero %v", v)
		}
	}
}
