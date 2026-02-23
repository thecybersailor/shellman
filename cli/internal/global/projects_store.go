package global

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"time"
)

const activeProjectsFileName = "active-projects.json"

type ActiveProject struct {
	ProjectID   string    `json:"project_id"`
	RepoRoot    string    `json:"repo_root"`
	DisplayName string    `json:"display_name"`
	UpdatedAt   time.Time `json:"updated_at"`
}

type ProjectsStore struct {
	dir string
}

func NewProjectsStore(dir string) *ProjectsStore {
	return &ProjectsStore{dir: dir}
}

func (s *ProjectsStore) ListProjects() ([]ActiveProject, error) {
	if err := os.MkdirAll(s.dir, 0o755); err != nil {
		return nil, err
	}
	path := filepath.Join(s.dir, activeProjectsFileName)
	b, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return []ActiveProject{}, nil
		}
		return nil, err
	}
	var list []ActiveProject
	if err := json.Unmarshal(b, &list); err != nil {
		return nil, err
	}
	return list, nil
}

func (s *ProjectsStore) AddProject(p ActiveProject) error {
	list, err := s.ListProjects()
	if err != nil {
		return err
	}
	p.DisplayName = strings.TrimSpace(p.DisplayName)
	now := time.Now().UTC()
	updated := false
	for i := range list {
		if list[i].ProjectID == p.ProjectID {
			list[i].RepoRoot = p.RepoRoot
			if p.DisplayName != "" {
				list[i].DisplayName = p.DisplayName
			}
			if strings.TrimSpace(list[i].DisplayName) == "" {
				list[i].DisplayName = p.ProjectID
			}
			list[i].UpdatedAt = now
			updated = true
			break
		}
	}
	if !updated {
		if p.DisplayName == "" {
			p.DisplayName = p.ProjectID
		}
		p.UpdatedAt = now
		list = append(list, p)
	}
	return s.save(list)
}

func (s *ProjectsStore) RemoveProject(projectID string) error {
	list, err := s.ListProjects()
	if err != nil {
		return err
	}
	out := make([]ActiveProject, 0, len(list))
	for _, p := range list {
		if p.ProjectID != projectID {
			out = append(out, p)
		}
	}
	return s.save(out)
}

func (s *ProjectsStore) SetProjectDisplayName(projectID, displayName string) error {
	list, err := s.ListProjects()
	if err != nil {
		return err
	}
	nextName := strings.TrimSpace(displayName)
	for i := range list {
		if list[i].ProjectID == projectID {
			list[i].DisplayName = nextName
			list[i].UpdatedAt = time.Now().UTC()
			return s.save(list)
		}
	}
	return s.save(list)
}

func (s *ProjectsStore) save(list []ActiveProject) error {
	if err := os.MkdirAll(s.dir, 0o755); err != nil {
		return err
	}
	return writeJSONAtomically(filepath.Join(s.dir, activeProjectsFileName), list)
}
