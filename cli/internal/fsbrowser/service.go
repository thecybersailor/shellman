package fsbrowser

import (
	"errors"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

type Item struct {
	Name  string `json:"name"`
	Path  string `json:"path"`
	IsDir bool   `json:"is_dir"`
}

type ListResult struct {
	Path  string `json:"path"`
	Items []Item `json:"items"`
}

type Service struct{}

func NewService() *Service { return &Service{} }

func (s *Service) Roots() ([]string, error) {
	home, _ := os.UserHomeDir()
	roots := make([]string, 0, 2)
	if strings.TrimSpace(home) != "" {
		roots = append(roots, filepath.Clean(home))
	}
	roots = append(roots, string(filepath.Separator))
	seen := make(map[string]struct{}, len(roots))
	out := make([]string, 0, len(roots))
	for _, p := range roots {
		if _, ok := seen[p]; ok {
			continue
		}
		seen[p] = struct{}{}
		out = append(out, p)
	}
	return out, nil
}

func (s *Service) Resolve(path string) (string, error) {
	path = strings.TrimSpace(path)
	if path == "" {
		return "", errors.New("path is required")
	}
	if path == "~" || strings.HasPrefix(path, "~/") || strings.HasPrefix(path, "~\\") {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", err
		}
		if strings.TrimSpace(home) == "" {
			return "", errors.New("home directory is unavailable")
		}
		if path == "~" {
			path = home
		} else {
			suffix := strings.TrimPrefix(strings.TrimPrefix(path, "~/"), "~\\")
			path = filepath.Join(home, suffix)
		}
	}
	abs, err := filepath.Abs(path)
	if err != nil {
		return "", err
	}
	st, err := os.Stat(abs)
	if err != nil {
		return "", err
	}
	if !st.IsDir() {
		return "", errors.New("path is not a directory")
	}
	return filepath.Clean(abs), nil
}

func (s *Service) List(path string) (ListResult, error) {
	resolved, err := s.Resolve(path)
	if err != nil {
		return ListResult{}, err
	}
	entries, err := os.ReadDir(resolved)
	if err != nil {
		return ListResult{}, err
	}
	items := make([]Item, 0, len(entries))
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		name := entry.Name()
		if strings.HasPrefix(name, ".") {
			continue
		}
		items = append(items, Item{
			Name:  name,
			Path:  filepath.Join(resolved, name),
			IsDir: true,
		})
	}
	sort.Slice(items, func(i, j int) bool {
		return strings.ToLower(items[i].Name) < strings.ToLower(items[j].Name)
	})
	return ListResult{Path: resolved, Items: items}, nil
}

func (s *Service) Search(base, q string, limit int) ([]Item, error) {
	resolvedBase, err := s.Resolve(base)
	if err != nil {
		return nil, err
	}
	query := strings.ToLower(strings.TrimSpace(q))
	if query == "" {
		return []Item{}, nil
	}
	if limit <= 0 {
		limit = 20
	}
	entries, err := os.ReadDir(resolvedBase)
	if err != nil {
		return nil, err
	}
	out := make([]Item, 0, len(entries))
	for _, entry := range entries {
		if !entry.IsDir() || entry.Type()&os.ModeSymlink != 0 {
			continue
		}
		name := entry.Name()
		if strings.HasPrefix(name, ".") {
			continue
		}
		child := filepath.Clean(filepath.Join(resolvedBase, name))
		if strings.Contains(strings.ToLower(name), query) {
			out = append(out, Item{Name: name, Path: child, IsDir: true})
		}
	}
	sort.Slice(out, func(i, j int) bool {
		left := strings.ToLower(out[i].Name)
		right := strings.ToLower(out[j].Name)
		if left != right {
			return left < right
		}
		return strings.ToLower(out[i].Path) < strings.ToLower(out[j].Path)
	})
	if len(out) > limit {
		out = out[:limit]
	}
	return out, nil
}
