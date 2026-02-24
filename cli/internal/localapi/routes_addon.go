package localapi

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"shellman/cli/internal/helperconfig"
)

type taskFileRevision struct {
	MTimeNS int64  `json:"mtime_ns"`
	Size    int64  `json:"size"`
	SHA256  string `json:"sha256"`
}

func (s *Server) handleGetTaskDiff(w http.ResponseWriter, _ *http.Request, taskID string) {
	projectID, _, _, err := s.findTask(taskID)
	if err != nil {
		respondError(w, http.StatusNotFound, "TASK_NOT_FOUND", err.Error())
		return
	}
	repoRoot, err := s.findProjectRepoRoot(projectID)
	if err != nil {
		respondError(w, http.StatusNotFound, "PROJECT_NOT_FOUND", err.Error())
		return
	}
	diff, err := runGitOutput(repoRoot, "diff", "--no-ext-diff")
	if err != nil {
		respondError(w, http.StatusInternalServerError, "GIT_DIFF_FAILED", err.Error())
		return
	}
	respondOK(w, map[string]any{
		"task_id": taskID,
		"diff":    diff,
	})
}

func (s *Server) handleGetTaskFiles(w http.ResponseWriter, _ *http.Request, taskID string) {
	projectID, _, _, err := s.findTask(taskID)
	if err != nil {
		respondError(w, http.StatusNotFound, "TASK_NOT_FOUND", err.Error())
		return
	}
	repoRoot, err := s.findProjectRepoRoot(projectID)
	if err != nil {
		respondError(w, http.StatusNotFound, "PROJECT_NOT_FOUND", err.Error())
		return
	}
	status, err := runGitOutput(repoRoot, "status", "--porcelain")
	if err != nil {
		respondError(w, http.StatusInternalServerError, "GIT_STATUS_FAILED", err.Error())
		return
	}
	files := parseGitStatusPorcelain(status)

	respondOK(w, map[string]any{
		"task_id": taskID,
		"files":   files,
	})
}

func (s *Server) handlePostTaskCommitMessageGenerate(w http.ResponseWriter, _ *http.Request, taskID string) {
	projectID, _, _, err := s.findTask(taskID)
	if err != nil {
		respondError(w, http.StatusNotFound, "TASK_NOT_FOUND", err.Error())
		return
	}
	repoRoot, err := s.findProjectRepoRoot(projectID)
	if err != nil {
		respondError(w, http.StatusNotFound, "PROJECT_NOT_FOUND", err.Error())
		return
	}

	status, err := runGitOutput(repoRoot, "status", "--porcelain")
	if err != nil {
		respondError(w, http.StatusInternalServerError, "GIT_STATUS_FAILED", err.Error())
		return
	}
	diff, err := runGitOutput(repoRoot, "diff", "--no-ext-diff")
	if err != nil {
		respondError(w, http.StatusInternalServerError, "GIT_DIFF_FAILED", err.Error())
		return
	}

	files := parseGitStatusPorcelain(status)
	message, err := s.buildCommitMessageWithHelper(context.Background(), taskID, files, diff)
	if err != nil {
		respondError(w, http.StatusInternalServerError, "COMMIT_MESSAGE_GENERATE_FAILED", err.Error())
		return
	}
	message = strings.TrimSpace(message)
	if message == "" {
		respondError(w, http.StatusInternalServerError, "COMMIT_MESSAGE_GENERATE_FAILED", "helper returned empty output")
		return
	}

	respondOK(w, map[string]any{
		"task_id": taskID,
		"message": message,
	})
}

func (s *Server) handleGetTaskFileContent(w http.ResponseWriter, r *http.Request, taskID string) {
	_, _, rel, fileAbs, err := s.resolveTaskFilePath(taskID, strings.TrimSpace(r.URL.Query().Get("path")))
	if err != nil {
		s.respondFilePathResolveError(w, err)
		return
	}
	data, err := os.ReadFile(fileAbs)
	if err != nil {
		if os.IsNotExist(err) {
			respondError(w, http.StatusNotFound, "FILE_NOT_FOUND", err.Error())
			return
		}
		respondError(w, http.StatusInternalServerError, "FILE_READ_FAILED", err.Error())
		return
	}
	respondOK(w, map[string]any{
		"task_id": taskID,
		"path":    rel,
		"content": string(data),
		"rev":     computeTaskFileRevision(data, fileAbs),
	})
}

