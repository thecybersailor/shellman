package tmux

import (
	"strings"
	"testing"
	"time"
)

type FakeExec struct {
	OutputText string
	LastArgs   string
	RunCalls   []string
}

func (f *FakeExec) Output(name string, args ...string) ([]byte, error) {
	f.LastArgs = strings.Join(append([]string{name}, args...), " ")
	return []byte(f.OutputText), nil
}

func (f *FakeExec) Run(name string, args ...string) error {
	f.LastArgs = strings.Join(append([]string{name}, args...), " ")
	f.RunCalls = append(f.RunCalls, f.LastArgs)
	return nil
}

func TestAdapter_ListSessions_UsesExactCommand(t *testing.T) {
	f := &FakeExec{OutputText: "s1: 1 windows"}
	a := NewAdapter(f)
	_, err := a.ListSessions()
	if err != nil {
		t.Fatalf("list failed: %v", err)
	}
	if f.LastArgs != "tmux list-panes -a -F #{session_name}:#{window_index}.#{pane_index}" {
		t.Fatalf("unexpected command: %s", f.LastArgs)
	}
}

func TestAdapter_ListSessions_WithTmuxSocket(t *testing.T) {
	f := &FakeExec{OutputText: "s1"}
	a := NewAdapterWithSocket(f, "tt_e2e")
	_, err := a.ListSessions()
	if err != nil {
		t.Fatalf("list failed: %v", err)
	}
	if f.LastArgs != "tmux -L tt_e2e list-panes -a -F #{session_name}:#{window_index}.#{pane_index}" {
		t.Fatalf("unexpected command: %s", f.LastArgs)
	}
}

func TestAdapter_CapturePane_UsesVisualLineLayout(t *testing.T) {
	f := &FakeExec{OutputText: "ok"}
	a := NewAdapter(f)
	_, err := a.CapturePane("e2e:0.0")
	if err != nil {
		t.Fatalf("capture failed: %v", err)
	}
	if f.LastArgs != "tmux capture-pane -p -e -N -t e2e:0.0" {
		t.Fatalf("unexpected command: %s", f.LastArgs)
	}
}

func TestAdapter_SendInput_UsesLiteralMode(t *testing.T) {
	f := &FakeExec{}
	a := NewAdapter(f)
	err := a.SendInput("e2e:0.0", "\x1b[<64;80;12M")
	if err != nil {
		t.Fatalf("send input failed: %v", err)
	}
	if f.LastArgs != "tmux send-keys -l -t e2e:0.0 \x1b[<64;80;12M" {
		t.Fatalf("unexpected command: %s", f.LastArgs)
	}
}

func TestAdapter_Resize_ResizesWindowThenPane(t *testing.T) {
	f := &FakeExec{}
	a := NewAdapter(f)
	err := a.Resize("e2e:0.1", 120, 40)
	if err != nil {
		t.Fatalf("resize failed: %v", err)
	}
	if len(f.RunCalls) != 2 {
		t.Fatalf("expected 2 resize commands, got %d: %#v", len(f.RunCalls), f.RunCalls)
	}
	if f.RunCalls[0] != "tmux resize-window -t e2e:0 -x 120 -y 40" {
		t.Fatalf("unexpected resize-window command: %s", f.RunCalls[0])
	}
	if f.RunCalls[1] != "tmux resize-pane -t e2e:0.1 -x 120 -y 40" {
		t.Fatalf("unexpected resize-pane command: %s", f.RunCalls[1])
	}
}

func TestAdapter_CaptureHistory_Last2000Lines(t *testing.T) {
	f := &FakeExec{OutputText: "ok"}
	a := NewAdapter(f)
	_, err := a.CaptureHistory("e2e:0.0", 2000)
	if err != nil {
		t.Fatalf("capture history failed: %v", err)
	}
	if f.LastArgs != "tmux capture-pane -p -e -N -S -2000 -E - -t e2e:0.0" {
		t.Fatalf("unexpected command: %s", f.LastArgs)
	}
}

func TestAdapter_StartPipePane(t *testing.T) {
	f := &FakeExec{}
	a := NewAdapter(f)
	err := a.StartPipePane("e2e:0.0", "cat > /tmp/tt.pipe")
	if err != nil {
		t.Fatalf("start pipe-pane failed: %v", err)
	}
	if f.LastArgs != "tmux pipe-pane -O -t e2e:0.0 cat > /tmp/tt.pipe" {
		t.Fatalf("unexpected command: %s", f.LastArgs)
	}
}

func TestAdapter_GetPaneOption(t *testing.T) {
	f := &FakeExec{OutputText: "42\n"}
	a := NewAdapter(f)
	got, err := a.GetPaneOption("e2e:0.0", "@shellman_cmd_seq")
	if err != nil {
		t.Fatalf("get pane option failed: %v", err)
	}
	if got != "42" {
		t.Fatalf("unexpected pane option value: %q", got)
	}
	if f.LastArgs != "tmux show-options -p -v -t e2e:0.0 @shellman_cmd_seq" {
		t.Fatalf("unexpected command: %s", f.LastArgs)
	}
}

func TestAdapter_SetPaneOption(t *testing.T) {
	f := &FakeExec{}
	a := NewAdapter(f)
	if err := a.SetPaneOption("e2e:0.0", "@shellman_cmd_state", "running"); err != nil {
		t.Fatalf("set pane option failed: %v", err)
	}
	if f.LastArgs != "tmux set-option -p -t e2e:0.0 @shellman_cmd_state running" {
		t.Fatalf("unexpected command: %s", f.LastArgs)
	}
}

