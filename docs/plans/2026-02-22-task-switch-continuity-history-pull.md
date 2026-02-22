# Task Switch Terminal Continuity & History Pull Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** 在 task 切换时稳定保持终端连续性，支持最近 5 个活跃 task 持续收流、断流恢复首帧增强，以及向上滚动按需拉取更多历史。

**Architecture:** 后端从“单选订阅”升级为“当前选中 + 最近活跃 LRU 订阅”，持续推送最近 5 个 task 的流；前端对每个 pane 做独立缓存并检测流连续性（seq/epoch），在检测到断流时切回触发增强 reset。新增历史拉取 API 由前端滚动到顶触发，返回更大窗口快照并 reset 重绘。

**Tech Stack:** Go (`cli/cmd/shellman`, `cli/internal/localapi`, `cli/internal/projectstate`), Vue 3 + Vitest (`webui/src/stores`, `webui/src/components`), Playwright e2e (`webui/e2e/full-real-tmux.spec.ts`), Docker (`make e2e-ui-docker`)

---

### Task 1: 独立策略 - 每个 Task 仅保留最近 N 行（后端快照裁剪）

**Files:**
- Modify: `cli/cmd/shellman/runtime_task_state_actor.go:244-352`
- Modify: `cli/cmd/shellman/runtime_task_state_actor_test.go:74-127`

**Step 1: Write the failing test**

```go
func TestTaskStateActor_Tick_TrimsSnapshotToRecentLines(t *testing.T) {
    // 构造 6 行 snapshot，配置最大行数=3（测试内覆盖）
    // 断言 BatchUpsertRuntime 收到的 Snapshot 只有最后 3 行
}
```

**Step 2: Run test to verify it fails**

Run:
```bash
docker run --rm -v "$PWD:/workspace" -w /workspace/cli golang:1.24-bookworm \
  bash -lc "go test ./cmd/shellman -run TestTaskStateActor_Tick_TrimsSnapshotToRecentLines -count=1"
```
Expected: FAIL（当前不会裁剪 snapshot 行数）

**Step 3: Write minimal implementation**

```go
var runtimeSnapshotMaxLines = 2000

func trimToRecentLines(text string, maxLines int) string {
    if maxLines <= 0 { return text }
    lines := strings.Split(text, "\n")
    if len(lines) <= maxLines { return text }
    return strings.Join(lines[len(lines)-maxLines:], "\n")
}

// flushDirtyRuntime 中写入 paneRecord 前调用
paneRecord.Snapshot = trimToRecentLines(report.Snapshot, runtimeSnapshotMaxLines)
paneRecord.SnapshotHash = sha1Text(paneRecord.Snapshot)
```

**Step 4: Run test to verify it passes**

Run:
```bash
docker run --rm -v "$PWD:/workspace" -w /workspace/cli golang:1.24-bookworm \
  bash -lc "go test ./cmd/shellman -run TestTaskStateActor_Tick_TrimsSnapshotToRecentLines -count=1"
```
Expected: PASS

**Step 5: Commit**

```bash
git add cli/cmd/shellman/runtime_task_state_actor.go cli/cmd/shellman/runtime_task_state_actor_test.go
git commit -m "feat(runtime): cap persisted pane snapshot to recent N lines"
```

### Task 2: 后端连接模型升级为最近 5 个活跃 pane 持续订阅

**Files:**
- Create: `cli/cmd/shellman/runtime_conn_actor_test.go`
- Modify: `cli/cmd/shellman/runtime_conn_actor.go:11-96`
- Modify: `cli/cmd/shellman/runtime_registry_actor.go:154-181`
- Modify: `cli/cmd/shellman/runtime_registry_actor_test.go:36-81`

**Step 1: Write the failing test**

```go
func TestConnActor_SelectAndWatch_KeepRecentLimit(t *testing.T) {
    // 顺序选择 p1..p6，限制 5
    // 断言 watched={p2..p6}，evicted=p1
}

func TestRegistryActor_Subscribe_EvictsOldPaneWhenWatchLimitExceeded(t *testing.T) {
    // 断言旧 pane 被 Unsubscribe，新 pane 被 Subscribe
}
```

**Step 2: Run test to verify it fails**

