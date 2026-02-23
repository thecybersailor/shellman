package localapi

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"shellman/cli/internal/global"
)

func TestBuildSkillIndex_MergeWithProjectOverride(t *testing.T) {
	systemBase := t.TempDir()
	projectBase := filepath.Join(t.TempDir(), ".shellman", "skills")
	if err := os.MkdirAll(filepath.Join(systemBase, "writing-plans"), 0o755); err != nil {
		t.Fatalf("mkdir system writing-plans failed: %v", err)
	}
	if err := os.MkdirAll(filepath.Join(projectBase, "writing-plans"), 0o755); err != nil {
		t.Fatalf("mkdir project writing-plans failed: %v", err)
	}
	if err := os.MkdirAll(filepath.Join(projectBase, "brainstorming"), 0o755); err != nil {
		t.Fatalf("mkdir project brainstorming failed: %v", err)
	}

	sysSkill := "---\nname: writing-plans\ndescription: system-version\n---\n# sys\n"
	if err := os.WriteFile(filepath.Join(systemBase, "writing-plans", "SKILL.md"), []byte(sysSkill), 0o644); err != nil {
		t.Fatalf("write system SKILL.md failed: %v", err)
	}
	projectSkill := "---\nname: writing-plans\ndescription: project-version\n---\n# project\n"
	if err := os.WriteFile(filepath.Join(projectBase, "writing-plans", "SKILL.md"), []byte(projectSkill), 0o644); err != nil {
		t.Fatalf("write project SKILL.md failed: %v", err)
	}
	otherProjectSkill := "---\nname: brainstorming\ndescription: local-brainstorm\n---\n# local\n"
	if err := os.WriteFile(filepath.Join(projectBase, "brainstorming", "SKILL.md"), []byte(otherProjectSkill), 0o644); err != nil {
		t.Fatalf("write project brainstorming SKILL.md failed: %v", err)
	}

	index, err := BuildSkillIndex(systemBase, projectBase)
	if err != nil {
		t.Fatalf("BuildSkillIndex failed: %v", err)
	}

	if len(index) != 2 {
		t.Fatalf("expected 2 skills, got %d", len(index))
	}
	wp, ok := index["writing-plans"]
	if !ok {
		t.Fatalf("writing-plans not found")
	}
	if wp.Description != "project-version" {
		t.Fatalf("expected writing-plans from project override, got %q", wp.Description)
	}
	if wp.Source != skillSourceProject {
		t.Fatalf("expected writing-plans source project, got %q", wp.Source)
	}
	if filepath.Base(filepath.Dir(wp.Path)) != "writing-plans" {
		t.Fatalf("unexpected writing-plans path: %q", wp.Path)
	}

	bs, ok := index["brainstorming"]
	if !ok {
		t.Fatalf("brainstorming not found")
	}
	if bs.Description != "local-brainstorm" {
		t.Fatalf("unexpected brainstorming description: %q", bs.Description)
	}
	if bs.Source != skillSourceProject {
		t.Fatalf("expected brainstorming source project, got %q", bs.Source)
	}
}

func TestLoadSkillIndex_RefreshOnSkillFileModTimeChange(t *testing.T) {
	repo := t.TempDir()
	configDir := t.TempDir()
	t.Setenv("SHELLMAN_CONFIG_DIR", configDir)
	projectBase := filepath.Join(repo, ".shellman", "skills", "writing-plans")
	systemBase := filepath.Join(configDir, "skills", "writing-plans")
	if err := os.MkdirAll(projectBase, 0o755); err != nil {
		t.Fatalf("mkdir project base failed: %v", err)
	}
	if err := os.MkdirAll(systemBase, 0o755); err != nil {
		t.Fatalf("mkdir system base failed: %v", err)
	}
	projectSkillPath := filepath.Join(projectBase, "SKILL.md")
	systemSkillPath := filepath.Join(systemBase, "SKILL.md")
	if err := os.WriteFile(systemSkillPath, []byte("---\nname: writing-plans\ndescription: sys\n---\n"), 0o644); err != nil {
		t.Fatalf("write system SKILL.md failed: %v", err)
	}
	if err := os.WriteFile(projectSkillPath, []byte("---\nname: writing-plans\ndescription: v1\n---\n"), 0o644); err != nil {
		t.Fatalf("write project SKILL.md failed: %v", err)
	}
	oldTime := time.Unix(1735689600, 0)
	newTime := oldTime.Add(2 * time.Second)
	if err := os.Chtimes(projectSkillPath, oldTime, oldTime); err != nil {
		t.Fatalf("chtimes project old failed: %v", err)
	}

	srv := NewServer(Deps{
		ConfigStore: &staticConfigStore{},
		ProjectsStore: &memProjectsStore{
			projects: []global.ActiveProject{{ProjectID: "p1", RepoRoot: filepath.Clean(repo)}},
		},
	})
	first := srv.loadSkillIndex("p1")
	if len(first) != 1 {
		t.Fatalf("expected one skill, got %d", len(first))
	}
	if strings.TrimSpace(first[0].Description) != "v1" {
		t.Fatalf("expected first description=v1, got %q", first[0].Description)
	}

	if err := os.WriteFile(projectSkillPath, []byte("---\nname: writing-plans\ndescription: v2\n---\n"), 0o644); err != nil {
		t.Fatalf("rewrite project SKILL.md failed: %v", err)
	}
	if err := os.Chtimes(projectSkillPath, newTime, newTime); err != nil {
		t.Fatalf("chtimes project new failed: %v", err)
	}
	second := srv.loadSkillIndex("p1")
	if len(second) != 1 {
		t.Fatalf("expected one skill after refresh, got %d", len(second))
	}
	if strings.TrimSpace(second[0].Description) != "v2" {
		t.Fatalf("expected refreshed description=v2, got %q", second[0].Description)
	}
}
