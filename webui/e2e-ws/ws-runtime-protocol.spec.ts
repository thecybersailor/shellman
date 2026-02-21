import { spawn, spawnSync, type ChildProcessWithoutNullStreams } from "node:child_process";
import net from "node:net";
import os from "node:os";
import path from "node:path";
import { mkdtemp, rm } from "node:fs/promises";
import { fileURLToPath } from "node:url";
import { afterAll, beforeAll, describe, expect, it } from "vitest";
import WS from "ws";
import xtermHeadless from "@xterm/headless";

type ProtocolError = { code?: string; message?: string };
type ProtocolMessage = {
  id?: string;
  type?: string;
  op?: string;
  payload?: Record<string, unknown>;
  error?: ProtocolError;
};

type RuntimeContext = {
  apiBase: string;
  cliProcess: ChildProcessWithoutNullStreams;
  cliLogs: () => string;
  tmuxSocket: string;
  paneTarget: string;
  probe: WSProbe;
  output: {
    text: string;
    cursor: { x: number; y: number } | null;
  };
  frames: Array<{
    mode: "reset" | "append";
    data: string;
    cursor: { x: number; y: number } | null;
  }>;
  wsErrors: ProtocolMessage[];
  dispose: () => Promise<void>;
};

const TERM_OUTPUT_TIMEOUT_MS = 10000;
const { Terminal } = xtermHeadless as unknown as {
  Terminal: new (options?: Record<string, unknown>) => {
    write(data: string): void;
    reset(): void;
    buffer: {
      active: {
        getLine(idx: number): { translateToString(trimRight?: boolean): string } | undefined;
      };
    };
  };
};
const here = path.dirname(fileURLToPath(import.meta.url));
const cliDir = path.resolve(here, "../../cli");

class WSProbe {
  private ws: WS;
  private inbox: ProtocolMessage[] = [];
  private handlers: Array<(msg: ProtocolMessage) => void> = [];

  private constructor(ws: WS) {
    this.ws = ws;
    ws.on("message", (raw) => {
      const text = typeof raw === "string" ? raw : raw.toString();
      try {
        const parsed = JSON.parse(text) as ProtocolMessage;
        this.inbox.push(parsed);
        for (const handler of this.handlers) {
          handler(parsed);
        }
      } catch {
        // ignore malformed frames
      }
    });
  }

  static async connect(url: string): Promise<WSProbe> {
    const ws = new WS(url);
    await new Promise<void>((resolve, reject) => {
      ws.once("open", () => resolve());
      ws.once("error", (err) => reject(err));
    });
    return new WSProbe(ws);
  }

  onMessage(fn: (msg: ProtocolMessage) => void) {
    this.handlers.push(fn);
  }

  close() {
    this.ws.close();
  }

  send(msg: ProtocolMessage) {
    this.ws.send(JSON.stringify(msg));
  }

  async waitFor(predicate: (msg: ProtocolMessage) => boolean, timeoutMs: number): Promise<ProtocolMessage> {
    const deadline = Date.now() + timeoutMs;
    while (Date.now() < deadline) {
      const idx = this.inbox.findIndex(predicate);
      if (idx >= 0) {
        const [hit] = this.inbox.splice(idx, 1);
        return hit;
      }
      await delay(20);
    }
    throw new Error(`waitFor timeout after ${timeoutMs}ms`);
  }
}

let runtime: RuntimeContext | null = null;

