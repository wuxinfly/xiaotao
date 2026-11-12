package modelsource

import (
	"maps"
	"strings"
)

// PluginPrefix reserves the short first-party name in the same persistent namespace
// as manual sources. Existing and deleted manual prefixes retain priority.
func (s *Store) PluginPrefix(id string) (string, error) {
	if !strings.HasPrefix(id, "xiaotao.") || strings.Contains(strings.TrimPrefix(id, "xiaotao."), ".") {
		return id, nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	identity := "plugin:" + id
	if prefix := s.routes[identity]; prefix != "" {
		return prefix, nil
	}
	prefix := strings.TrimPrefix(id, "xiaotao.")
	for _, used := range s.routes {
		if used == prefix {
			prefix = id
			break
		}
	}
	previous := s.routes
	s.routes = maps.Clone(previous)
	if s.routes == nil {
		s.routes = map[string]string{}
	}
	s.routes[identity] = prefix
	if err := s.commit(s.entries); err != nil {
		s.routes = previous
		return "", err
	}
	return prefix, nil
}
