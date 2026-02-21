package global

import (
	"testing"
	"time"
)

func TestProjectsStore_AddProject_DedupAndRefreshUpdatedAt(t *testing.T) {
	dir := t.TempDir()
	s := NewProjectsStore(dir)

	if err := s.AddProject(ActiveProject{ProjectID: "p1", RepoRoot: "/tmp/repo1"}); err != nil {
		t.Fatalf("AddProject failed: %v", err)
	}
	list1, err := s.ListProjects()
	if err != nil {
		t.Fatalf("ListProjects failed: %v", err)
	}
	if len(list1) != 1 {
		t.Fatalf("expected 1 project, got %d", len(list1))
	}

	firstUpdated := list1[0].UpdatedAt
	time.Sleep(2 * time.Millisecond)

	if err := s.AddProject(ActiveProject{ProjectID: "p1", RepoRoot: "/tmp/repo1"}); err != nil {
		t.Fatalf("AddProject dedup failed: %v", err)
	}
	list2, err := s.ListProjects()
	if err != nil {
		t.Fatalf("ListProjects failed: %v", err)
	}
	if len(list2) != 1 {
		t.Fatalf("expected dedup to keep 1 project, got %d", len(list2))
	}
	if !list2[0].UpdatedAt.After(firstUpdated) {
		t.Fatalf("expected UpdatedAt to be refreshed")
	}
}

func TestProjectsStore_RemoveProject(t *testing.T) {
	dir := t.TempDir()
	s := NewProjectsStore(dir)

	if err := s.AddProject(ActiveProject{ProjectID: "p1", RepoRoot: "/tmp/repo1"}); err != nil {
		t.Fatalf("AddProject failed: %v", err)
	}
	if err := s.AddProject(ActiveProject{ProjectID: "p2", RepoRoot: "/tmp/repo2"}); err != nil {
		t.Fatalf("AddProject failed: %v", err)
	}
	if err := s.RemoveProject("p1"); err != nil {
		t.Fatalf("RemoveProject failed: %v", err)
	}

	list, err := s.ListProjects()
	if err != nil {
		t.Fatalf("ListProjects failed: %v", err)
	}
	if len(list) != 1 || list[0].ProjectID != "p2" {
		t.Fatalf("expected only p2 to remain, got %#v", list)
	}
}