describe("ws runtime protocol e2e", () => {
  beforeAll(async () => {
    runtime = await startRuntime();
  }, 120000);

  afterAll(async () => {
    if (runtime) {
      await runtime.dispose();
      runtime = null;
    }
  }, 120000);

  it("keeps cursor position aligned with tmux cursor", async () => {
    const rt = mustRuntime(runtime);
    const marker = `CURSOR_${Date.now()}`;
    await sendTextAsKeyEvents(rt, `echo ${marker}\r`);
    await waitForOutputContains(rt, marker);
    await assertCursorEventuallyMatchesTmux(rt, 6000);
    expect(rt.wsErrors).toEqual([]);
  });

  it("preserves per-key input/output consistency across repeated commands", async () => {
    const rt = mustRuntime(runtime);
    const payloads = [
      "SEQ_A_0123456789ABCDEFGHIJKLMNOPQRSTUVWXYZ",
      "SEQ_B_zyxwvutsrqponmlkjihgfedcba9876543210",
      "SEQ_C_MIXED_ABCabc123XYZxyz"
    ];
    for (const payload of payloads) {
      await sendTextAsKeyEvents(rt, `echo ${payload}\r`);
      await waitForOutputContains(rt, payload);
    }
    expect(rt.wsErrors).toEqual([]);
  });

  it("keeps first character visible when typing two keys with 50ms gap", async () => {
    const rt = mustRuntime(runtime);
    const baseline = normalizeTmuxSnapshot(tmuxSnapshot(rt.tmuxSocket, rt.paneTarget));

    await sendReqAndExpectOK(rt.probe, {
      id: `req_input_l_${Date.now()}`,
      type: "event",
      op: "term.input",
      payload: { target: rt.paneTarget, text: "l" }
    });
    await delay(50);
    const afterL = normalizeTmuxSnapshot(tmuxSnapshot(rt.tmuxSocket, rt.paneTarget));
    expect(afterL).not.toEqual(baseline);
    expect(afterL.includes("l")).toBe(true);
    await waitForOutputContains(rt, "l");

    await sendReqAndExpectOK(rt.probe, {
      id: `req_input_s_${Date.now()}`,
      type: "event",
      op: "term.input",
      payload: { target: rt.paneTarget, text: "s" }
    });
    await delay(50);
    const afterLS = normalizeTmuxSnapshot(tmuxSnapshot(rt.tmuxSocket, rt.paneTarget));
    expect(afterLS).not.toEqual(afterL);
    expect(afterLS.includes("ls")).toBe(true);
    await waitForOutputContains(rt, "ls");

    await sendReqAndExpectOK(rt.probe, {
      id: `req_input_enter_${Date.now()}`,
      type: "event",
      op: "term.input",
      payload: { target: rt.paneTarget, text: "\r" }
    });
    expect(rt.wsErrors).toEqual([]);
  });

  it("keeps leading character on first interaction right after select_pane", async () => {
    const rt = await startRuntime({ autoSelect: false });
    try {
      await sendReqAndExpectOK(rt.probe, {
        id: `req_race_select_${Date.now()}`,
        type: "req",
        op: "tmux.select_pane",
        payload: { target: rt.paneTarget }
      });
      await sendReqAndExpectOK(rt.probe, {
        id: `req_race_resize_${Date.now()}`,
        type: "event",
        op: "term.resize",
        payload: { target: rt.paneTarget, cols: 120, rows: 40 }
      });

      await sendReqAndExpectOK(rt.probe, {
        id: `req_race_input_l_${Date.now()}`,
        type: "event",
        op: "term.input",
        payload: { target: rt.paneTarget, text: "l" }
      });
      await delay(50);
      await sendReqAndExpectOK(rt.probe, {
        id: `req_race_input_s_${Date.now()}`,
        type: "event",
        op: "term.input",
        payload: { target: rt.paneTarget, text: "s" }
      });
      await delay(50);

      const tmuxAfterLS = normalizeTmuxSnapshot(tmuxSnapshot(rt.tmuxSocket, rt.paneTarget));
      expect(tmuxAfterLS.includes("ls")).toBe(true);
      await waitForOutputContains(rt, "ls");
      await assertCursorEventuallyMatchesTmux(rt, 6000);
      expect(rt.wsErrors).toEqual([]);
    } finally {
      await rt.dispose();
    }
  });

  it("replays ws frames through terminal renderer without losing leading key", async () => {
    const rt = await startRuntime({ autoSelect: false });
    try {
      await sendReqAndExpectOK(rt.probe, {
        id: `req_render_select_${Date.now()}`,
        type: "req",
        op: "tmux.select_pane",
        payload: { target: rt.paneTarget }
      });
      await sendReqAndExpectOK(rt.probe, {
        id: `req_render_resize_${Date.now()}`,
        type: "event",
        op: "term.resize",
        payload: { target: rt.paneTarget, cols: 120, rows: 40 }
      });

      const frameStart = rt.frames.length;
      await sendReqAndExpectOK(rt.probe, {
        id: `req_render_input_l_${Date.now()}`,
        type: "event",
        op: "term.input",
        payload: { target: rt.paneTarget, text: "l" }
      });
      await delay(50);
      await sendReqAndExpectOK(rt.probe, {
        id: `req_render_input_s_${Date.now()}`,
        type: "event",
        op: "term.input",
        payload: { target: rt.paneTarget, text: "s" }
      });
      await delay(250);

      const relevantFrames = rt.frames.slice(frameStart);
      expect(relevantFrames.length).toBeGreaterThan(0);

      const rendered = await replayFramesWithTerminalPaneLogic(relevantFrames, { cols: 120, rows: 40 });
      expect(rendered.includes("ls"), `rendered=${JSON.stringify(rendered)}`).toBe(true);
      expect(rt.wsErrors).toEqual([]);
    } finally {
      await rt.dispose();
    }
  });
});