func (s *Server) handlePatchTaskFileContent(w http.ResponseWriter, r *http.Request, taskID string) {
	var req struct {
		Path            string           `json:"path"`
		Content         *string          `json:"content"`
		Patch           string           `json:"patch"`
		ExpectedRev     taskFileRevision `json:"expected_rev"`
		CreateIfMissing bool             `json:"create_if_missing"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondError(w, http.StatusBadRequest, "INVALID_JSON", err.Error())
		return
	}
	if req.Content == nil && strings.TrimSpace(req.Patch) == "" {
		respondError(w, http.StatusBadRequest, "INVALID_PAYLOAD", "content or patch is required")
		return
	}
	_, _, rel, fileAbs, err := s.resolveTaskFilePath(taskID, strings.TrimSpace(req.Path))
	if err != nil {
		s.respondFilePathResolveError(w, err)
		return
	}

	currentData, err := os.ReadFile(fileAbs)
	exists := err == nil
	if err != nil && !os.IsNotExist(err) {
		respondError(w, http.StatusInternalServerError, "FILE_READ_FAILED", err.Error())
		return
	}
	if !exists && !req.CreateIfMissing {
		respondError(w, http.StatusNotFound, "FILE_NOT_FOUND", "file not found")
		return
	}

	currentRev := taskFileRevision{}
	if exists {
		currentRev = computeTaskFileRevision(currentData, fileAbs)
	}
	if !taskFileRevisionEquals(currentRev, req.ExpectedRev) {
		latestContent := ""
		if exists {
			latestContent = string(currentData)
		}
		writeJSON(w, http.StatusConflict, map[string]any{
			"ok": false,
			"error": map[string]any{
				"code":    "FILE_REV_CONFLICT",
				"message": "file revision changed",
			},
			"data": map[string]any{
				"path":    rel,
				"content": latestContent,
				"rev":     currentRev,
			},
		})
		return
	}

	nextContent := ""
	if req.Content != nil {
		nextContent = *req.Content
	} else {
		patched, patchErr := applyUnifiedPatchText(string(currentData), req.Patch)
		if patchErr != nil {
			respondError(w, http.StatusBadRequest, "INVALID_PATCH", patchErr.Error())
			return
		}
		nextContent = patched
	}

	if err := os.MkdirAll(filepath.Dir(fileAbs), 0o755); err != nil {
		respondError(w, http.StatusInternalServerError, "FILE_WRITE_FAILED", err.Error())
		return
	}
	if err := os.WriteFile(fileAbs, []byte(nextContent), 0o644); err != nil {
		respondError(w, http.StatusInternalServerError, "FILE_WRITE_FAILED", err.Error())
		return
	}
	latestData, err := os.ReadFile(fileAbs)
	if err != nil {
		respondError(w, http.StatusInternalServerError, "FILE_READ_FAILED", err.Error())
		return
	}
	respondOK(w, map[string]any{
		"task_id": taskID,
		"path":    rel,
		"content": string(latestData),
		"rev":     computeTaskFileRevision(latestData, fileAbs),
	})
}

func (s *Server) handleGetTaskFileRaw(w http.ResponseWriter, r *http.Request, taskID string) {
	projectID, _, _, err := s.findTask(taskID)
	if err != nil {
		respondError(w, http.StatusNotFound, "TASK_NOT_FOUND", err.Error())
		return
	}
	repoRoot, err := s.findProjectRepoRoot(projectID)
	if err != nil {
		respondError(w, http.StatusNotFound, "PROJECT_NOT_FOUND", err.Error())
		return
	}
	rel := strings.TrimSpace(r.URL.Query().Get("path"))
	if rel == "" {
		respondError(w, http.StatusBadRequest, "INVALID_PATH", "path is required")
		return
	}

	rootAbs, err := filepath.Abs(repoRoot)
	if err != nil {
		respondError(w, http.StatusInternalServerError, "INVALID_REPO_ROOT", err.Error())
		return
	}
	fileAbs, err := filepath.Abs(filepath.Join(repoRoot, rel))
	if err != nil {
		respondError(w, http.StatusBadRequest, "INVALID_PATH", err.Error())
		return
	}
	if fileAbs != rootAbs && !strings.HasPrefix(fileAbs, rootAbs+string(os.PathSeparator)) {
		respondError(w, http.StatusBadRequest, "INVALID_PATH", "path escapes repo root")
		return
	}

	data, err := os.ReadFile(fileAbs)
	if err != nil {
		if os.IsNotExist(err) {
			respondError(w, http.StatusNotFound, "FILE_NOT_FOUND", err.Error())
			return
		}
		respondError(w, http.StatusInternalServerError, "FILE_READ_FAILED", err.Error())
		return
	}
	contentType := mime.TypeByExtension(strings.ToLower(filepath.Ext(rel)))
	if strings.TrimSpace(contentType) == "" {
		contentType = "application/octet-stream"
	}
	w.Header().Set("Content-Type", contentType)
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(data)
}

var unifiedHunkHeader = regexp.MustCompile(`^@@ -(\d+)(?:,(\d+))? \+(\d+)(?:,(\d+))? @@`)

func applyUnifiedPatchText(base, patch string) (string, error) {
	normalizedPatch := strings.ReplaceAll(patch, "\r\n", "\n")
	lines := strings.Split(normalizedPatch, "\n")
	if len(lines) > 0 && strings.TrimSpace(lines[len(lines)-1]) == "" {
		lines = lines[:len(lines)-1]
	}
	baseLines := strings.Split(strings.ReplaceAll(base, "\r\n", "\n"), "\n")
	srcIndex := 0
	out := make([]string, 0, len(baseLines)+8)
	i := 0

	for i < len(lines) {
		line := lines[i]
		if strings.HasPrefix(line, "--- ") || strings.HasPrefix(line, "+++ ") {
			i++
			continue
		}
		matches := unifiedHunkHeader.FindStringSubmatch(line)
		if matches == nil {
			i++
			continue
		}
		oldStart, err := strconv.Atoi(matches[1])
		if err != nil || oldStart < 1 {
			return "", errors.New("invalid hunk old start")
		}
		targetIndex := oldStart - 1
		if targetIndex < srcIndex || targetIndex > len(baseLines) {
			return "", errors.New("invalid hunk position")
		}
		out = append(out, baseLines[srcIndex:targetIndex]...)
		srcIndex = targetIndex
		i++

		for i < len(lines) {
			hunkLine := lines[i]
			if unifiedHunkHeader.MatchString(hunkLine) {
				break
			}
			if strings.HasPrefix(hunkLine, "--- ") || strings.HasPrefix(hunkLine, "+++ ") {
				break
			}
			if strings.HasPrefix(hunkLine, "\\ No newline at end of file") {
				i++
				continue
			}
			if hunkLine == "" {
				return "", errors.New("invalid patch line")
			}
			op := hunkLine[0]
			text := hunkLine[1:]
			switch op {
			case ' ':
				if srcIndex >= len(baseLines) || baseLines[srcIndex] != text {
					return "", errors.New("patch context mismatch")
				}
				out = append(out, text)
				srcIndex++
			case '-':
				if srcIndex >= len(baseLines) || baseLines[srcIndex] != text {
					return "", errors.New("patch remove mismatch")
				}
				srcIndex++
			case '+':
				out = append(out, text)
			default:
				return "", errors.New("invalid patch op")
			}
			i++
		}
	}

	out = append(out, baseLines[srcIndex:]...)
	return strings.Join(out, "\n"), nil
}

func computeTaskFileRevision(data []byte, fileAbs string) taskFileRevision {
	sum := sha256.Sum256(data)
	mtime := int64(0)
	if stat, err := os.Stat(fileAbs); err == nil {
		mtime = stat.ModTime().UnixNano()
	}
	return taskFileRevision{
		MTimeNS: mtime,
		Size:    int64(len(data)),
		SHA256:  hex.EncodeToString(sum[:]),
	}
}

func taskFileRevisionEquals(a, b taskFileRevision) bool {
	if a.MTimeNS != b.MTimeNS {
		return false
	}
	if a.Size != b.Size {
		return false
	}
	return strings.EqualFold(strings.TrimSpace(a.SHA256), strings.TrimSpace(b.SHA256))
}

func (s *Server) resolveTaskFilePath(taskID, relPath string) (string, string, string, string, error) {
	if strings.TrimSpace(relPath) == "" {
		return "", "", "", "", errors.New("INVALID_PATH:path is required")
	}
	projectID, _, _, err := s.findTask(taskID)
	if err != nil {
		return "", "", "", "", errors.New("TASK_NOT_FOUND:" + err.Error())
	}
	repoRoot, err := s.findProjectRepoRoot(projectID)
	if err != nil {
		return "", "", "", "", errors.New("PROJECT_NOT_FOUND:" + err.Error())
	}
	rootAbs, err := filepath.Abs(repoRoot)
	if err != nil {
		return "", "", "", "", errors.New("INVALID_REPO_ROOT:" + err.Error())
	}
	fileAbs, err := filepath.Abs(filepath.Join(repoRoot, relPath))
	if err != nil {
		return "", "", "", "", errors.New("INVALID_PATH:" + err.Error())
	}
	if fileAbs != rootAbs && !strings.HasPrefix(fileAbs, rootAbs+string(os.PathSeparator)) {
		return "", "", "", "", errors.New("INVALID_PATH:path escapes repo root")
	}
	return projectID, rootAbs, relPath, fileAbs, nil
}

func (s *Server) respondFilePathResolveError(w http.ResponseWriter, err error) {
	text := strings.TrimSpace(err.Error())
	parts := strings.SplitN(text, ":", 2)
	code := strings.TrimSpace(parts[0])
	msg := "request failed"
	if len(parts) > 1 {
		msg = strings.TrimSpace(parts[1])
	}
	switch code {
	case "TASK_NOT_FOUND":
		respondError(w, http.StatusNotFound, code, msg)
	case "PROJECT_NOT_FOUND":
		respondError(w, http.StatusNotFound, code, msg)
	case "INVALID_REPO_ROOT":
		respondError(w, http.StatusInternalServerError, code, msg)
	default:
		respondError(w, http.StatusBadRequest, "INVALID_PATH", msg)
	}
}

func (s *Server) isTaskPathGitIgnored(repoRoot, relPath string) bool {
	if strings.TrimSpace(relPath) == "" || relPath == "." || strings.HasPrefix(relPath, "../") {
		return false
	}
	_, err := runGitCombined(repoRoot, "check-ignore", "-q", "--", relPath)
	if err == nil {
		return true
	}
	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) && exitErr.ExitCode() == 1 {
		return false
	}
	return false
}

func (s *Server) handleGetTaskFileTree(w http.ResponseWriter, r *http.Request, taskID string) {
	projectID, _, _, err := s.findTask(taskID)
	if err != nil {
		respondError(w, http.StatusNotFound, "TASK_NOT_FOUND", err.Error())
		return
	}
	repoRoot, err := s.findProjectRepoRoot(projectID)
	if err != nil {
		respondError(w, http.StatusNotFound, "PROJECT_NOT_FOUND", err.Error())
		return
	}

	rel := strings.TrimSpace(r.URL.Query().Get("path"))
	if rel == "" {
		rel = "."
	}

	rootAbs, err := filepath.Abs(repoRoot)
	if err != nil {
		respondError(w, http.StatusInternalServerError, "INVALID_REPO_ROOT", err.Error())
		return
	}

	targetAbs, err := filepath.Abs(filepath.Join(repoRoot, rel))
	if err != nil {
		respondError(w, http.StatusBadRequest, "INVALID_PATH", err.Error())
		return
	}
	if targetAbs != rootAbs && !strings.HasPrefix(targetAbs, rootAbs+string(os.PathSeparator)) {
		respondError(w, http.StatusBadRequest, "INVALID_PATH", "path escapes repo root")
		return
	}

	st, err := os.Stat(targetAbs)
	if err != nil {
		if os.IsNotExist(err) {
			respondError(w, http.StatusNotFound, "PATH_NOT_FOUND", err.Error())
			return
		}
		respondError(w, http.StatusInternalServerError, "FS_STAT_FAILED", err.Error())
		return
	}
	if !st.IsDir() {
		respondError(w, http.StatusBadRequest, "INVALID_PATH", "path is not a directory")
		return
	}

	rows, err := os.ReadDir(targetAbs)
	if err != nil {
		respondError(w, http.StatusInternalServerError, "FS_LIST_FAILED", err.Error())
		return
	}

	type fileTreeEntry struct {
		Name    string `json:"name"`
		Path    string `json:"path"`
		IsDir   bool   `json:"is_dir"`
		Ignored bool   `json:"ignored"`
	}
	entries := make([]fileTreeEntry, 0, len(rows))
	for _, row := range rows {
		name := strings.TrimSpace(row.Name())
		if name == "" {
			continue
		}
		absChild := filepath.Join(targetAbs, name)
		relChild, err := filepath.Rel(rootAbs, absChild)
		if err != nil {
			continue
		}
		relChild = filepath.ToSlash(filepath.Clean(relChild))
		if relChild == "." || strings.HasPrefix(relChild, "../") {
			continue
		}
		entries = append(entries, fileTreeEntry{
			Name:    name,
			Path:    relChild,
			IsDir:   row.IsDir(),
			Ignored: s.isTaskPathGitIgnored(repoRoot, relChild),
		})
	}

	sort.Slice(entries, func(i, j int) bool {
		if entries[i].IsDir != entries[j].IsDir {
			return entries[i].IsDir && !entries[j].IsDir
		}
		return strings.ToLower(entries[i].Name) < strings.ToLower(entries[j].Name)
	})

	relPath, err := filepath.Rel(rootAbs, targetAbs)
	if err != nil || relPath == "." || strings.TrimSpace(relPath) == "" {
		relPath = "."
	} else {
		relPath = filepath.ToSlash(filepath.Clean(relPath))
	}

	respondOK(w, map[string]any{
		"task_id": taskID,
		"path":    relPath,
		"entries": entries,
	})
}

func (s *Server) handleGetTaskFileSearch(w http.ResponseWriter, r *http.Request, taskID string) {
	projectID, _, _, err := s.findTask(taskID)
	if err != nil {
		respondError(w, http.StatusNotFound, "TASK_NOT_FOUND", err.Error())
		return
	}
	repoRoot, err := s.findProjectRepoRoot(projectID)
	if err != nil {
		respondError(w, http.StatusNotFound, "PROJECT_NOT_FOUND", err.Error())
		return
	}
	query := strings.ToLower(strings.TrimSpace(r.URL.Query().Get("q")))
	if query == "" {
		respondError(w, http.StatusBadRequest, "FS_QUERY_REQUIRED", "q is required")
		return
	}
	limit := 50
	if raw := strings.TrimSpace(r.URL.Query().Get("limit")); raw != "" {
		parsed, parseErr := strconv.Atoi(raw)
		if parseErr != nil || parsed <= 0 {
			respondError(w, http.StatusBadRequest, "FS_LIMIT_INVALID", "limit must be positive integer")
			return
		}
		limit = parsed
	}

	type fileTreeEntry struct {
		Name    string `json:"name"`
		Path    string `json:"path"`
		IsDir   bool   `json:"is_dir"`
		Ignored bool   `json:"ignored"`
	}
	entries := make([]fileTreeEntry, 0, limit)
	err = filepath.WalkDir(repoRoot, func(path string, d os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return nil
		}
		if len(entries) >= limit {
			return filepath.SkipAll
		}
		name := strings.TrimSpace(d.Name())
		if name == ".git" && d.IsDir() {
			return filepath.SkipDir
		}
		if d.IsDir() {
			return nil
		}
		if !strings.Contains(strings.ToLower(name), query) {
			return nil
		}
		relPath, relErr := filepath.Rel(repoRoot, path)
		if relErr != nil {
			return nil
		}
		relPath = filepath.ToSlash(filepath.Clean(relPath))
		if relPath == "." || strings.HasPrefix(relPath, "../") {
			return nil
		}
		if s.isTaskPathGitIgnored(repoRoot, relPath) {
			return nil
		}
		entries = append(entries, fileTreeEntry{
			Name:    name,
			Path:    relPath,
			IsDir:   false,
			Ignored: false,
		})
		return nil
	})
	if err != nil {
		respondError(w, http.StatusInternalServerError, "FS_SEARCH_FAILED", err.Error())
		return
	}

	sort.Slice(entries, func(i, j int) bool {
		return strings.ToLower(entries[i].Name) < strings.ToLower(entries[j].Name)
	})
	respondOK(w, map[string]any{
		"task_id": taskID,
		"entries": entries,
	})
}

func (s *Server) handlePostTaskCommit(w http.ResponseWriter, r *http.Request, taskID string) {
	var req struct {
		Message string `json:"message"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondError(w, http.StatusBadRequest, "INVALID_JSON", err.Error())
		return
	}
	message := strings.TrimSpace(req.Message)
	if message == "" {
		respondError(w, http.StatusBadRequest, "INVALID_COMMIT_MESSAGE", "message is required")
		return
	}

	projectID, _, _, err := s.findTask(taskID)
	if err != nil {
		respondError(w, http.StatusNotFound, "TASK_NOT_FOUND", err.Error())
		return
	}
	repoRoot, err := s.findProjectRepoRoot(projectID)
	if err != nil {
		respondError(w, http.StatusNotFound, "PROJECT_NOT_FOUND", err.Error())
		return
	}

	status, err := runGitOutput(repoRoot, "status", "--porcelain")
	if err != nil {
		respondError(w, http.StatusInternalServerError, "GIT_STATUS_FAILED", err.Error())
		return
	}
	if strings.TrimSpace(status) == "" {
		respondError(w, http.StatusConflict, "NOTHING_TO_COMMIT", "working tree is clean")
		return
	}

	if _, err := runGitCombined(repoRoot, "add", "-A"); err != nil {
		respondError(w, http.StatusInternalServerError, "GIT_ADD_FAILED", err.Error())
		return
	}
	if out, err := runGitCombined(repoRoot, "commit", "-m", message); err != nil {
		text := strings.ToLower(strings.TrimSpace(string(out)))
		if strings.Contains(text, "nothing to commit") || strings.Contains(text, "no changes added to commit") {
			respondError(w, http.StatusConflict, "NOTHING_TO_COMMIT", "working tree is clean")
			return
		}
		respondError(w, http.StatusInternalServerError, "GIT_COMMIT_FAILED", err.Error())
		return
	}
	commitHash, err := runGitOutput(repoRoot, "rev-parse", "HEAD")
	if err != nil {
		respondError(w, http.StatusInternalServerError, "GIT_REV_PARSE_FAILED", err.Error())
		return
	}
	respondOK(w, map[string]any{
		"task_id":     taskID,
		"commit_hash": strings.TrimSpace(commitHash),
		"message":     message,
	})
}

