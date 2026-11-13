package office

import "path/filepath"

func (s *Service) knownProjects() ([]string, error) {
	var index projectIndex
	path := filepath.Join(s.teams.store.dataDir, "collaboration", "projects.yaml")
	if err := readYAML(path, &index); err != nil {
		return nil, err
	}
	return index.Projects, nil
}
