package tmux

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

type Adapter struct {
	exec       Exec
	tmuxSocket string
}

type processInfo struct {
	pid  int
	ppid int
	comm string
	args string
	name string
}

func NewAdapter(e Exec) *Adapter {
	return &Adapter{exec: e}
}

func NewAdapterWithSocket(e Exec, socket string) *Adapter {
	return &Adapter{exec: e, tmuxSocket: socket}
}

func (a *Adapter) SocketName() string {
	if a == nil {
		return ""
	}
	return strings.TrimSpace(a.tmuxSocket)
}

func (a *Adapter) ListSessions() ([]string, error) {
	out, err := a.exec.Output("tmux", a.withSocket("list-panes", "-a", "-F", "#{session_name}:#{window_index}.#{pane_index}")...)
	if err != nil {
		return nil, err
	}
	text := strings.TrimSpace(string(out))
	if text == "" {
		return []string{}, nil
	}
	return strings.Split(text, "\n"), nil
}

func (a *Adapter) PaneExists(target string) (bool, error) {
	needle := strings.TrimSpace(target)
	if needle == "" {
		return false, nil
	}
	panes, err := a.ListSessions()
	if err != nil {
		return false, err
	}
	for _, pane := range panes {
		if strings.TrimSpace(pane) == needle {
			return true, nil
		}
	}
	return false, nil
}

func (a *Adapter) SelectPane(target string) error {
	return a.exec.Run("tmux", a.withSocket("select-pane", "-t", target)...)
}

func (a *Adapter) SendInput(target, text string) error {
	return a.exec.Run("tmux", a.withSocket("send-keys", "-l", "-t", target, text)...)
}

func (a *Adapter) Resize(target string, cols, rows int) error {
	windowTarget := target
	if dot := strings.LastIndex(target, "."); dot > strings.LastIndex(target, ":") {
		windowTarget = target[:dot]
	}
	if err := a.exec.Run("tmux", a.withSocket("resize-window", "-t", windowTarget, "-x", strconv.Itoa(cols), "-y", strconv.Itoa(rows))...); err != nil {
		return err
	}
	if err := a.exec.Run("tmux", a.withSocket("resize-pane", "-t", target, "-x", strconv.Itoa(cols), "-y", strconv.Itoa(rows))...); err != nil {
		return err
	}

	// If the pane is still much shorter than requested rows, tmux layout is constraining it.
	// In this case, auto-zoom the selected pane so fullscreen apps (htop/top) get full height.
	paneHeight, zoomed, err := a.readPaneHeightAndZoomFlag(target)
	if err != nil {
		return nil
	}
	if !zoomed && paneHeight > 0 && paneHeight < rows-1 {
		if err := a.exec.Run("tmux", a.withSocket("resize-pane", "-Z", "-t", target)...); err != nil {
			return err
		}
	}
	return nil
}

func (a *Adapter) readPaneHeightAndZoomFlag(target string) (int, bool, error) {
	out, err := a.exec.Output("tmux", a.withSocket("display-message", "-p", "-t", target, "#{pane_height} #{window_zoomed_flag}")...)
	if err != nil {
		return 0, false, err
	}
	fields := strings.Fields(strings.TrimSpace(string(out)))
	if len(fields) < 2 {
		return 0, false, fmt.Errorf("unexpected tmux pane size output: %q", string(out))
	}
	height, err := strconv.Atoi(fields[0])
	if err != nil {
		return 0, false, err
	}
	return height, fields[1] == "1", nil
}

func (a *Adapter) CapturePane(target string) (string, error) {
	out, err := a.exec.Output("tmux", a.withSocket("capture-pane", "-p", "-e", "-N", "-t", target)...)
	if err != nil {
		return "", err
	}
	return string(out), nil
}

func (a *Adapter) CaptureHistory(target string, lines int) (string, error) {
	if lines <= 0 {
		lines = 2000
	}
	start := fmt.Sprintf("-%d", lines)
	out, err := a.exec.Output("tmux", a.withSocket("capture-pane", "-p", "-e", "-N", "-S", start, "-E", "-", "-t", target)...)
	if err != nil {
		return "", err
	}
	return string(out), nil
}

