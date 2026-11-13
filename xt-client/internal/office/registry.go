package office

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"

	"go.yaml.in/yaml/v3"
)

const maxRegistryBytes = 1 << 20

type registryPaths struct {
	global  string
	project string
	team    string
}

type RegistryStore struct {
	dataDir  string
	mu       sync.Mutex
	write    func(string, []byte, os.FileMode) error
	projects func() ([]string, error)
}

func (s *RegistryStore) SetProjectIndex(index func() ([]string, error)) { s.projects = index }

type ProjectRegistries struct {
	Global  RoleRegistry `json:"global"`
	Project RoleRegistry `json:"project"`
	Team    TeamRegistry `json:"team"`
}

type projectIndex struct {
	SchemaVersion int      `yaml:"schema_version"`
	Projects      []string `yaml:"projects"`
}

func NewRegistryStore(dataDir string) *RegistryStore {
	return &RegistryStore{dataDir: dataDir, write: writeAtomicFile}
}

func (s *RegistryStore) paths(projectPath string) registryPaths {
	return registryPaths{
		global:  filepath.Join(s.dataDir, "collaboration", "roles", "registry.yaml"),
		project: filepath.Join(projectPath, ".xiaotao", "roles", "registry.yaml"),
		team:    filepath.Join(projectPath, ".xiaotao", "team.yaml"),
	}
}

func (s *RegistryStore) Load(projectPath string) (ProjectRegistries, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.loadLocked(projectPath)
}

func (s *RegistryStore) Update(projectPath string, expected ProjectRegistries, mutate func(*ProjectRegistries) error) (ProjectRegistries, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	current, err := s.loadLocked(projectPath)
	if err != nil {
		return ProjectRegistries{}, err
	}
	if current.Global.Revision != expected.Global.Revision || current.Project.Revision != expected.Project.Revision || current.Team.Revision != expected.Team.Revision {
		return ProjectRegistries{}, ErrRoleConflict
	}
	membersBefore := append([]Membership(nil), current.Team.Members...)
	if err = mutate(&current); err != nil {
		return ProjectRegistries{}, err
	}
	if current.Global.Revision < expected.Global.Revision || current.Project.Revision < expected.Project.Revision || current.Team.Revision < expected.Team.Revision {
		return ProjectRegistries{}, errors.New("registry revision cannot decrease")
	}
	if err = ValidateRoleRegistry(current.Global, RoleScopeGlobal); err != nil {
		return ProjectRegistries{}, err
	}
	if err = ValidateRoleRegistry(current.Project, RoleScopeProject); err != nil {
		return ProjectRegistries{}, err
	}
	if err = ValidateTeam(current.Team, current.Global, current.Project); err != nil {
		return ProjectRegistries{}, err
	}
	if len(current.Team.Members) > 0 && !memberListsEqual(membersBefore, current.Team.Members) {
		if err = s.registerProject(projectPath); err != nil {
			return ProjectRegistries{}, err
		}
	}
	paths := s.paths(projectPath)
	if current.Global.Revision > expected.Global.Revision {
		if err = s.writeRegistry(paths.global, current.Global); err != nil {
			return ProjectRegistries{}, err
		}
	}
	if current.Project.Revision > expected.Project.Revision {
		if err = s.writeRegistry(paths.project, current.Project); err != nil {
			return ProjectRegistries{}, err
		}
	}
	if current.Team.Revision > expected.Team.Revision {
		if err = s.writeRegistry(paths.team, current.Team); err != nil {
			return ProjectRegistries{}, err
		}
	}
	return current, nil
}

func memberListsEqual(left, right []Membership) bool {
	if len(left) != len(right) {
		return false
	}
	for index := range left {
		if left[index] != right[index] {
			return false
		}
	}
	return true
}

func (s *RegistryStore) registerProject(projectPath string) error {
	path := filepath.Join(s.dataDir, "collaboration", "projects.yaml")
	index := projectIndex{SchemaVersion: 1}
	if err := readYAML(path, &index); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	if index.SchemaVersion != 1 {
		return fmt.Errorf("%w: 项目索引版本", ErrRoleCorrupt)
	}
	clean, err := filepath.Abs(projectPath)
	if err != nil {
		return err
	}
	for _, known := range index.Projects {
		if strings.EqualFold(filepath.Clean(known), filepath.Clean(clean)) {
			return nil
		}
	}
	index.Projects = append(index.Projects, clean)
	sort.Strings(index.Projects)
	return s.writeRegistry(path, index)
}

