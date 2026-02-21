import { describe, expect, it } from "vitest";
import { createTurnStore } from "./turn";

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

describe("turn store", () => {
  it("updates sessions on tmux.list response", () => {
    const store = createTurnStore();
    store.onMessage(
      JSON.stringify({
        id: "1",
        type: "res",
        op: "tmux.list",
        payload: { sessions: ["s1"] }
      })
    );
    expect(store.state.sessions).toEqual(["s1"]);
  });

  it("applies tmux.status event to session items", () => {
    const store = createTurnStore();
    store.onMessage(
      JSON.stringify({
        id: "1",
        type: "res",
        op: "tmux.list",
        payload: { sessions: ["e2e:0.0"] }
      })
    );

    store.onMessage(
      JSON.stringify({
        id: "evt_1",
        type: "event",
        op: "tmux.status",
        payload: {
          items: [
            {
              target: "e2e:0.0",
              title: "e2e",
              status: "running",
              updated_at: 1771315200
            }
          ]
        }
      })
    );

    expect(store.state.sessionItems).toEqual([
      { target: "e2e:0.0", title: "e2e", status: "running", updatedAt: 1771315200 }
    ]);
  });

  it("ignores malformed frames", () => {
    const store = createTurnStore();
    expect(() => store.onMessage("not-json")).not.toThrow();
    expect(store.state.sessions).toEqual([]);
  });

  it("connects websocket and requests tmux.list on open", () => {
    const sock = new FakeSocket();
    let usedURL = "";
    const store = createTurnStore({
      wsFactory: (url: string) => {
        usedURL = url;
        return sock as never;
      }
    });

    store.connect("ws://example/ws/client/u1");
    sock.emitOpen();

    expect(usedURL).toBe("ws://example/ws/client/u1");
    expect(sock.sent.length).toBe(1);
    const payload = JSON.parse(sock.sent[0]) as { op: string; type: string };
    expect(payload.op).toBe("tmux.list");
    expect(payload.type).toBe("req");
  });

  it("sends term.input and updates output on term.output event", () => {
    const sock = new FakeSocket();
    const store = createTurnStore({
      wsFactory: () => sock as never
    });

    store.connect("ws://127.0.0.1/ws/client/u1");
    sock.emitOpen();

    store.sendTerminalInput("e2e:0.0", "echo hi\n");
    const sent = JSON.parse(sock.sent.at(-1) as string);
    expect(sent.op).toBe("term.input");

    store.onMessage(
      JSON.stringify({
        id: "evt_1",
        type: "event",
        op: "term.output",
        payload: { target: "e2e:0.0", mode: "reset", data: "__TT_E2E_OK__\n", cursor: { x: 3, y: 2 } }
      })
    );

    expect(store.state.terminalOutput).toContain("__TT_E2E_OK__");
    expect(store.state.terminalFrame).toEqual({ mode: "reset", data: "__TT_E2E_OK__\n" });
    expect(store.state.terminalCursor).toEqual({ x: 3, y: 2 });
  });

  it("sends term.resize event", () => {
    const sock = new FakeSocket();
    const store = createTurnStore({
      wsFactory: () => sock as never
    });
    store.connect("ws://127.0.0.1/ws/client/u1");
    sock.emitOpen();

    store.sendTerminalResize("e2e:0.0", 120, 40);
    const sent = JSON.parse(sock.sent.at(-1) as string);
    expect(sent.op).toBe("term.resize");
    expect(sent.payload).toEqual({ target: "e2e:0.0", cols: 120, rows: 40 });
  });

  it("supports stream frames with payload.data and mode append", () => {
    const store = createTurnStore();
    store.state.selectedPane = "e2e:0.0";

    store.onMessage(
      JSON.stringify({
        id: "evt_1",
        type: "event",
        op: "term.output",
        payload: { target: "e2e:0.0", mode: "reset", data: "bash-5.3$ " }
      })
    );
    expect(store.state.terminalOutput).toBe("bash-5.3$ ");

    store.onMessage(
      JSON.stringify({
        id: "evt_2",
        type: "event",
        op: "term.output",
        payload: { target: "e2e:0.0", mode: "append", data: "ls\n" }
      })
    );
    expect(store.state.terminalOutput).toBe("bash-5.3$ ls\n");
    expect(store.state.terminalFrame).toEqual({ mode: "append", data: "ls\n" });
  });

  it("ignores legacy payload.text format", () => {
    const store = createTurnStore();
    store.state.selectedPane = "e2e:0.0";

    store.onMessage(
      JSON.stringify({
        id: "evt_legacy",
        type: "event",
        op: "term.output",
        payload: { target: "e2e:0.0", text: "legacy-only" }
      })
    );

    expect(store.state.terminalOutput).toBe("");
  });
});