func (a *Adapter) StartPipePane(target, shellCmd string) error {
	return a.exec.Run("tmux", a.withSocket("pipe-pane", "-O", "-t", target, shellCmd)...)
}

func (a *Adapter) StopPipePane(target string) error {
	return a.exec.Run("tmux", a.withSocket("pipe-pane", "-t", target)...)
}

func (a *Adapter) GetPaneOption(target, key string) (string, error) {
	out, err := a.exec.Output("tmux", a.withSocket("show-options", "-p", "-v", "-t", target, key)...)
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(out)), nil
}

func (a *Adapter) SetPaneOption(target, key, value string) error {
	return a.exec.Run("tmux", a.withSocket("set-option", "-p", "-t", target, key, value)...)
}

func (a *Adapter) CursorPosition(target string) (int, int, error) {
	out, err := a.exec.Output("tmux", a.withSocket("display-message", "-p", "-t", target, "#{cursor_x} #{cursor_y}")...)
	if err != nil {
		return 0, 0, err
	}
	fields := strings.Fields(string(out))
	if len(fields) < 2 {
		return 0, 0, fmt.Errorf("unexpected tmux cursor output: %q", string(out))
	}
	x, err := strconv.Atoi(fields[0])
	if err != nil {
		return 0, 0, err
	}
	y, err := strconv.Atoi(fields[1])
	if err != nil {
		return 0, 0, err
	}
	return x, y, nil
}

func (a *Adapter) PaneLastActiveAt(target string) (time.Time, error) {
	out, err := a.exec.Output("tmux", a.withSocket("display-message", "-p", "-t", target, "#{pane_activity}")...)
	if err != nil {
		return time.Time{}, err
	}
	raw := strings.TrimSpace(string(out))
	if raw == "" || raw == "0" {
		return time.Time{}, nil
	}
	sec, err := strconv.ParseInt(raw, 10, 64)
	if err != nil || sec <= 0 {
		return time.Time{}, nil
	}
	return time.Unix(sec, 0).UTC(), nil
}

func (a *Adapter) PaneTitleAndCurrentCommand(target string) (string, string, error) {
	out, err := a.exec.Output("tmux", a.withSocket("display-message", "-p", "-t", target, "#{pane_title}\t#{pane_current_command}\t#{pane_pid}")...)
	if err != nil {
		return "", "", err
	}
	text := strings.TrimSpace(string(out))
	if text == "" {
		return "", "", nil
	}
	parts := strings.SplitN(text, "\t", 3)
	if len(parts) == 1 {
		return strings.TrimSpace(parts[0]), "", nil
	}
	title := strings.TrimSpace(parts[0])
	current := strings.TrimSpace(parts[1])
	if len(parts) < 3 {
		return title, current, nil
	}
	pid, err := strconv.Atoi(strings.TrimSpace(parts[2]))
	if err != nil || pid <= 0 {
		return title, current, nil
	}
	derived, derr := a.deriveProcessLabel(pid, current)
	if derr != nil || derived == "" {
		return title, current, nil
	}
	return title, derived, nil
}

