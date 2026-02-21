# E2E-TTY (Real tmux/tty, Non-UI Request Source)

`e2e-tty` 用于验证 tmux/tty 的真实生命周期链路，除“请求发起端不是浏览器 UI”之外，其余链路全部保持真实。

## 边界

- 非真实项：输入请求由测试代码发起（HTTP/WS），不是人工点击 UI。
- 真实项：`StartApplication` 启动路径、`runWSRuntime`、`PaneActor`、真实 tmux、真实 zsh、真实 codex、真实退出交互节奏。

## 运行

在 `shellman/` 目录执行：

```bash
make e2e-tty
```

当前入口执行：

```bash
cd cli && go test -tags e2e_tty ./cmd/shellman -run TestStartApplication_RealCodexLifecycle_NonUIRequestOnly -count=1 -v
```

## 前置条件

- 本机已安装并可用：`tmux`、`zsh`、`codex`
- 当前用户可创建临时 tmux socket/session
- codex 已完成可交互初始化（可正常接收 prompt 并响应）

若前置条件不满足，用例会显式 `Skip` 或失败并输出日志证据。