Run:
```bash
docker run --rm -v "$PWD:/workspace" -w /workspace/cli golang:1.24-bookworm \
  bash -lc "go test ./cmd/shellman -run 'TestConnActor_SelectAndWatch_KeepRecentLimit|TestRegistryActor_Subscribe_EvictsOldPaneWhenWatchLimitExceeded' -count=1"
```
Expected: FAIL（当前 `ConnActor` 只有 `selected`，没有 watched LRU）

**Step 3: Write minimal implementation**

```go
const activePaneWatchLimit = 5

type ConnActor struct {
    selected string
    watchOrder []string
    watchSet map[string]struct{}
}

func (c *ConnActor) SelectAndWatch(target string, limit int) (evicted string) {
    // 维护 LRU：去重后追加到末尾，超限淘汰头部
}

func (c *ConnActor) WatchedTargets() []string { /* copy */ }
```

```go
// RegistryActor.Subscribe
// 1) conn.SelectAndWatch(target, activePaneWatchLimit)
// 2) 订阅新 target
// 3) 若有 evicted，则对 evicted pane 执行 Unsubscribe(connID)
// 4) selected 仍是当前 target（输入/resize 使用 selected）
```

**Step 4: Run test to verify it passes**

Run:
```bash
docker run --rm -v "$PWD:/workspace" -w /workspace/cli golang:1.24-bookworm \
  bash -lc "go test ./cmd/shellman -run 'TestConnActor_SelectAndWatch_KeepRecentLimit|TestRegistryActor_Subscribe_EvictsOldPaneWhenWatchLimitExceeded' -count=1"
```
Expected: PASS

**Step 5: Commit**

```bash
git add cli/cmd/shellman/runtime_conn_actor.go cli/cmd/shellman/runtime_conn_actor_test.go cli/cmd/shellman/runtime_registry_actor.go cli/cmd/shellman/runtime_registry_actor_test.go
git commit -m "feat(runtime): keep last 5 active pane subscriptions per connection"
```

### Task 3: 流连续性元数据 + 切回断流时增强首帧

**Files:**
- Modify: `cli/cmd/shellman/runtime_pane_actor.go:188-353,568-613`
- Modify: `cli/cmd/shellman/runtime_pane_actor_test.go:130-380`
- Modify: `cli/cmd/shellman/main.go:300-357`
- Modify: `cli/cmd/shellman/main_test.go:237-340`

**Step 1: Write the failing test**

```go
func TestPaneActor_Subscribe_ResetContainsEpochSeq(t *testing.T) {
    // 断言 term.output payload 带 stream_epoch / stream_seq
}

func TestPaneActor_Subscribe_GapRecoverUsesHistoryLines(t *testing.T) {
    // 传入 gap_recover=true, history_lines=4000
    // 断言 CaptureHistory(target, 4000) 被调用
}
```

**Step 2: Run test to verify it fails**

Run:
```bash
docker run --rm -v "$PWD:/workspace" -w /workspace/cli golang:1.24-bookworm \
  bash -lc "go test ./cmd/shellman -run 'TestPaneActor_Subscribe_ResetContainsEpochSeq|TestPaneActor_Subscribe_GapRecoverUsesHistoryLines' -count=1"
```
Expected: FAIL（当前 payload 无 seq/epoch，Subscribe 无 recovery 参数）

**Step 3: Write minimal implementation**

```go
type paneSubscribeOptions struct {
    GapRecover bool
    HistoryLines int
}

// PaneActor 内部状态
streamEpoch uint64
streamSeq uint64

func (p *PaneActor) Subscribe(connID string, out chan protocol.Message, opt paneSubscribeOptions) {
    // GapRecover=true 时优先 CaptureHistory(target, opt.HistoryLines)
}

payload["stream_epoch"] = p.streamEpoch
payload["stream_seq"] = nextSeq
```

```go
// bindMessageLoop 解析 tmux.select_pane payload 新字段
type selectPayload struct {
    Target string `json:"target"`
    Cols int `json:"cols"`
    Rows int `json:"rows"`
    GapRecover bool `json:"gap_recover"`
    HistoryLines int `json:"history_lines"`
}
```

**Step 4: Run test to verify it passes**

Run:
```bash
docker run --rm -v "$PWD:/workspace" -w /workspace/cli golang:1.24-bookworm \
  bash -lc "go test ./cmd/shellman -run 'TestPaneActor_Subscribe_ResetContainsEpochSeq|TestPaneActor_Subscribe_GapRecoverUsesHistoryLines' -count=1"
```
Expected: PASS

