package sourceplugin

import "testing"

func TestSettingsDefaultsAndValidation(t *testing.T) {
	entry := Saved{Manifest: Manifest{Settings: []SettingField{
		{Key: "region", Label: "区域", Default: "cn", Options: []string{"cn", "global"}},
		{Key: "note", Label: "备注"},
	}}}
	values := entry.Settings()
	if values["region"] != "cn" || values["note"] != "" {
		t.Fatalf("defaults missing: %+v", values)
	}
	entry.SettingValues = map[string]string{"region": "global", "unknown": "x"}
	if got := entry.Settings()["region"]; got != "global" {
		t.Fatalf("declared value lost: %q", got)
	}
	if _, ok := entry.Settings()["unknown"]; ok {
		t.Fatal("undeclared value leaked")
	}
}