func (a *Adapter) deriveProcessLabel(panePID int, fallback string) (string, error) {
	out, err := a.exec.Output("ps", "-axo", "pid=,ppid=,comm=,args=")
	if err != nil {
		return "", err
	}
	byPID := map[int]processInfo{}
	children := map[int][]int{}
	for _, line := range strings.Split(string(out), "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) < 3 {
			continue
		}
		pid, err1 := strconv.Atoi(fields[0])
		ppid, err2 := strconv.Atoi(fields[1])
		if err1 != nil || err2 != nil {
			continue
		}
		comm := normalizeProcName(fields[2])
		if comm == "" {
			continue
		}
		args := ""
		if len(fields) > 3 {
			args = strings.TrimSpace(strings.Join(fields[3:], " "))
		}
		name := deriveDisplayProcessName(comm, args)
		if name == "" {
			name = comm
		}
		byPID[pid] = processInfo{pid: pid, ppid: ppid, comm: comm, args: args, name: name}
		children[ppid] = append(children[ppid], pid)
	}
	if _, ok := byPID[panePID]; !ok {
		return strings.TrimSpace(fallback), nil
	}
	chain := collectActiveProcessChain(panePID, byPID, children)
	if len(chain) == 0 {
		return strings.TrimSpace(fallback), nil
	}
	primary := chain[0]
	secondary := chain[len(chain)-1]
	if primary == "" {
		return strings.TrimSpace(fallback), nil
	}
	if secondary == "" || secondary == primary {
		return primary, nil
	}
	return fmt.Sprintf("%s (%s)", primary, secondary), nil
}

func deriveDisplayProcessName(comm, args string) string {
	comm = normalizeProcName(comm)
	if comm == "" {
		return ""
	}
	derived := deriveEntrypointName(comm, args)
	if derived != "" {
		return derived
	}
	return comm
}

func deriveEntrypointName(comm, args string) string {
	tokens := strings.Fields(strings.TrimSpace(args))
	if len(tokens) < 2 {
		return ""
	}
	i := 1
	for i < len(tokens) {
		token := strings.TrimSpace(tokens[i])
		if token == "" {
			i++
			continue
		}
		if token == "--" {
			if i+1 < len(tokens) {
				return normalizeEntrypointToken(tokens[i+1])
			}
			return ""
		}
		if strings.HasPrefix(token, "-") {
			if optionConsumesValue(token) {
				i += 2
				continue
			}
			i++
			continue
		}
		if normalizeProcName(token) == comm {
			i++
			continue
		}
		return normalizeEntrypointToken(token)
	}
	return ""
}

func optionConsumesValue(token string) bool {
	if token == "" || !strings.HasPrefix(token, "-") {
		return false
	}
	if strings.Contains(token, "=") {
		return false
	}
	switch token {
	case "-m", "-c", "-e", "-r", "-cp", "-classpath", "-jar", "--require", "--import", "--loader", "--project", "--config", "--eval":
		return true
	default:
		return false
	}
}

func normalizeEntrypointToken(token string) string {
	token = strings.TrimSpace(strings.Trim(token, `"'`))
	if token == "" {
		return ""
	}
	if !looksLikeEntrypointToken(token) {
		return ""
	}
	name := normalizeProcName(token)
	if name == "" {
		return ""
	}
	ext := strings.ToLower(filepath.Ext(name))
	switch ext {
	case ".js", ".mjs", ".cjs", ".ts", ".tsx", ".jsx", ".py", ".rb", ".pl", ".php", ".lua", ".jar", ".sh", ".bash", ".zsh", ".fish":
		name = strings.TrimSuffix(name, ext)
	}
	return strings.TrimSpace(name)
}

