import { reactive } from "vue";

type TurnSocket = {
  addEventListener(type: string, listener: (event: any) => void): void;
  send(text: string): void;
  close(): void;
};

type CreateTurnStoreOptions = {
  wsFactory?: (url: string) => TurnSocket;
};

export type SessionStatus = "running" | "ready" | "unknown";

export type SessionItem = {
  target: string;
  title: string;
  status: SessionStatus;
  updatedAt: number;
};

export type TerminalFrame = {
  mode: "reset" | "append";
  data: string;
};

function titleFromTarget(target: string): string {
  const idx = target.indexOf(":");
  return idx > 0 ? target.slice(0, idx) : target;
}

function parseUnixSecond(value: unknown): number {
  if (typeof value === "number" && Number.isFinite(value)) {
    return Math.max(0, Math.floor(value));
  }
  if (typeof value === "string") {
    const trimmed = value.trim();
    if (!trimmed) {
      return 0;
    }
    const parsed = Number(trimmed);
    if (Number.isFinite(parsed)) {
      return Math.max(0, Math.floor(parsed));
    }
  }
  return 0;
}

export function createTurnStore(options: CreateTurnStoreOptions = {}) {
  const state = reactive({
    sessions: [] as string[],
    sessionItems: [] as SessionItem[],
    connected: false,
    selectedPane: "",
    terminalOutput: "",
    terminalFrame: { mode: "reset", data: "" } as TerminalFrame,
    terminalCursor: null as { x: number; y: number } | null
  });

  const wsFactory = options.wsFactory ?? ((url: string) => new WebSocket(url));
  let socket: TurnSocket | null = null;
  let reqID = 0;

  function nextID() {
    reqID += 1;
    return `req_${reqID}`;
  }

  function onMessage(text: string) {
    let msg: {
      type?: string;
      op?: string;
      payload?: {
        sessions?: string[];
        items?: Array<{
          target?: string;
          title?: string;
          status?: string;
          updated_at?: number | string;
        }>;
      };
    };

    try {
      msg = JSON.parse(text);
    } catch {
      return;
    }

    if (msg.type === "res" && msg.op === "tmux.list") {
      state.sessions = msg.payload?.sessions ?? [];
      state.sessionItems = state.sessions.map((target) => ({
        target,
        title: titleFromTarget(target),
        status: "unknown",
        updatedAt: 0
      }));
      if (!state.selectedPane && state.sessions.length > 0) {
        selectPane(state.sessions[0]);
      }
      return;
    }

    if (msg.type === "event" && msg.op === "tmux.status") {
      const incoming = msg.payload?.items ?? [];
      state.sessionItems = incoming
        .filter((item) => typeof item.target === "string" && item.target.length > 0)
        .map((item) => {
          const target = String(item.target ?? "");
          const status: SessionStatus =
            item.status === "running" || item.status === "ready" ? item.status : "unknown";
          return {
            target,
            title: String(item.title ?? titleFromTarget(target)),
            status,
            updatedAt: parseUnixSecond(item.updated_at)
          };
        });
      state.sessions = state.sessionItems.map((item) => item.target);
      if (!state.selectedPane && state.sessions.length > 0) {
        selectPane(state.sessions[0]);
      }
      return;
    }

    if (msg.type === "event" && msg.op === "term.output") {
      const payload = (msg as {
        payload?: {
          target?: string;
          mode?: "reset" | "append";
          data?: string;
          cursor?: { x?: number; y?: number };
        };
      }).payload;
      if (payload?.target && state.selectedPane && payload.target !== state.selectedPane) {
        return;
      }
      const mode = payload?.mode === "append" ? "append" : "reset";
      const text = payload?.data ?? "";
      state.terminalOutput = mode === "append" ? state.terminalOutput + text : text;
      state.terminalFrame = {
        mode,
        data: text
      };
      if (typeof payload?.cursor?.x === "number" && typeof payload?.cursor?.y === "number") {
        state.terminalCursor = { x: payload.cursor.x, y: payload.cursor.y };
      } else {
        state.terminalCursor = null;
      }
    }
  }

  function send(data: unknown) {
    if (!socket) {
      return;
    }
    socket.send(JSON.stringify(data));
  }

  function requestTmuxList() {
    send({
      id: nextID(),
      type: "req",
      op: "tmux.list",
      payload: { scope: "all" }
    });
  }

  function selectPane(target: string) {
    state.selectedPane = target;
    state.terminalOutput = "";
    state.terminalFrame = { mode: "reset", data: "" };
    state.terminalCursor = null;
    send({
      id: nextID(),
      type: "req",
      op: "tmux.select_pane",
      payload: { target }
    });
  }

  function sendTerminalInput(target: string, text: string) {
    send({
      id: nextID(),
      type: "event",
      op: "term.input",
      payload: { target, text }
    });
  }

  function sendTerminalResize(target: string, cols: number, rows: number) {
    send({
      id: nextID(),
      type: "event",
      op: "term.resize",
      payload: { target, cols, rows }
    });
  }

  function connect(wsURL: string) {
    socket = wsFactory(wsURL);
    socket.addEventListener("open", () => {
      state.connected = true;
      requestTmuxList();
    });
    socket.addEventListener("close", () => {
      state.connected = false;
    });
    socket.addEventListener("message", (event: { data: string }) => {
      onMessage(event.data);
    });
  }

  function disconnect() {
    socket?.close();
    socket = null;
    state.connected = false;
  }

  return {
    state,
    onMessage,
    connect,
    disconnect,
    requestTmuxList,
    selectPane,
    sendTerminalInput,
    sendTerminalResize
  };
}
