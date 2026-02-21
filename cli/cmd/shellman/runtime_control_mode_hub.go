package main

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"log/slog"
	"os/exec"
	"strings"
	"sync"
	"unicode/utf8"

	tmuxpkg "shellman/cli/internal/tmux"
)

type paneOutputRealtimeSource interface {
	Subscribe(target string) (<-chan string, func(), error)
}

type controlSessionClient interface {
	Lines() <-chan string
	PaneMap() map[string]string
	Close() error
}

type controlSessionPaneMapRefresher interface {
	RefreshPaneMap() error
}

type controlSessionFactory func(ctx context.Context, socket, session string) (controlSessionClient, error)

type controlModeSubscription struct {
	target string
	out    chan string
}

type controlModeSessionWatcher struct {
	session     string
	client      controlSessionClient
	nextID      int
	subs        map[int]controlModeSubscription
	utf8Pending map[string][]byte
}

type ControlModeHub struct {
	ctx     context.Context
	socket  string
	logger  *slog.Logger
	factory controlSessionFactory

	mu       sync.Mutex
	sessions map[string]*controlModeSessionWatcher
}

func NewControlModeHub(ctx context.Context, socket string, logger *slog.Logger) *ControlModeHub {
	return newControlModeHubWithFactory(ctx, socket, logger, newRealControlSessionClient)
}

func newControlModeHubWithFactory(ctx context.Context, socket string, logger *slog.Logger, factory controlSessionFactory) *ControlModeHub {
	if ctx == nil {
		ctx = context.Background()
	}
	if logger == nil {
		logger = newRuntimeLogger(io.Discard)
	}
	if factory == nil {
		factory = newRealControlSessionClient
	}
	return &ControlModeHub{
		ctx:      ctx,
		socket:   strings.TrimSpace(socket),
		logger:   logger,
		factory:  factory,
		sessions: map[string]*controlModeSessionWatcher{},
	}
}

func (h *ControlModeHub) Subscribe(target string) (<-chan string, func(), error) {
	if h == nil {
		return nil, nil, fmt.Errorf("nil control mode hub")
	}
	target = strings.TrimSpace(target)
	if target == "" {
		return nil, nil, fmt.Errorf("empty target")
	}
	session := sessionFromPaneTarget(target)
	if session == "" {
		return nil, nil, fmt.Errorf("invalid pane target: %s", target)
	}

	h.mu.Lock()
	watcher, ok := h.sessions[session]
	if !ok {
		client, err := h.factory(h.ctx, h.socket, session)
		if err != nil {
			h.mu.Unlock()
			return nil, nil, err
		}
		watcher = &controlModeSessionWatcher{
			session:     session,
			client:      client,
			subs:        map[int]controlModeSubscription{},
			utf8Pending: map[string][]byte{},
		}
		h.sessions[session] = watcher
		go h.loopSession(watcher)
	}
	watcher.nextID++
	subID := watcher.nextID
	out := make(chan string, 128)
	watcher.subs[subID] = controlModeSubscription{target: target, out: out}
	subsTotal := len(watcher.subs)
	h.mu.Unlock()
	if refresher, ok := watcher.client.(controlSessionPaneMapRefresher); ok {
		if err := refresher.RefreshPaneMap(); err != nil {
			h.logger.Warn("control mode pane map refresh failed", "session", session, "target", target, "error", err)
		}
	}
	h.logger.Debug("control mode subscribe", "session", session, "target", target, "subs_total", subsTotal)

	unsubscribe := func() {
		h.mu.Lock()
		defer h.mu.Unlock()
		w := h.sessions[session]
		if w == nil {
			return
		}
		delete(w.subs, subID)
		subsRemaining := len(w.subs)
		sessionClosed := false
		if len(w.subs) == 0 {
			_ = w.client.Close()
			delete(h.sessions, session)
			sessionClosed = true
		}
		h.logger.Debug("control mode unsubscribe", "session", session, "target", target, "subs_remaining", subsRemaining, "session_closed", sessionClosed)
	}
	return out, unsubscribe, nil
}

func (h *ControlModeHub) loopSession(w *controlModeSessionWatcher) {
	for {
		select {
		case <-h.ctx.Done():
			_ = w.client.Close()
			return
		case line, ok := <-w.client.Lines():
			if !ok {
				return
			}
			ev, ok := tmuxpkg.ParseControlOutputLine(line)
			if !ok {
				continue
			}
			paneMap := w.client.PaneMap()
			target := strings.TrimSpace(paneMap[ev.PaneID])
			if target == "" {
				if refresher, ok := w.client.(controlSessionPaneMapRefresher); ok {
					if err := refresher.RefreshPaneMap(); err != nil {
						h.logger.Warn("control mode pane map refresh failed", "session", w.session, "pane_id", ev.PaneID, "error", err)
					}
					paneMap = w.client.PaneMap()
					target = strings.TrimSpace(paneMap[ev.PaneID])
				}
			}
			if target == "" {
				h.logger.Debug("control mode output dropped", "session", w.session, "pane_id", ev.PaneID, "reason", "unknown-pane-id")
				continue
			}
			data := h.consumeUTF8Chunk(w, target, ev.Data)
			if data == "" {
				continue
			}
			h.broadcast(w.session, target, data)
		}
	}
}

func (h *ControlModeHub) consumeUTF8Chunk(w *controlModeSessionWatcher, target, chunk string) string {
	if w == nil {
		return chunk
	}
	if w.utf8Pending == nil {
		w.utf8Pending = map[string][]byte{}
	}
	merged := make([]byte, 0, len(w.utf8Pending[target])+len(chunk))
	merged = append(merged, w.utf8Pending[target]...)
	merged = append(merged, chunk...)
	emit, pending := splitUTF8Payload(merged)
	if len(pending) == 0 {
		delete(w.utf8Pending, target)
	} else {
		w.utf8Pending[target] = append([]byte(nil), pending...)
	}
	return string(emit)
}