func runGitOutput(repoRoot string, args ...string) (string, error) {
	cmd := exec.Command("git", append([]string{"-C", repoRoot}, args...)...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return "", err
	}
	return string(out), nil
}

func runGitCombined(repoRoot string, args ...string) ([]byte, error) {
	cmd := exec.Command("git", append([]string{"-C", repoRoot}, args...)...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return out, err
	}
	return out, nil
}

func (s *Server) buildCommitMessageWithHelper(ctx context.Context, taskID string, files []map[string]string, diff string) (string, error) {
	if s.deps.ConfigStore == nil || s.deps.AppProgramsStore == nil {
		return "", fmt.Errorf("helper dependencies unavailable")
	}
	prompt := buildCommitMessagePrompt(taskID, files, diff)
	prompt = strings.TrimSpace(prompt)
	if prompt == "" {
		return "", nil
	}

	if s.deps.HelperConfigStore != nil {
		openAICfg, err := s.deps.HelperConfigStore.LoadOpenAI()
		if err != nil {
			return "", err
		}
		if isCompleteOpenAIConfig(openAICfg) {
			return s.buildCommitMessageWithOpenAI(ctx, prompt, openAICfg)
		}
	}

	cfg, err := s.deps.ConfigStore.LoadOrInit()
	if err != nil {
		return "", err
	}
	helperID := strings.TrimSpace(cfg.Defaults.HelperProgram)
	if helperID == "" {
		helperID = "codex"
	}

	appPrograms, err := s.deps.AppProgramsStore.LoadOrInit()
	if err != nil {
		return "", err
	}

	var command string
	for _, provider := range appPrograms.Providers {
		if provider.ID == helperID {
			command = strings.TrimSpace(provider.CommitMessageCommand)
			if command == "" {
				command = strings.TrimSpace(provider.Command)
			}
			break
		}
	}
	if command == "" {
		return "", fmt.Errorf("helper command not found: %s", helperID)
	}
	commandName, commandArgs, err := splitCommand(command)
	if err != nil {
		return "", err
	}

	execute := s.deps.ExecuteCommand
	if execute == nil {
		execute = runLocalCommand
	}

	ctx, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()

	var lastErr error
	attempts := [][]string{
		{"-p", prompt},
		{prompt},
		{},
	}
	for _, args := range attempts {
		callArgs := make([]string, 0, len(commandArgs)+len(args))
		callArgs = append(callArgs, commandArgs...)
		callArgs = append(callArgs, args...)
		output, err := execute(ctx, commandName, callArgs...)
		if err != nil {
			lastErr = buildHelperExecError(err, output)
			continue
		}
		message := strings.TrimSpace(string(output))
		if message == "" {
			lastErr = fmt.Errorf("empty helper output")
			continue
		}
		return message, nil
	}

	if lastErr == nil {
		lastErr = fmt.Errorf("helper command returned empty output")
	}
	return "", lastErr
}

