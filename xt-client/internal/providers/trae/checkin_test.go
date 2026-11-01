package trae

import (
	"testing"

	"xiaotao/xt-client/internal/providers"
)

func TestDeriveDeviceStability(t *testing.T) {
	id1, b1, t1 := deriveDevice("test-user-1", 0)
	id2, b2, t2 := deriveDevice("test-user-1", 0)
	if id1 != id2 || b1 != b2 || t1 != t2 {
		t.Fatalf("deriveDevice not stable for same identity: (%s, %s, %s) vs (%s, %s, %s)", id1, b1, t1, id2, b2, t2)
	}
	if len(id1) != 16 {
		t.Fatalf("expected 16 digits, got %s", id1)
	}

	// Alt should change device id / model
	idAlt, _, _ := deriveDevice("test-user-1", 1)
	if idAlt == id1 {
		t.Fatalf("expected alt device id to differ from original")
	}
}

func TestTraeGlobalRegionCheckin(t *testing.T) {
	cred := &providers.Credential{
		AccessToken: "mock-token",
		Region:      "global",
	}
	st, err := FetchCheckinStatus(cred, "trae:global:123")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if st.Supported {
		t.Fatalf("global region should not be supported")
	}

	claim, err := ClaimCheckin(cred, "trae:global:123")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if claim.OK {
		t.Fatalf("global claim should not succeed")
	}
}
