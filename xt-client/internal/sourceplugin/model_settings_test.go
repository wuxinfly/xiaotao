package sourceplugin

import (
	"errors"
	"testing"

	"xiaotao/xt-client/internal/modelsource"
)

func TestModelSettingsControlRoutesAfterRestart(t *testing.T) {
	dir := t.TempDir()
	manager, err := Open(dir, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	const id = "test.models"
	manager.entries = []Saved{{
		ID: id, Enabled: true, Manifest: Manifest{ID: id},
		Models: []Model{{Key: "one", UpstreamID: "Model-One"}, {Key: "two", UpstreamID: "Model-Two"}},
	}}
	if len(manager.Sources()[0].Models) != 2 {
		t.Fatal("existing installations must keep all models available")
	}
	settings := ModelSettings{SelectedKeys: []string{"one", "manual:org/New-Model"}, CustomIDs: []string{"org/New-Model"}}
	if err := manager.SaveModelSettings(id, settings); err != nil {
		t.Fatal(err)
	}
	if err := manager.SaveModelSettings(id, ModelSettings{SelectedKeys: []string{"unknown"}}); err == nil {
		t.Fatal("saved an unknown selection")
	}
	manager, err = Open(dir, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := manager.Resolve(route(id, "two")); !errors.Is(err, modelsource.ErrNotFound) {
		t.Fatalf("deselected model still resolves: %v", err)
	}
	for key, upstream := range map[string]string{"one": "Model-One", "manual:org/New-Model": "org/New-Model"} {
		_, model, err := manager.Resolve(route(id, key))
		if err != nil || model.UpstreamModelID != upstream {
			t.Fatalf("%s route: %+v, %v", key, model, err)
		}
	}
	if err := manager.SaveModelSettings(id, ModelSettings{SelectedKeys: []string{}}); err != nil {
		t.Fatal(err)
	}
	manager, err = Open(dir, nil, nil)
	if err != nil || len(manager.Sources()[0].Models) != 0 {
		t.Fatalf("explicit empty selection must survive restart: %v", err)
	}
}

func TestReadablePluginRoutesPreserveIdentityAndSelection(t *testing.T) {
	m, err := Open(t.TempDir(), nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	m.entries = []Saved{{ID: "test.models", Manifest: Manifest{ID: "test.models"}, Enabled: true,
		Models: []Model{{Key: "org/key /+%", UpstreamID: "actual-upstream"}}}}
	model := m.Sources()[0].Models[0]
	if model.CallID != "test.models/org%2Fkey%20%2F%2B%25" {
		t.Fatal(model.CallID)
	}
	for _, id := range []string{model.ID, model.CallID} {
		_, resolved, err := m.Resolve(id)
		if err != nil || resolved.ID != model.ID || resolved.UpstreamModelID != "actual-upstream" {
			t.Fatal(resolved, err)
		}
	}
	if err := m.SaveModelSettings("test.models", ModelSettings{SelectedKeys: []string{}}); err != nil {
		t.Fatal(err)
	}
	for _, id := range []string{model.ID, model.CallID} {
		if _, _, err := m.Resolve(id); !errors.Is(err, modelsource.ErrNotFound) {
			t.Fatal("deselected alias resolves", err)
		}
	}
}