func isCompleteOpenAIConfig(cfg helperconfig.OpenAIConfig) bool {
	return strings.TrimSpace(cfg.Endpoint) != "" &&
		strings.TrimSpace(cfg.Model) != "" &&
		strings.TrimSpace(cfg.APIKey) != ""
}

func (s *Server) buildCommitMessageWithOpenAI(ctx context.Context, prompt string, cfg helperconfig.OpenAIConfig) (string, error) {
	ctx, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()

	payload := map[string]any{
		"model": cfg.Model,
		"messages": []map[string]string{
			{
				"role":    "user",
				"content": prompt,
			},
		},
		"temperature": 0.2,
	}
	data, err := json.Marshal(payload)
	if err != nil {
		return "", err
	}

	base := strings.TrimSuffix(strings.TrimSpace(cfg.Endpoint), "/")
	url := base
	if !strings.HasSuffix(strings.ToLower(base), "/chat/completions") {
		url = base + "/chat/completions"
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(data))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+strings.TrimSpace(cfg.APIKey))

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", err
	}
	defer func() {
		_ = resp.Body.Close()
	}()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		errText := strings.TrimSpace(string(body))
		if errText == "" {
			errText = http.StatusText(resp.StatusCode)
		}
		const maxLen = 600
		if len(errText) > maxLen {
			errText = errText[:maxLen] + "..."
		}
		return "", fmt.Errorf("openai request failed: status %d: %s", resp.StatusCode, errText)
	}

	var result struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
	}
	if err := json.Unmarshal(body, &result); err != nil {
		return "", err
	}
	if len(result.Choices) == 0 {
		return "", fmt.Errorf("openai response missing choices")
	}
	out := strings.TrimSpace(result.Choices[0].Message.Content)
	if out == "" {
		return "", fmt.Errorf("openai returned empty output")
	}
	return out, nil
}

