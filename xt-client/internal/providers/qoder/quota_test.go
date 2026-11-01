package qoder

import "testing"

func TestQuotaPackagesAndExpiry(t *testing.T) {
	snap, err := parseQuota([]byte(`{"expiresAt":1800000000000,"userQuota":{"used":20,"total":100,"remaining":80,"percentage":0.8},"addOnQuota":{"total":50},"dedicatedResourcePackages":[{"name":"event","used":1,"total":10,"remaining":9,"expiresAt":1795000000000,"displayLabels":[{"dimension":"title","valueI18n":{"zh-CN":"活动积分"}}]},{"name":"expired","available":false},{"status":"QUOTA_DETAIL_STATUS_EXPIRED"}]}`))
	if err != nil {
		t.Fatal(err)
	}
	if len(snap.Items) != 3 || *snap.Items[0].Used != 20 || snap.Items[0].ExpireAt != 1800000000 || snap.Items[1].Used != nil || snap.Items[1].Remaining != nil || snap.Items[2].Label != "活动积分" || snap.Items[2].ExpireAt != 1795000000 {
		t.Fatalf("incorrect quota %#v", snap)
	}
	if _, err := parseQuota([]byte(`{"error":"failed"}`)); err == nil {
		t.Fatal("accepted invalid quota")
	}
}
