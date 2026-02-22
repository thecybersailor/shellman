import { describe, expect, it, vi } from "vitest";
import { classifyTermFrame, createShellmanStore } from "./shellman";

class FakeSocket {
  sent: string[] = [];
  private listeners: Record<string, Array<(event: any) => void>> = {};

  addEventListener(type: string, cb: (event: any) => void) {
    this.listeners[type] ??= [];
    this.listeners[type].push(cb);
  }

  send(text: string) {
    this.sent.push(text);
  }

  close() {
    // noop
  }

  emitOpen() {
    for (const cb of this.listeners.open ?? []) cb({});
  }

  emitMessage(data: string) {
    for (const cb of this.listeners.message ?? []) cb({ data });
  }
}

describe("shellman store", () => {
  it("classifies terminal frames for profiling", () => {
    expect(classifyTermFrame("reset", "snapshot")).toBe("reset");
    expect(classifyTermFrame("append", "")).toBe("append_empty");
    expect(classifyTermFrame("append", "\u001b[0m\u001b[H\u001b[2Jfull-screen")).toBe("append_ansi_repaint");
    expect(classifyTermFrame("append", "delta")).toBe("append_delta");
  });

  it("reports completion via /api/v1/runs/:run_id/report-result with request_id", async () => {
    const calls: Array<{ url: string; method: string; body?: string }> = [];
    const fakeFetch = async (input: RequestInfo | URL, init?: RequestInit) => {
      const url = String(input);
      calls.push({
        url,
        method: init?.method ?? "GET",
        body: typeof init?.body === "string" ? init.body : undefined
      });
      if (url.includes("/api/v1/runs/r1/report-result")) {
        return {
          json: async () => ({ ok: true, data: { run_id: "r1", status: "completed", idempotent: false } })
        } as Response;
      }
      return { json: async () => ({ ok: true, data: [] }) } as Response;
    };

    const store = createShellmanStore(fakeFetch as typeof fetch, () => null as unknown as WebSocket);
    const out = await store.reportRunResult("r1", "done");
    expect(out.runId).toBe("r1");
    expect(out.status).toBe("completed");

    const hit = calls.find((c) => c.url.endsWith("/api/v1/runs/r1/report-result"));
    expect(hit).toBeTruthy();
    expect(hit?.method).toBe("POST");
    const payload = JSON.parse(String(hit?.body ?? "{}")) as { summary?: string; request_id?: string };
    expect(payload.summary).toBe("done");
    expect(typeof payload.request_id).toBe("string");
    expect((payload.request_id ?? "").length).toBeGreaterThan(0);
  });

  it("loads active projects/tree and pane binding", async () => {
    const calls: string[] = [];
    const fakeFetch = async (url: string) => {
      calls.push(url);
      if (url.endsWith("/api/v1/projects/active")) {
        return { json: async () => ({ ok: true, data: [{ project_id: "p1", repo_root: "/tmp/p1" }] }) } as Response;
      }
      if (url.endsWith("/api/v1/projects/p1/tree")) {
        return {
          json: async () => ({
            ok: true,
            data: {
              project_id: "p1",
              nodes: [{
                task_id: "t1",
                title: "root",
                status: "running",
                flag: "notify",
                flag_desc: "check later",
                flag_readed: true,
                updated_at: 1771408800
              }]
            }
          })
        } as Response;
      }
      if (url.endsWith("/api/v1/tasks/t1/pane")) {
        return { json: async () => ({ ok: true, data: { task_id: "t1", pane_uuid: "uuid-t1", pane_id: "e2e:0.0", pane_target: "e2e:0.0" } }) } as Response;
      }
      return { json: async () => ({ ok: false, error: { code: "NOT_FOUND" } }) } as Response;
    };

    const store = createShellmanStore(fakeFetch as typeof fetch, () => null as unknown as WebSocket);
    await store.load();

    expect(calls.some((c) => c.endsWith("/api/v1/tasks/t1/pane"))).toBe(true);
    expect(store.state.projects.length).toBe(1);
    expect(store.state.treesByProject.p1.length).toBe(1);
    expect(store.state.paneByTaskId.t1?.paneTarget).toBe("e2e:0.0");
    expect(store.state.treesByProject.p1[0]?.flag).toBe("notify");
    expect(store.state.treesByProject.p1[0]?.flagDesc).toBe("check later");
    expect(store.state.treesByProject.p1[0]?.flagReaded).toBe(true);
    expect(store.state.treesByProject.p1[0]?.updatedAt).toBe(1771408800);
  });

  it("patches task flag-readed and updates local tree node", async () => {
    const calls: Array<{ url: string; method: string; body?: string }> = [];
    const fakeFetch = async (input: RequestInfo | URL, init?: RequestInit) => {
      const url = String(input);
      const method = init?.method ?? "GET";
      const body = typeof init?.body === "string" ? init.body : undefined;
      calls.push({ url, method, body });
      if (url.endsWith("/api/v1/projects/active")) {
        return { json: async () => ({ ok: true, data: [{ project_id: "p1", repo_root: "/tmp/p1" }] }) } as Response;
      }
      if (url.endsWith("/api/v1/projects/p1/tree")) {
        return {
          json: async () => ({
            ok: true,
            data: { project_id: "p1", nodes: [{ task_id: "t1", title: "root", status: "running", flag_readed: false }] }
          })
        } as Response;
      }
      if (url.endsWith("/api/v1/tasks/t1/pane")) {
        return { json: async () => ({ ok: true, data: { task_id: "t1", pane_uuid: "uuid-t1", pane_id: "e2e:0.0", pane_target: "e2e:0.0" } }) } as Response;
      }
      if (url.endsWith("/api/v1/tasks/t1/flag-readed") && method === "PATCH") {
        return { json: async () => ({ ok: true, data: { task_id: "t1", flag_readed: true } }) } as Response;
      }
      return { json: async () => ({ ok: false, error: { code: "NOT_FOUND" } }) } as Response;
    };

    const store = createShellmanStore(fakeFetch as typeof fetch, () => null as unknown as WebSocket);
    await store.load();
    await store.markTaskFlagReaded("t1", true);

    const call = calls.find((it) => it.url.endsWith("/api/v1/tasks/t1/flag-readed") && it.method === "PATCH");
    expect(call).toBeTruthy();
    expect(call?.body).toContain("\"flag_readed\":true");
    expect(store.state.treesByProject.p1[0]?.flagReaded).toBe(true);
  });

  it("loads and updates task sidecar mode", async () => {
    const calls: Array<{ url: string; method: string; body?: string }> = [];
    const fakeFetch = async (input: RequestInfo | URL, init?: RequestInit) => {
      const url = String(input);
      const method = init?.method ?? "GET";
      const body = typeof init?.body === "string" ? init.body : undefined;
      calls.push({ url, method, body });
      if (url.endsWith("/api/v1/tasks/t1/sidecar-mode") && method === "GET") {
        return { json: async () => ({ ok: true, data: { task_id: "t1", sidecar_mode: "observer" } }) } as Response;
      }
      if (url.endsWith("/api/v1/tasks/t1/sidecar-mode") && method === "PATCH") {
        return { json: async () => ({ ok: true, data: { task_id: "t1", sidecar_mode: "autopilot" } }) } as Response;
      }
      return { json: async () => ({ ok: true, data: [] }) } as Response;
    };

    const store = createShellmanStore(fakeFetch as typeof fetch, () => null as unknown as WebSocket);
    const loaded = await store.loadTaskSidecarMode("t1");
    expect(loaded).toBe("observer");
    expect(store.state.taskSidecarModeByTaskId.t1).toBe("observer");

    const updated = await store.setTaskSidecarMode("t1", "autopilot");
    expect(updated).toBe("autopilot");
    expect(store.state.taskSidecarModeByTaskId.t1).toBe("autopilot");
    const patchCall = calls.find((it) => it.url.endsWith("/api/v1/tasks/t1/sidecar-mode") && it.method === "PATCH");
    expect(patchCall?.body).toContain("\"sidecar_mode\":\"autopilot\"");
  });

  it("supports loading only preferred task pane when prefetchAllTaskPanes is disabled", async () => {
    const calls: string[] = [];
    const fakeFetch = async (url: string) => {
      calls.push(url);
      if (url.endsWith("/api/v1/projects/active")) {
        return { json: async () => ({ ok: true, data: [{ project_id: "p1", repo_root: "/tmp/p1" }] }) } as Response;
      }
      if (url.endsWith("/api/v1/projects/p1/tree")) {
        return {
          json: async () => ({
            ok: true,
            data: {
              project_id: "p1",
              nodes: [
                { task_id: "t1", title: "task-1", status: "running" },
                { task_id: "t2", title: "task-2", status: "running" }
              ]
            }
          })
        } as Response;
      }
      if (url.endsWith("/api/v1/tasks/t2/pane")) {
        return { json: async () => ({ ok: true, data: { task_id: "t2", pane_uuid: "uuid-t2", pane_id: "e2e:2.0", pane_target: "e2e:2.0" } }) } as Response;
      }
      return { json: async () => ({ ok: false, error: { code: "NOT_FOUND" } }) } as Response;
    };

    const store = createShellmanStore(fakeFetch as typeof fetch, () => null as unknown as WebSocket);
    await store.load({ preferredTaskId: "t2", prefetchAllTaskPanes: false });

    const paneCalls = calls.filter((c) => c.includes("/api/v1/tasks/") && c.endsWith("/pane"));
    expect(paneCalls.length).toBe(1);
    expect(paneCalls[0]?.endsWith("/api/v1/tasks/t2/pane")).toBe(true);
    expect(store.state.selectedTaskId).toBe("t2");
    expect(store.state.paneByTaskId.t1).toBeUndefined();
    expect(store.state.paneByTaskId.t2?.paneTarget).toBe("e2e:2.0");
  });

  it("loads task messages and sends user message", async () => {
    const calls: Array<{ url: string; method: string }> = [];
    const fakeFetch = async (input: RequestInfo | URL, init?: RequestInit) => {
      const url = String(input);
      calls.push({ url, method: init?.method ?? "GET" });
      if (url.endsWith("/api/v1/tasks/t1/messages") && (init?.method ?? "GET") === "GET") {
        return {
          json: async () => ({
            ok: true,
            data: {
              task_id: "t1",
              messages: [{ id: 1, task_id: "t1", role: "user", content: "hello", status: "completed", created_at: 1, updated_at: 1 }]
            }
          })
        } as Response;
      }
      if (url.endsWith("/api/v1/tasks/t1/messages") && init?.method === "POST") {
        return { json: async () => ({ ok: true, data: { task_id: "t1" } }) } as Response;
      }
      return { json: async () => ({ ok: true, data: [] }) } as Response;
    };

    const store = createShellmanStore(fakeFetch as typeof fetch, () => null as unknown as WebSocket);
    await store.loadTaskMessages("t1");
    await store.sendTaskMessage("t1", "hello");
    expect(store.state.taskMessagesByTaskId.t1[0]?.role).toBe("user");
    expect(calls.some((c) => c.url.endsWith("/api/v1/tasks/t1/messages") && c.method === "POST")).toBe(true);
  });

  it("stops running task message via messages/stop endpoint", async () => {
    const calls: Array<{ url: string; method: string }> = [];
    const fakeFetch = async (input: RequestInfo | URL, init?: RequestInit) => {
      const url = String(input);
      const method = init?.method ?? "GET";
      calls.push({ url, method });
      if (url.endsWith("/api/v1/tasks/t1/messages/stop") && method === "POST") {
        return { json: async () => ({ ok: true, data: { task_id: "t1", canceled: true } }) } as Response;
      }
      return { json: async () => ({ ok: true, data: [] }) } as Response;
    };
    const store = createShellmanStore(fakeFetch as typeof fetch, () => null as unknown as WebSocket);
    const out = await store.stopTaskMessage("t1");
    expect(out).toBe(true);
    expect(calls.some((c) => c.url.endsWith("/api/v1/tasks/t1/messages/stop") && c.method === "POST")).toBe(true);
  });

  it("does not refetch task messages for repeated selectTask on same task", async () => {
    const calls: string[] = [];
    const fakeFetch = async (url: string) => {
      calls.push(url);
      if (url.endsWith("/api/v1/projects/active")) {
        return { json: async () => ({ ok: true, data: [{ project_id: "p1", repo_root: "/tmp/p1" }] }) } as Response;
      }
      if (url.endsWith("/api/v1/projects/p1/tree")) {
        return {
          json: async () => ({ ok: true, data: { project_id: "p1", nodes: [{ task_id: "t1", title: "root", status: "running" }] } })
        } as Response;
      }
      if (url.endsWith("/api/v1/tasks/t1/pane")) {
        return { json: async () => ({ ok: true, data: { task_id: "t1", pane_uuid: "uuid-t1", pane_id: "e2e:0.0", pane_target: "e2e:0.0" } }) } as Response;
      }
      if (url.endsWith("/api/v1/tasks/t1/messages")) {
        return { json: async () => ({ ok: true, data: { task_id: "t1", messages: [] } }) } as Response;
      }
      return { json: async () => ({ ok: false, error: { code: "NOT_FOUND" } }) } as Response;
    };

    const store = createShellmanStore(fakeFetch as typeof fetch, () => null as unknown as WebSocket);
    await store.load();
    await store.selectTask("t1");
    await store.selectTask("t1");
    await store.selectTask("t1", { forceRefreshNotes: true });

    const messageCalls = calls.filter((c) => c.endsWith("/api/v1/tasks/t1/messages"));
    expect(messageCalls.length).toBe(2);
  });

  it("startup flow avoids repeated pane and notes fetch when selected-only prefetch is enabled", async () => {
    const sock = new FakeSocket();
    const calls: string[] = [];
    const fakeFetch = async (url: string) => {
      calls.push(url);
      if (url.endsWith("/api/v1/projects/active")) {
        return { json: async () => ({ ok: true, data: [{ project_id: "p1", repo_root: "/tmp/p1" }] }) } as Response;
      }
      if (url.endsWith("/api/v1/projects/p1/tree")) {
        return {
          json: async () => ({
            ok: true,
            data: {
              project_id: "p1",
              nodes: [
                { task_id: "t1", title: "root", status: "running" },
                { task_id: "t2", title: "child", status: "running" },
                { task_id: "t3", title: "child-2", status: "running" }
              ]
            }
          })
        } as Response;
      }
      if (url.endsWith("/api/v1/tasks/t1/pane")) {
        return { json: async () => ({ ok: true, data: { task_id: "t1", pane_uuid: "uuid-t1", pane_id: "e2e:0.0", pane_target: "e2e:0.0" } }) } as Response;
      }
      if (url.endsWith("/api/v1/tasks/t1/messages")) {
        return { json: async () => ({ ok: true, data: { task_id: "t1", messages: [] } }) } as Response;
      }
      return { json: async () => ({ ok: false, error: { code: "NOT_FOUND" } }) } as Response;
    };

    const store = createShellmanStore(fakeFetch as typeof fetch, () => sock as unknown as WebSocket);
    await store.load({ preferredTaskId: "t1", prefetchAllTaskPanes: false });
    await store.selectTask("t1");
    store.connectWS("ws://127.0.0.1:4621/ws/client/local");
    sock.emitOpen();
    await Promise.resolve();

    const paneCalls = calls.filter((c) => c.includes("/api/v1/tasks/") && c.endsWith("/pane"));
    const notesCalls = calls.filter((c) => c.endsWith("/api/v1/tasks/t1/messages"));
    expect(paneCalls.length).toBe(1);
    expect(notesCalls.length).toBe(1);
  });

  it("connects /ws/client/local and requests tmux.list", () => {
    const sock = new FakeSocket();
    let usedURL = "";
    const store = createShellmanStore(async () => ({ json: async () => ({ ok: true, data: [] }) }) as Response, (url: string) => {
      usedURL = url;
      return sock as unknown as WebSocket;
    });

    store.connectWS("ws://127.0.0.1:4621/ws/client/local");
    sock.emitOpen();

    expect(usedURL).toBe("ws://127.0.0.1:4621/ws/client/local");
    expect(sock.sent.length).toBeGreaterThan(0);
    const req = JSON.parse(sock.sent[0]) as { type: string; op: string };
    expect(req.type).toBe("req");
    expect(req.op).toBe("tmux.list");
  });

  it("selects task by pane_target and consumes term.output", async () => {
    const sock = new FakeSocket();
    const fakeFetch = async (url: string) => {
      if (url.endsWith("/api/v1/projects/active")) {
        return { json: async () => ({ ok: true, data: [{ project_id: "p1", repo_root: "/tmp/p1" }] }) } as Response;
      }
      if (url.endsWith("/api/v1/projects/p1/tree")) {
        return {
          json: async () => ({ ok: true, data: { project_id: "p1", nodes: [{ task_id: "t1", title: "root", status: "running" }] } })
        } as Response;
      }
      if (url.endsWith("/api/v1/tasks/t1/pane")) {
        return { json: async () => ({ ok: true, data: { task_id: "t1", pane_uuid: "uuid-t1", pane_id: "e2e:0.0", pane_target: "e2e:0.0" } }) } as Response;
      }
      return { json: async () => ({ ok: false, error: { code: "NOT_FOUND" } }) } as Response;
    };

    const store = createShellmanStore(fakeFetch as typeof fetch, () => sock as unknown as WebSocket);
    await store.load();
    store.connectWS("ws://127.0.0.1:4621/ws/client/local");
    sock.emitOpen();

    await store.selectTask("t1");

    const sentOps = sock.sent.map((s) => JSON.parse(s).op);
    expect(sentOps).toContain("tmux.select_pane");

    store.sendTerminalInput("echo hi\n");
    store.sendTerminalResize({ cols: 120, rows: 40 });
    const lastTwo = sock.sent.slice(-2).map((s) => JSON.parse(s).op);
    expect(lastTwo).toEqual(["term.input", "term.resize"]);

    sock.emitMessage(
      JSON.stringify({
        id: "evt_1",
        type: "event",
        op: "term.output",
        payload: { target: "e2e:0.0", mode: "reset", data: "hello\n", cursor: { x: 1, y: 0 } }
      })
    );
    expect(store.state.terminalOutput).toContain("hello");
    expect(store.state.terminalFrame).toEqual({ mode: "reset", data: "hello\n" });
    expect(store.state.terminalCursor).toEqual({ x: 1, y: 0 });

    sock.emitMessage(
      JSON.stringify({
        id: "evt_2",
        type: "event",
        op: "term.output",
        payload: { target: "e2e:0.0", mode: "append", data: "world", cursor: { x: 3, y: 1 } }
      })
    );
    expect(store.state.terminalOutput).toContain("world");
    expect(store.state.terminalFrame).toEqual({ mode: "append", data: "world" });
    expect(store.state.terminalCursor).toEqual({ x: 3, y: 1 });
  });

  it("does not send duplicate tmux.select_pane for same task and target", async () => {
    const sock = new FakeSocket();
    const fakeFetch = async (url: string) => {
      if (url.endsWith("/api/v1/projects/active")) {
        return { json: async () => ({ ok: true, data: [{ project_id: "p1", repo_root: "/tmp/p1" }] }) } as Response;
      }
      if (url.endsWith("/api/v1/projects/p1/tree")) {
        return {
          json: async () => ({ ok: true, data: { project_id: "p1", nodes: [{ task_id: "t1", title: "root", status: "running" }] } })
        } as Response;
      }
      if (url.endsWith("/api/v1/tasks/t1/pane")) {
        return { json: async () => ({ ok: true, data: { task_id: "t1", pane_uuid: "uuid-t1", pane_id: "e2e:0.0", pane_target: "e2e:0.0" } }) } as Response;
      }
      return { json: async () => ({ ok: false, error: { code: "NOT_FOUND" } }) } as Response;
    };

    const store = createShellmanStore(fakeFetch as typeof fetch, () => sock as unknown as WebSocket);
    await store.load();
    store.connectWS("ws://127.0.0.1:4621/ws/client/local");
    sock.emitOpen();
    await store.selectTask("t1");
    const beforeSelectCount = sock.sent
      .map((raw) => JSON.parse(raw) as { op?: string })
      .filter((msg) => msg.op === "tmux.select_pane").length;

    await store.selectTask("t1");
    await store.selectTask("t1");

    const afterSelectCount = sock.sent
      .map((raw) => JSON.parse(raw) as { op?: string })
      .filter((msg) => msg.op === "tmux.select_pane").length;
    expect(afterSelectCount).toBe(beforeSelectCount);
  });

  it("caches watched pane output even when not currently selected", async () => {
    const sock = new FakeSocket();
    const fakeFetch = async (url: string) => {
      if (url.endsWith("/api/v1/projects/active")) {
        return { json: async () => ({ ok: true, data: [{ project_id: "p1", repo_root: "/tmp/p1" }] }) } as Response;
      }
      if (url.endsWith("/api/v1/projects/p1/tree")) {
        return {
          json: async () => ({
            ok: true,
            data: {
              project_id: "p1",
              nodes: [
                { task_id: "t1", title: "root", status: "running" },
                { task_id: "t2", title: "sibling", status: "running" }
              ]
            }
          })
        } as Response;
      }
      if (url.endsWith("/api/v1/tasks/t1/pane")) {
        return { json: async () => ({ ok: true, data: { task_id: "t1", pane_uuid: "uuid-t1", pane_id: "e2e:0.1", pane_target: "e2e:0.1" } }) } as Response;
      }
      if (url.endsWith("/api/v1/tasks/t2/pane")) {
        return { json: async () => ({ ok: true, data: { task_id: "t2", pane_uuid: "uuid-t2", pane_id: "e2e:0.2", pane_target: "e2e:0.2" } }) } as Response;
      }
      return { json: async () => ({ ok: false, error: { code: "NOT_FOUND" } }) } as Response;
    };

    const store = createShellmanStore(fakeFetch as typeof fetch, () => sock as unknown as WebSocket);
    await store.load();
    store.connectWS("ws://127.0.0.1:4621/ws/client/local");
    sock.emitOpen();

    await store.selectTask("t1");
    await store.selectTask("t2");

    sock.emitMessage(
      JSON.stringify({
        id: "evt_t2_base",
        type: "event",
        op: "term.output",
        payload: { target: "e2e:0.2", mode: "reset", data: "T2_BASE\n", cursor: { x: 0, y: 0 } }
      })
    );
    expect(store.state.terminalOutput).toContain("T2_BASE");

    sock.emitMessage(
      JSON.stringify({
        id: "evt_t1_append",
        type: "event",
        op: "term.output",
        payload: { target: "e2e:0.1", mode: "append", data: "T1_BG\n", cursor: { x: 0, y: 1 } }
      })
    );

    expect(store.state.terminalOutput).toContain("T2_BASE");
    expect(store.state.terminalOutput).not.toContain("T1_BG");
    expect(store.state.terminalByPaneUuid["uuid-t1"]?.output).toContain("T1_BG");
    expect(store.state.terminalByPaneUuid["uuid-t2"]?.output).toContain("T2_BASE");
  });

  it("marks pane as gap and sends gap_recover on next select", async () => {
    const sock = new FakeSocket();
    const fakeFetch = async (url: string) => {
      if (url.endsWith("/api/v1/projects/active")) {
        return { json: async () => ({ ok: true, data: [{ project_id: "p1", repo_root: "/tmp/p1" }] }) } as Response;
      }
      if (url.endsWith("/api/v1/projects/p1/tree")) {
        return {
          json: async () => ({
            ok: true,
            data: {
              project_id: "p1",
              nodes: Array.from({ length: 6 }, (_, idx) => ({
                task_id: `t${idx + 1}`,
                title: `task-${idx + 1}`,
                status: "running"
              }))
            }
          })
        } as Response;
      }
      const paneMatch = url.match(/\/api\/v1\/tasks\/(t\d+)\/pane$/);
      if (paneMatch) {
        const taskID = paneMatch[1];
        const idx = Number(taskID.replace("t", ""));
        return {
          json: async () => ({
            ok: true,
            data: {
              task_id: taskID,
              pane_uuid: `uuid-${taskID}`,
              pane_id: `e2e:0.${idx}`,
              pane_target: `e2e:0.${idx}`
            }
          })
        } as Response;
      }
      return { json: async () => ({ ok: false, error: { code: "NOT_FOUND" } }) } as Response;
    };

    const store = createShellmanStore(fakeFetch as typeof fetch, () => sock as unknown as WebSocket);
    await store.load();
    store.connectWS("ws://127.0.0.1:4621/ws/client/local");
    sock.emitOpen();

    await store.selectTask("t1");
    await store.selectTask("t2");
    await store.selectTask("t3");
    await store.selectTask("t4");
    await store.selectTask("t5");
    await store.selectTask("t6");
    await store.selectTask("t1");

    const selectMessages = sock.sent
      .map((raw) => JSON.parse(raw) as { op?: string; payload?: Record<string, unknown> })
      .filter((msg) => msg.op === "tmux.select_pane");
    const lastSelectPayload = selectMessages[selectMessages.length - 1]?.payload ?? {};
    expect(lastSelectPayload.gap_recover).toBe(true);
    expect(lastSelectPayload.history_lines).toBe(4000);
  });

  it("loadMorePaneHistory requests pane-history and applies reset frame", async () => {
    const calls: string[] = [];
    const fakeFetch = async (input: RequestInfo | URL) => {
      const url = String(input);
      calls.push(url);
      if (url.endsWith("/api/v1/projects/active")) {
        return { json: async () => ({ ok: true, data: [{ project_id: "p1", repo_root: "/tmp/p1" }] }) } as Response;
      }
      if (url.endsWith("/api/v1/projects/p1/tree")) {
        return {
          json: async () => ({ ok: true, data: { project_id: "p1", nodes: [{ task_id: "t1", title: "root", status: "running" }] } })
        } as Response;
      }
      if (url.endsWith("/api/v1/tasks/t1/pane")) {
        return { json: async () => ({ ok: true, data: { task_id: "t1", pane_uuid: "uuid-t1", pane_id: "e2e:0.0", pane_target: "e2e:0.0" } }) } as Response;
      }
      if (url.includes("/api/v1/tasks/t1/pane-history?lines=4000")) {
        return {
          json: async () => ({
            ok: true,
            data: {
              task_id: "t1",
              pane_uuid: "uuid-t1",
              pane_id: "e2e:0.0",
              pane_target: "e2e:0.0",
              snapshot: {
                output: "older-1\nolder-2\n",
                frame: { mode: "reset", data: "older-1\nolder-2\n" },
                cursor: null
              }
            }
          })
        } as Response;
      }
      return { json: async () => ({ ok: false, error: { code: "NOT_FOUND" } }) } as Response;
    };

    const store = createShellmanStore(fakeFetch as typeof fetch, () => null as unknown as WebSocket);
    await store.load();
    await store.selectTask("t1");
    await store.loadMorePaneHistory("t1", 4000);

    expect(calls.some((url) => url.includes("/api/v1/tasks/t1/pane-history?lines=4000"))).toBe(true);
    expect(store.state.terminalFrame).toEqual({ mode: "reset", data: "older-1\nolder-2\n" });
    expect(store.state.terminalOutput).toBe("older-1\nolder-2\n");
  });

  it("does not restore persisted snapshot for running task and does not patch pane snapshot", async () => {
    const sock = new FakeSocket();
    const calls: Array<{ url: string; method: string; body?: string }> = [];
    const fakeFetch = async (input: RequestInfo | URL, init?: RequestInit) => {
      const url = String(input);
      const method = init?.method ?? "GET";
      calls.push({
        url,
        method,
        body: typeof init?.body === "string" ? init.body : undefined
      });
      if (url.endsWith("/api/v1/projects/active")) {
        return { json: async () => ({ ok: true, data: [{ project_id: "p1", repo_root: "/tmp/p1" }] }) } as Response;
      }
      if (url.endsWith("/api/v1/projects/p1/tree")) {
        return {
          json: async () => ({ ok: true, data: { project_id: "p1", nodes: [{ task_id: "t1", title: "root", status: "running" }] } })
        } as Response;
      }
      if (url.endsWith("/api/v1/tasks/t1/pane")) {
        return {
          json: async () => ({
            ok: true,
            data: {
              task_id: "t1",
              pane_uuid: "uuid-t1",
              pane_id: "e2e:0.0",
              pane_target: "e2e:0.0",
              snapshot: {
                output: "restored\n",
                frame: { mode: "reset", data: "restored\n" },
                cursor: { x: 1, y: 0 }
              }
            }
          })
        } as Response;
      }
      return { json: async () => ({ ok: false, error: { code: "NOT_FOUND" } }) } as Response;
    };

    const store = createShellmanStore(fakeFetch as typeof fetch, () => sock as unknown as WebSocket);
    await store.load();
    await store.selectTask("t1");
    expect(store.state.terminalOutput).toBe("");

    store.connectWS("ws://127.0.0.1:4621/ws/client/local");
    sock.emitOpen();
    await store.selectTask("t1");
    sock.emitMessage(
      JSON.stringify({
        id: "evt_new",
        type: "event",
        op: "term.output",
        payload: { target: "e2e:0.0", mode: "append", data: "next\n", cursor: { x: 0, y: 1 } }
      })
    );

    await new Promise((resolve) => setTimeout(resolve, 200));
    const patchCall = calls.find((c) => c.url.endsWith("/api/v1/tasks/t1/pane-snapshot") && c.method === "PATCH");
    expect(patchCall).toBeUndefined();
  });

  it("does not mark completed task as ended without pane-ended evidence", async () => {
    const fakeFetch = async (input: RequestInfo | URL) => {
      const url = String(input);
      if (url.endsWith("/api/v1/projects/active")) {
        return { json: async () => ({ ok: true, data: [{ project_id: "p1", repo_root: "/tmp/p1" }] }) } as Response;
      }
      if (url.endsWith("/api/v1/projects/p1/tree")) {
        return {
          json: async () => ({ ok: true, data: { project_id: "p1", nodes: [{ task_id: "t1", title: "root", status: "completed" }] } })
        } as Response;
      }
      if (url.endsWith("/api/v1/tasks/t1/pane")) {
        return {
          json: async () => ({
            ok: true,
            data: {
              task_id: "t1",
              pane_uuid: "uuid-t1",
              pane_id: "e2e:0.0",
              pane_target: "e2e:0.0",
              snapshot: {
                output: "restored-completed\n",
                frame: { mode: "reset", data: "restored-completed\n" },
                cursor: { x: 1, y: 0 }
              }
            }
          })
        } as Response;
      }
      return { json: async () => ({ ok: false, error: { code: "NOT_FOUND" } }) } as Response;
    };

    const store = createShellmanStore(fakeFetch as typeof fetch, () => null as unknown as WebSocket);
    await store.load();
    await store.selectTask("t1");
    expect(store.state.terminalOutput).toBe("");
    expect(store.state.terminalCursor).toBeNull();
    expect(store.state.terminalEnded).toBe(false);
  });

  it("refreshes task tree and pane binding when pane.ended event arrives", async () => {
    const sock = new FakeSocket();
    const calls: string[] = [];
    let treeFetchCount = 0;
    let paneFetchCount = 0;
    const fakeFetch = async (input: RequestInfo | URL) => {
      const url = String(input);
      calls.push(url);
      if (url.endsWith("/api/v1/projects/active")) {
        return { json: async () => ({ ok: true, data: [{ project_id: "p1", repo_root: "/tmp/p1" }] }) } as Response;
      }
      if (url.endsWith("/api/v1/projects/p1/tree")) {
        treeFetchCount += 1;
        return {
          json: async () => ({
            ok: true,
            data: {
              project_id: "p1",
              nodes: [{ task_id: "t1", title: "root", status: treeFetchCount >= 2 ? "failed" : "running" }]
            }
          })
        } as Response;
      }
      if (url.endsWith("/api/v1/tasks/t1/pane")) {
        paneFetchCount += 1;
        return {
          json: async () => ({
            ok: true,
            data: { task_id: "t1", pane_uuid: "uuid-t1", pane_id: "e2e:0.0", pane_target: "e2e:0.0" }
          })
        } as Response;
      }
      return { json: async () => ({ ok: false, error: { code: "NOT_FOUND" } }) } as Response;
    };

    const store = createShellmanStore(fakeFetch as typeof fetch, () => sock as unknown as WebSocket);
    await store.load();
    store.connectWS("ws://127.0.0.1:4621/ws/client/local");
    sock.emitOpen();
    await store.selectTask("t1");

    sock.emitMessage(
      JSON.stringify({
        id: "evt_end",
        type: "event",
        op: "pane.ended",
        payload: { target: "e2e:0.0", reason: "can't find pane" }
      })
    );
    await new Promise((resolve) => setTimeout(resolve, 220));

    expect(treeFetchCount).toBeGreaterThanOrEqual(2);
    expect(paneFetchCount).toBeGreaterThanOrEqual(2);
    expect(store.state.treesByProject.p1[0]?.status).toBe("failed");
    expect(calls.some((url) => url.endsWith("/api/v1/projects/p1/tree"))).toBe(true);
  });

  it("keeps cursor for append prompt frame with trailing spaces after enter", () => {
    const sock = new FakeSocket();
    const store = createShellmanStore(async () => ({ json: async () => ({ ok: true, data: [] }) }) as Response, () => sock as unknown as WebSocket);
    store.connectWS("ws://127.0.0.1:4621/ws/client/local");
    sock.emitOpen();
    store.state.selectedPaneTarget = "e2e:0.0";

    sock.emitMessage(
      JSON.stringify({
        id: "evt_reset",
        type: "event",
        op: "term.output",
        payload: {
          target: "e2e:0.0",
          mode: "reset",
          data: "prompt",
          cursor: { x: 6, y: 0 }
        }
      })
    );

    const appendPrompt = `\nprompt${" ".repeat(33)}`;
    sock.emitMessage(
      JSON.stringify({
        id: "evt_append_prompt",
        type: "event",
        op: "term.output",
        payload: {
          target: "e2e:0.0",
          mode: "append",
          data: appendPrompt,
          cursor: { x: 6, y: 1 }
        }
      })
    );

    expect(store.state.terminalOutput.endsWith(appendPrompt)).toBe(true);
    expect(store.state.terminalCursor).toEqual({ x: 6, y: 1 });
  });

  it("keeps previous cursor when append frame has no cursor payload", () => {
    const sock = new FakeSocket();
    const store = createShellmanStore(async () => ({ json: async () => ({ ok: true, data: [] }) }) as Response, () => sock as unknown as WebSocket);
    store.connectWS("ws://127.0.0.1:4621/ws/client/local");
    sock.emitOpen();
    store.state.selectedPaneTarget = "e2e:0.0";

    sock.emitMessage(
      JSON.stringify({
        id: "evt_reset",
        type: "event",
        op: "term.output",
        payload: {
          target: "e2e:0.0",
          mode: "reset",
          data: "prompt",
          cursor: { x: 6, y: 0 }
        }
      })
    );
    expect(store.state.terminalCursor).toEqual({ x: 6, y: 0 });

    sock.emitMessage(
      JSON.stringify({
        id: "evt_append_no_cursor",
        type: "event",
        op: "term.output",
        payload: {
          target: "e2e:0.0",
          mode: "append",
          data: " ls"
        }
      })
    );

    expect(store.state.terminalOutput.endsWith(" ls")).toBe(true);
    expect(store.state.terminalCursor).toEqual({ x: 6, y: 0 });
  });

  it("caps per-pane terminal cache to recent N lines", () => {
    const sock = new FakeSocket();
    const store = createShellmanStore(async () => ({ json: async () => ({ ok: true, data: [] }) }) as Response, () => sock as unknown as WebSocket);
    store.connectWS("ws://127.0.0.1:4621/ws/client/local");
    sock.emitOpen();
    store.state.selectedPaneTarget = "e2e:0.0";
    store.state.selectedPaneUuid = "uuid-t1";

    sock.emitMessage(
      JSON.stringify({
        id: "evt_reset",
        type: "event",
        op: "term.output",
        payload: {
          target: "e2e:0.0",
          mode: "reset",
          data: "line-0000\n",
          cursor: { x: 0, y: 0 }
        }
      })
    );

    const longAppend = `${Array.from({ length: 2100 }, (_, idx) => `line-${String(idx + 1).padStart(4, "0")}`).join("\n")}\n`;
    sock.emitMessage(
      JSON.stringify({
        id: "evt_append_long",
        type: "event",
        op: "term.output",
        payload: {
          target: "e2e:0.0",
          mode: "append",
          data: longAppend
        }
      })
    );

    const cached = store.state.terminalByPaneUuid["uuid-t1"];
    expect(cached).toBeDefined();
    const lines = String(cached?.output ?? "")
      .split("\n")
      .filter((line) => line.length > 0);
    expect(lines.length).toBeLessThanOrEqual(2000);
    expect(lines[0]).toBe("line-0101");
    expect(lines[lines.length - 1]).toBe("line-2100");
    expect(store.state.terminalOutput.includes("line-0000")).toBe(false);
  });

  it("registers active project and loads its tree", async () => {
    const calls: Array<{ url: string; method: string }> = [];
    const fakeFetch = async (input: RequestInfo | URL, init?: RequestInit) => {
      const url = String(input);
      const method = init?.method ?? "GET";
      calls.push({ url, method });
      if (url.endsWith("/api/v1/projects/active") && method === "GET") {
        return { json: async () => ({ ok: true, data: [] }) } as Response;
      }
      if (url.endsWith("/api/v1/projects/active") && method === "POST") {
        return { json: async () => ({ ok: true, data: { project_id: "p2" } }) } as Response;
      }
      if (url.endsWith("/api/v1/projects/p2/tree")) {
        return { json: async () => ({ ok: true, data: { project_id: "p2", nodes: [] } }) } as Response;
      }
      return { json: async () => ({ ok: false, error: { code: "NOT_FOUND" } }) } as Response;
    };

    const store = createShellmanStore(fakeFetch as typeof fetch, () => null as unknown as WebSocket);
    await store.load();
    await store.addActiveProject("p2", "/tmp/p2");

    expect(store.state.projects.some((p) => p.projectId === "p2")).toBe(true);
    expect(calls.some((c) => c.url.endsWith("/api/v1/projects/active") && c.method === "POST")).toBe(true);
    expect(calls.some((c) => c.url.endsWith("/api/v1/projects/p2/tree"))).toBe(true);
  });

  it("removes active project via DELETE and updates local state", async () => {
    const calls: Array<{ url: string; method: string }> = [];
    const fakeFetch = async (input: RequestInfo | URL, init?: RequestInit) => {
      const url = String(input);
      const method = init?.method ?? "GET";
      calls.push({ url, method });
      if (url.endsWith("/api/v1/projects/active") && method === "GET") {
        return {
          json: async () => ({
            ok: true,
            data: [
              { project_id: "p1", repo_root: "/tmp/p1" },
              { project_id: "p2", repo_root: "/tmp/p2" }
            ]
          })
        } as Response;
      }
      if (url.endsWith("/api/v1/projects/p1/tree") && method === "GET") {
        return { json: async () => ({ ok: true, data: { project_id: "p1", nodes: [] } }) } as Response;
      }
      if (url.endsWith("/api/v1/projects/p2/tree") && method === "GET") {
        return { json: async () => ({ ok: true, data: { project_id: "p2", nodes: [] } }) } as Response;
      }
      if (url.endsWith("/api/v1/projects/active/p1") && method === "DELETE") {
        return { json: async () => ({ ok: true, data: { project_id: "p1" } }) } as Response;
      }
      return { json: async () => ({ ok: false, error: { code: "NOT_FOUND" } }) } as Response;
    };

    const store = createShellmanStore(fakeFetch as typeof fetch, () => null as unknown as WebSocket);
    await store.load();
    await store.removeActiveProject("p1");

    expect(calls.some((c) => c.url.endsWith("/api/v1/projects/active/p1") && c.method === "DELETE")).toBe(true);
    expect(store.state.projects.map((p) => p.projectId)).toEqual(["p2"]);
    expect(store.state.treesByProject.p1).toBeUndefined();
  });

  it("throws remove error when delete active project fails", async () => {
    const fakeFetch = async (input: RequestInfo | URL, init?: RequestInit) => {
      const url = String(input);
      const method = init?.method ?? "GET";
      if (url.endsWith("/api/v1/projects/active") && method === "GET") {
        return { json: async () => ({ ok: true, data: [{ project_id: "p1", repo_root: "/tmp/p1" }] }) } as Response;
      }
      if (url.endsWith("/api/v1/projects/p1/tree") && method === "GET") {
        return { json: async () => ({ ok: true, data: { project_id: "p1", nodes: [] } }) } as Response;
      }
      if (url.endsWith("/api/v1/projects/active/p1") && method === "DELETE") {
        return { json: async () => ({ ok: false, error: { code: "PROJECT_REMOVE_FAILED" } }) } as Response;
      }
      return { json: async () => ({ ok: false, error: { code: "NOT_FOUND" } }) } as Response;
    };

    const store = createShellmanStore(fakeFetch as typeof fetch, () => null as unknown as WebSocket);
    await store.load();

    await expect(store.removeActiveProject("p1")).rejects.toThrow("PROJECT_REMOVE_FAILED");
    expect(store.state.projects.map((p) => p.projectId)).toEqual(["p1"]);
  });

  it("archives done tasks by project and refreshes tree", async () => {
    const calls: Array<{ url: string; method: string }> = [];
    const fakeFetch = async (input: RequestInfo | URL, init?: RequestInit) => {
      const url = String(input);
      const method = init?.method ?? "GET";
      calls.push({ url, method });
      if (url.endsWith("/api/v1/projects/active") && method === "GET") {
        return { json: async () => ({ ok: true, data: [{ project_id: "p1", repo_root: "/tmp/p1" }] }) } as Response;
      }
      if (url.endsWith("/api/v1/projects/p1/tree") && method === "GET") {
        const archived = calls.some((c) => c.url.endsWith("/api/v1/projects/p1/archive-done") && c.method === "POST");
        if (!archived) {
          return {
            json: async () => ({
              ok: true,
              data: {
                project_id: "p1",
                nodes: [
                  { task_id: "t_open", title: "open", status: "running", checked: false },
                  { task_id: "t_done", title: "done", status: "completed", checked: true }
                ]
              }
            })
          } as Response;
        }
        return {
          json: async () => ({
            ok: true,
            data: { project_id: "p1", nodes: [{ task_id: "t_open", title: "open", status: "running", checked: false }] }
          })
        } as Response;
      }
      if (url.endsWith("/api/v1/projects/p1/archive-done") && method === "POST") {
        return { json: async () => ({ ok: true, data: { project_id: "p1", archived_count: 1 } }) } as Response;
      }
      return { json: async () => ({ ok: false, error: { code: "NOT_FOUND" } }) } as Response;
    };

    const store = createShellmanStore(fakeFetch as typeof fetch, () => null as unknown as WebSocket);
    await store.load();
    await store.archiveDoneTasksByProject("p1");

    expect(calls.some((c) => c.url.endsWith("/api/v1/projects/p1/archive-done") && c.method === "POST")).toBe(true);
    expect(store.state.treesByProject.p1.map((n) => n.taskId)).toEqual(["t_open"]);
  });

  it("lists directories via /api/v1/fs/list", async () => {
    const calls: string[] = [];
    const fakeFetch = async (input: RequestInfo | URL) => {
      calls.push(String(input));
      return {
        json: async () => ({
          ok: true,
          data: { path: "/tmp", items: [{ name: "a", path: "/tmp/a", is_dir: true }] }
        })
      } as Response;
    };
    const store = createShellmanStore(fakeFetch as typeof fetch, () => null as unknown as WebSocket, {
      baseOrigin: "http://127.0.0.1:8787",
      turnUUID: "u1"
    });
    const out = await store.listDirectories("/tmp");
    expect(out.items.length).toBe(1);
    expect(calls[0]).toContain("/api/v1/fs/list");
  });

  it("records directory history via /api/v1/fs/history", async () => {
    const calls: Array<{ url: string; method: string; body?: string }> = [];
    const fakeFetch = async (input: RequestInfo | URL, init?: RequestInit) => {
      calls.push({
        url: String(input),
        method: init?.method ?? "GET",
        body: typeof init?.body === "string" ? init.body : undefined
      });
      return { json: async () => ({ ok: true, data: { path: "/tmp/demo" } }) } as Response;
    };
    const store = createShellmanStore(fakeFetch as typeof fetch, () => null as unknown as WebSocket, {
      baseOrigin: "http://127.0.0.1:8787",
      turnUUID: "u1"
    });

    await store.recordDirectoryHistory("/tmp/demo");

    expect(calls[0]?.url).toBe("http://127.0.0.1:8787/api/v1/fs/history");
    expect(calls[0]?.method).toBe("POST");
    expect(calls[0]?.body).toContain("/tmp/demo");
  });

  it("selectDirectory posts to /api/v1/system/select-directory with turn header", async () => {
    const calls: Array<{ url: string; headers: Record<string, string> }> = [];
    const fakeFetch = async (input: RequestInfo | URL, init?: RequestInit) => {
      const headers = (init?.headers ?? {}) as Record<string, string>;
      calls.push({ url: String(input), headers });
      return { json: async () => ({ ok: true, data: { repo_root: "/tmp/demo" } }) } as Response;
    };

    const store = createShellmanStore(fakeFetch as typeof fetch, () => null as unknown as WebSocket, {
      baseOrigin: "http://127.0.0.1:8787",
      turnUUID: "u1"
    });

    await expect(store.selectDirectory()).resolves.toBe("/tmp/demo");
    expect(calls[0].url).toBe("http://127.0.0.1:8787/api/v1/system/select-directory");
    expect(calls[0].headers["X-Shellman-Turn-UUID"]).toBe("u1");
  });

  it("uploads image then sends terminal temp path with trailing space", async () => {
    const sock = new FakeSocket();
    const calls: Array<{ url: string; method: string; bodyType: string }> = [];
    const fakeFetch = async (input: RequestInfo | URL, init?: RequestInit) => {
      const url = String(input);
      const method = init?.method ?? "GET";
      calls.push({
        url,
        method,
        bodyType: init?.body?.constructor?.name ?? typeof init?.body ?? "none"
      });

      if (url.endsWith("/api/v1/projects/active")) {
        return { json: async () => ({ ok: true, data: [{ project_id: "p1", repo_root: "/tmp/p1" }] }) } as Response;
      }
      if (url.endsWith("/api/v1/projects/p1/tree")) {
        return {
          json: async () => ({ ok: true, data: { project_id: "p1", nodes: [{ task_id: "t1", title: "root", status: "running" }] } })
        } as Response;
      }
      if (url.endsWith("/api/v1/tasks/t1/pane")) {
        return { json: async () => ({ ok: true, data: { task_id: "t1", pane_uuid: "uuid-t1", pane_id: "e2e:0.0", pane_target: "e2e:0.0" } }) } as Response;
      }
      if (url.endsWith("/api/v1/system/uploads/image") && method === "POST") {
        return { json: async () => ({ ok: true, data: { path: "/tmp/shellman-img-upload.png", size: 123, mime: "image/png" } }) } as Response;
      }
      return { json: async () => ({ ok: false, error: { code: "NOT_FOUND" } }) } as Response;
    };

    const store = createShellmanStore(fakeFetch as typeof fetch, (url: string) => sock as unknown as WebSocket);
    await store.load();
    store.connectWS("ws://127.0.0.1:4621/ws/client/local");
    sock.emitOpen();
    await store.selectTask("t1");

    const file = new File([new Uint8Array([1, 2, 3])], "x.png", { type: "image/png" });
    await store.sendImagePasteToTerminal(file);

    const msg = JSON.parse(sock.sent.at(-1) ?? "{}");
    expect(msg.op).toBe("term.input");
    expect(msg.payload.text).toBe("/tmp/shellman-img-upload.png ");
    expect(calls.some((c) => c.url.endsWith("/api/v1/system/uploads/image") && c.method === "POST")).toBe(true);
    const uploadCall = calls.find((c) => c.url.endsWith("/api/v1/system/uploads/image"));
    expect(uploadCall?.bodyType).toBe("FormData");
  });

  it("rejects non-image paste file without upload", async () => {
    const calls: string[] = [];
    const fakeFetch = async (input: RequestInfo | URL, init?: RequestInit) => {
      calls.push(String(input));
      if (String(input).endsWith("/api/v1/projects/active")) {
        return { json: async () => ({ ok: true, data: [{ project_id: "p1", repo_root: "/tmp/p1" }] }) } as Response;
      }
      if (String(input).endsWith("/api/v1/projects/p1/tree")) {
        return {
          json: async () => ({ ok: true, data: { project_id: "p1", nodes: [{ task_id: "t1", title: "root", status: "running" }] } })
        } as Response;
      }
      if (String(input).endsWith("/api/v1/tasks/t1/pane")) {
        return { json: async () => ({ ok: true, data: { task_id: "t1", pane_uuid: "uuid-t1", pane_id: "e2e:0.0", pane_target: "e2e:0.0" } }) } as Response;
      }
      return { json: async () => ({ ok: true, data: {} }) } as Response;
    };

    const store = createShellmanStore(fakeFetch as typeof fetch, () => null as unknown as WebSocket);
    const textFile = new File([new Uint8Array([1, 2, 3])], "x.txt", { type: "text/plain" });

    await expect(store.sendImagePasteToTerminal(textFile)).rejects.toThrow("INVALID_PASTE_IMAGE");
    expect(calls.some((u) => u.endsWith("/api/v1/system/uploads/image"))).toBe(false);
  });

  it("rejects empty image paste file without upload", async () => {
    const calls: string[] = [];
    const fakeFetch = async (input: RequestInfo | URL) => {
      calls.push(String(input));
      if (String(input).endsWith("/api/v1/projects/active")) {
        return { json: async () => ({ ok: true, data: [{ project_id: "p1", repo_root: "/tmp/p1" }] }) } as Response;
      }
      if (String(input).endsWith("/api/v1/projects/p1/tree")) {
        return {
          json: async () => ({ ok: true, data: { project_id: "p1", nodes: [{ task_id: "t1", title: "root", status: "running" }] } })
        } as Response;
      }
      if (String(input).endsWith("/api/v1/tasks/t1/pane")) {
        return { json: async () => ({ ok: true, data: { task_id: "t1", pane_uuid: "uuid-t1", pane_id: "e2e:0.0", pane_target: "e2e:0.0" } }) } as Response;
      }
      return { json: async () => ({ ok: false, error: { code: "NOT_FOUND" } }) } as Response;
    };

    const store = createShellmanStore(fakeFetch as typeof fetch, () => null as unknown as WebSocket);
    await store.load();
    const file = new File([], "x.png", { type: "image/png" });

    await expect(store.sendImagePasteToTerminal(file)).rejects.toThrow("INVALID_PASTE_IMAGE");
    expect(calls.some((u) => u.endsWith("/api/v1/system/uploads/image"))).toBe(false);
  });

  it("throws upload error when image upload returns failure", async () => {
    const sock = new FakeSocket();
    const fakeFetch = async (input: RequestInfo | URL, init?: RequestInit) => {
      const url = String(input);
      if (url.endsWith("/api/v1/projects/active")) {
        return { json: async () => ({ ok: true, data: [{ project_id: "p1", repo_root: "/tmp/p1" }] }) } as Response;
      }
      if (url.endsWith("/api/v1/projects/p1/tree")) {
        return {
          json: async () => ({ ok: true, data: { project_id: "p1", nodes: [{ task_id: "t1", title: "root", status: "running" }] } })
        } as Response;
      }
      if (url.endsWith("/api/v1/tasks/t1/pane")) {
        return { json: async () => ({ ok: true, data: { task_id: "t1", pane_uuid: "uuid-t1", pane_id: "e2e:0.0", pane_target: "e2e:0.0" } }) } as Response;
      }
      if (url.endsWith("/api/v1/system/uploads/image") && (init?.method ?? "GET") === "POST") {
        return { json: async () => ({ ok: false, error: { code: "UPLOAD_FAILED" } }) } as Response;
      }
      return { json: async () => ({ ok: false, error: { code: "NOT_FOUND" } }) } as Response;
    };

    const store = createShellmanStore(fakeFetch as typeof fetch, (url: string) => sock as unknown as WebSocket);
    await store.load();
    store.connectWS("ws://127.0.0.1:4621/ws/client/local");
    sock.emitOpen();
    await store.selectTask("t1");

    const imageFile = new File([new Uint8Array([1, 2, 3])], "x.png", { type: "image/png" });
    await expect(store.sendImagePasteToTerminal(imageFile)).rejects.toThrow("UPLOAD_FAILED");
    expect(sock.sent.some((raw) => JSON.parse(raw).op === "term.input")).toBe(false);
  });

  it("does not send term.input if no selected pane target", async () => {
    const sock = new FakeSocket();
    const fakeFetch = async (input: RequestInfo | URL) => {
      const url = String(input);
      if (url.endsWith("/api/v1/projects/active")) {
        return { json: async () => ({ ok: true, data: [{ project_id: "p1", repo_root: "/tmp/p1" }] }) } as Response;
      }
      if (url.endsWith("/api/v1/projects/p1/tree")) {
        return { json: async () => ({ ok: true, data: { project_id: "p1", nodes: [] } }) } as Response;
      }
      if (url.endsWith("/api/v1/system/uploads/image")) {
        return {
          json: async () => ({ ok: true, data: { path: "/tmp/shellman-img-upload.png", size: 123, mime: "image/png" } })
        } as Response;
      }
      return { json: async () => ({ ok: false, error: { code: "NOT_FOUND" } }) } as Response;
    };

    const store = createShellmanStore(fakeFetch as typeof fetch, (url: string) => sock as unknown as WebSocket);
    await store.load();
    store.connectWS("ws://127.0.0.1:4621/ws/client/local");
    sock.emitOpen();

    const file = new File([new Uint8Array([1, 2, 3])], "x.png", { type: "image/png" });
    await store.sendImagePasteToTerminal(file);

    expect(sock.sent.some((raw) => JSON.parse(raw).op === "term.input")).toBe(false);
  });

  it("creates root task without pane binding and selects it", async () => {
    const sock = new FakeSocket();
    const fakeFetch = async (input: RequestInfo | URL, init?: RequestInit) => {
      const url = String(input);
      const method = init?.method ?? "GET";
      if (url.endsWith("/api/v1/projects/active") && method === "GET") {
        return { json: async () => ({ ok: true, data: [{ project_id: "p1", repo_root: "/tmp/p1" }] }) } as Response;
      }
      if (url.endsWith("/api/v1/projects/p1/tree") && method === "GET") {
        return { json: async () => ({ ok: true, data: { project_id: "p1", nodes: [] } }) } as Response;
      }
      if (url.endsWith("/api/v1/tasks") && method === "POST") {
        return { json: async () => ({ ok: true, data: { task_id: "t_root" } }) } as Response;
      }
      if (url.endsWith("/api/v1/tasks/t_root/pane") && method === "GET") {
        return { json: async () => ({ ok: false, error: { code: "TASK_PANE_NOT_FOUND" } }) } as Response;
      }
      return { json: async () => ({ ok: false, error: { code: "NOT_FOUND", message: `${method} ${url}` } }) } as Response;
    };

    const store = createShellmanStore(fakeFetch as typeof fetch, () => sock as unknown as WebSocket);
    await store.load();
    store.connectWS("ws://127.0.0.1:4621/ws/client/local");
    sock.emitOpen();
    sock.sent = [];
    await store.createRootTask("p1", "root task");

    expect(store.state.treesByProject.p1.length).toBe(1);
    expect(store.state.treesByProject.p1[0]?.taskId).toBe("t_root");
    expect(store.state.treesByProject.p1[0]?.status).toBe("pending");
    expect(store.state.paneByTaskId.t_root).toBeUndefined();
    expect(store.state.selectedTaskId).toBe("t_root");
    expect(store.state.selectedPaneTarget).toBe("");
    const sentOps = sock.sent.map((s) => JSON.parse(s).op);
    expect(sentOps).not.toContain("tmux.select_pane");
  });

  it("createRootTask should call /api/v1/tasks and not call root-pane endpoint", async () => {
    const sock = new FakeSocket();
    const calls: Array<{ url: string; method: string }> = [];
    const fakeFetch = async (input: RequestInfo | URL, init?: RequestInit) => {
      const url = String(input);
      const method = init?.method ?? "GET";
      calls.push({ url, method });
      if (url.endsWith("/api/v1/projects/active") && method === "GET") {
        return { json: async () => ({ ok: true, data: [{ project_id: "p1", repo_root: "/tmp/p1" }] }) } as Response;
      }
      if (url.endsWith("/api/v1/projects/p1/tree") && method === "GET") {
        return { json: async () => ({ ok: true, data: { project_id: "p1", nodes: [] } }) } as Response;
      }
      if (url.endsWith("/api/v1/tasks/t_root2/pane") && method === "GET") {
        return { json: async () => ({ ok: false, error: { code: "TASK_PANE_NOT_FOUND" } }) } as Response;
      }
      if (url.endsWith("/api/v1/tasks") && method === "POST") {
        return { json: async () => ({ ok: true, data: { task_id: "t_root2" } }) } as Response;
      }
      return { json: async () => ({ ok: false, error: { code: "NOT_FOUND" } }) } as Response;
    };

    const store = createShellmanStore(fakeFetch as typeof fetch, () => sock as unknown as WebSocket);
    await store.load();
    store.connectWS("ws://127.0.0.1:4621/ws/client/local");
    sock.emitOpen();
    await store.createRootTask("p1", "root task");

    expect(calls.some((c) => c.method === "POST" && c.url.endsWith("/api/v1/tasks"))).toBe(true);
    expect(calls.some((c) => c.method === "POST" && c.url.endsWith("/api/v1/projects/p1/panes/root"))).toBe(false);
  });

  it("createChildTask should call derive endpoint and not call child-pane endpoint", async () => {
    const sock = new FakeSocket();
    const calls: Array<{ url: string; method: string }> = [];
    const fakeFetch = async (input: RequestInfo | URL, init?: RequestInit) => {
      const url = String(input);
      const method = init?.method ?? "GET";
      calls.push({ url, method });
      if (url.endsWith("/api/v1/projects/active") && method === "GET") {
        return { json: async () => ({ ok: true, data: [{ project_id: "p1", repo_root: "/tmp/p1" }] }) } as Response;
      }
      if (url.endsWith("/api/v1/projects/p1/tree") && method === "GET") {
        return {
          json: async () => ({
            ok: true,
            data: { project_id: "p1", nodes: [{ task_id: "t_parent2", title: "parent2", status: "running" }] }
          })
        } as Response;
      }
      if (url.endsWith("/api/v1/tasks/t_parent2/pane") && method === "GET") {
        return {
          json: async () => ({ ok: true, data: { task_id: "t_parent2", pane_uuid: "uuid-parent2", pane_id: "e2e:8.0", pane_target: "e2e:8.0" } })
        } as Response;
      }
      if (url.endsWith("/api/v1/tasks/t_child2/pane") && method === "GET") {
        return { json: async () => ({ ok: false, error: { code: "TASK_PANE_NOT_FOUND" } }) } as Response;
      }
      if (url.endsWith("/api/v1/tasks/t_parent2/derive") && method === "POST") {
        return { json: async () => ({ ok: true, data: { task_id: "t_child2" } }) } as Response;
      }
      return { json: async () => ({ ok: false, error: { code: "NOT_FOUND" } }) } as Response;
    };

    const store = createShellmanStore(fakeFetch as typeof fetch, () => sock as unknown as WebSocket);
    await store.load();
    store.connectWS("ws://127.0.0.1:4621/ws/client/local");
    sock.emitOpen();
    await store.createChildTask("t_parent2", "child task");

    expect(calls.some((c) => c.method === "POST" && c.url.endsWith("/api/v1/tasks/t_parent2/derive"))).toBe(true);
    expect(calls.some((c) => c.method === "POST" && c.url.endsWith("/api/v1/tasks/t_parent2/panes/child"))).toBe(false);
  });

  it("logs ws response errors to browser console", () => {
    const sock = new FakeSocket();
    const warn = vi.spyOn(console, "warn").mockImplementation(() => {});
    const store = createShellmanStore(async () => ({ json: async () => ({ ok: true, data: [] }) }) as Response, () => sock as unknown as WebSocket);

    store.connectWS("ws://127.0.0.1:4621/ws/client/local");
    sock.emitOpen();
    sock.emitMessage(
      JSON.stringify({
        id: "req_3",
        type: "res",
        op: "tmux.select_pane",
        error: { code: "TMUX_ERROR", message: "can't find pane" }
      })
    );

    expect(warn).toHaveBeenCalledTimes(1);
    warn.mockRestore();
  });

  it("ignores tmux.select_pane non-missing TMUX_ERROR and keeps terminal live", async () => {
    const sock = new FakeSocket();
    const fakeFetch = async (url: string) => {
      if (url.endsWith("/api/v1/projects/active")) {
        return { json: async () => ({ ok: true, data: [{ project_id: "p1", repo_root: "/tmp/p1" }] }) } as Response;
      }
      if (url.endsWith("/api/v1/projects/p1/tree")) {
        return {
          json: async () => ({ ok: true, data: { project_id: "p1", nodes: [{ task_id: "t1", title: "root", status: "running" }] } })
        } as Response;
      }
      if (url.endsWith("/api/v1/tasks/t1/pane")) {
        return { json: async () => ({ ok: true, data: { task_id: "t1", pane_uuid: "uuid-t1", pane_id: "e2e:0.0", pane_target: "e2e:0.0" } }) } as Response;
      }
      return { json: async () => ({ ok: false, error: { code: "NOT_FOUND" } }) } as Response;
    };

    const store = createShellmanStore(fakeFetch as typeof fetch, () => sock as unknown as WebSocket);
    await store.load();
    store.connectWS("ws://127.0.0.1:4621/ws/client/local");
    sock.emitOpen();
    await store.selectTask("t1");

    const selectReq = sock.sent
      .map((raw) => JSON.parse(raw) as { id?: string; op?: string })
      .find((msg) => msg.op === "tmux.select_pane");
    expect(selectReq?.id).toBeTruthy();

    sock.emitMessage(
      JSON.stringify({
        id: selectReq?.id,
        type: "res",
        op: "tmux.select_pane",
        error: { code: "TMUX_ERROR", message: "resize failed: layout constrained" }
      })
    );

    expect(store.state.terminalEnded).toBe(false);
    expect(store.state.selectedPaneTarget).toBe("e2e:0.0");
    expect(store.state.terminalOutput).not.toContain("Session ended. Pane is no longer available.");
  });

  it("shows session-ended message when selected pane reports TMUX_PANE_NOT_FOUND and ignores other pane output", async () => {
    const sock = new FakeSocket();
    const fakeFetch = async (url: string) => {
      if (url.endsWith("/api/v1/projects/active")) {
        return { json: async () => ({ ok: true, data: [{ project_id: "p1", repo_root: "/tmp/p1" }] }) } as Response;
      }
      if (url.endsWith("/api/v1/projects/p1/tree")) {
        return {
          json: async () => ({
            ok: true,
            data: {
              project_id: "p1",
              nodes: [{ task_id: "t1", title: "root", status: "running" }]
            }
          })
        } as Response;
      }
      if (url.endsWith("/api/v1/tasks/t1/pane")) {
        return { json: async () => ({ ok: true, data: { task_id: "t1", pane_uuid: "uuid-t1", pane_id: "e2e:0.0", pane_target: "e2e:0.0" } }) } as Response;
      }
      return { json: async () => ({ ok: false, error: { code: "NOT_FOUND" } }) } as Response;
    };

    const store = createShellmanStore(fakeFetch as typeof fetch, () => sock as unknown as WebSocket);
    await store.load();
    store.connectWS("ws://127.0.0.1:4621/ws/client/local");
    sock.emitOpen();
    await store.selectTask("t1");

    const selectReq = sock.sent
      .map((raw) => JSON.parse(raw) as { id?: string; op?: string })
      .find((msg) => msg.op === "tmux.select_pane");
    expect(selectReq?.id).toBeTruthy();

    sock.emitMessage(
      JSON.stringify({
        id: selectReq?.id,
        type: "res",
        op: "tmux.select_pane",
        error: { code: "TMUX_PANE_NOT_FOUND", message: "" }
      })
    );

    expect(store.state.terminalOutput).toContain("Session ended. Pane is no longer available.");
    expect(store.state.terminalOutput).not.toContain("Reason:");
    expect(store.state.selectedPaneTarget).toBe("");

    sock.emitMessage(
      JSON.stringify({
        id: "evt_other",
        type: "event",
        op: "term.output",
        payload: { target: "e2e:0.9", mode: "reset", data: "other-pane\n" }
      })
    );

    expect(store.state.terminalOutput).toContain("Session ended. Pane is no longer available.");
    expect(store.state.terminalOutput).not.toContain("other-pane");
  });

  it("treats synthetic missing pane target as ended even when response code is generic", async () => {
    const sock = new FakeSocket();
    const fakeFetch = async (url: string) => {
      if (url.endsWith("/api/v1/projects/active")) {
        return { json: async () => ({ ok: true, data: [{ project_id: "p1", repo_root: "/tmp/p1" }] }) } as Response;
      }
      if (url.endsWith("/api/v1/projects/p1/tree")) {
        return {
          json: async () => ({
            ok: true,
            data: {
              project_id: "p1",
              nodes: [{ task_id: "t_missing", title: "missing", status: "running" }]
            }
          })
        } as Response;
      }
      if (url.endsWith("/api/v1/tasks/t_missing/pane")) {
        return {
          json: async () => ({
            ok: true,
            data: {
              task_id: "t_missing",
              pane_uuid: "uuid-missing",
              pane_id: "missing:p1_1",
              pane_target: "missing:p1_1"
            }
          })
        } as Response;
      }
      return { json: async () => ({ ok: false, error: { code: "NOT_FOUND" } }) } as Response;
    };

    const store = createShellmanStore(fakeFetch as typeof fetch, () => sock as unknown as WebSocket);
    await store.load();
    store.connectWS("ws://127.0.0.1:4621/ws/client/local");
    sock.emitOpen();
    await store.selectTask("t_missing");

    const selectReq = sock.sent
      .map((raw) => JSON.parse(raw) as { id?: string; op?: string })
      .find((msg) => msg.op === "tmux.select_pane");
    expect(selectReq?.id).toBeTruthy();

    sock.emitMessage(
      JSON.stringify({
        id: selectReq?.id,
        type: "res",
        op: "tmux.select_pane",
        error: { code: "TMUX_ERROR", message: "exit status 1" }
      })
    );

    expect(store.state.terminalEnded).toBe(true);
    expect(store.state.terminalOutput).toContain("Session ended. Pane is no longer available.");
    expect(store.state.selectedPaneTarget).toBe("");
  });

  it("sanitizes ended terminal text by stripping ansi and trimming trailing blank lines", async () => {
    const sock = new FakeSocket();
    const fakeFetch = async (url: string) => {
      if (url.endsWith("/api/v1/projects/active")) {
        return { json: async () => ({ ok: true, data: [{ project_id: "p1", repo_root: "/tmp/p1" }] }) } as Response;
      }
      if (url.endsWith("/api/v1/projects/p1/tree")) {
        return {
          json: async () => ({
            ok: true,
            data: {
              project_id: "p1",
              nodes: [{ task_id: "t1", title: "root", status: "running" }]
            }
          })
        } as Response;
      }
      if (url.endsWith("/api/v1/tasks/t1/pane")) {
        return { json: async () => ({ ok: true, data: { task_id: "t1", pane_uuid: "uuid-t1", pane_id: "e2e:0.0", pane_target: "e2e:0.0" } }) } as Response;
      }
      return { json: async () => ({ ok: false, error: { code: "NOT_FOUND" } }) } as Response;
    };

    const store = createShellmanStore(fakeFetch as typeof fetch, () => sock as unknown as WebSocket);
    await store.load();
    store.connectWS("ws://127.0.0.1:4621/ws/client/local");
    sock.emitOpen();
    await store.selectTask("t1");

    sock.emitMessage(
      JSON.stringify({
        id: "evt_color",
        type: "event",
        op: "term.output",
        payload: { target: "e2e:0.0", mode: "reset", data: "\u001b[31mred\u001b[0m\n\n\n", cursor: { x: 0, y: 0 } }
      })
    );

    const selectReq = sock.sent
      .map((raw) => JSON.parse(raw) as { id?: string; op?: string })
      .find((msg) => msg.op === "tmux.select_pane");
    sock.emitMessage(
      JSON.stringify({
        id: selectReq?.id,
        type: "res",
        op: "tmux.select_pane",
        error: { code: "TMUX_PANE_NOT_FOUND", message: "" }
      })
    );

    expect(store.state.terminalOutput).toContain("red\nSession ended. Pane is no longer available.\n");
    expect(store.state.terminalOutput).not.toContain("\u001b[31m");
    expect(store.state.terminalOutput).not.toContain("\n\n\nSession ended");
    expect(store.state.terminalEnded).toBe(true);
  });

  it("reopens pane for existing task and reselects the task", async () => {
    const sock = new FakeSocket();
    const fakeFetch = async (input: RequestInfo | URL, init?: RequestInit) => {
      const url = String(input);
      const method = init?.method ?? "GET";
      if (url.endsWith("/api/v1/projects/active")) {
        return { json: async () => ({ ok: true, data: [{ project_id: "p1", repo_root: "/tmp/p1" }] }) } as Response;
      }
      if (url.endsWith("/api/v1/projects/p1/tree")) {
        return {
          json: async () => ({
            ok: true,
            data: {
              project_id: "p1",
              nodes: [{ task_id: "t1", title: "root", status: "running" }]
            }
          })
        } as Response;
      }
      if (url.endsWith("/api/v1/tasks/t1/pane")) {
        return { json: async () => ({ ok: true, data: { task_id: "t1", pane_uuid: "uuid-t1", pane_id: "e2e:0.0", pane_target: "e2e:0.0" } }) } as Response;
      }
      if (url.endsWith("/api/v1/tasks/t1/panes/reopen") && method === "POST") {
        return { json: async () => ({ ok: true, data: { task_id: "t1", pane_uuid: "uuid-t1b", pane_id: "e2e:1.0", pane_target: "e2e:1.0" } }) } as Response;
      }
      return { json: async () => ({ ok: false, error: { code: "NOT_FOUND" } }) } as Response;
    };

    const store = createShellmanStore(fakeFetch as typeof fetch, () => sock as unknown as WebSocket);
    await store.load();
    store.connectWS("ws://127.0.0.1:4621/ws/client/local");
    sock.emitOpen();
    sock.sent = [];

    await store.reopenPaneForTask("t1");

    expect(store.state.paneByTaskId.t1?.paneUuid).toBe("uuid-t1b");
    expect(store.state.paneByTaskId.t1?.paneTarget).toBe("e2e:1.0");
    expect(store.state.selectedTaskId).toBe("t1");
    const sentOps = sock.sent.map((raw) => JSON.parse(raw).op);
    expect(sentOps).toContain("tmux.select_pane");
  });

  it("derives task title from init prompt when reopening pane", async () => {
    const sock = new FakeSocket();
    const calls: Array<{ url: string; method: string; body?: string }> = [];
    const fakeFetch = async (input: RequestInfo | URL, init?: RequestInit) => {
      const url = String(input);
      const method = init?.method ?? "GET";
      calls.push({ url, method, body: typeof init?.body === "string" ? init.body : undefined });
      if (url.endsWith("/api/v1/projects/active")) {
        return { json: async () => ({ ok: true, data: [{ project_id: "p1", repo_root: "/tmp/p1" }] }) } as Response;
      }
      if (url.endsWith("/api/v1/projects/p1/tree")) {
        return {
          json: async () => ({
            ok: true,
            data: {
              project_id: "p1",
              nodes: [{ task_id: "t1", title: "root", status: "running" }]
            }
          })
        } as Response;
      }
      if (url.endsWith("/api/v1/tasks/t1/pane")) {
        return { json: async () => ({ ok: true, data: { task_id: "t1", pane_uuid: "uuid-t1", pane_id: "e2e:0.0", pane_target: "e2e:0.0" } }) } as Response;
      }
      if (url.endsWith("/api/v1/tasks/t1/panes/reopen") && method === "POST") {
        return { json: async () => ({ ok: true, data: { task_id: "t1", pane_uuid: "uuid-t1b", pane_id: "e2e:1.0", pane_target: "e2e:1.0" } }) } as Response;
      }
      if (url.endsWith("/api/v1/tasks/t1/title") && method === "PATCH") {
        return { json: async () => ({ ok: true, data: { task_id: "t1", title: "updated" } }) } as Response;
      }
      return { json: async () => ({ ok: false, error: { code: "NOT_FOUND" } }) } as Response;
    };

    const store = createShellmanStore(fakeFetch as typeof fetch, () => sock as unknown as WebSocket);
    await store.load();
    store.connectWS("ws://127.0.0.1:4621/ws/client/local");
    sock.emitOpen();
    sock.sent = [];

    await store.reopenPaneForTask("t1", {
      program: "codex",
      prompt: "Refactor authentication middleware to avoid duplicated token parsing between admin and user endpoints."
    });

    const titlePatch = calls.find((c) => c.url.endsWith("/api/v1/tasks/t1/title") && c.method === "PATCH");
    expect(titlePatch).toBeTruthy();
    const payload = JSON.parse(String(titlePatch?.body ?? "{}")) as { title?: string };
    expect(payload.title).toBeTruthy();
    expect(String(payload.title).length).toBeLessThanOrEqual(48);
    expect(String(payload.title)).toContain("Refactor authentication middleware");
  });

  it("caches missing pane lookup and warns once", async () => {
    const calls: string[] = [];
    const warn = vi.spyOn(console, "warn").mockImplementation(() => {});
    const fakeFetch = async (url: string) => {
      calls.push(url);
      if (url.endsWith("/api/v1/projects/active")) {
        return { json: async () => ({ ok: true, data: [{ project_id: "p1", repo_root: "/tmp/p1" }] }) } as Response;
      }
      if (url.endsWith("/api/v1/projects/p1/tree")) {
        return {
          json: async () => ({ ok: true, data: { project_id: "p1", nodes: [{ task_id: "t_missing", title: "missing", status: "running" }] } })
        } as Response;
      }
      if (url.endsWith("/api/v1/tasks/t_missing/pane")) {
        return { json: async () => ({ ok: false, error: { code: "TASK_PANE_NOT_FOUND", message: "task pane binding not found" } }) } as Response;
      }
      return { json: async () => ({ ok: false, error: { code: "NOT_FOUND" } }) } as Response;
    };

    const store = createShellmanStore(fakeFetch as typeof fetch, () => null as unknown as WebSocket);
    await store.load();
    await store.selectTask("t_missing");
    await store.selectTask("t_missing");

    const paneCalls = calls.filter((c) => c.endsWith("/api/v1/tasks/t_missing/pane"));
    expect(paneCalls.length).toBe(1);
    expect(warn.mock.calls.some((call) => String(call[0]).includes("pane binding missing"))).toBe(true);
    warn.mockRestore();
  });

  it("falls back to selected task pane binding when selectedPaneTarget is temporarily empty", async () => {
    const sock = new FakeSocket();
    const fakeFetch = async (url: string) => {
      if (url.endsWith("/api/v1/projects/active")) {
        return { json: async () => ({ ok: true, data: [{ project_id: "p1", repo_root: "/tmp/p1" }] }) } as Response;
      }
      if (url.endsWith("/api/v1/projects/p1/tree")) {
        return {
          json: async () => ({ ok: true, data: { project_id: "p1", nodes: [{ task_id: "t1", title: "root", status: "running" }] } })
        } as Response;
      }
      if (url.endsWith("/api/v1/tasks/t1/pane")) {
        return { json: async () => ({ ok: true, data: { task_id: "t1", pane_uuid: "uuid-t1", pane_id: "e2e:0.0", pane_target: "e2e:0.0" } }) } as Response;
      }
      return { json: async () => ({ ok: false, error: { code: "NOT_FOUND" } }) } as Response;
    };

    const store = createShellmanStore(fakeFetch as typeof fetch, () => sock as unknown as WebSocket);
    await store.load();
    store.connectWS("ws://127.0.0.1:4621/ws/client/local");
    sock.emitOpen();
    sock.sent = [];

    store.state.selectedTaskId = "t1";
    store.state.selectedPaneTarget = "";
    store.sendTerminalInput("l");

    const inputReq = sock.sent.map((raw) => JSON.parse(raw)).find((msg) => msg.op === "term.input");
    expect(inputReq).toBeTruthy();
    expect(inputReq.payload.target).toBe("e2e:0.0");
    expect(inputReq.payload.text).toBe("l");
  });

  it("keeps terminal content on task switch and restores cached pane output", async () => {
    const sock = new FakeSocket();
    const fakeFetch = async (url: string) => {
      if (url.endsWith("/api/v1/projects/active")) {
        return { json: async () => ({ ok: true, data: [{ project_id: "p1", repo_root: "/tmp/p1" }] }) } as Response;
      }
      if (url.endsWith("/api/v1/projects/p1/tree")) {
        return {
          json: async () => ({
            ok: true,
            data: {
              project_id: "p1",
              nodes: [
                { task_id: "t1", title: "one", status: "running" },
                { task_id: "t2", title: "two", status: "running" }
              ]
            }
          })
        } as Response;
      }
      if (url.endsWith("/api/v1/tasks/t1/pane")) {
        return { json: async () => ({ ok: true, data: { task_id: "t1", pane_uuid: "uuid-t1", pane_id: "e2e:0.0", pane_target: "e2e:0.0" } }) } as Response;
      }
      if (url.endsWith("/api/v1/tasks/t2/pane")) {
        return { json: async () => ({ ok: true, data: { task_id: "t2", pane_uuid: "uuid-t2", pane_id: "e2e:0.1", pane_target: "e2e:0.1" } }) } as Response;
      }
      return { json: async () => ({ ok: false, error: { code: "NOT_FOUND" } }) } as Response;
    };

    const store = createShellmanStore(fakeFetch as typeof fetch, () => sock as unknown as WebSocket);
    await store.load();
    store.connectWS("ws://127.0.0.1:4621/ws/client/local");
    sock.emitOpen();

    await store.selectTask("t1");
    sock.emitMessage(
      JSON.stringify({
        id: "evt_t1",
        type: "event",
        op: "term.output",
        payload: { target: "e2e:0.0", mode: "reset", data: "pane-one\n", cursor: { x: 0, y: 1 } }
      })
    );
    expect(store.state.terminalOutput).toBe("pane-one\n");

    await store.selectTask("t2");
    expect(store.state.terminalOutput).toBe("pane-one\n");

    sock.emitMessage(
      JSON.stringify({
        id: "evt_t2",
        type: "event",
        op: "term.output",
        payload: { target: "e2e:0.1", mode: "reset", data: "pane-two\n", cursor: { x: 0, y: 1 } }
      })
    );
    expect(store.state.terminalOutput).toBe("pane-two\n");

    await store.selectTask("t1");
    expect(store.state.terminalOutput).toBe("pane-one\n");
    expect(store.state.terminalFrame).toEqual({ mode: "reset", data: "pane-one\n" });
    expect(store.state.terminalCursor).toEqual({ x: 0, y: 1 });
  });

  it("applies tmux.status to task runtime status via pane target mapping", async () => {
    const sock = new FakeSocket();
    const fakeFetch = async (url: string) => {
      if (url.endsWith("/api/v1/projects/active")) {
        return { json: async () => ({ ok: true, data: [{ project_id: "p1", repo_root: "/tmp/p1" }] }) } as Response;
      }
      if (url.endsWith("/api/v1/projects/p1/tree")) {
        return {
          json: async () => ({
            ok: true,
            data: {
              project_id: "p1",
              nodes: [{ task_id: "t1", title: "one", status: "running" }]
            }
          })
        } as Response;
      }
      if (url.endsWith("/api/v1/tasks/t1/pane")) {
        return { json: async () => ({ ok: true, data: { task_id: "t1", pane_uuid: "uuid-t1", pane_id: "e2e:0.0", pane_target: "e2e:0.0" } }) } as Response;
      }
      return { json: async () => ({ ok: false, error: { code: "NOT_FOUND" } }) } as Response;
    };

    const store = createShellmanStore(fakeFetch as typeof fetch, () => sock as unknown as WebSocket);
    await store.load();
    store.connectWS("ws://127.0.0.1:4621/ws/client/local");
    sock.emitOpen();

    sock.emitMessage(
      JSON.stringify({
        id: "evt_status_1",
        type: "event",
        op: "tmux.status",
        payload: {
          items: [
            {
              target: "e2e:0.0",
              title: "main-pane",
              current_command: "npm",
              status: "ready",
              updated_at: 1771408800
            }
          ]
        }
      })
    );

    expect(store.state.taskRuntimeByTaskId.t1?.status).toBe("ready");
    expect(store.state.taskRuntimeByTaskId.t1?.updatedAt).toBe(1771408800);
    expect(store.state.paneMetaByTarget["e2e:0.0"]).toEqual({
      title: "main-pane",
      currentCommand: "npm"
    });
  });

  it("applies tmux.status tree delta to left panel nodes", async () => {
    const sock = new FakeSocket();
    const fakeFetch = async (url: string) => {
      if (url.endsWith("/api/v1/projects/active")) {
        return { json: async () => ({ ok: true, data: [{ project_id: "p1", repo_root: "/tmp/p1" }] }) } as Response;
      }
      if (url.endsWith("/api/v1/projects/p1/tree")) {
        return {
          json: async () => ({
            ok: true,
            data: {
              project_id: "p1",
              nodes: [
                { task_id: "t_root", title: "root", status: "pending" },
                { task_id: "t_old", title: "old", status: "pending" }
              ]
            }
          })
        } as Response;
      }
      if (url.endsWith("/api/v1/tasks/t_root/pane")) {
        return { json: async () => ({ ok: true, data: { task_id: "t_root", pane_uuid: "uuid-root", pane_id: "e2e:0.0", pane_target: "e2e:0.0" } }) } as Response;
      }
      return { json: async () => ({ ok: false, error: { code: "NOT_FOUND" } }) } as Response;
    };

    const store = createShellmanStore(fakeFetch as typeof fetch, () => sock as unknown as WebSocket);
    await store.load();
    store.connectWS("ws://127.0.0.1:4621/ws/client/local");
    sock.emitOpen();

    sock.emitMessage(
      JSON.stringify({
        id: "evt_tmux_status_delta",
        type: "event",
        op: "tmux.status",
        payload: {
          mode: "delta",
          tree: {
            project_id: "p1",
            added: [{ task_id: "t_new", title: "new", status: "pending" }],
            removed: ["t_old"],
            updated: [{ task_id: "t_root", title: "root-v2", status: "running" }],
            reparented: [{ task_id: "t_new", new_parent_task_id: "t_root" }]
          }
        }
      })
    );

    const nodes = store.state.treesByProject.p1;
    expect(nodes.find((n) => n.taskId === "t_old")).toBeUndefined();
    expect(nodes.find((n) => n.taskId === "t_root")?.title).toBe("root-v2");
    expect(nodes.find((n) => n.taskId === "t_root")?.status).toBe("running");
    expect(nodes.find((n) => n.taskId === "t_new")?.parentTaskId).toBe("t_root");
  });

  it("applies tmux.status runtime task current_command to left panel nodes", async () => {
    const sock = new FakeSocket();
    const fakeFetch = async (url: string) => {
      if (url.endsWith("/api/v1/projects/active")) {
        return { json: async () => ({ ok: true, data: [{ project_id: "p1", repo_root: "/tmp/p1" }] }) } as Response;
      }
      if (url.endsWith("/api/v1/projects/p1/tree")) {
        return {
          json: async () => ({
            ok: true,
            data: {
              project_id: "p1",
              nodes: [{ task_id: "t_root", title: "root", status: "running" }]
            }
          })
        } as Response;
      }
      if (url.endsWith("/api/v1/tasks/t_root/pane")) {
        return { json: async () => ({ ok: true, data: { task_id: "t_root", pane_uuid: "uuid-root", pane_id: "e2e:0.0", pane_target: "e2e:0.0" } }) } as Response;
      }
      return { json: async () => ({ ok: false, error: { code: "NOT_FOUND" } }) } as Response;
    };

    const store = createShellmanStore(fakeFetch as typeof fetch, () => sock as unknown as WebSocket);
    await store.load();
    store.connectWS("ws://127.0.0.1:4621/ws/client/local");
    sock.emitOpen();

    sock.emitMessage(
      JSON.stringify({
        id: "evt_tmux_status_delta_runtime",
        type: "event",
        op: "tmux.status",
        payload: {
          mode: "delta",
          runtime: {
            tasks: [
              {
                task_id: "t_root",
                source_pane_id: "e2e:0.0",
                current_command: "npm run dev",
                runtime_status: "running",
                snapshot_hash: "h1",
                updated_at: 1771408800
              }
            ]
          }
        }
      })
    );

    const node = store.state.treesByProject.p1?.find((n) => n.taskId === "t_root");
    expect(node?.title).toBe("root");
    expect(node?.currentCommand).toBe("npm run dev");
  });

  it("keeps tmux pane inventory and derives orphan panes", async () => {
    const sock = new FakeSocket();
    const fakeFetch = async (url: string) => {
      if (url.endsWith("/api/v1/projects/active")) {
        return { json: async () => ({ ok: true, data: [{ project_id: "p1", repo_root: "/tmp/p1" }] }) } as Response;
      }
      if (url.endsWith("/api/v1/projects/p1/tree")) {
        return {
          json: async () => ({
            ok: true,
            data: { project_id: "p1", nodes: [{ task_id: "t1", title: "one", status: "running" }] }
          })
        } as Response;
      }
      if (url.endsWith("/api/v1/tasks/t1/pane")) {
        return { json: async () => ({ ok: true, data: { task_id: "t1", pane_uuid: "uuid-t1", pane_id: "e2e:0.0", pane_target: "e2e:0.0" } }) } as Response;
      }
      return { json: async () => ({ ok: false, error: { code: "NOT_FOUND" } }) } as Response;
    };

    const store = createShellmanStore(fakeFetch as typeof fetch, () => sock as unknown as WebSocket);
    await store.load();
    store.connectWS("ws://127.0.0.1:4621/ws/client/local");
    sock.emitOpen();

    sock.emitMessage(
      JSON.stringify({
        id: "evt_status_orphan",
        type: "event",
        op: "tmux.status",
        payload: {
          items: [
            {
              target: "e2e:0.0",
              title: "bound-pane",
              current_command: "npm",
              status: "running",
              updated_at: 1771408800
            },
            {
              target: "e2e:0.1",
              title: "orphan-pane",
              current_command: "bash",
              status: "ready",
              updated_at: 1771408801
            }
          ]
        }
      })
    );

    const orphans = store.getOrphanPaneItems();
    expect(orphans.length).toBe(1);
    expect(orphans[0]?.target).toBe("e2e:0.1");
    expect(orphans[0]?.title).toBe("orphan-pane");
  });

  it("adopts pane as child without selecting it", async () => {
    const calls: Array<{ url: string; method: string; body?: string }> = [];
    const fakeFetch = async (input: RequestInfo | URL, init?: RequestInit) => {
      const url = String(input);
      const method = init?.method ?? "GET";
      calls.push({ url, method, body: typeof init?.body === "string" ? init.body : undefined });
      if (url.endsWith("/api/v1/projects/active")) {
        return { json: async () => ({ ok: true, data: [{ project_id: "p1", repo_root: "/tmp/p1" }] }) } as Response;
      }
      if (url.endsWith("/api/v1/projects/p1/tree")) {
        return {
          json: async () => ({
            ok: true,
            data: {
              project_id: "p1",
              nodes: [
                { task_id: "t_current", title: "current", status: "running" },
                { task_id: "t_parent", title: "parent", status: "pending" }
              ]
            }
          })
        } as Response;
      }
      if (url.endsWith("/api/v1/tasks/t_current/pane")) {
        return {
          json: async () => ({ ok: true, data: { task_id: "t_current", pane_uuid: "uuid-current", pane_id: "e2e:0.0", pane_target: "e2e:0.0" } })
        } as Response;
      }
      if (url.endsWith("/api/v1/tasks/t_parent/pane")) {
        return { json: async () => ({ ok: false, error: { code: "TASK_PANE_NOT_FOUND" } }) } as Response;
      }
      if (url.endsWith("/api/v1/tasks/t_parent/adopt-pane") && method === "POST") {
        return {
          json: async () => ({
            ok: true,
            data: {
              task_id: "t_adopted",
              title: "adopted",
              pane_uuid: "uuid-adopted",
              pane_id: "e2e:0.9",
              pane_target: "e2e:0.9",
              relation: "child"
            }
          })
        } as Response;
      }
      return { json: async () => ({ ok: false, error: { code: "NOT_FOUND" } }) } as Response;
    };

    const store = createShellmanStore(fakeFetch as typeof fetch, () => null as unknown as WebSocket);
    await store.load();
    expect(store.state.selectedTaskId).toBe("t_current");

    await store.adoptPaneAsChild("t_parent", "e2e:0.9", "adopted");

    expect(calls.some((c) => c.url.endsWith("/api/v1/tasks/t_parent/adopt-pane") && c.method === "POST")).toBe(true);
    const adoptedNode = store.state.treesByProject.p1.find((n) => n.taskId === "t_adopted");
    expect(adoptedNode?.parentTaskId).toBe("t_parent");
    expect(store.state.paneByTaskId.t_adopted?.paneTarget).toBe("e2e:0.9");
    expect(store.state.selectedTaskId).toBe("t_current");
  });

  it("updates task checked state via /api/v1/tasks/:id/check", async () => {
    const calls: Array<{ url: string; method: string; body?: string }> = [];
    const fakeFetch = async (input: RequestInfo | URL, init?: RequestInit) => {
      const url = String(input);
      const method = init?.method ?? "GET";
      calls.push({
        url,
        method,
        body: typeof init?.body === "string" ? init.body : undefined
      });
      if (url.endsWith("/api/v1/projects/active")) {
        return { json: async () => ({ ok: true, data: [{ project_id: "p1", repo_root: "/tmp/p1" }] }) } as Response;
      }
      if (url.endsWith("/api/v1/projects/p1/tree")) {
        return {
          json: async () => ({ ok: true, data: { project_id: "p1", nodes: [{ task_id: "t1", title: "root", status: "running" }] } })
        } as Response;
      }
      if (url.endsWith("/api/v1/tasks/t1/pane")) {
        return { json: async () => ({ ok: true, data: { task_id: "t1", pane_uuid: "uuid-t1", pane_id: "e2e:0.0", pane_target: "e2e:0.0" } }) } as Response;
      }
      if (url.endsWith("/api/v1/tasks/t1/check") && method === "PATCH") {
        return { json: async () => ({ ok: true, data: { task_id: "t1", checked: true } }) } as Response;
      }
      return { json: async () => ({ ok: false, error: { code: "NOT_FOUND" } }) } as Response;
    };

    const store = createShellmanStore(fakeFetch as typeof fetch, () => null as unknown as WebSocket);
    await store.load();
    await store.setTaskChecked("t1", true);

    const req = calls.find((c) => c.url.endsWith("/api/v1/tasks/t1/check") && c.method === "PATCH");
    expect(req).toBeTruthy();
    expect(req?.body).toContain(`"checked":true`);
    expect(store.state.treesByProject.p1[0]?.checked).toBe(true);
  });
});