**Step 5: Commit**

```bash
git add cli/cmd/shellman/runtime_pane_actor.go cli/cmd/shellman/runtime_pane_actor_test.go cli/cmd/shellman/main.go cli/cmd/shellman/main_test.go
git commit -m "feat(runtime): add stream continuity metadata and gap-recover reset options"
```

### Task 4: 前端缓存模型支持“非选中但活跃 pane 持续收流”

**Files:**
- Modify: `webui/src/stores/shellman.ts:83-118,429-472,1069-1150`
- Modify: `webui/src/stores/shellman.spec.ts:1544-1606`

**Step 1: Write the failing test**

```ts
it("caches watched pane output even when not currently selected", async () => {
  // t1 选中后切到 t2
  // 收到 target=t1 的 term.output
  // 断言 state.terminalByPaneUuid[uuid-t1] 被更新
  // 且 state.terminalOutput 仍保持 t2 当前显示
})
```

**Step 2: Run test to verify it fails**

Run:
```bash
docker run --rm -v "$PWD:/workspace" -w /workspace/webui node:20-bookworm \
  bash -lc "npm ci && npm run test -- src/stores/shellman.spec.ts -t 'caches watched pane output even when not currently selected'"
```
Expected: FAIL（当前 target mismatch 会直接 ignore）

**Step 3: Write minimal implementation**

```ts
const ACTIVE_PANE_CACHE_LIMIT = 5;
const TERMINAL_CACHE_MAX_LINES = 2000;

function trimToRecentLines(text: string, maxLines: number): string {
  const lines = text.split("\n");
  return lines.length <= maxLines ? text : lines.slice(lines.length - maxLines).join("\n");
}

// term.output 到达时：
// 1) 总是更新对应 pane 缓存（按 pane_uuid/target 定位）
// 2) 仅在 incomingPaneUuid === selectedPaneUuid 时更新 state.terminalOutput
```

**Step 4: Run test to verify it passes**

Run:
```bash
docker run --rm -v "$PWD:/workspace" -w /workspace/webui node:20-bookworm \
  bash -lc "npm ci && npm run test -- src/stores/shellman.spec.ts -t 'caches watched pane output even when not currently selected'"
```
Expected: PASS

**Step 5: Commit**

```bash
git add webui/src/stores/shellman.ts webui/src/stores/shellman.spec.ts
git commit -m "feat(webui): keep cache updated for active non-selected pane streams"
```

### Task 5: 前端断流检测与切回恢复参数

**Files:**
- Modify: `webui/src/stores/shellman.ts:1069-1150,1500-1593`
- Modify: `webui/src/stores/shellman.spec.ts:1608-1700`

**Step 1: Write the failing test**

```ts
it("marks pane gap on seq discontinuity and sends gap_recover on next select", async () => {
  // 收到 seq: 1 -> 4
  // 断言 paneGapByPaneUuid[uuid] = true
  // selectTask 时 tmux.select_pane payload 包含 { gap_recover: true, history_lines: 4000 }
})
```

**Step 2: Run test to verify it fails**

Run:
```bash
docker run --rm -v "$PWD:/workspace" -w /workspace/webui node:20-bookworm \
  bash -lc "npm ci && npm run test -- src/stores/shellman.spec.ts -t 'marks pane gap on seq discontinuity and sends gap_recover on next select'"
```
Expected: FAIL

**Step 3: Write minimal implementation**

```ts
const GAP_RECOVER_HISTORY_LINES = 4000;
const paneLastSeqByPaneUuid: Record<string, number> = {};
const paneLastEpochByPaneUuid: Record<string, number> = {};
const paneGapByPaneUuid: Record<string, true> = {};

// term.output 处理：epoch变化重置seq；seq跳变则标记 gap
// selectTask 发送 tmux.select_pane 时：若 paneGap=true 附带 gap_recover/history_lines
```

**Step 4: Run test to verify it passes**

Run:
```bash
docker run --rm -v "$PWD:/workspace" -w /workspace/webui node:20-bookworm \
  bash -lc "npm ci && npm run test -- src/stores/shellman.spec.ts -t 'marks pane gap on seq discontinuity and sends gap_recover on next select'"
```
Expected: PASS

**Step 5: Commit**