func buildHelperExecError(err error, output []byte) error {
	text := strings.TrimSpace(string(output))
	if text == "" {
		return err
	}
	const maxLen = 600
	if len(text) > maxLen {
		text = text[:maxLen] + "..."
	}
	return fmt.Errorf("%w: %s", err, text)
}

func splitCommand(raw string) (name string, args []string, err error) {
	fields := strings.Fields(strings.TrimSpace(raw))
	if len(fields) == 0 {
		return "", nil, fmt.Errorf("helper command is empty")
	}
	return fields[0], fields[1:], nil
}

func buildCommitMessagePrompt(taskID string, files []map[string]string, diff string) string {
	var b strings.Builder
	fmt.Fprintf(&b, "Generate a concise commit message for task %s.\n\n", taskID)
	fmt.Fprintf(&b, "Changed files (%d):\n", len(files))
	for _, file := range files {
		status := strings.TrimSpace(file["status"])
		path := strings.TrimSpace(file["path"])
		if path == "" {
			continue
		}
		fmt.Fprintf(&b, "- %s %s\n", status, path)
	}
	fmt.Fprintf(&b, "\nDiff:\n%s\n", strings.TrimSpace(diff))
	fmt.Fprint(&b, "\nReturn only the commit message. 要填写有意义的信息")
	return b.String()
}

func runLocalCommand(ctx context.Context, name string, args ...string) ([]byte, error) {
	cmd := exec.CommandContext(ctx, name, args...)
	return cmd.CombinedOutput()
}

func parseGitStatusPorcelain(status string) []map[string]string {
	files := make([]map[string]string, 0)
	lines := strings.Split(strings.ReplaceAll(status, "\r\n", "\n"), "\n")
	for _, line := range lines {
		if strings.TrimSpace(line) == "" {
			continue
		}
		raw := line
		if len(raw) < 3 {
			continue
		}
		code := strings.TrimSpace(raw[:2])
		path := strings.TrimSpace(raw[3:])
		if idx := strings.LastIndex(path, " -> "); idx >= 0 {
			path = strings.TrimSpace(path[idx+4:])
		}
		if path == "" {
			continue
		}
		files = append(files, map[string]string{
			"path":   path,
			"status": code,
		})
	}
	return files
}
