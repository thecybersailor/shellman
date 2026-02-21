package fsbrowser

import (
	"os"
	"path/filepath"
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