func splitUTF8Payload(data []byte) (emit []byte, pending []byte) {
	if len(data) == 0 {
		return nil, nil
	}
	pos := 0
	for pos < len(data) {
		if !utf8.FullRune(data[pos:]) {
			return data[:pos], data[pos:]
		}
		r, size := utf8.DecodeRune(data[pos:])
		if r == utf8.RuneError && size == 1 {
			// Invalid standalone byte: forward it immediately rather than stalling.
			pos++
			continue
		}
		pos += size
	}
	return data, nil
}

func (h *ControlModeHub) broadcast(session, target, data string) {
	h.mu.Lock()
	watcher := h.sessions[session]
	if watcher == nil {
		h.mu.Unlock()
		return
	}
	outputs := make([]chan string, 0, len(watcher.subs))
	for _, sub := range watcher.subs {
		if sub.target == target {
			outputs = append(outputs, sub.out)
		}
	}
	h.mu.Unlock()
	if len(outputs) == 0 {
		h.logger.Debug("control mode output dropped", "session", session, "target", target, "reason", "no-subscriber")
		return
	}

	for _, out := range outputs {
		select {
		case out <- data:
		default:
			h.logger.Warn("control mode output subscriber backpressure", "session", session, "target", target)
		}
	}
}

func sessionFromPaneTarget(target string) string {
	target = strings.TrimSpace(target)
	idx := strings.Index(target, ":")
	if idx <= 0 {
		return ""
	}
	return target[:idx]
}

type realControlSessionClient struct {
	lines      chan string
	paneMapMu  sync.RWMutex
	paneIDToTG map[string]string
	socket     string
	session    string

	stdin io.WriteCloser
	cmd   *exec.Cmd

	closeOnce sync.Once
	linesOnce sync.Once
}

func newRealControlSessionClient(ctx context.Context, socket, session string) (controlSessionClient, error) {
	paneMap, err := loadPaneIDMap(socket, session)
	if err != nil {
		return nil, err
	}

	args := append(tmuxArgsWithSocket(socket), "-C", "attach-session", "-t", session)
	cmd := exec.CommandContext(ctx, "tmux", args...)
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, err
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		return nil, err
	}
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return nil, err
	}
	if err := cmd.Start(); err != nil {
		return nil, err
	}

	c := &realControlSessionClient{
		lines:      make(chan string, 512),
		paneIDToTG: paneMap,
		socket:     strings.TrimSpace(socket),
		session:    strings.TrimSpace(session),
		stdin:      stdin,
		cmd:        cmd,
	}

	go c.scanStdout(stdout)
	go func() {
		_, _ = io.Copy(io.Discard, stderr)
	}()
	return c, nil
}

func (c *realControlSessionClient) Lines() <-chan string {
	return c.lines
}

func (c *realControlSessionClient) PaneMap() map[string]string {
	c.paneMapMu.RLock()
	defer c.paneMapMu.RUnlock()
	out := make(map[string]string, len(c.paneIDToTG))
	for k, v := range c.paneIDToTG {
		out[k] = v
	}
	return out
}

func (c *realControlSessionClient) RefreshPaneMap() error {
	paneMap, err := loadPaneIDMap(c.socket, c.session)
	if err != nil {
		return err
	}
	c.paneMapMu.Lock()
	c.paneIDToTG = paneMap
	c.paneMapMu.Unlock()
	return nil
}

func (c *realControlSessionClient) Close() error {
	c.closeOnce.Do(func() {
		if c.stdin != nil {
			_, _ = io.WriteString(c.stdin, "detach-client\n")
			_ = c.stdin.Close()
		}
		if c.cmd != nil {
			if c.cmd.Process != nil {
				_ = c.cmd.Process.Kill()
			}
			_, _ = c.cmd.Process.Wait()
		}
	})
	return nil
}

func (c *realControlSessionClient) scanStdout(stdout io.Reader) {
	defer c.closeLines()
	sc := bufio.NewScanner(stdout)
	sc.Buffer(make([]byte, 1024), 1024*1024)
	for sc.Scan() {
		line := sc.Text()
		select {
		case c.lines <- line:
		default:
		}
	}
	_ = c.Close()
}

func (c *realControlSessionClient) closeLines() {
	c.linesOnce.Do(func() {
		close(c.lines)
	})
}

func loadPaneIDMap(socket, session string) (map[string]string, error) {
	args := append(tmuxArgsWithSocket(socket), "list-panes", "-s", "-t", session, "-F", "#{pane_id}\t#{session_name}:#{window_index}.#{pane_index}")
	out, err := exec.Command("tmux", args...).CombinedOutput()
	if err != nil {
		msg := strings.TrimSpace(string(out))
		if msg == "" {
			return nil, err
		}
		return nil, fmt.Errorf("%w: %s", err, msg)
	}
	paneMap := map[string]string{}
	for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		parts := strings.SplitN(line, "\t", 2)
		if len(parts) != 2 {
			continue
		}
		paneID := strings.TrimSpace(parts[0])
		target := strings.TrimSpace(parts[1])
		if paneID == "" || target == "" {
			continue
		}
		paneMap[paneID] = target
	}
	if len(paneMap) == 0 {
		return nil, fmt.Errorf("no panes found for session %s", session)
	}
	return paneMap, nil
}

func tmuxArgsWithSocket(socket string) []string {
	socket = strings.TrimSpace(socket)
	if socket == "" {
		return nil
	}
	return []string{"-L", socket}
}