function mustRuntime(rt: RuntimeContext | null): RuntimeContext {
  if (!rt) {
    throw new Error("runtime is not ready");
  }
  return rt;
}

async function startRuntime(opts: { autoSelect?: boolean } = {}): Promise<RuntimeContext> {
  const autoSelect = opts.autoSelect ?? true;
  const port = await pickFreePort();
  const apiBase = `http://127.0.0.1:${port}`;
  const tmuxSocket = `muxt_ws_${Date.now()}_${Math.floor(Math.random() * 100000)}`;
  const repoRoot = await mkdtemp(path.join(os.tmpdir(), "muxt-ws-e2e-"));
  const projectID = `e2e_ws_${Date.now()}`;
  const gitInit = spawnSync("git", ["init", "-q", repoRoot], { encoding: "utf8" });
  if (gitInit.status !== 0) {
    throw new Error(`git init failed: ${gitInit.stderr || gitInit.stdout}`);
  }

  const boot = spawnSync(
    "tmux",
    ["-L", tmuxSocket, "-f", "/dev/null", "new-session", "-d", "-s", "e2e", "bash --noprofile --norc"],
    { encoding: "utf8" }
  );
  if (boot.status !== 0) {
    throw new Error(`tmux bootstrap failed: ${boot.stderr || boot.stdout}`);
  }

  const logs: string[] = [];
  const cliProcess = spawn("go", ["run", "./cmd/termteam"], {
    cwd: cliDir,
    env: {
      ...process.env,
      TERMTEAM_MODE: "local",
      TERMTEAM_LOCAL_PORT: String(port),
      TERMTEAM_TMUX_SOCKET: tmuxSocket,
      TERMTEAM_WEBUI_MODE: "dist",
      TERMTEAM_STREAM_MODE: "1"
    },
    stdio: ["ignore", "pipe", "pipe"]
  });
  cliProcess.stdout.on("data", (d) => logs.push(d.toString()));
  cliProcess.stderr.on("data", (d) => logs.push(d.toString()));

  try {
    await waitForHealth(`${apiBase}/healthz`, 30000);

    await postJSON(`${apiBase}/api/v1/projects/active`, {
      project_id: projectID,
      repo_root: repoRoot
    });
    const rootPane = await postJSON<{ pane_target: string }>(`${apiBase}/api/v1/projects/${projectID}/panes/root`, {
      title: "root pane"
    });
    const paneTarget = String(rootPane.pane_target);

    const probe = await WSProbe.connect(`ws://127.0.0.1:${port}/ws/client/local`);
    const output = {
      text: "",
      cursor: null as { x: number; y: number } | null
    };
    const frames: Array<{
      mode: "reset" | "append";
      data: string;
      cursor: { x: number; y: number } | null;
    }> = [];
    const wsErrors: ProtocolMessage[] = [];

    probe.onMessage((msg) => {
      if (msg.type === "res" && msg.error) {
        wsErrors.push(msg);
      }
      if (msg.type !== "event" || msg.op !== "term.output") {
        return;
      }
      const payload = msg.payload ?? {};
      if (String(payload.target ?? "") !== paneTarget) {
        return;
      }
      const mode = payload.mode === "append" ? "append" : "reset";
      const data = String(payload.data ?? "");
      output.text = mode === "append" ? output.text + data : data;
      output.cursor =
        typeof payload.cursor?.x === "number" && typeof payload.cursor?.y === "number"
          ? { x: payload.cursor.x, y: payload.cursor.y }
          : null;
      frames.push({
        mode,
        data,
        cursor: output.cursor
      });
    });

    if (autoSelect) {
      await sendReqAndExpectOK(probe, {
        id: "req_select_pane",
        type: "req",
        op: "tmux.select_pane",
        payload: { target: paneTarget }
      });
      await sendReqAndExpectOK(probe, {
        id: "req_resize",
        type: "event",
        op: "term.resize",
        payload: { target: paneTarget, cols: 120, rows: 40 }
      });
    }

    return {
      apiBase,
      cliProcess,
      cliLogs: () => logs.join(""),
      tmuxSocket,
      paneTarget,
      probe,
      output,
      frames,
      wsErrors,
      dispose: async () => {
        probe.close();
        await stopProcess(cliProcess);
        spawnSync("tmux", ["-L", tmuxSocket, "kill-server"], { stdio: "ignore" });
        await rm(repoRoot, { recursive: true, force: true });
      }
    };
  } catch (err) {
    await stopProcess(cliProcess);
    spawnSync("tmux", ["-L", tmuxSocket, "kill-server"], { stdio: "ignore" });
    await rm(repoRoot, { recursive: true, force: true });
    throw err;
  }
}

