package localapi

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strings"
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
