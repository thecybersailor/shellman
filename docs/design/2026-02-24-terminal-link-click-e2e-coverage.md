# Terminal Link Click E2E Coverage

## Scope

新增 `webui/e2e/full-real-tmux.spec.ts` 场景：

1. 向 xterm 注入 URL 与文件路径文本
2. 点击 URL 后打开新窗口（`window.open`）
3. 点击 `path:line:col` 后打开文件 Sheet
4. 校验文件编辑区光标定位到目标行列（`selectionStart > 0`）

## Run

```bash
make e2e-ui-docker
```

该场景在 docker 全链路中执行，覆盖 `Playwright -> UI -> API -> tmux pane`。