func TestAdapter_CursorPosition_UsesDisplayMessage(t *testing.T) {
	f := &FakeExec{OutputText: "9 3\n"}
	a := NewAdapter(f)
	x, y, err := a.CursorPosition("e2e:0.0")
	if err != nil {
		t.Fatalf("cursor failed: %v", err)
	}
	if x != 9 || y != 3 {
		t.Fatalf("unexpected cursor: %d,%d", x, y)
	}
	if f.LastArgs != "tmux display-message -p -t e2e:0.0 #{cursor_x} #{cursor_y}" {
		t.Fatalf("unexpected command: %s", f.LastArgs)
	}
}

func TestAdapter_PaneLastActiveAt_UsesDisplayMessage(t *testing.T) {
	f := &FakeExec{OutputText: "1771524000\n"}
	a := NewAdapter(f)
	got, err := a.PaneLastActiveAt("e2e:0.0")
	if err != nil {
		t.Fatalf("pane last active failed: %v", err)
	}
	want := time.Unix(1771524000, 0).UTC()
	if !got.Equal(want) {
		t.Fatalf("unexpected pane activity time: got=%s want=%s", got.Format(time.RFC3339), want.Format(time.RFC3339))
	}
	if f.LastArgs != "tmux display-message -p -t e2e:0.0 #{pane_activity}" {
		t.Fatalf("unexpected command: %s", f.LastArgs)
	}
}

func TestAdapter_PaneTitleAndCurrentCommand_UsesDisplayMessage(t *testing.T) {
	f := &FakeExec{OutputText: "main\tzsh\tinvalid\n"}
	a := NewAdapter(f)
	title, cmd, err := a.PaneTitleAndCurrentCommand("e2e:0.0")
	if err != nil {
		t.Fatalf("pane metadata failed: %v", err)
	}
	if title != "main" || cmd != "zsh" {
		t.Fatalf("unexpected pane metadata: title=%q cmd=%q", title, cmd)
	}
	if f.LastArgs != "tmux display-message -p -t e2e:0.0 #{pane_title}\t#{pane_current_command}\t#{pane_pid}" {
		t.Fatalf("unexpected command: %s", f.LastArgs)
	}
}

func TestAdapter_CreateSiblingPane(t *testing.T) {
	f := &FakeExec{OutputText: "e2e:0.2\n"}
	a := NewAdapter(f)
	pane, err := a.CreateSiblingPane("e2e:0.0")
	if err != nil {
		t.Fatalf("create sibling pane failed: %v", err)
	}
	if pane != "e2e:0.2" {
		t.Fatalf("unexpected pane id: %s", pane)
	}
	if f.LastArgs != "tmux split-window -h -t e2e:0.0 -P -F #{session_name}:#{window_index}.#{pane_index}" {
		t.Fatalf("unexpected command: %s", f.LastArgs)
	}
}

func TestAdapter_CreateChildPane(t *testing.T) {
	f := &FakeExec{OutputText: "e2e:0.3\n"}
	a := NewAdapter(f)
	pane, err := a.CreateChildPane("e2e:0.1")
	if err != nil {
		t.Fatalf("create child pane failed: %v", err)
	}
	if pane != "e2e:0.3" {
		t.Fatalf("unexpected pane id: %s", pane)
	}
	if f.LastArgs != "tmux split-window -v -t e2e:0.1 -P -F #{session_name}:#{window_index}.#{pane_index}" {
		t.Fatalf("unexpected command: %s", f.LastArgs)
	}
}

func TestAdapter_CreateRootPane(t *testing.T) {
	f := &FakeExec{OutputText: "e2e:5.0\n"}
	a := NewAdapter(f)
	pane, err := a.CreateRootPane()
	if err != nil {
		t.Fatalf("create root pane failed: %v", err)
	}
	if pane != "e2e:5.0" {
		t.Fatalf("unexpected pane id: %s", pane)
	}
	if f.LastArgs != "tmux new-window -P -F #{session_name}:#{window_index}.#{pane_index}" {
		t.Fatalf("unexpected command: %s", f.LastArgs)
	}
}

func TestAdapter_ServerInstanceID(t *testing.T) {
	f := &FakeExec{OutputText: "12345\n"}
	a := NewAdapterWithSocket(f, "tt_e2e")
	id, err := a.ServerInstanceID()
	if err != nil {
		t.Fatalf("server instance id failed: %v", err)
	}
	if id != "tt_e2e:12345" {
		t.Fatalf("unexpected server instance id: %q", id)
	}
	if f.LastArgs != "tmux -L tt_e2e display-message -p #{pid}" {
		t.Fatalf("unexpected command: %s", f.LastArgs)
	}
}

func TestDeriveDisplayProcessName_FromEntrypointScript(t *testing.T) {
	got := deriveDisplayProcessName("node", "node /usr/local/lib/node_modules/@acme/toolkit/bin/toolkit.js --stdio")
	if got != "toolkit" {
		t.Fatalf("unexpected display name: %q", got)
	}
}

func TestDeriveDisplayProcessName_PlainSubcommandsFallback(t *testing.T) {
	got := deriveDisplayProcessName("npm", "npm run dev")
	if got != "npm" {
		t.Fatalf("unexpected display name: %q", got)
	}
}

func TestDeriveDisplayProcessName_SkipsOptionValues(t *testing.T) {
	got := deriveDisplayProcessName("node", "node --loader tsx ./src/main.ts")
	if got != "main" {
		t.Fatalf("unexpected display name: %q", got)
	}
}
