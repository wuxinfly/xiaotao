package codex

import (
	"testing"
)

func TestParseUsagePayloadWindowFormat(t *testing.T) {
	// Primary window + Secondary window + Credits
	raw := []byte(`{
		"plan_type": "team",
		"rate_limit": {
			"allowed": true,
			"limit_reached": false,
			"primary_window": {
				"used_percent": 12.5,
				"limit_window_seconds": 18000,
				"reset_at": 1792477637
			},
			"secondary_window": {
				"used_percent": 40.0,
				"limit_window_seconds": 604800,
				"reset_at": 1792940980
			}
		},
		"credits": {
			"has_credits": true,
			"unlimited": false,
			"balance": 25.5
		}
	}`)

	snap, err := ParseUsagePayload(raw)
	if err != nil {
		t.Fatalf("ParseUsagePayload failed: %v", err)
	}
	if snap.Status != "ok" || len(snap.Items) != 3 {
		t.Fatalf("unexpected snapshot items count: %d, %+v", len(snap.Items), snap.Items)
	}

	// Item 0: 5 小时窗口 (18000s = 300m)
	item0 := snap.Items[0]
	if item0.Label != "5 小时窗口" {
		t.Fatalf("item 0 label mismatch: %s", item0.Label)
	}
	if *item0.Used != 12.5 || *item0.Remaining != 87.5 || *item0.Total != 100.0 {
		t.Fatalf("item 0 used/remaining mismatch: used=%v, remaining=%v", *item0.Used, *item0.Remaining)
	}
	if item0.ExpireAt != 1792477637 {
		t.Fatalf("item 0 expireAt mismatch: %d", item0.ExpireAt)
	}

	// Item 1: 周窗口 (604800s = 10080m)
	item1 := snap.Items[1]
	if item1.Label != "周窗口" {
		t.Fatalf("item 1 label mismatch: %s", item1.Label)
	}
	if *item1.Used != 40.0 || *item1.Remaining != 60.0 {
		t.Fatalf("item 1 used/remaining mismatch: used=%v, remaining=%v", *item1.Used, *item1.Remaining)
	}
	if item1.ExpireAt != 1792940980 {
		t.Fatalf("item 1 expireAt mismatch: %d", item1.ExpireAt)
	}

	// Item 2: Credits
	item2 := snap.Items[2]
	if item2.Label != "附加额度" || *item2.Remaining != 25.5 {
		t.Fatalf("item 2 credits mismatch: %+v", item2)
	}
}

func TestParseUsagePayloadLegacyMinutesFormat(t *testing.T) {
	raw := []byte(`{
		"rate_limits": {
			"primary": {
				"used_percent": 20,
				"window_minutes": 300,
				"reset_after_seconds": 3600
			}
		}
	}`)

	snap, err := ParseUsagePayload(raw)
	if err != nil {
		t.Fatalf("ParseUsagePayload failed: %v", err)
	}
	if len(snap.Items) != 1 {
		t.Fatalf("expected 1 item, got %d", len(snap.Items))
	}
	item := snap.Items[0]
	if item.Label != "5 小时窗口" {
		t.Fatalf("expected 5 小时窗口, got %s", item.Label)
	}
	if *item.Remaining != 80.0 {
		t.Fatalf("expected remaining 80.0, got %v", *item.Remaining)
	}
	if item.ExpireAt <= 0 {
		t.Fatalf("expected non-zero expireAt calculated from reset_after_seconds")
	}
}