func (s *RegistryStore) loadLocked(projectPath string) (ProjectRegistries, error) {
	path, _, err := resolveProject(projectPath, "")
	if err != nil {
		return ProjectRegistries{}, err
	}
	paths := s.paths(path)
	global, err := readRoles(paths.global, RoleScopeGlobal)
	if err != nil {
		return ProjectRegistries{}, err
	}
	project, err := readRoles(paths.project, RoleScopeProject)
	if err != nil {
		return ProjectRegistries{}, err
	}
	team, err := readTeam(paths.team)
	if err != nil {
		return ProjectRegistries{}, err
	}
	if err = ValidateTeam(team, global, project); err != nil {
		return ProjectRegistries{}, fmt.Errorf("%s: %w", paths.team, err)
	}
	return ProjectRegistries{Global: global, Project: project, Team: team}, nil
}

func readRoles(path, scope string) (RoleRegistry, error) {
	registry := RoleRegistry{SchemaVersion: RoleSchemaVersion, Roles: []Role{}}
	if err := readYAML(path, &registry); err != nil {
		return RoleRegistry{}, err
	}
	if registry.Roles == nil {
		registry.Roles = []Role{}
	}
	if err := ValidateRoleRegistry(registry, scope); err != nil {
		return RoleRegistry{}, fmt.Errorf("%s: %w", path, err)
	}
	return registry, nil
}

func readTeam(path string) (TeamRegistry, error) {
	team := TeamRegistry{SchemaVersion: RoleSchemaVersion, Onboarding: OnboardingPending, Members: []Membership{}}
	if err := readYAML(path, &team); err != nil {
		return TeamRegistry{}, err
	}
	if team.Members == nil {
		team.Members = []Membership{}
	}
	if team.Onboarding == "" {
		team.Onboarding = OnboardingPending
	}
	if err := ValidateTeam(team, RoleRegistry{SchemaVersion: RoleSchemaVersion}, RoleRegistry{SchemaVersion: RoleSchemaVersion}); err != nil && len(team.Members) == 0 {
		return TeamRegistry{}, fmt.Errorf("%s: %w", path, err)
	}
	return team, nil
}

func readYAML(path string, target any) error {
	file, err := os.Open(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("读取 %s: %w", path, err)
	}
	defer file.Close()
	decoder := yaml.NewDecoder(io.LimitReader(file, maxRegistryBytes+1))
	decoder.KnownFields(true)
	if err = decoder.Decode(target); err != nil {
		return fmt.Errorf("%w: %s: %v", ErrRoleCorrupt, path, err)
	}
	var extra any
	if err = decoder.Decode(&extra); !errors.Is(err, io.EOF) {
		return fmt.Errorf("%w: %s 含多个 YAML 文档或超过大小限制", ErrRoleCorrupt, path)
	}
	return nil
}

func readYAMLBytes(raw []byte, target any) error {
	decoder := yaml.NewDecoder(bytes.NewReader(raw))
	decoder.KnownFields(true)
	if err := decoder.Decode(target); err != nil {
		return err
	}
	var extra any
	if err := decoder.Decode(&extra); !errors.Is(err, io.EOF) {
		return errors.New("multiple YAML documents")
	}
	return nil
}

func encodeYAML(value any) ([]byte, error) {
	var output bytes.Buffer
	encoder := yaml.NewEncoder(&output)
	encoder.SetIndent(2)
	if err := encoder.Encode(value); err != nil {
		return nil, err
	}
	if err := encoder.Close(); err != nil {
		return nil, err
	}
	if output.Len() > maxRegistryBytes {
		return nil, errors.New("角色记录超过 1 MiB")
	}
	return output.Bytes(), nil
}

func (s *RegistryStore) writeRegistry(path string, value any) error {
	raw, err := encodeYAML(value)
	if err != nil {
		return err
	}
	return s.write(path, raw, 0o600)
}

func writeAtomicFile(path string, raw []byte, mode os.FileMode) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	temporary, err := os.CreateTemp(filepath.Dir(path), ".xiaotao-write-*")
	if err != nil {
		return err
	}
	temporaryPath := temporary.Name()
	defer os.Remove(temporaryPath)
	if err = temporary.Chmod(mode); err == nil {
		_, err = temporary.Write(raw)
	}
	if syncErr := temporary.Sync(); err == nil {
		err = syncErr
	}
	if closeErr := temporary.Close(); err == nil {
		err = closeErr
	}
	if err != nil {
		return fmt.Errorf("写入临时文件: %w", err)
	}
	if err = replaceFile(temporaryPath, path); err != nil {
		return fmt.Errorf("替换角色记录: %w", err)
	}
	syncDirectory(filepath.Dir(path))
	return nil
}
