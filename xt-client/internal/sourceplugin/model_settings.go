package sourceplugin

import (
	"errors"
	"slices"
	"strings"
	"unicode"
)

// A missing settings record exposes the whole discovered catalog for existing installations.
type ModelSettings struct {
	SelectedKeys []string `json:"selectedKeys"`
	CustomIDs    []string `json:"customIds"`
}

func customModel(id string) Model {
	return Model{Key: "manual:" + id, Label: id, UpstreamID: id}
}

func (e Saved) selectedModels() []Model {
	if e.ModelSettings == nil {
		return e.Models
	}
	keys := make(map[string]bool, len(e.ModelSettings.SelectedKeys))
	for _, key := range e.ModelSettings.SelectedKeys {
		keys[key] = true
	}
	out := []Model{}
	for _, model := range e.Models {
		if keys[model.Key] {
			out = append(out, model)
			delete(keys, model.Key)
		}
	}
	for _, id := range e.ModelSettings.CustomIDs {
		model := customModel(id)
		if keys[model.Key] {
			out = append(out, model)
		}
	}
	return out
}

func (m *Manager) SaveModelSettings(id string, settings ModelSettings) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	for i, saved := range m.entries {
		if saved.ID != id {
			continue
		}
		if err := validateModelSettings(saved, settings); err != nil {
			return err
		}
		settings.SelectedKeys = slices.Clone(settings.SelectedKeys)
		settings.CustomIDs = slices.Clone(settings.CustomIDs)
		m.entries[i].ModelSettings = &settings
		if err := m.persist(); err != nil {
			m.entries[i] = saved
			return errors.New("无法保存模型设置")
		}
		return nil
	}
	return errors.New("插件未安装")
}

func validateModelSettings(saved Saved, settings ModelSettings) error {
	if settings.SelectedKeys == nil || len(settings.SelectedKeys) > 4096 || len(settings.CustomIDs) > 256 {
		return errors.New("模型设置无效或超出数量限制")
	}
	keys := make(map[string]bool, len(saved.Models)+len(settings.CustomIDs))
	for _, model := range saved.Models {
		keys[model.Key] = true
	}
	for _, id := range settings.CustomIDs {
		model := customModel(id)
		if id == "" || len(id) > 512 || id != strings.TrimSpace(id) || strings.ContainsFunc(id, unicode.IsControl) || keys[model.Key] {
			return errors.New("手动模型 ID 无效或重复")
		}
		keys[model.Key] = true
	}
	for _, key := range settings.SelectedKeys {
		if !keys[key] {
			return errors.New("所选模型已变化，请刷新页面后再试")
		}
		delete(keys, key)
	}
	return nil
}