```bash
git add webui/src/stores/shellman.ts webui/src/stores/shellman.spec.ts
git commit -m "feat(webui): detect stream gaps and request gap-recover reset on task switch"
```

### Task 6: 向上滚动拉取更多历史 API（含前端调用，避免空接口）

**Files:**
- Modify: `cli/internal/localapi/routes_tasks.go:245-310`
- Modify: `cli/internal/localapi/routes_panes.go:383-454`
- Modify: `cli/internal/localapi/routes_panes_test.go:834-911`
- Modify: `webui/src/stores/shellman.ts:1745-2105`
- Modify: `webui/src/stores/shellman.spec.ts:1700-1785`

**Step 1: Write the failing tests (backend + frontend caller)**

```go
func TestTaskPaneHistoryEndpoint_ReturnsLargerSnapshotByLinesQuery(t *testing.T) {
    // GET /api/v1/tasks/:id/pane-history?lines=4000
    // 断言调用 CaptureHistory 并返回 snapshot/frame/cursor
}
```

```ts
it("loadMorePaneHistory requests pane-history and applies reset frame", async () => {
  // 断言发起 /api/v1/tasks/t1/pane-history?lines=...
  // 并更新 state.terminalFrame={mode:'reset'}
})
```

**Step 2: Run tests to verify they fail**

Run:
```bash
docker run --rm -v "$PWD:/workspace" -w /workspace/cli golang:1.24-bookworm \
  bash -lc "go test ./internal/localapi -run TestTaskPaneHistoryEndpoint_ReturnsLargerSnapshotByLinesQuery -count=1"
```

```bash
docker run --rm -v "$PWD:/workspace" -w /workspace/webui node:20-bookworm \
  bash -lc "npm ci && npm run test -- src/stores/shellman.spec.ts -t 'loadMorePaneHistory requests pane-history and applies reset frame'"
```
Expected: FAIL

**Step 3: Write minimal implementation**

```go
// routes_tasks.go 增加路由分支
case r.Method == http.MethodGet && action == "pane-history":
    s.handleGetTaskPaneHistory(w, r, taskID)

// routes_panes.go
// lines query: default 2000, clamp [200, 10000]
snapshot, _ := s.deps.PaneService.CaptureHistory(binding.PaneTarget, lines)
respondOK(w, map[string]any{ "task_id": taskID, "snapshot": ... })
```

```ts
async function loadMorePaneHistory(taskId: string, lines: number) {
  const res = await fetch(apiURL(`/api/v1/tasks/${taskId}/pane-history?lines=${lines}`));
  // 更新 terminalByPaneUuid + selected terminalFrame(reset)
}
```

**Step 4: Run tests to verify they pass**

Run:
```bash
docker run --rm -v "$PWD:/workspace" -w /workspace/cli golang:1.24-bookworm \
  bash -lc "go test ./internal/localapi -run TestTaskPaneHistoryEndpoint_ReturnsLargerSnapshotByLinesQuery -count=1"
```

```bash
docker run --rm -v "$PWD:/workspace" -w /workspace/webui node:20-bookworm \
  bash -lc "npm ci && npm run test -- src/stores/shellman.spec.ts -t 'loadMorePaneHistory requests pane-history and applies reset frame'"
```
Expected: PASS

**Step 5: Commit**

```bash
git add cli/internal/localapi/routes_tasks.go cli/internal/localapi/routes_panes.go cli/internal/localapi/routes_panes_test.go webui/src/stores/shellman.ts webui/src/stores/shellman.spec.ts
git commit -m "feat(history): add pane-history api and frontend history pull caller"
```

### Task 7: Terminal 顶部滚动触发历史加载（桌面+移动端）

**Files:**
- Modify: `webui/src/components/TerminalPane.vue:28-34,454-533`
- Modify: `webui/src/components/TerminalPane.spec.ts:264-320`
- Modify: `webui/src/components/MobileStackView.vue:35-51,166-181`
- Modify: `webui/src/App.vue:620-639,689-704`

**Step 1: Write the failing component test**

```ts
it("emits terminal-history-more when terminal scroll reaches top", async () => {
  // mock term.onScroll(0)
  // expect(wrapper.emitted("terminal-history-more")).toHaveLength(1)
})
```

**Step 2: Run test to verify it fails**

