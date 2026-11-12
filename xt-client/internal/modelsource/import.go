package modelsource

import (
	"errors"
	"os"
	"path/filepath"
)

// ImportFrom copies only manual source configuration from another data root.
// Source files are read only; keys are re-encrypted before a single target commit.
func (s *Store) ImportFrom(sourceDataDir string, sourceDecrypt func([]byte) ([]byte, error)) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if len(s.entries) > 0 {
		return 0, nil
	}
	if _, err := os.Stat(filepath.Join(sourceDataDir, "model-sources", "sources.json")); os.IsNotExist(err) {
		return 0, nil
	} else if err != nil {
		return 0, errors.New("cannot read source model sources")
	}
	if sourceDecrypt == nil {
		return 0, errors.New("source key decryption is unavailable")
	}
	source, err := open(sourceDataDir, s.encrypt, sourceDecrypt, false)
	if err != nil {
		return 0, err
	}
	if len(source.entries) == 0 {
		return 0, nil
	}
	next := make([]entry, 0, len(source.entries))
	for _, e := range source.entries {
		plain, err := sourceDecrypt(e.Secret)
		if err != nil || len(plain) == 0 {
			return 0, errors.New("cannot decrypt imported model source key")
		}
		encrypted, err := s.encrypt(plain)
		if err != nil || len(encrypted) == 0 {
			return 0, errors.New("cannot encrypt imported model source key")
		}
		e.Secret = encrypted
		next = append(next, e)
	}
	previous := s.routes
	s.routes = source.routes
	if err := s.commit(next); err != nil {
		s.routes = previous
		return 0, err
	}
	s.entries = next
	return len(next), nil
}