async function sendTextAsKeyEvents(rt: RuntimeContext, text: string) {
  for (let i = 0; i < text.length; i++) {
    await sendReqAndExpectOK(rt.probe, {
      id: `req_input_${Date.now()}_${i}`,
      type: "event",
      op: "term.input",
      payload: { target: rt.paneTarget, text: text[i] }
    });
  }
}

async function waitForOutputContains(rt: RuntimeContext, marker: string) {
  const deadline = Date.now() + TERM_OUTPUT_TIMEOUT_MS;
  while (Date.now() < deadline) {
    if (rt.output.text.includes(marker)) {
      return;
    }
    await delay(50);
  }
  throw new Error(`output does not contain marker: ${marker}\nlogs:\n${tailText(rt.cliLogs(), 80)}`);
}

async function assertCursorEventuallyMatchesTmux(rt: RuntimeContext, timeoutMs: number) {
  const deadline = Date.now() + timeoutMs;
  while (Date.now() < deadline) {
    const observed = rt.output.cursor;
    const actual = tmuxCursor(rt.tmuxSocket, rt.paneTarget);
    if (observed && observed.x === actual.x && observed.y === actual.y) {
      return;
    }
    await delay(80);
  }
  const actual = tmuxCursor(rt.tmuxSocket, rt.paneTarget);
  throw new Error(
    `cursor mismatch; observed=${JSON.stringify(rt.output.cursor)} actual=${JSON.stringify(actual)}\nlogs:\n${tailText(rt.cliLogs(), 80)}`
  );
}

async function sendReqAndExpectOK(probe: WSProbe, msg: ProtocolMessage) {
  if (!msg.id) {
    throw new Error("request id required");
  }
  probe.send(msg);
  const res = await probe.waitFor((m) => m.type === "res" && m.id === msg.id, 5000);
  if (res.error) {
    throw new Error(`ws op failed: op=${res.op} code=${res.error.code ?? ""} msg=${res.error.message ?? ""}`);
  }
}

async function postJSON<T = Record<string, unknown>>(url: string, body: Record<string, unknown>): Promise<T> {
  const resp = await fetch(url, {
    method: "POST",
    headers: { "content-type": "application/json" },
    body: JSON.stringify(body)
  });
  const text = await resp.text();
  let payload: { ok?: boolean; data?: T; error?: { code?: string; message?: string } } = {};
  try {
    payload = JSON.parse(text) as typeof payload;
  } catch {
    // leave payload empty
  }
  if (!resp.ok || !payload.ok) {
    throw new Error(`http failed: ${url} status=${resp.status} body=${text}`);
  }
  return payload.data as T;
}

async function waitForHealth(url: string, timeoutMs: number) {
  const deadline = Date.now() + timeoutMs;
  while (Date.now() < deadline) {
    try {
      const resp = await fetch(url);
      if (resp.ok) {
        return;
      }
    } catch {
      // retry
    }
    await delay(100);
  }
  throw new Error(`healthz timeout: ${url}`);
}