Run:
```bash
docker run --rm -v "$PWD:/workspace" -w /workspace/webui node:20-bookworm \
  bash -lc "npm ci && npm run test -- src/components/TerminalPane.spec.ts -t 'emits terminal-history-more when terminal scroll reaches top'"
```
Expected: FAIL

**Step 3: Write minimal implementation**

```ts
// TerminalPane emit 新增
(event: "terminal-history-more"): void;

terminal.onScroll?.((y: number) => {
  if (y <= 0) emit("terminal-history-more");
});
```

```vue
<!-- App.vue / MobileStackView.vue 透传事件 -->
@terminal-history-more="onTerminalHistoryMore"
```

**Step 4: Run test to verify it passes**

Run:
```bash
docker run --rm -v "$PWD:/workspace" -w /workspace/webui node:20-bookworm \
  bash -lc "npm ci && npm run test -- src/components/TerminalPane.spec.ts -t 'emits terminal-history-more when terminal scroll reaches top'"
```
Expected: PASS

**Step 5: Commit**

```bash
git add webui/src/components/TerminalPane.vue webui/src/components/TerminalPane.spec.ts webui/src/components/MobileStackView.vue webui/src/App.vue
git commit -m "feat(webui): trigger history pull when terminal scroll reaches top"
```

### Task 8: Docker e2e 扩展（人类节奏 + 连续性断档恢复 + 上拉历史）

**Files:**
- Modify: `webui/e2e/full-real-tmux.spec.ts:160-380`

**Step 1: Write the failing e2e test**

```ts
test("task switch gap recover sends larger first reset and avoids viewport drift", async ({ page, request }) => {
  // 1) Codex 风格重绘 + 人类延时切换
  // 2) 注入 seq 跳变（模拟断档）
  // 3) 切回后断言无重复 prompt 行，且可继续输入
});

test("scroll to top loads more pane history", async ({ page, request }) => {
  // 1) 先制造大量输出
  // 2) 滚动到顶部触发 history-more
  // 3) 断言旧行可见且无输入卡死
});
```

**Step 2: Run e2e to verify it fails**

Run:
```bash
make e2e-ui-docker
```
Expected: FAIL（新场景未实现）

**Step 3: Write minimal implementation adjustments for test hooks**

```ts
// 复用现有 runCodexStyleRepaintStorm / selectTask
// 新增历史上拉触发辅助方法与断言工具
```

**Step 4: Run e2e to verify it passes**

Run:
```bash
make e2e-ui-docker
```
Expected: PASS（含新增 case）

**Step 5: Commit**

```bash
git add webui/e2e/full-real-tmux.spec.ts
git commit -m "test(e2e): cover gap-recover task switch and scroll-up history loading"
```

### Task 9: 文档同步 + 全量回归（防止代码/文档漂移）

**Files:**
- Create: `docs/design/task-switch-terminal-continuity.md`

**Step 1: Write the failing doc-check expectation in plan checklist**

```md
- 必须记录：active watcher LRU=5、gap_recover 行为、history API 参数、N 行裁剪策略
```

**Step 2: Run verification commands before writing docs (collect facts)**

Run:
```bash
docker run --rm -v "$PWD:/workspace" -w /workspace/cli golang:1.24-bookworm bash -lc "go test ./cmd/shellman ./internal/localapi"
docker run --rm -v "$PWD:/workspace" -w /workspace/webui node:20-bookworm bash -lc "npm ci && npm run test -- src/stores/shellman.spec.ts src/components/TerminalPane.spec.ts"
make e2e-ui-docker
```
Expected: 全部 PASS

**Step 3: Write minimal documentation**

```md
# Task Switch Terminal Continuity
- Active Pane LRU: 5
- Gap Detection: stream_epoch + stream_seq
- Gap Recovery: tmux.select_pane{gap_recover=true, history_lines=4000}
- History Pull API: GET /api/v1/tasks/:task_id/pane-history?lines=...
- Snapshot Retention: per-task recent N lines
```

**Step 4: Run final regression again**

Run:
```bash
docker run --rm -v "$PWD:/workspace" -w /workspace/cli golang:1.24-bookworm bash -lc "go test ./cmd/shellman ./internal/localapi"
make e2e-ui-docker
```
Expected: PASS

**Step 5: Commit**

```bash
git add docs/design/task-switch-terminal-continuity.md
git commit -m "docs: align terminal continuity design with implementation"
```

