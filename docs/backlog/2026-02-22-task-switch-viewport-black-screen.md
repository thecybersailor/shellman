# Task 切换后 Terminal 视口错位/黑屏 - Backlog

## 问题现象
- 在启动 `codex`（TUI）后切换 task，终端视口不稳定，出现以下现象之一：
- 光标与 prompt 不在同一行（常见为光标多 1 行）。
- 切换后终端区域偶发全黑/空白（header 仍存在，terminal body 无内容）。
- 同一轮操作中，不同 task 的恢复表现不一致：有的能恢复到 `PANE_Txx_LINE_049xx`，有的只剩 prompt 或黑屏。

## 当前分析（基于日志与 e2e 证据）
- 已确认 `gap_recover` 存在分片发送行为：首片 `mode=reset`，后续 `mode=append`，最后一片携带 `cursor`。
- 在大快照恢复时，末尾换行与 cursor 恢复顺序会触发 `cursorPromptDelta = 1`（光标比 prompt 低一行）的场景。
- 对该链路已做一次后端修正（仅对 `reset` 最后分片的尾换行裁剪），Go 单测通过，且部分 e2e 轮次改善。
- 但问题未完全收敛：仍有 e2e 轮次出现 `visualCursorDelta = null` 且截图为黑屏，说明还有未覆盖的状态路径（可能是渲染时序、可视 cursor 判定、或恢复帧被后续帧覆盖）。

## 相关文件
- 后端恢复分片逻辑：
  - `cli/cmd/shellman/runtime_pane_actor.go`
  - `cli/cmd/shellman/runtime_pane_actor_test.go`
- e2e 场景与断言：
  - `webui/e2e/full-real-tmux.spec.ts`
- 前端终端渲染与状态（历史排查涉及）：
  - `webui/src/components/TerminalPane.vue`
  - `webui/src/components/TerminalPane.spec.ts`
  - `webui/src/stores/shellman.ts`
  - `webui/src/stores/shellman.spec.ts`

## 复现方法（必须 docker）
1. 进入仓库：`cd /Users/wanglei/Projects/cybersailor/shellman-project/shellman`
2. 执行目标 e2e（10 panes + 每窗 5000 行 + LRU 扇出/回补）：
```bash
docker compose -f docker-compose.e2e.yml run --rm \
  -e CI=1 \
  -e SHELLMAN_E2E_USE_MOCK=0 \
  e2e-runner \
  sh -lc "cd /workspace/shellman/webui && npx playwright test e2e/full-real-tmux.spec.ts -g '10 panes with 5000 lines should evict LRU and recover evicted panes with gap_recover'"
```
3. 观察视频与截图（见下方产物路径），重点关注切换瞬间 terminal body 是否变空、光标是否与 prompt 对齐。

## 测试过程记录（摘要）
- 阶段 1：复现到 `cursorPromptDelta = 1`，并在调试输出中看到 `gap_recover` 分片末尾换行 + cursor 恢复导致的错位。
- 阶段 2：增加后端最小修复（`reset` 最后分片裁剪尾换行），对应 Go 单测通过。
- 阶段 3：重复 docker e2e，出现两类结果：
  - 通过：光标与 prompt 对齐。
  - 失败：`visualCursorDelta = null`，截图为黑屏（终端 body 空白）。
- 结论：当前修复只覆盖了“光标 +1”其中一条链路，尚未完全解决“黑屏/空白”链路。

## 已有测试与产物
- 最近失败产物目录：
  - `/Users/wanglei/Projects/cybersailor/shellman-project/shellman/logs/playwright-artifacts/full-real-tmux-shellman-lo-59d84-cted-panes-with-gap-recover/`
- 包含：
  - `video.webm`
  - `test-failed-1.png`
  - `trace.zip`
  - `error-context.md`

## 待办（下一轮排查建议）
- 在 e2e 中增加“仅失败时”的状态打印（避免常态日志影响时序），输出：
  - 当前 active pane id
  - `promptLineIndex / cursorY / cursorPromptDelta / visualCursorDelta`
  - 最近 5 条 `term_output` 事件（mode/chunk/gap_recover/cursor）
- 对比“黑屏成功轮次 vs 失败轮次”的同一时刻事件序列，确认是：
  - 渲染层丢帧/覆盖（前端问题），还是
  - 服务端恢复帧被后续空帧/错序帧覆盖（后端/事件流问题）。
- 在确认根因前，不继续扩大修复面。
