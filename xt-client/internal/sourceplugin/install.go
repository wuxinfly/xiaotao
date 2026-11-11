package sourceplugin

import (
	"errors"
	"io"
	"io/fs"
	"os"
	"path/filepath"
)

// Install copies a user-selected local package without executing it. Symlinks,
// devices and oversized packages are refused. Existing identities cannot overwrite.
func (m *Manager) Install(dir string) (Saved, error) {
	dir, err := filepath.Abs(dir)
	if err != nil {
		return Saved{}, err
	}
	manifest, _, err := ReadManifest(dir)
	if err != nil {
		return Saved{}, err
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, e := range m.entries {
		if e.ID == manifest.ID {
			return Saved{}, errors.New("plugin already installed")
		}
	}
	if err = os.MkdirAll(m.dir, 0700); err != nil {
		return Saved{}, err
	}
	tmp, err := os.MkdirTemp(m.dir, ".install-")
	if err != nil {
		return Saved{}, err
	}
	defer os.RemoveAll(tmp)
	var total int64
	err = filepath.WalkDir(dir, func(path string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		rel, err := filepath.Rel(dir, path)
		if err != nil {
			return err
		}
		target := filepath.Join(tmp, rel)
		if d.Type()&os.ModeSymlink != 0 {
			return errors.New("package symlinks are unsupported")
		}
		if d.IsDir() {
			return os.MkdirAll(target, 0700)
		}
		info, err := d.Info()
		if err != nil {
			return err
		}
		total += info.Size()
		if !info.Mode().IsRegular() || total > 256<<20 {
			return errors.New("invalid or oversized package")
		}
		in, err := os.Open(path)
		if err != nil {
			return err
		}
		defer in.Close()
		out, err := os.OpenFile(target, os.O_WRONLY|os.O_CREATE|os.O_EXCL, info.Mode().Perm())
		if err != nil {
			return err
		}
		_, err = io.Copy(out, io.LimitReader(in, info.Size()+1))
		closeErr := out.Close()
		if err != nil {
			return err
		}
		return closeErr
	})
	if err != nil {
		return Saved{}, err
	}
	checked, _, err := ReadManifest(tmp)
	if err != nil || checked.ID != manifest.ID {
		return Saved{}, errors.New("copied package validation failed")
	}
	target := filepath.Join(m.dir, manifest.ID)
	if err = os.Rename(tmp, target); err != nil {
		return Saved{}, err
	}
	entry := Saved{ID: manifest.ID, Manifest: checked, Enabled: false, Models: []Model{}}
	m.entries = append(m.entries, entry)
	if err = m.persist(); err != nil {
		m.entries = m.entries[:len(m.entries)-1]
		os.RemoveAll(target)
		return Saved{}, err
	}
	return entry, nil
}
func (m *Manager) Enable(id string, enabled bool) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	for i, e := range m.entries {
		if e.ID == id {
			if !enabled {
				if cancel := m.active[id]; cancel != nil {
					cancel()
				}
			}
			m.entries[i].Enabled = enabled
			if err := m.persist(); err != nil {
				m.entries[i] = e
				return err
			}
			return nil
		}
	}
	return errors.New("plugin not installed")
}
