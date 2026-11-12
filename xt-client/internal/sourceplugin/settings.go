package sourceplugin

import (
	"errors"
	"slices"
)

// Settings returns the declared option values, filling in manifest defaults.
func (e Saved) Settings() map[string]string {
	out := map[string]string{}
	for _, field := range e.Manifest.Settings {
		out[field.Key] = field.Default
	}
	for key, value := range e.SettingValues {
		for _, field := range e.Manifest.Settings {
			if field.Key == key {
				out[key] = value
				break
			}
		}
	}
	return out
}

// SaveSettings validates values against the manifest schema and persists them.
func (m *Manager) SaveSettings(id string, values map[string]string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	for i, entry := range m.entries {
		if entry.ID != id {
			continue
		}
		if len(values) > len(entry.Manifest.Settings) {
			return NewError(CodeInvalid, "插件设置无效")
		}
		clean := map[string]string{}
		for _, field := range entry.Manifest.Settings {
			value, ok := values[field.Key]
			if !ok {
				continue
			}
			if len(value) > 200 {
				return NewError(CodeInvalid, "插件设置无效")
			}
			if len(field.Options) > 0 && !slices.Contains(field.Options, value) {
				return NewError(CodeInvalid, "插件设置无效")
			}
			clean[field.Key] = value
		}
		for key := range values {
			declared := false
			for _, field := range entry.Manifest.Settings {
				if field.Key == key {
					declared = true
					break
				}
			}
			if !declared {
				return NewError(CodeInvalid, "插件设置无效")
			}
		}
		next := entry
		next.SettingValues = clean
		m.entries[i] = next
		if err := m.persist(); err != nil {
			m.entries[i] = entry
			return errors.New("无法保存插件设置")
		}
		return nil
	}
	return NewError(CodeInvalid, "插件未安装")
}
