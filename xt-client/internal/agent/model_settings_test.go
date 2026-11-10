package agent

import "testing"

func TestManualSourceDefaultsAreSnapshotted(t *testing.T) {
	option := ModelOption{Model: "model_stable", ContextWindow: 64000, MaxOutputTokens: 2048}
	svc := NewService(t.TempDir(), "http://127.0.0.1:1", nil, nil, func() []ModelOption { return []ModelOption{option} })
	session, err := svc.Create(Options{Model: option.Model})
	if err != nil {
		t.Fatal(err)
	}
	if session.ModelSettings == nil || session.ModelSettings.ContextWindow != 64000 || session.ModelSettings.MaxOutputTokens != 2048 {
		t.Fatalf("source defaults missing: %+v", session.ModelSettings)
	}
	option.ContextWindow, option.MaxOutputTokens = 128000, 4096
	saved, err := svc.Get(session.ID)
	if err != nil {
		t.Fatal(err)
	}
	if saved.ModelSettings.ContextWindow != 64000 || saved.ModelSettings.MaxOutputTokens != 2048 {
		t.Fatalf("old snapshot changed: %+v", saved.ModelSettings)
	}
	next, err := svc.Create(Options{Model: option.Model, ModelSettings: &ModelSettings{ContextWindow: 32000}})
	if err != nil {
		t.Fatal(err)
	}
	if next.ModelSettings.ContextWindow != 32000 || next.ModelSettings.MaxOutputTokens != 4096 {
		t.Fatalf("partial overrides lost source output default: %+v", next.ModelSettings)
	}
}

func TestManualSourceDefaultOutputValidatedAgainstOverride(t *testing.T) {
	_, err := normalizeModelSettings(&ModelSettings{ContextWindow: 4096}, &ModelSettings{ContextWindow: 64000, MaxOutputTokens: 8192})
	if err == nil {
		t.Fatal("source default output exceeds overridden context but was accepted")
	}
}
