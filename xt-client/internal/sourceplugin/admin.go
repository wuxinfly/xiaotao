package sourceplugin

import (
	"os"
	"path/filepath"
)

// SetDiagnostics controls whether plugin stderr is appended to a local log.
func (m *Manager) SetDiagnostics(id string, enabled bool) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	for i, entry := range m.entries {
		if entry.ID != id {
			continue
		}
		next := entry
		next.Diagnostics = enabled
		m.entries[i] = next
		if err := m.persist(); err != nil {
			m.entries[i] = entry
			return err
		}
		return nil
	}
	return NewError(CodeInvalid, "插件未安装")
}

// Uninstall removes the plugin state, accounts and installed packages. It refuses
// to run while the plugin is enabled or an operation is active.
func (m *Manager) Uninstall(id string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	for i, entry := range m.entries {
		if entry.ID != id {
			continue
		}
		if entry.Enabled || m.active[id] != nil {
			return NewError(CodeUnsupported, "请先停用插件再卸载")
		}
		previous := m.entries
		m.entries = append(append([]Saved{}, previous[:i]...), previous[i+1:]...)
		if err := m.persist(); err != nil {
			m.entries = previous
			return err
		}
		delete(m.gates, id)
		m.removePackageDir(entry.PackageDir)
		if entry.Previous != nil {
			m.removePackageDir(entry.Previous.PackageDir)
		}
		return nil
	}
	return NewError(CodeInvalid, "插件未安装")
}

// Rollback swaps the current package with the single retained previous package.
func (m *Manager) Rollback(id string) (Saved, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	for i, entry := range m.entries {
		if entry.ID != id {
			continue
		}
		if entry.Enabled || m.active[id] != nil {
			return Saved{}, NewError(CodeUnsupported, "请先停用插件再回滚")
		}
		if entry.Previous == nil {
			return Saved{}, NewError(CodeInvalid, "没有可回滚的旧版本")
		}
		next := entry
		next.Manifest = entry.Previous.Manifest
		next.PackageDir = entry.Previous.PackageDir
		next.PackageHash = entry.Previous.PackageHash
		next.Models = append([]Model(nil), entry.Previous.Models...)
		next.Previous = &PackageRef{
			Manifest:    entry.Manifest,
			PackageDir:  entry.PackageDir,
			PackageHash: entry.PackageHash,
			Models:      append([]Model(nil), entry.Models...),
		}
		m.entries[i] = next
		if err := m.persist(); err != nil {
			m.entries[i] = entry
			return Saved{}, err
		}
		return m.publicEntry(next), nil
	}
	return Saved{}, NewError(CodeInvalid, "插件未安装")
}

func (m *Manager) removePackageDir(name string) {
	if !validPackageDir(name) {
		return
	}
	_ = os.RemoveAll(filepath.Join(m.dir, name))
}
