package global

import "testing"

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
	if list1[0].DisplayName != "p1" {
		t.Fatalf("expected default display name to fallback to project id, got %q", list1[0].DisplayName)
	}
	if list1[0].SortOrder != 1 {
		t.Fatalf("expected default sort order 1, got %d", list1[0].SortOrder)
	}
	if list1[0].Collapsed {
		t.Fatalf("expected default collapsed=false")
	}

	if err := s.AddProject(ActiveProject{
		ProjectID:   "p1",
		RepoRoot:    "/tmp/repo1",
		DisplayName: "Project One",
		SortOrder:   3,
		Collapsed:   true,
	}); err != nil {
		t.Fatalf("AddProject dedup failed: %v", err)
	}
	list2, err := s.ListProjects()
	if err != nil {
		t.Fatalf("ListProjects failed: %v", err)
	}
	if len(list2) != 1 {
		t.Fatalf("expected dedup to keep 1 project, got %d", len(list2))
	}
	if list2[0].DisplayName != "Project One" {
		t.Fatalf("expected display name to refresh, got %q", list2[0].DisplayName)
	}
	if list2[0].SortOrder != 3 {
		t.Fatalf("expected sort_order=3, got %d", list2[0].SortOrder)
	}
	if !list2[0].Collapsed {
		t.Fatalf("expected collapsed=true")
	}
}

func TestProjectsStore_ListProjects_SortedBySortOrder(t *testing.T) {
	dir := t.TempDir()
	s := NewProjectsStore(dir)

	if err := s.AddProject(ActiveProject{ProjectID: "p1", RepoRoot: "/tmp/repo1", SortOrder: 2}); err != nil {
		t.Fatalf("AddProject p1 failed: %v", err)
	}
	if err := s.AddProject(ActiveProject{ProjectID: "p2", RepoRoot: "/tmp/repo2", SortOrder: 1}); err != nil {
		t.Fatalf("AddProject p2 failed: %v", err)
	}

	list, err := s.ListProjects()
	if err != nil {
		t.Fatalf("ListProjects failed: %v", err)
	}
	if len(list) != 2 {
		t.Fatalf("expected 2 projects, got %d", len(list))
	}
	if list[0].ProjectID != "p2" || list[1].ProjectID != "p1" {
		t.Fatalf("unexpected order: %#v", list)
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

func TestProjectsStore_UpdateProjectDisplayName(t *testing.T) {
	dir := t.TempDir()
	s := NewProjectsStore(dir)

	if err := s.AddProject(ActiveProject{ProjectID: "p1", RepoRoot: "/tmp/repo1"}); err != nil {
		t.Fatalf("AddProject failed: %v", err)
	}
	if err := s.SetProjectDisplayName("p1", "ShellMan Core"); err != nil {
		t.Fatalf("SetProjectDisplayName failed: %v", err)
	}

	list, err := s.ListProjects()
	if err != nil {
		t.Fatalf("ListProjects failed: %v", err)
	}
	if len(list) != 1 {
		t.Fatalf("expected 1 project, got %d", len(list))
	}
	if list[0].DisplayName != "ShellMan Core" {
		t.Fatalf("expected display name updated, got %q", list[0].DisplayName)
	}
}