func looksLikeEntrypointToken(token string) bool {
	if token == "" || token == "-" {
		return false
	}
	if strings.HasPrefix(token, "@") {
		return true
	}
	if strings.Contains(token, "/") || strings.Contains(token, `\`) {
		return true
	}
	if strings.HasPrefix(token, ".") {
		return true
	}
	lower := strings.ToLower(token)
	switch {
	case strings.HasSuffix(lower, ".js"),
		strings.HasSuffix(lower, ".mjs"),
		strings.HasSuffix(lower, ".cjs"),
		strings.HasSuffix(lower, ".ts"),
		strings.HasSuffix(lower, ".tsx"),
		strings.HasSuffix(lower, ".jsx"),
		strings.HasSuffix(lower, ".py"),
		strings.HasSuffix(lower, ".rb"),
		strings.HasSuffix(lower, ".pl"),
		strings.HasSuffix(lower, ".php"),
		strings.HasSuffix(lower, ".lua"),
		strings.HasSuffix(lower, ".jar"),
		strings.HasSuffix(lower, ".sh"),
		strings.HasSuffix(lower, ".bash"),
		strings.HasSuffix(lower, ".zsh"),
		strings.HasSuffix(lower, ".fish"):
		return true
	default:
		return false
	}
}

func normalizeProcName(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ""
	}
	base := filepath.Base(raw)
	if base == "." || base == "/" {
		return raw
	}
	return base
}

func isShellProcess(name string) bool {
	switch strings.ToLower(strings.TrimSpace(name)) {
	case "", "sh", "bash", "zsh", "fish", "dash", "ksh", "tcsh", "csh", "login":
		return true
	default:
		return false
	}
}

func collectActiveProcessChain(rootPID int, byPID map[int]processInfo, children map[int][]int) []string {
	bestLeaf := 0
	var visit func(pid int)
	visit = func(pid int) {
		for _, child := range children[pid] {
			visit(child)
		}
		if pid == rootPID {
			return
		}
		p := byPID[pid]
		if isShellProcess(p.comm) {
			return
		}
		nonShellChild := false
		for _, child := range children[pid] {
			cp := byPID[child]
			if !isShellProcess(cp.comm) {
				nonShellChild = true
				break
			}
		}
		if !nonShellChild && pid > bestLeaf {
			bestLeaf = pid
		}
	}
	visit(rootPID)
	if bestLeaf == 0 {
		return nil
	}
	reversed := make([]string, 0, 8)
	for pid := bestLeaf; pid != 0 && pid != rootPID; pid = byPID[pid].ppid {
		p := byPID[pid]
		if !isShellProcess(p.comm) {
			reversed = append(reversed, p.name)
		}
	}
	if len(reversed) == 0 {
		return nil
	}
	chain := make([]string, 0, len(reversed))
	for i := len(reversed) - 1; i >= 0; i-- {
		chain = append(chain, reversed[i])
	}
	return chain
}

func (a *Adapter) CreateSiblingPane(target string) (string, error) {
	shellCmd, err := paneBootstrapShellCommand()
	if err != nil {
		return "", err
	}
	out, err := a.exec.Output("tmux", a.withSocket("split-window", "-h", "-t", target, "-P", "-F", "#{session_name}:#{window_index}.#{pane_index}", shellCmd)...)
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(out)), nil
}

func (a *Adapter) CreateSiblingPaneInDir(target, cwd string) (string, error) {
	if strings.TrimSpace(cwd) == "" {
		return "", errors.New("pane cwd is required")
	}
	shellCmd, err := paneBootstrapShellCommand()
	if err != nil {
		return "", err
	}
	out, err := a.exec.Output("tmux", a.withSocket("split-window", "-h", "-t", target, "-c", cwd, "-P", "-F", "#{session_name}:#{window_index}.#{pane_index}", shellCmd)...)
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(out)), nil
}

func (a *Adapter) CreateChildPane(target string) (string, error) {
	shellCmd, err := paneBootstrapShellCommand()
	if err != nil {
		return "", err
	}
	out, err := a.exec.Output("tmux", a.withSocket("split-window", "-v", "-t", target, "-P", "-F", "#{session_name}:#{window_index}.#{pane_index}", shellCmd)...)
	if err == nil {
		return strings.TrimSpace(string(out)), nil
	}

	// Fallback to full-window split when target pane is too small for a local vertical split.
	out, err2 := a.exec.Output("tmux", a.withSocket("split-window", "-v", "-f", "-t", target, "-P", "-F", "#{session_name}:#{window_index}.#{pane_index}", shellCmd)...)
	if err2 != nil {
		return "", err
	}
	return strings.TrimSpace(string(out)), nil
}

func (a *Adapter) CreateChildPaneInDir(target, cwd string) (string, error) {
	if strings.TrimSpace(cwd) == "" {
		return "", errors.New("pane cwd is required")
	}
	shellCmd, err := paneBootstrapShellCommand()
	if err != nil {
		return "", err
	}
	out, err := a.exec.Output("tmux", a.withSocket("split-window", "-v", "-t", target, "-c", cwd, "-P", "-F", "#{session_name}:#{window_index}.#{pane_index}", shellCmd)...)
	if err == nil {
		return strings.TrimSpace(string(out)), nil
	}

	// Fallback to full-window split when target pane is too small for a local vertical split.
	out, err2 := a.exec.Output("tmux", a.withSocket("split-window", "-v", "-f", "-t", target, "-c", cwd, "-P", "-F", "#{session_name}:#{window_index}.#{pane_index}", shellCmd)...)
	if err2 != nil {
		return "", err
	}
	return strings.TrimSpace(string(out)), nil
}

func (a *Adapter) CreateRootPane() (string, error) {
	shellCmd, err := paneBootstrapShellCommand()
	if err != nil {
		return "", err
	}
	out, err := a.exec.Output("tmux", a.withSocket("new-window", "-P", "-F", "#{session_name}:#{window_index}.#{pane_index}", shellCmd)...)
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(out)), nil
}

func (a *Adapter) CreateRootPaneInDir(cwd string) (string, error) {
	if strings.TrimSpace(cwd) == "" {
		return "", errors.New("pane cwd is required")
	}
	shellCmd, err := paneBootstrapShellCommand()
	if err != nil {
		return "", err
	}
	out, err := a.exec.Output("tmux", a.withSocket("new-window", "-c", cwd, "-P", "-F", "#{session_name}:#{window_index}.#{pane_index}", shellCmd)...)
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(out)), nil
}

func paneBootstrapShellCommand() (string, error) {
	rcPath, err := ensurePaneBootstrapRCFile()
	if err != nil {
		return "", err
	}
	return "bash --rcfile " + shellSingleQuote(rcPath) + " -i", nil
}

func ensurePaneBootstrapRCFile() (string, error) {
	dir := filepath.Join(os.TempDir(), "shellman-bootstrap")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}
	path := filepath.Join(dir, "bash-shell-ready.rc")
	if err := os.WriteFile(path, []byte(paneBootstrapRCContent), 0o644); err != nil {
		return "", err
	}
	return path, nil
}

func shellSingleQuote(input string) string {
	if input == "" {
		return "''"
	}
	return "'" + strings.ReplaceAll(input, "'", `'"'"'`) + "'"
}

const paneBootstrapRCContent = `
if [ -f "$HOME/.bashrc" ]; then
  . "$HOME/.bashrc"
fi

__shellman_ready_once() {
  tmux set-option -p -t "$TMUX_PANE" @shellman_ready 1 >/dev/null 2>&1 || true
  if [ -n "${PROMPT_COMMAND:-}" ]; then
    PROMPT_COMMAND="${PROMPT_COMMAND/__shellman_ready_once; /}"
    PROMPT_COMMAND="${PROMPT_COMMAND/__shellman_ready_once;/}"
    PROMPT_COMMAND="${PROMPT_COMMAND/__shellman_ready_once/}"
    PROMPT_COMMAND="${PROMPT_COMMAND#; }"
    PROMPT_COMMAND="${PROMPT_COMMAND#;}"
  fi
}

if [ -n "${PROMPT_COMMAND:-}" ]; then
  PROMPT_COMMAND="__shellman_ready_once; ${PROMPT_COMMAND}"
else
  PROMPT_COMMAND="__shellman_ready_once"
fi
`

func (a *Adapter) ServerInstanceID() (string, error) {
	out, err := a.exec.Output("tmux", a.withSocket("display-message", "-p", "#{pid}")...)
	if err != nil {
		return "", err
	}
	pid := strings.TrimSpace(string(out))
	socket := strings.TrimSpace(a.tmuxSocket)
	if socket == "" {
		socket = "default"
	}
	return socket + ":" + pid, nil
}

func (a *Adapter) withSocket(args ...string) []string {
	if a.tmuxSocket == "" {
		return args
	}
	return append([]string{"-L", a.tmuxSocket}, args...)
}
