package localapi

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"shellman/cli/internal/global"
)

const (
	skillSourceSystem  = "system"
	skillSourceProject = "project"
)

type SkillIndexEntry struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	Path        string `json:"path"`
	Source      string `json:"source"`
}

type skillIndexCacheEntry struct {
	Fingerprint string
	Entries     []SkillIndexEntry
}

func BuildSkillIndex(systemBase, projectBase string) (map[string]SkillIndexEntry, error) {
	index := map[string]SkillIndexEntry{}
	if err := scanSkillBase(index, strings.TrimSpace(systemBase), skillSourceSystem); err != nil {
		return nil, err
	}
	if err := scanSkillBase(index, strings.TrimSpace(projectBase), skillSourceProject); err != nil {
		return nil, err
	}
	return index, nil
}

func scanSkillBase(index map[string]SkillIndexEntry, basePath, source string) error {
	if strings.TrimSpace(basePath) == "" {
		return nil
	}
	info, err := os.Stat(basePath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	if !info.IsDir() {
		return nil
	}
	return filepath.WalkDir(basePath, func(path string, d os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if d.IsDir() || !strings.EqualFold(strings.TrimSpace(d.Name()), "SKILL.md") {
			return nil
		}
		meta, err := readSkillFrontMatter(path)
		if err != nil {
			return fmt.Errorf("parse skill front matter %s: %w", path, err)
		}
		name := strings.TrimSpace(meta["name"])
		if name == "" {
			return nil
		}
		description := strings.TrimSpace(meta["description"])
		absPath, err := filepath.Abs(path)
		if err != nil {
			return err
		}
		index[name] = SkillIndexEntry{
			Name:        name,
			Description: description,
			Path:        filepath.ToSlash(absPath),
			Source:      source,
		}
		return nil
	})
}

func readSkillFrontMatter(path string) (map[string]string, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	if !scanner.Scan() {
		if err := scanner.Err(); err != nil {
			return nil, err
		}
		return map[string]string{}, nil
	}
	if strings.TrimSpace(scanner.Text()) != "---" {
		return map[string]string{}, nil
	}
	meta := map[string]string{}
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "---" {
			return meta, nil
		}
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		parts := strings.SplitN(line, ":", 2)
		if len(parts) != 2 {
			continue
		}
		key := strings.TrimSpace(parts[0])
		value := strings.TrimSpace(parts[1])
		value = strings.Trim(value, `"'`)
		if key != "" {
			meta[key] = value
		}
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}
	return meta, nil
}

func (s *Server) loadSkillIndex(projectID string) []SkillIndexEntry {
	systemBase := ""
	if configDir, err := global.DefaultConfigDir(); err == nil {
		systemBase = filepath.Join(strings.TrimSpace(configDir), "skills")
	}
	projectBase := ""
	if strings.TrimSpace(projectID) != "" {
		if repoRoot, err := s.findProjectRepoRoot(projectID); err == nil {
			projectBase = filepath.Join(strings.TrimSpace(repoRoot), ".shellman", "skills")
		}
	}
	fingerprint, err := buildSkillIndexFingerprint(systemBase, projectBase)
	if err != nil {
		return []SkillIndexEntry{}
	}
	cacheKey := systemBase + "|" + projectBase

	s.skillIndexMu.Lock()
	if cached, ok := s.skillIndexCache[cacheKey]; ok && strings.TrimSpace(cached.Fingerprint) == strings.TrimSpace(fingerprint) {
		out := cloneSkillEntries(cached.Entries)
		s.skillIndexMu.Unlock()
		return out
	}
	s.skillIndexMu.Unlock()

	indexMap, err := BuildSkillIndex(systemBase, projectBase)
	if err != nil {
		return []SkillIndexEntry{}
	}
	entries := make([]SkillIndexEntry, 0, len(indexMap))
	for _, item := range indexMap {
		entries = append(entries, item)
	}
	sort.Slice(entries, func(i, j int) bool {
		if entries[i].Name == entries[j].Name {
			return entries[i].Path < entries[j].Path
		}
		return entries[i].Name < entries[j].Name
	})
	s.skillIndexMu.Lock()
	if s.skillIndexCache == nil {
		s.skillIndexCache = map[string]skillIndexCacheEntry{}
	}
	s.skillIndexCache[cacheKey] = skillIndexCacheEntry{
		Fingerprint: fingerprint,
		Entries:     cloneSkillEntries(entries),
	}
	s.skillIndexMu.Unlock()
	return entries
}

func buildSkillIndexFingerprint(systemBase, projectBase string) (string, error) {
	rows := []string{
		"system_base=" + strings.TrimSpace(systemBase),
		"project_base=" + strings.TrimSpace(projectBase),
	}
	appendRows := func(source, base string) error {
		files, err := collectSkillFiles(base)
		if err != nil {
			return err
		}
		for _, filePath := range files {
			info, err := os.Stat(filePath)
			if err != nil {
				if os.IsNotExist(err) {
					continue
				}
				return err
			}
			abs, err := filepath.Abs(filePath)
			if err != nil {
				return err
			}
			rows = append(rows, fmt.Sprintf("%s|%s|%d|%d", source, filepath.ToSlash(abs), info.ModTime().UnixNano(), info.Size()))
		}
		return nil
	}
	if err := appendRows(skillSourceSystem, systemBase); err != nil {
		return "", err
	}
	if err := appendRows(skillSourceProject, projectBase); err != nil {
		return "", err
	}
	sort.Strings(rows)
	return strings.Join(rows, "\n"), nil
}

func collectSkillFiles(basePath string) ([]string, error) {
	if strings.TrimSpace(basePath) == "" {
		return []string{}, nil
	}
	info, err := os.Stat(basePath)
	if err != nil {
		if os.IsNotExist(err) {
			return []string{}, nil
		}
		return nil, err
	}
	if !info.IsDir() {
		return []string{}, nil
	}
	files := make([]string, 0, 16)
	err = filepath.WalkDir(basePath, func(path string, d os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if d.IsDir() || !strings.EqualFold(strings.TrimSpace(d.Name()), "SKILL.md") {
			return nil
		}
		files = append(files, path)
		return nil
	})
	if err != nil {
		return nil, err
	}
	sort.Strings(files)
	return files, nil
}

func cloneSkillEntries(in []SkillIndexEntry) []SkillIndexEntry {
	if len(in) == 0 {
		return []SkillIndexEntry{}
	}
	out := make([]SkillIndexEntry, 0, len(in))
	for _, item := range in {
		out = append(out, item)
	}
	return out
}
