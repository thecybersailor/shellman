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
	if strings.TrimSpace(path) == "" {
		return "", errors.New("path is required")
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
	const maxDepth = 6
	type node struct {
		path  string
		depth int
	}
	queue := []node{{path: resolvedBase, depth: 0}}
	seen := map[string]struct{}{resolvedBase: {}}
	out := make([]Item, 0, limit)

	for len(queue) > 0 && len(out) < limit {
		cur := queue[0]
		queue = queue[1:]
		entries, err := os.ReadDir(cur.path)
		if err != nil {
			continue
		}
		for _, entry := range entries {
			if len(out) >= limit {
				break
			}
			if !entry.IsDir() || entry.Type()&os.ModeSymlink != 0 {
				continue
			}
			name := entry.Name()
			child := filepath.Clean(filepath.Join(cur.path, name))
			if _, ok := seen[child]; ok {
				continue
			}
			seen[child] = struct{}{}
			if strings.Contains(strings.ToLower(name), query) || strings.Contains(strings.ToLower(child), query) {
				out = append(out, Item{Name: name, Path: child, IsDir: true})
			}
			if cur.depth+1 <= maxDepth {
				queue = append(queue, node{path: child, depth: cur.depth + 1})
			}
		}
	}

	sort.Slice(out, func(i, j int) bool {
		return strings.ToLower(out[i].Name) < strings.ToLower(out[j].Name)
	})
	return out, nil
}
