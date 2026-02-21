package logging

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"testing"
)

func TestNoFmtOrStdLogPrintingInRuntimePaths(t *testing.T) {
	t.Helper()

	banned := regexp.MustCompile(`\bfmt\.(Print|Printf|Println|Fprint|Fprintf|Fprintln)\b|\blog\.(Print|Printf|Println)\b|\bdebugf\s*\(`)
	roots := []string{"cmd/shellman", "internal/localapi"}
	violations := make([]string, 0)

	for _, root := range roots {
		walkRoot := filepath.Join("..", "..", root)
		_ = filepath.WalkDir(walkRoot, func(path string, d os.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if d.IsDir() {
				return nil
			}
			if !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
				return nil
			}
			raw, readErr := os.ReadFile(path)
			if readErr != nil {
				return readErr
			}
			lines := strings.Split(string(raw), "\n")
			for i, line := range lines {
				if banned.MatchString(line) {
					if isAllowedNonLoggingPrint(filepath.ToSlash(path), line) {
						continue
					}
					violations = append(violations, fmt.Sprintf("%s:%d: %s", filepath.ToSlash(path), i+1, strings.TrimSpace(line)))
				}
			}
			return nil
		})
	}

	if len(violations) > 0 {
		sort.Strings(violations)
		t.Fatalf("found banned logging calls:\n%s", strings.Join(violations, "\n"))
	}
}

func isAllowedNonLoggingPrint(path, line string) bool {
	trimmed := strings.TrimSpace(line)
	if strings.HasSuffix(path, "/cmd/shellman/main.go") && strings.Contains(trimmed, "fmt.Fprintf(out,") {
		return true
	}
	if strings.HasSuffix(path, "/internal/localapi/task_completion.go") && strings.Contains(trimmed, "&b") {
		return true
	}
	if strings.HasSuffix(path, "/internal/localapi/routes_addon.go") && strings.Contains(trimmed, "&b") {
		return true
	}
	return false
}
