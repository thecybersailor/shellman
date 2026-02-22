package fsbrowser

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestService_ListDirectoriesOnly(t *testing.T) {
	root := t.TempDir()
	if err := os.Mkdir(filepath.Join(root, "a"), 0o755); err != nil {
		t.Fatalf("mkdir failed: %v", err)
	}
	if err := os.WriteFile(filepath.Join(root, "f.txt"), []byte("x"), 0o644); err != nil {
		t.Fatalf("write file failed: %v", err)
	}

	svc := NewService()
	out, err := svc.List(root)
	if err != nil {
		t.Fatalf("list failed: %v", err)
	}
	if len(out.Items) != 1 || out.Items[0].Name != "a" || !out.Items[0].IsDir {
		t.Fatalf("unexpected items: %+v", out.Items)
	}
}

func TestService_ResolveAbsolutePath(t *testing.T) {
	root := t.TempDir()
	svc := NewService()
	abs, err := svc.Resolve(root)
	if err != nil {
		t.Fatalf("resolve failed: %v", err)
	}
	if abs != filepath.Clean(root) {
		t.Fatalf("unexpected abs: %s", abs)
	}
}

func TestService_ResolveTildeHome(t *testing.T) {
	home, err := os.UserHomeDir()
	if err != nil {
		t.Fatalf("home dir failed: %v", err)
	}
	svc := NewService()
	abs, err := svc.Resolve("~")
	if err != nil {
		t.Fatalf("resolve ~ failed: %v", err)
	}
	if abs != filepath.Clean(home) {
		t.Fatalf("unexpected abs: %s", abs)
	}
}

func TestService_ResolveTildeSubPath(t *testing.T) {
	home, err := os.UserHomeDir()
	if err != nil {
		t.Fatalf("home dir failed: %v", err)
	}
	tmp, err := os.MkdirTemp(home, "shellman-fsbrowser-test-")
	if err != nil {
		t.Fatalf("mkdir temp under home failed: %v", err)
	}
	defer func() { _ = os.RemoveAll(tmp) }()

	rel := strings.TrimPrefix(tmp, filepath.Clean(home))
	rel = strings.TrimPrefix(rel, string(filepath.Separator))
	input := "~/" + rel

	svc := NewService()
	abs, err := svc.Resolve(input)
	if err != nil {
		t.Fatalf("resolve %q failed: %v", input, err)
	}
	if abs != filepath.Clean(tmp) {
		t.Fatalf("unexpected abs: %s", abs)
	}
}