function tmuxCursor(socket: string, target: string): { x: number; y: number } {
  const out = spawnSync("tmux", ["-L", socket, "display-message", "-p", "-t", target, "#{cursor_x} #{cursor_y}"], {
    encoding: "utf8"
  });
  if (out.status !== 0) {
    throw new Error(`tmux cursor query failed: ${out.stderr || out.stdout}`);
  }
  const fields = out.stdout.trim().split(/\s+/);
  if (fields.length < 2) {
    throw new Error(`unexpected tmux cursor output: ${out.stdout}`);
  }
  return { x: Number(fields[0]), y: Number(fields[1]) };
}

function tmuxSnapshot(socket: string, target: string): string {
  const out = spawnSync("tmux", ["-L", socket, "capture-pane", "-p", "-e", "-t", target], {
    encoding: "utf8"
  });
  if (out.status !== 0) {
    throw new Error(`tmux snapshot query failed: ${out.stderr || out.stdout}`);
  }
  return out.stdout;
}

function normalizeTmuxSnapshot(text: string): string {
  return text.replace(/[\r\n]+$/g, "");
}

async function replayFramesWithTerminalPaneLogic(
  frames: Array<{
    mode: "reset" | "append";
    data: string;
    cursor: { x: number; y: number } | null;
  }>,
  size: { cols: number; rows: number }
): Promise<string> {
  const term = new Terminal({
    cols: size.cols,
    rows: size.rows,
    convertEol: true,
    allowProposedApi: true
  });
  let prevOutput = "";
  let nextOutput = "";

  const writeTerm = async (data: string) => {
    await new Promise<void>((resolve) => {
      (term as unknown as { write: (d: string, cb?: () => void) => void }).write(data, () => resolve());
    });
  };

  const moveCursor = async (cursor: { x: number; y: number } | null) => {
    if (!cursor) {
      return;
    }
    const row = Math.max(1, cursor.y + 1);
    const col = Math.max(1, cursor.x + 1);
    await writeTerm(`\u001b[${row};${col}H`);
  };

  for (const frame of frames) {
    nextOutput = frame.mode === "append" ? nextOutput + frame.data : frame.data;

    if (!nextOutput) {
      term.reset();
      await writeTerm("");
      await moveCursor(frame.cursor);
    } else if (nextOutput.startsWith(prevOutput)) {
      const appended = nextOutput.slice(prevOutput.length);
      if (appended) {
        await writeTerm(appended);
        await moveCursor(frame.cursor);
      }
    } else {
      term.reset();
      await writeTerm(nextOutput);
      await moveCursor(frame.cursor);
    }

    await moveCursor(frame.cursor);
    prevOutput = nextOutput;
  }
  await writeTerm("");

  const lines: string[] = [];
  for (let i = 0; i < size.rows; i++) {
    const line = term.buffer.active.getLine(i);
    if (!line) {
      continue;
    }
    lines.push(line.translateToString(true));
  }
  return lines.join("\n");
}

async function pickFreePort(): Promise<number> {
  return await new Promise<number>((resolve, reject) => {
    const server = net.createServer();
    server.listen(0, "127.0.0.1", () => {
      const addr = server.address();
      if (!addr || typeof addr === "string") {
        server.close();
        reject(new Error("failed to get free port"));
        return;
      }
      const port = addr.port;
      server.close((err) => {
        if (err) {
          reject(err);
          return;
        }
        resolve(port);
      });
    });
    server.on("error", (err) => reject(err));
  });
}

async function stopProcess(proc: ChildProcessWithoutNullStreams) {
  if (proc.killed || proc.exitCode !== null) {
    return;
  }
  proc.kill("SIGTERM");
  const exited = await Promise.race([
    onceExit(proc),
    delay(3000).then(() => false)
  ]);
  if (exited) {
    return;
  }
  proc.kill("SIGKILL");
  await onceExit(proc);
}

async function onceExit(proc: ChildProcessWithoutNullStreams): Promise<boolean> {
  return await new Promise((resolve) => {
    proc.once("exit", () => resolve(true));
  });
}

async function delay(ms: number) {
  await new Promise((resolve) => setTimeout(resolve, ms));
}

function tailText(text: string, lines: number): string {
  const all = text.split("\n");
  return all.slice(Math.max(0, all.length - lines)).join("\n");
}
