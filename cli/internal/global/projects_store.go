package global

import (
	"errors"
	"os"
	"path/filepath"
	"shellman/cli/internal/db"
	"strings"
	"time"

	"gorm.io/gorm"
)

type ActiveProject struct {
	ProjectID   string    `json:"project_id"`
	RepoRoot    string    `json:"repo_root"`
	DisplayName string    `json:"display_name"`
	SortOrder   int64     `json:"sort_order"`
	Collapsed   bool      `json:"collapsed"`
	UpdatedAt   time.Time `json:"updated_at"`
}

type ProjectsStore struct {
	dir    string
	dbPath string
}

func NewProjectsStore(dir string) *ProjectsStore {
	return &ProjectsStore{
		dir:    dir,
		dbPath: filepath.Join(dir, "shellman.db"),
	}
}

func (s *ProjectsStore) ListProjects() ([]ActiveProject, error) {
	gdb, err := s.openDB()
	if err != nil {
		return nil, err
	}
	rows := []db.Project{}
	if err := gdb.Order("sort_order ASC").Order("updated_at DESC").Find(&rows).Error; err != nil {
		return nil, err
	}
	out := make([]ActiveProject, 0, len(rows))
	for _, row := range rows {
		updatedAt := time.Time{}
		if row.UpdatedAt > 0 {
			updatedAt = time.Unix(0, row.UpdatedAt).UTC()
		}
		out = append(out, ActiveProject{
			ProjectID:   strings.TrimSpace(row.ProjectID),
			RepoRoot:    strings.TrimSpace(row.RepoRoot),
			DisplayName: strings.TrimSpace(row.DisplayName),
			SortOrder:   row.SortOrder,
			Collapsed:   row.Collapsed,
			UpdatedAt:   updatedAt,
		})
	}
	return out, nil
}

func (s *ProjectsStore) AddProject(p ActiveProject) error {
	gdb, err := s.openDB()
	if err != nil {
		return err
	}
	projectID := strings.TrimSpace(p.ProjectID)
	repoRoot := strings.TrimSpace(p.RepoRoot)
	displayName := strings.TrimSpace(p.DisplayName)
	nowMillis := time.Now().UTC().UnixNano()

	return gdb.Transaction(func(tx *gorm.DB) error {
		var row db.Project
		err := tx.Where("project_id = ?", projectID).Take(&row).Error
		if errors.Is(err, gorm.ErrRecordNotFound) {
			sortOrder := p.SortOrder
			if sortOrder <= 0 {
				var maxSortOrder int64
				if err := tx.Model(&db.Project{}).Select("COALESCE(MAX(sort_order), 0)").Scan(&maxSortOrder).Error; err != nil {
					return err
				}
				sortOrder = maxSortOrder + 1
			}
			if displayName == "" {
				displayName = projectID
			}
			return tx.Create(&db.Project{
				ProjectID:   projectID,
				RepoRoot:    repoRoot,
				DisplayName: displayName,
				SortOrder:   sortOrder,
				Collapsed:   p.Collapsed,
				UpdatedAt:   nowMillis,
			}).Error
		}
		if err != nil {
			return err
		}
		row.RepoRoot = repoRoot
		if displayName != "" {
			row.DisplayName = displayName
		}
		if strings.TrimSpace(row.DisplayName) == "" {
			row.DisplayName = projectID
		}
		if p.SortOrder > 0 {
			row.SortOrder = p.SortOrder
		}
		row.Collapsed = p.Collapsed
		row.UpdatedAt = nowMillis
		return tx.Save(&row).Error
	})
}

func (s *ProjectsStore) RemoveProject(projectID string) error {
	gdb, err := s.openDB()
	if err != nil {
		return err
	}
	return gdb.Where("project_id = ?", strings.TrimSpace(projectID)).Delete(&db.Project{}).Error
}

func (s *ProjectsStore) SetProjectDisplayName(projectID, displayName string) error {
	gdb, err := s.openDB()
	if err != nil {
		return err
	}
	projectID = strings.TrimSpace(projectID)
	nextName := strings.TrimSpace(displayName)
	if nextName == "" {
		return nil
	}
	nowMillis := time.Now().UTC().UnixNano()
	return gdb.Transaction(func(tx *gorm.DB) error {
		var row db.Project
		if err := tx.Where("project_id = ?", projectID).Take(&row).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return nil
			}
			return err
		}
		row.DisplayName = nextName
		row.UpdatedAt = nowMillis
		return tx.Save(&row).Error
	})
}

func (s *ProjectsStore) openDB() (*gorm.DB, error) {
	if err := os.MkdirAll(s.dir, 0o755); err != nil {
		return nil, err
	}
	return db.OpenSQLiteGORMWithMigrations(s.dbPath)
}
