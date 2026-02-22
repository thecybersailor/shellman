import { reactive } from "vue";
import { resolveAPIContext, type APIContext } from "@/lib/api_context";

export interface ActiveProject {
  projectId: string;
  repoRoot: string;
}

export interface TaskNode {
  taskId: string;
  parentTaskId?: string;
  title: string;
  currentCommand?: string;
  description?: string;
  flag?: "success" | "notify" | "error";
  flagDesc?: string;
  flagReaded?: boolean;
  checked?: boolean;
  archived?: boolean;
  status: string;
  updatedAt?: number;
}

export interface PaneBinding {
  paneUuid: string;
  paneId: string;
  paneTarget: string;
  relation?: "sibling" | "child" | "root";
}

export interface TaskNote {
  task_id: string;
  created_at: number;
  flag?: "success" | "notify" | "error";
  notes: string;
}

export interface TaskMessage {
  id: number;
  task_id: string;
  role: "user" | "assistant";
  content: string;
  status: "running" | "completed" | "error";
  error_text?: string;
  created_at: number;
  updated_at: number;
}

export interface TerminalFrame {
  mode: "reset" | "append";
  data: string;
}

export type PaneRuntimeStatus = "running" | "ready" | "unknown";
export type TermFrameKind = "reset" | "append_delta" | "append_ansi_repaint" | "append_empty";

const ANSI_REPAINT_PREFIX = "\u001b[0m\u001b[H\u001b[2J";
const TERM_FRAME_PROFILE_WINDOW_MS = 1000;
const TMUX_PANE_NOT_FOUND_CODE = "TMUX_PANE_NOT_FOUND";
const TERMINAL_CACHE_MAX_LINES = 2000;
const ACTIVE_PANE_CACHE_LIMIT = 5;
const GAP_RECOVER_HISTORY_LINES = 4000;

export function classifyTermFrame(mode: TerminalFrame["mode"], data: string): TermFrameKind {
  if (mode === "reset") {
    return "reset";
  }
  if (!data) {
    return "append_empty";
  }
  if (data.startsWith(ANSI_REPAINT_PREFIX)) {
    return "append_ansi_repaint";
  }
  return "append_delta";
}

function isTermFrameProfileEnabled() {
  const globalRef = globalThis as typeof globalThis & { __SHELLMAN_TERM_PROFILE__?: boolean };
  return globalRef.__SHELLMAN_TERM_PROFILE__ === true;
}

type LaunchProgram = "shell" | "codex" | "claude" | "cursor";
type HelperProgram = "codex" | "claude" | "cursor";
export type SidecarMode = "advisor" | "observer" | "autopilot";

interface TerminalCacheEntry {
  output: string;
  frame: TerminalFrame;
  cursor: { x: number; y: number } | null;
  source: "live" | "persisted" | "ended";
}

interface PaneMeta {
  title: string;
  currentCommand: string;
}

interface PaneSnapshotPayload {
  output?: string;
  frame?: { mode?: string; data?: string };
  cursor?: { x?: number; y?: number } | null;
}

type TmuxPaneItem = {
  target: string;
  title: string;
  currentCommand: string;
  status: PaneRuntimeStatus;
  updatedAt: number;
};

export interface DirectoryItem {
  name: string;
  path: string;
  is_dir: boolean;
}

export interface DirectoryListResult {
  path: string;
  items: DirectoryItem[];
}

export interface DirectoryHistoryItem {
  path: string;
  first_accessed_at: number;
  last_accessed_at: number;
  access_count: number;
}

interface GlobalConfig {
  local_port: number;
  defaults?: {
    session_program?: string;
    helper_program?: string;
  };
  helper_openai?: {
    endpoint?: string;
    model?: string;
    api_key?: string;
    api_key_set?: boolean;
  };
  task_completion_mode?: string;
  task_completion_command?: string;
  task_completion_idle_duration_seconds?: number;
}

interface AppProgramsConfig {
  version?: number;
  providers?: Array<{
    id?: string;
    display_name?: string;
    command?: string;
  }>;
}

type TaskCompletionMode = "none" | "command";

interface ReopenPaneOptions {
  program?: LaunchProgram;
  prompt?: string;
}

interface LoadOptions {
  preferredTaskId?: string;
  prefetchAllTaskPanes?: boolean;
}

interface APIResponse<T> {
  ok: boolean;
  data: T;
  error?: { code?: string; message?: string };
}

interface ImageUploadResponse {
  path?: string;
  size?: number;
  mime?: string;
}

interface ProtocolMessage {
  id?: string;
  type?: string;
  op?: string;
  payload?: Record<string, any>;
  error?: { code?: string; message?: string };
}

export function createShellmanStore(
  fetchImpl: typeof fetch = fetch,
  wsFactory: (url: string) => WebSocket = (url) => new WebSocket(url),
  context?: APIContext
) {
  type TermFrameProfileStats = {
    startedAtMs: number;
    total: number;
    reset: number;
    appendDelta: number;
    appendAnsiRepaint: number;
    appendEmpty: number;
    bytes: number;
    maxDataLen: number;
    targets: Record<string, true>;
  };

  function createTermFrameProfileStats(startedAtMs: number): TermFrameProfileStats {
    return {
      startedAtMs,
      total: 0,
      reset: 0,
      appendDelta: 0,
      appendAnsiRepaint: 0,
      appendEmpty: 0,
      bytes: 0,
      maxDataLen: 0,
      targets: {}
    };
  }

  let termFrameProfileStats = createTermFrameProfileStats(0);

  function trackTermFrameProfile(target: string, mode: TerminalFrame["mode"], data: string) {
    if (!isTermFrameProfileEnabled()) {
      return;
    }
    const nowMs = Date.now();
    if (!termFrameProfileStats.startedAtMs) {
      termFrameProfileStats = createTermFrameProfileStats(nowMs);
    }
    const dataLen = data.length;
    const frameKind = classifyTermFrame(mode, data);
    termFrameProfileStats.total += 1;
    termFrameProfileStats.bytes += dataLen;
    termFrameProfileStats.maxDataLen = Math.max(termFrameProfileStats.maxDataLen, dataLen);
    termFrameProfileStats.targets[target || "(empty-target)"] = true;
    if (frameKind === "reset") {
      termFrameProfileStats.reset += 1;
    } else if (frameKind === "append_ansi_repaint") {
      termFrameProfileStats.appendAnsiRepaint += 1;
    } else if (frameKind === "append_empty") {
      termFrameProfileStats.appendEmpty += 1;
    } else {
      termFrameProfileStats.appendDelta += 1;
    }

    if (nowMs - termFrameProfileStats.startedAtMs < TERM_FRAME_PROFILE_WINDOW_MS) {
      return;
    }

    const windowMs = Math.max(1, nowMs - termFrameProfileStats.startedAtMs);
    const record = {
      ts: new Date(nowMs).toISOString(),
      event: "shellman.term.output.profile",
      windowMs,
      frameRate: Number(((termFrameProfileStats.total * 1000) / windowMs).toFixed(2)),
      total: termFrameProfileStats.total,
      reset: termFrameProfileStats.reset,
      appendDelta: termFrameProfileStats.appendDelta,
      appendAnsiRepaint: termFrameProfileStats.appendAnsiRepaint,
      appendEmpty: termFrameProfileStats.appendEmpty,
      bytes: termFrameProfileStats.bytes,
      maxDataLen: termFrameProfileStats.maxDataLen,
      targets: Object.keys(termFrameProfileStats.targets).slice(0, 6),
      lastFrameKind: frameKind,
      lastFrameDataLen: dataLen
    };
    try {
      console.info(JSON.stringify(record));
    } catch {
      console.info(record);
    }
    termFrameProfileStats = createTermFrameProfileStats(nowMs);
  }

  let termInputSeq = 0;
  let termOutputSeq = 0;
  const pendingTermInputByReqID: Record<
    string,
    {
      inputSeq: number;
      target: string;
      textLen: number;
      sentAtMs: number;
    }
  > = {};
  let lastTermInputAckAtMs = 0;
  const ctx =
    context ??
    resolveAPIContext(
      typeof window !== "undefined" ? window.location.href : "http://127.0.0.1:4621/"
    );

  function apiURL(path: string) {
    return `${ctx.baseOrigin}${path}`;
  }

  function apiHeaders(extra: HeadersInit = {}) {
    const out: Record<string, string> = {};
    if (Array.isArray(extra)) {
      for (const [k, v] of extra) out[k] = String(v);
    } else if (extra instanceof Headers) {
      extra.forEach((v, k) => {
        out[k] = v;
      });
    } else {
      Object.assign(out, extra as Record<string, string>);
    }
    if (ctx.turnUUID) {
      out["X-Shellman-Turn-UUID"] = ctx.turnUUID;
    }
    return out;
  }

  function escapeForLog(text: string, max = 240) {
    const escaped = text
      .replace(/\\/g, "\\\\")
      .replace(/\r/g, "\\r")
      .replace(/\n/g, "\\n")
      .replace(/\t/g, "\\t")
      .replace(/\u001b/g, "\\u001b");
    return escaped.length > max ? `${escaped.slice(0, max)}...(truncated:${escaped.length})` : escaped;
  }

  function logInfo(_event: string, _payload: Record<string, unknown> = {}) {
    // Logging disabled
  }

  logInfo("shellman.store.created");

  function normalizeLaunchProgram(value: unknown): LaunchProgram {
    const raw = String(value ?? "").trim().toLowerCase();
    if (raw === "codex" || raw === "claude" || raw === "cursor") {
      return raw;
    }
    return "shell";
  }

  function normalizeHelperProgram(value: unknown): HelperProgram {
    const raw = String(value ?? "").trim().toLowerCase();
    if (raw === "codex" || raw === "claude" || raw === "cursor") {
      return raw;
    }
    return "codex";
  }

  function normalizeAppPrograms(value: unknown): Array<{ id: HelperProgram; display_name: string; command: string }> {
    const rawList = Array.isArray(value) ? value : [];
    const seen = new Set<string>();
    const out: Array<{ id: HelperProgram; display_name: string; command: string }> = [];
    for (const rawItem of rawList) {
      if (!rawItem || typeof rawItem !== "object") {
        continue;
      }
      const item = rawItem as Record<string, unknown>;
      const idRaw = String(item.id ?? "").trim().toLowerCase();
      if (idRaw !== "codex" && idRaw !== "claude" && idRaw !== "cursor") {
        continue;
      }
      if (seen.has(idRaw)) {
        continue;
      }
      seen.add(idRaw);
      out.push({
        id: idRaw,
        display_name: String(item.display_name ?? "").trim() || idRaw,
        command: String(item.command ?? "").trim() || idRaw
      });
    }
    if (out.length > 0) {
      return out;
    }
    return [
      { id: "codex", display_name: "codex", command: "codex" },
      { id: "claude", display_name: "Claude", command: "claude" },
      { id: "cursor", display_name: "Cursor", command: "cursor" }
    ];
  }

  function normalizeTaskCompletionMode(value: unknown): TaskCompletionMode {
    const raw = String(value ?? "").trim().toLowerCase();
    switch (raw) {
      case "command":
        return "command";
      case "none":
        return "none";
    }
    return "none";
  }

  function shellQuote(value: string): string {
    if (!value) {
      return "''";
    }
    return `'${value.replace(/'/g, `'\\''`)}'`;
  }

  function deriveTitleFromPrompt(prompt: string): string {
    const compact = prompt
      .replace(/\r/g, "\n")
      .split("\n")
      .map((line) => line.trim())
      .filter(Boolean)
      .join(" ")
      .replace(/\s+/g, " ")
      .trim();
    if (!compact) {
      return "";
    }
    const max = 48;
    if (compact.length <= max) {
      return compact;
    }
    return `${compact.slice(0, max - 3).trimEnd()}...`;
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

  const state = reactive({
    projects: [] as ActiveProject[],
    treesByProject: {} as Record<string, TaskNode[]>,
    paneByTaskId: {} as Record<string, PaneBinding>,
    taskMessagesByTaskId: {} as Record<string, TaskMessage[]>,
    taskSidecarModeByTaskId: {} as Record<string, SidecarMode>,
    selectedTaskId: "",
    selectedPaneUuid: "",
    selectedPaneTarget: "",
    terminalOutput: "",
    terminalFrame: { mode: "reset", data: "" } as TerminalFrame,
    terminalCursor: null as { x: number; y: number } | null,
    terminalEnded: false,
    terminalByPaneUuid: {} as Record<string, TerminalCacheEntry>,
    paneMetaByTarget: {} as Record<string, PaneMeta>,
    taskRuntimeByTaskId: {} as Record<string, { status: PaneRuntimeStatus; updatedAt: number }>,
    tmuxPaneItems: [] as TmuxPaneItem[],
    paneLookupComplete: false,
    wsConnected: false,
    localPort: 4621,
    defaultLaunchProgram: "shell" as LaunchProgram,
    defaultHelperProgram: "codex" as HelperProgram,
    helperOpenAIEndpoint: "",
    helperOpenAIModel: "",
    helperOpenAIApiKey: "",
    appPrograms: [] as Array<{ id: HelperProgram; display_name: string; command: string }>,
    taskCompletionMode: "none" as TaskCompletionMode,
    taskCompletionCommand: "",
    taskCompletionIdleDuration: 0
  });

  let ws: WebSocket | null = null;
  let reqID = 0;
  let wsConnectionSeq = 0;
  let configLoadSeq = 0;
  let lastSelectSent: { target: string; connectionSeq: number } | null = null;
  const paneLookupStatus: Record<string, "bound" | "missing"> = {};
  const selectReqPaneUuidByReqId: Record<string, string> = {};
  const endedPaneUuids: Record<string, true> = {};
  const refreshTimerByTaskID: Record<string, ReturnType<typeof setTimeout> | undefined> = {};
  const messagesRefreshTimerByTaskID: Record<string, ReturnType<typeof setTimeout> | undefined> = {};
  const messagesHydratedByTaskID: Record<string, true> = {};
  const persistedSnapshotByPaneUuid: Record<string, TerminalCacheEntry> = {};
  const paneGapByPaneUuid: Record<string, true> = {};
  const activePaneOrderByUuid: string[] = [];
  let latestTerminalSize: { cols: number; rows: number } | null = null;

  function nextID() {
    reqID += 1;
    return `req_${reqID}`;
  }

  function nextRequestID() {
    if (typeof crypto !== "undefined" && typeof crypto.randomUUID === "function") {
      return `req_${crypto.randomUUID().replace(/-/g, "")}`;
    }
    return `req_${Date.now()}_${Math.random().toString(16).slice(2, 10)}`;
  }

  function toTerminalCacheEntry(snapshot: PaneSnapshotPayload | null | undefined): TerminalCacheEntry | null {
    if (!snapshot || typeof snapshot !== "object") {
      return null;
    }
    const output = trimToRecentLines(String(snapshot.output ?? ""), TERMINAL_CACHE_MAX_LINES);
    const frameMode = snapshot.frame?.mode === "append" ? "append" : "reset";
    const rawFrameData = String(snapshot.frame?.data ?? output);
    const frameData = frameMode === "reset" ? trimToRecentLines(rawFrameData, TERMINAL_CACHE_MAX_LINES) : rawFrameData;
    const cursorRaw = snapshot.cursor;
    const cursor =
      cursorRaw && typeof cursorRaw.x === "number" && typeof cursorRaw.y === "number"
        ? { x: cursorRaw.x, y: cursorRaw.y }
        : null;
    return {
      output,
      frame: { mode: frameMode, data: frameData },
      cursor,
      source: "persisted"
    };
  }

  function isSyntheticMissingPaneTarget(target: string): boolean {
    return String(target ?? "").trim().toLowerCase().startsWith("missing:");
  }

  function toTaskNodes(
    nodes: Array<{
      task_id: string;
      parent_task_id?: string;
      title: string;
      current_command?: string;
      description?: string;
      flag?: "success" | "notify" | "error";
      flag_desc?: string;
      flag_readed?: boolean;
      checked?: boolean;
      archived?: boolean;
      status: string;
      updated_at?: number | string;
    }> | undefined
  ): TaskNode[] {
    return (nodes ?? []).map((n) => ({
      taskId: n.task_id,
      parentTaskId: n.parent_task_id,
      title: n.title,
      currentCommand: String(n.current_command ?? ""),
      description: String(n.description ?? ""),
      flag: n.flag,
      flagDesc: String(n.flag_desc ?? ""),
      flagReaded: Boolean(n.flag_readed),
      checked: Boolean(n.checked),
      archived: Boolean(n.archived),
      status: n.status,
      updatedAt: parseUnixSecond(n.updated_at)
    }));
  }

  function applyRuntimeDelta(runtimePayload: Record<string, any>) {
    const paneUpdates = Array.isArray(runtimePayload?.panes) ? runtimePayload.panes : [];
    const taskUpdates = Array.isArray(runtimePayload?.tasks) ? runtimePayload.tasks : [];

    const paneMetaByTarget: Record<string, PaneMeta> = { ...state.paneMetaByTarget };
    const byTarget: Record<string, TmuxPaneItem> = {};
    for (const item of state.tmuxPaneItems) {
      if (!item?.target) {
        continue;
      }
      byTarget[item.target] = item;
    }

    for (const raw of paneUpdates) {
      const pane = raw as Record<string, unknown>;
      const target = String(pane.pane_target ?? pane.pane_id ?? "").trim();
      if (!target) {
        continue;
      }
      const runtimeStatusRaw = String(pane.runtime_status ?? "");
      const status: PaneRuntimeStatus =
        runtimeStatusRaw === "running" || runtimeStatusRaw === "ready" ? runtimeStatusRaw : "unknown";
      const currentCommand = String(pane.current_command ?? paneMetaByTarget[target]?.currentCommand ?? "");
      const title = String(paneMetaByTarget[target]?.title ?? byTarget[target]?.title ?? "");
      const updatedAt = parseUnixSecond(pane.updated_at);

      byTarget[target] = {
        target,
        title,
        currentCommand,
        status,
        updatedAt
      };
      paneMetaByTarget[target] = {
        title,
        currentCommand
      };
    }

    state.tmuxPaneItems = Object.values(byTarget);
    state.paneMetaByTarget = paneMetaByTarget;

    const runtimeByTaskId: Record<string, { status: PaneRuntimeStatus; updatedAt: number }> = {
      ...state.taskRuntimeByTaskId
    };
    const setTaskCurrentCommand = (taskId: string, currentCommand: string) => {
      const normalized = String(currentCommand ?? "").trim();
      for (const [projectId, nodes] of Object.entries(state.treesByProject)) {
        const idx = nodes.findIndex((it) => it.taskId === taskId);
        if (idx < 0) {
          continue;
        }
        if (String(nodes[idx]?.currentCommand ?? "").trim() === normalized) {
          return;
        }
        const next = [...nodes];
        next[idx] = {
          ...next[idx],
          currentCommand: normalized
        };
        state.treesByProject[projectId] = next;
        return;
      }
    };

    if (taskUpdates.length > 0) {
      for (const raw of taskUpdates) {
        const task = raw as Record<string, unknown>;
        const taskId = String(task.task_id ?? "").trim();
        if (!taskId) {
          continue;
        }
        const runtimeStatusRaw = String(task.runtime_status ?? "");
        const status: PaneRuntimeStatus =
          runtimeStatusRaw === "running" || runtimeStatusRaw === "ready" ? runtimeStatusRaw : "unknown";
        runtimeByTaskId[taskId] = {
          status,
          updatedAt: parseUnixSecond(task.updated_at)
        };
        setTaskCurrentCommand(taskId, String(task.current_command ?? ""));
      }
    } else {
      for (const [taskId, pane] of Object.entries(state.paneByTaskId)) {
        const target = String(pane?.paneTarget ?? "").trim();
        if (!target) {
          continue;
        }
        const runtime = byTarget[target];
        if (!runtime) {
          continue;
        }
        runtimeByTaskId[taskId] = {
          status: runtime.status,
          updatedAt: runtime.updatedAt
        };
        setTaskCurrentCommand(taskId, runtime.currentCommand);
      }
    }

    state.taskRuntimeByTaskId = runtimeByTaskId;
  }

  function applyTreeDelta(treePayload: Record<string, any> | Array<Record<string, any>>) {
    const deltas = Array.isArray(treePayload) ? treePayload : [treePayload];
    let treeChanged = false;
    for (const raw of deltas) {
      const delta = raw as Record<string, any>;
      const projectId = String(delta.project_id ?? "").trim();
      if (!projectId) {
        continue;
      }
      let nodes = [...(state.treesByProject[projectId] ?? [])];

      const removed = new Set(
        (Array.isArray(delta.removed) ? delta.removed : []).map((id: unknown) => String(id ?? "").trim()).filter(Boolean)
      );
      if (removed.size > 0) {
        nodes = nodes.filter((node) => !removed.has(node.taskId));
      }

      for (const node of toTaskNodes(Array.isArray(delta.added) ? delta.added : [])) {
        const idx = nodes.findIndex((it) => it.taskId === node.taskId);
        if (idx >= 0) {
          nodes[idx] = { ...nodes[idx], ...node };
        } else {
          nodes.push(node);
        }
      }

      for (const node of toTaskNodes(Array.isArray(delta.updated) ? delta.updated : [])) {
        const idx = nodes.findIndex((it) => it.taskId === node.taskId);
        if (idx >= 0) {
          nodes[idx] = { ...nodes[idx], ...node };
        } else {
          nodes.push(node);
        }
      }

      const reparented = Array.isArray(delta.reparented) ? delta.reparented : [];
      for (const rawReparent of reparented) {
        const reparent = rawReparent as Record<string, unknown>;
        const taskId = String(reparent.task_id ?? "").trim();
        if (!taskId) {
          continue;
        }
        const idx = nodes.findIndex((it) => it.taskId === taskId);
        if (idx < 0) {
          continue;
        }
        nodes[idx] = {
          ...nodes[idx],
          parentTaskId: String(reparent.new_parent_task_id ?? "").trim() || undefined
        };
      }

      state.treesByProject[projectId] = nodes;
      treeChanged = true;
    }

    if (treeChanged) {
      recomputePaneLookupComplete();
    }
  }

  function recomputePaneLookupComplete() {
    const taskIDs = new Set<string>();
    for (const nodes of Object.values(state.treesByProject)) {
      for (const node of nodes) {
        taskIDs.add(node.taskId);
      }
    }
    if (taskIDs.size === 0) {
      state.paneLookupComplete = true;
      return;
    }
    for (const taskID of taskIDs) {
      if (!paneLookupStatus[taskID]) {
        state.paneLookupComplete = false;
        return;
      }
    }
    state.paneLookupComplete = true;
  }

  async function load(options: LoadOptions = {}) {
    const prefetchAllTaskPanes = options.prefetchAllTaskPanes !== false;
    const preferredTaskID = String(options.preferredTaskId ?? "").trim();
    const taskIDsInTree = new Set<string>();
    logInfo("shellman.load.start");
    const projectsRes = (await fetchImpl(apiURL("/api/v1/projects/active"), {
      headers: apiHeaders()
    }).then((r) => r.json())) as APIResponse<
      Array<{ project_id: string; repo_root: string }>
    >;
    logInfo("shellman.load.projects", {
      ok: projectsRes.ok,
      count: projectsRes.data?.length ?? 0
    });
    state.projects = projectsRes.data.map((p) => ({ projectId: p.project_id, repoRoot: p.repo_root }));

    for (const project of state.projects) {
      logInfo("shellman.load.project_tree.start", { projectId: project.projectId });
      const treeRes = (await fetchImpl(apiURL(`/api/v1/projects/${project.projectId}/tree`), {
        headers: apiHeaders()
      }).then((r) => r.json())) as APIResponse<{
        project_id: string;
        nodes: Array<{
          task_id: string;
          parent_task_id?: string;
          title: string;
          description?: string;
          flag?: "success" | "notify" | "error";
          flag_desc?: string;
          checked?: boolean;
          status: string;
          updated_at?: number | string;
        }>;
      }>;
      logInfo("shellman.load.project_tree.done", {
        projectId: project.projectId,
        ok: treeRes.ok,
        nodeCount: treeRes.data?.nodes?.length ?? 0
      });
      const nodes = toTaskNodes(treeRes.data.nodes);
      state.treesByProject[project.projectId] = nodes;
      for (const node of nodes) {
        taskIDsInTree.add(node.taskId);
      }
      if (prefetchAllTaskPanes) {
        for (const node of nodes) {
          await loadTaskPane(node.taskId);
        }
      }
    }

    if (!state.selectedTaskId) {
      if (preferredTaskID && taskIDsInTree.has(preferredTaskID)) {
        state.selectedTaskId = preferredTaskID;
      } else {
        const firstWithPane = Object.keys(state.paneByTaskId)[0] ?? "";
        if (firstWithPane) {
          state.selectedTaskId = firstWithPane;
        } else {
          const firstProject = state.projects[0]?.projectId;
          const firstTask = firstProject ? state.treesByProject[firstProject]?.[0] : undefined;
          state.selectedTaskId = firstTask?.taskId ?? "";
        }
      }
    }
    if (!prefetchAllTaskPanes && state.selectedTaskId) {
      await loadTaskPane(state.selectedTaskId);
    }
    recomputePaneLookupComplete();
    logInfo("shellman.load.done", {
      selectedTaskId: state.selectedTaskId,
      projectCount: state.projects.length
    });
  }

  async function refreshTaskFromServer(taskId: string) {
    if (!taskId) {
      return;
    }
    const projectID = findProjectIDByTaskStrict(taskId);
    if (!projectID) {
      return;
    }
    try {
      const treeRes = (await fetchImpl(apiURL(`/api/v1/projects/${projectID}/tree`), {
        headers: apiHeaders()
      }).then((r) => r.json())) as APIResponse<{
        project_id: string;
        nodes: Array<{
          task_id: string;
          parent_task_id?: string;
          title: string;
          description?: string;
          flag?: "success" | "notify" | "error";
          flag_desc?: string;
          checked?: boolean;
          status: string;
          updated_at?: number | string;
        }>;
      }>;
      if (treeRes.ok) {
        state.treesByProject[projectID] = toTaskNodes(treeRes.data.nodes);
        recomputePaneLookupComplete();
      }
    } catch {
      logInfo("shellman.task.refresh.tree.error", { taskId, projectID });
    }
    await loadTaskPane(taskId);
    logInfo("shellman.task.refresh.done", { taskId, projectID });
  }

  function scheduleRefreshTaskFromServer(taskId: string) {
    if (!taskId) {
      return;
    }
    const timer = refreshTimerByTaskID[taskId];
    if (timer) {
      clearTimeout(timer);
    }
    refreshTimerByTaskID[taskId] = setTimeout(() => {
      delete refreshTimerByTaskID[taskId];
      void refreshTaskFromServer(taskId);
    }, 80);
  }

  function scheduleRefreshTaskMessages(taskId: string) {
    const nextTaskID = String(taskId ?? "").trim();
    if (!nextTaskID) {
      return;
    }
    const timer = messagesRefreshTimerByTaskID[nextTaskID];
    if (timer) {
      clearTimeout(timer);
    }
    messagesRefreshTimerByTaskID[nextTaskID] = setTimeout(() => {
      delete messagesRefreshTimerByTaskID[nextTaskID];
      void loadTaskMessages(nextTaskID, true);
    }, 80);
  }

  async function loadTaskPane(taskId: string) {
    if (paneLookupStatus[taskId] === "missing") {
      logInfo("shellman.http.pane_lookup.skip", { taskId, reason: "cached-missing" });
      recomputePaneLookupComplete();
      return;
    }
    logInfo("shellman.http.pane_lookup.start", { taskId });
    try {
      const paneRes = (await fetchImpl(apiURL(`/api/v1/tasks/${taskId}/pane`), {
        headers: apiHeaders()
      }).then((r) => r.json())) as APIResponse<{
        task_id: string;
        pane_uuid?: string;
        pane_id: string;
        pane_target?: string;
        snapshot?: PaneSnapshotPayload | null;
      }>;
      if (!paneRes.ok) {
        paneLookupStatus[taskId] = "missing";
        console.warn("[shellman] pane binding missing", {
          taskId,
          code: String(paneRes.error?.code ?? "UNKNOWN"),
          message: String(paneRes.error?.message ?? "")
        });
        logInfo("shellman.http.pane_lookup.missing", {
          taskId,
          code: String(paneRes.error?.code ?? "UNKNOWN"),
          message: String(paneRes.error?.message ?? "")
        });

        return;
      }
      const paneID = String(paneRes.data.pane_id ?? "");
      if (!paneID) {
        paneLookupStatus[taskId] = "missing";
        logInfo("shellman.http.pane_lookup.empty", { taskId });

        return;
      }
      state.paneByTaskId[taskId] = {
        paneUuid: String(paneRes.data.pane_uuid ?? paneID),
        paneId: paneID,
        paneTarget: String(paneRes.data.pane_target ?? paneID)
      };
      const paneUuid = state.paneByTaskId[taskId]?.paneUuid ?? "";
      const snapshotCache = toTerminalCacheEntry(paneRes.data.snapshot);
      if (paneUuid && snapshotCache) {
        persistedSnapshotByPaneUuid[paneUuid] = snapshotCache;
      }
      paneLookupStatus[taskId] = "bound";
      logInfo("shellman.http.pane_lookup.bound", {
        taskId,
        paneId: paneID,
        paneTarget: state.paneByTaskId[taskId]?.paneTarget ?? "",
        hasSnapshot: Boolean(snapshotCache)
      });
    } catch {
      paneLookupStatus[taskId] = "missing";
      logInfo("shellman.http.pane_lookup.error", { taskId });
    }
    recomputePaneLookupComplete();
  }

  async function loadTaskMessages(taskId: string, force = false) {
    const nextTaskID = String(taskId ?? "").trim();
    if (!nextTaskID) {
      return;
    }
    if (!force && messagesHydratedByTaskID[nextTaskID]) {
      return;
    }
    try {
      const res = (await fetchImpl(apiURL(`/api/v1/tasks/${nextTaskID}/messages`), {
        headers: apiHeaders()
      }).then((r) => r.json())) as APIResponse<{ task_id: string; messages: TaskMessage[] }>;
      if (!res.ok) {
        state.taskMessagesByTaskId[nextTaskID] = [];
        messagesHydratedByTaskID[nextTaskID] = true;
        return;
      }
      state.taskMessagesByTaskId[nextTaskID] = (res.data?.messages ?? []).map((item) => ({
        id: Number(item.id ?? 0),
        task_id: String(item.task_id ?? nextTaskID),
        role: item.role === "assistant" ? "assistant" : "user",
        content: String(item.content ?? ""),
        status: item.status === "running" || item.status === "error" ? item.status : "completed",
        error_text: String(item.error_text ?? ""),
        created_at: Number(item.created_at ?? 0),
        updated_at: Number(item.updated_at ?? 0)
      }));
      messagesHydratedByTaskID[nextTaskID] = true;
    } catch {
      state.taskMessagesByTaskId[nextTaskID] = [];
      messagesHydratedByTaskID[nextTaskID] = true;
    }
  }

  function applyProtocolMessage(msg: ProtocolMessage) {
    logInfo("shellman.ws.recv.message", {
      id: String(msg.id ?? ""),
      type: String(msg.type ?? ""),
      op: String(msg.op ?? "")
    });
    if (msg.type === "res" && msg.op === "term.input") {
      const reqID = String(msg.id ?? "");
      const pending = reqID ? pendingTermInputByReqID[reqID] : undefined;
      if (reqID && pending) {
        delete pendingTermInputByReqID[reqID];
      }
      const nowMs = Date.now();
      const hasError = Boolean(msg.error);
      if (pending) {
        if (!hasError) {
          lastTermInputAckAtMs = nowMs;
        }
        logInfo("shellman.term.input.ack", {
          reqID,
          inputSeq: pending.inputSeq,
          target: pending.target,
          textLen: pending.textLen,
          latencyMs: Math.max(0, nowMs - pending.sentAtMs),
          hasError,
          code: String(msg.error?.code ?? ""),
          message: String(msg.error?.message ?? "")
        });
      } else {
        logInfo("shellman.term.input.ack", {
          reqID,
          inputSeq: null,
          hasError,
          code: String(msg.error?.code ?? ""),
          message: String(msg.error?.message ?? ""),
          reason: "pending-missing"
        });
      }
    }
    if (msg.type === "res" && msg.error) {
      const code = String(msg.error.code ?? "UNKNOWN");
      const message = String(msg.error.message ?? "");
      const reqID = String(msg.id ?? "");
      const reqPaneUuid = reqID ? selectReqPaneUuidByReqId[reqID] ?? "" : "";
      console.warn("[shellman] ws response error", {
        id: reqID,
        op: String(msg.op ?? ""),
        code,
        message
      });
      if (reqID && selectReqPaneUuidByReqId[reqID]) {
        delete selectReqPaneUuidByReqId[reqID];
      }
      logInfo("shellman.ws.recv.error", {
        id: reqID,
        op: String(msg.op ?? ""),
        code,
        message,
        paneUuid: reqPaneUuid
      });
      if (String(msg.op ?? "") === "tmux.select_pane" && reqPaneUuid) {
        const reqPaneTarget = findPaneTargetByPaneUUID(reqPaneUuid);
        if (code === TMUX_PANE_NOT_FOUND_CODE || isSyntheticMissingPaneTarget(reqPaneTarget)) {
          markPaneEnded(reqPaneUuid, code || TMUX_PANE_NOT_FOUND_CODE);
        } else {
          logInfo("shellman.tmux.select_pane.error.ignore", {
            paneUuid: reqPaneUuid,
            paneTarget: reqPaneTarget,
            code,
            message
          });
        }
      }

      return;
    }

    if (msg.type === "res" && msg.op === "tmux.list") {
      const payload = msg.payload ?? {};
      const sessions = Array.isArray(payload.sessions) ? payload.sessions : [];
      const byTarget: Record<string, TmuxPaneItem> = {};
      for (const item of state.tmuxPaneItems) {
        if (item.target) {
          byTarget[item.target] = item;
        }
      }
      const next: TmuxPaneItem[] = [];
      for (const raw of sessions) {
        const target = String(raw ?? "");
        if (!target || next.some((item) => item.target === target)) {
          continue;
        }
        const prev = byTarget[target];
        next.push({
          target,
          title: prev?.title ?? target,
          currentCommand: prev?.currentCommand ?? "",
          status: prev?.status ?? "unknown",
          updatedAt: prev?.updatedAt ?? 0
        });
      }
      state.tmuxPaneItems = next;
      logInfo("shellman.tmux.list.apply", { itemCount: next.length });
      return;
    }

    if (msg.type === "event" && msg.op === "term.output") {
      const payload = msg.payload ?? {};
      const target = String(payload.target ?? "");
      const selectedBinding = resolveSelectedPaneBinding();
      const selectedTarget = selectedBinding?.paneTarget ?? state.selectedPaneTarget;
      const selectedPaneUuid = selectedBinding?.paneUuid || state.selectedPaneUuid;
      const payloadPaneUUID = String(payload.pane_uuid ?? "");
      const incomingPaneUuid =
        payloadPaneUUID ||
        (selectedTarget && target && selectedTarget === target ? selectedPaneUuid : "") ||
        findPaneUUIDByTarget(target);
      const isSelectedPane = Boolean(
        (selectedPaneUuid && incomingPaneUuid && selectedPaneUuid === incomingPaneUuid) ||
          (!selectedPaneUuid && selectedTarget && target && selectedTarget === target)
      );
      const mode = payload.mode === "append" ? "append" : "reset";
      const text = String(payload.data ?? "");
      trackTermFrameProfile(target, mode, text);
      const outputSeq = ++termOutputSeq;
      const cachePaneUUID = incomingPaneUuid || findPaneUUIDByTarget(target);
      const prevCache = cachePaneUUID ? state.terminalByPaneUuid[cachePaneUUID] ?? persistedSnapshotByPaneUuid[cachePaneUUID] : null;
      const baseOutput = isSelectedPane ? state.terminalOutput : String(prevCache?.output ?? "");
      const beforeLen = baseOutput.length;
      const nowMs = Date.now();
      const nextOutputRaw = mode === "append" ? baseOutput + text : text;
      const nextOutput = trimToRecentLines(nextOutputRaw, TERMINAL_CACHE_MAX_LINES);
      let nextCursor: { x: number; y: number } | null = isSelectedPane ? state.terminalCursor : prevCache?.cursor ?? null;
      if (typeof payload.cursor?.x === "number" && typeof payload.cursor?.y === "number") {
        nextCursor = { x: payload.cursor.x, y: payload.cursor.y };
      } else if (mode === "reset") {
        nextCursor = null;
      }
      logInfo("shellman.term.output.apply", {
        outputSeq,
        target,
        mode,
        dataLen: text.length,
        dataEscaped: escapeForLog(text),
        selectedPaneTarget: selectedTarget,
        selectedPaneUuid,
        incomingPaneUuid,
        cacheOnly: !isSelectedPane,
        msSinceLastInputAck: lastTermInputAckAtMs > 0 ? Math.max(0, nowMs - lastTermInputAckAtMs) : null,
        terminalOutputLenBefore: beforeLen,
        terminalOutputLenAfter: nextOutput.length
      });
      if (isSelectedPane) {
        state.terminalOutput = nextOutput;
        state.terminalFrame = {
          mode,
          data: text
        };
        state.terminalEnded = false;
        state.terminalCursor = nextCursor;
        if (typeof payload.cursor?.x === "number" && typeof payload.cursor?.y === "number") {
          logInfo("shellman.term.cursor.update", {
            outputSeq,
            target,
            x: payload.cursor.x,
            y: payload.cursor.y,
            mode,
            hasData: text.length > 0
          });
        } else if (mode === "reset") {
          logInfo("shellman.term.cursor.clear", { target });
        } else {
          logInfo("shellman.term.cursor.keep", { target, mode, hasData: text.length > 0 });
        }
      }
      if (cachePaneUUID) {
        if (endedPaneUuids[cachePaneUUID]) {
          delete endedPaneUuids[cachePaneUUID];
        }
        const cacheEntry = {
          output: nextOutput,
          frame: { mode, data: text } as TerminalFrame,
          cursor: nextCursor,
          source: "live" as const
        };
        state.terminalByPaneUuid[cachePaneUUID] = cacheEntry;
      }
      return;
    }

    if (msg.type === "event" && msg.op === "task.messages.updated") {
      const payload = msg.payload ?? {};
      const taskID = String(payload.task_id ?? "");
      if (!taskID) {
        return;
      }
      scheduleRefreshTaskMessages(taskID);
      return;
    }

    if (msg.type === "event" && msg.op === "pane.ended") {
      const payload = msg.payload ?? {};
      const target = String(payload.target ?? "");
      const reasonKey = String(payload.reason_key ?? payload.code ?? payload.reason ?? TMUX_PANE_NOT_FOUND_CODE);
      const paneUuid = String(payload.pane_uuid ?? "") || findPaneUUIDByTarget(target);
      if (!paneUuid) {
        logInfo("shellman.pane.ended.ignore", { reason: "missing-pane-uuid", target });
        return;
      }
      markPaneEnded(paneUuid, reasonKey);
      const taskID = findTaskIDByPaneUUID(paneUuid);
      if (taskID) {
        scheduleRefreshTaskFromServer(taskID);
      }
      return;
    }

    if (msg.type === "event" && msg.op === "pane.created") {
      const payload = msg.payload ?? {};
      const taskID = String(payload.task_id ?? "");
      const paneID = String(payload.pane_id ?? "");
      const paneUUID = String(payload.pane_uuid ?? paneID);
      if (!taskID || !paneID) {
        logInfo("shellman.ws.pane_created.ignore", { reason: "missing-task-or-pane" });
        return;
      }
      state.paneByTaskId[taskID] = {
        paneUuid: paneUUID,
        paneId: paneID,
        paneTarget: String(payload.pane_target ?? paneID),
        relation: payload.relation
      };
      if (taskID === state.selectedTaskId) {
        state.selectedPaneUuid = paneUUID;
        state.selectedPaneTarget = String(payload.pane_target ?? paneID);
      }
      logInfo("shellman.ws.pane_created.apply", {
        taskId: taskID,
        paneUuid: paneUUID,
        paneId: paneID,
        paneTarget: state.paneByTaskId[taskID]?.paneTarget ?? "",
        relation: String(payload.relation ?? "")
      });
      return;
    }

    if (msg.type === "event" && msg.op === "tmux.status") {
      const payload = msg.payload ?? {};
      const mode = String(payload.mode ?? "").trim().toLowerCase();
      if (mode === "delta") {
        applyRuntimeDelta((payload.runtime ?? {}) as Record<string, any>);
        applyTreeDelta((payload.tree ?? {}) as Record<string, any>);
        return;
      }
      const items = Array.isArray(payload.items) ? payload.items : [];
      const byTarget: Record<string, TmuxPaneItem> = {};
      const paneMetaByTarget: Record<string, PaneMeta> = { ...state.paneMetaByTarget };
      for (const raw of items) {
        const item = raw as { target?: string; title?: string; current_command?: string; status?: string; updated_at?: number | string };
        const target = String(item.target ?? "");
        if (!target) {
          continue;
        }
        const status: PaneRuntimeStatus =
          item.status === "running" || item.status === "ready" ? item.status : "unknown";
        const title = String(item.title ?? "");
        const currentCommand = String(item.current_command ?? "");
        byTarget[target] = {
          target,
          title,
          currentCommand,
          status,
          updatedAt: parseUnixSecond(item.updated_at)
        };
        paneMetaByTarget[target] = {
          title,
          currentCommand
        };
      }

      const existingByTarget: Record<string, TmuxPaneItem> = {};
      for (const item of state.tmuxPaneItems) {
        if (!item.target) {
          continue;
        }
        existingByTarget[item.target] = item;
      }
      const mergedTargets: string[] = [];
      for (const target of Object.keys(byTarget)) {
        mergedTargets.push(target);
      }
      for (const target of Object.keys(existingByTarget)) {
        if (!byTarget[target]) {
          mergedTargets.push(target);
        }
      }
      state.tmuxPaneItems = mergedTargets.map((target) => byTarget[target] ?? existingByTarget[target]);

      const runtimeByTaskId: Record<string, { status: PaneRuntimeStatus; updatedAt: number }> = {
        ...state.taskRuntimeByTaskId
      };
      for (const [taskId, pane] of Object.entries(state.paneByTaskId)) {
        const target = String(pane?.paneTarget ?? "");
        if (!target) {
          continue;
        }
        const runtime = byTarget[target];
        if (runtime) {
          runtimeByTaskId[taskId] = { status: runtime.status, updatedAt: runtime.updatedAt };
        }
      }
      state.taskRuntimeByTaskId = runtimeByTaskId;
      state.paneMetaByTarget = paneMetaByTarget;
      logInfo("shellman.tmux.status.apply", {
        itemCount: items.length,
        mappedTaskCount: Object.keys(runtimeByTaskId).length
      });
    }
  }

  function send(data: unknown) {
    if (!ws) {
      logInfo("shellman.ws.send.skip", { reason: "socket-null" });
      return;
    }
    if (typeof data === "object" && data !== null) {
      const msg = data as { id?: string; type?: string; op?: string; payload?: Record<string, unknown> };
      const payload = msg.payload ?? {};
      logInfo("shellman.ws.send", {
        id: String(msg.id ?? ""),
        type: String(msg.type ?? ""),
        op: String(msg.op ?? ""),
        target: String(payload.target ?? ""),
        textLen: typeof payload.text === "string" ? payload.text.length : 0,
        cols: typeof payload.cols === "number" ? payload.cols : undefined,
        rows: typeof payload.rows === "number" ? payload.rows : undefined
      });
    }
    ws.send(JSON.stringify(data));
  }

  function findPaneUUIDByTarget(target: string): string {
    if (!target) {
      return "";
    }
    for (const pane of Object.values(state.paneByTaskId)) {
      if (pane?.paneTarget === target) {
        return String(pane.paneUuid ?? "");
      }
    }
    return "";
  }

  function resolveSelectedPaneBinding(): PaneBinding | null {
    const selectedTaskID = state.selectedTaskId;
    if (selectedTaskID) {
      const binding = state.paneByTaskId[selectedTaskID];
      if (binding) {
        return binding;
      }
    }
    if (!state.selectedPaneUuid) {
      return null;
    }
    for (const pane of Object.values(state.paneByTaskId)) {
      if (String(pane?.paneUuid ?? "") === state.selectedPaneUuid) {
        return pane;
      }
    }
    return null;
  }

  function findTaskIDByPaneUUID(paneUuid: string): string {
    if (!paneUuid) {
      return "";
    }
    for (const [taskID, pane] of Object.entries(state.paneByTaskId)) {
      if (String(pane?.paneUuid ?? "") === paneUuid) {
        return taskID;
      }
    }
    return "";
  }

  function findPaneTargetByPaneUUID(paneUuid: string): string {
    const taskID = findTaskIDByPaneUUID(paneUuid);
    if (!taskID) {
      return "";
    }
    return String(state.paneByTaskId[taskID]?.paneTarget ?? "");
  }

  function resolveSelectedPaneTarget(): string {
    const selectedBinding = resolveSelectedPaneBinding();
    if (selectedBinding?.paneTarget) {
      if (endedPaneUuids[selectedBinding.paneUuid]) {
        logInfo("shellman.target.resolve.miss", {
          reason: "pane-ended",
          selectedTaskId: state.selectedTaskId,
          selectedPaneUuid: selectedBinding.paneUuid
        });
        return "";
      }
      state.selectedPaneUuid = selectedBinding.paneUuid;
      state.selectedPaneTarget = selectedBinding.paneTarget;
      logInfo("shellman.target.resolve.hit_selected", {
        selectedTaskId: state.selectedTaskId,
        selectedPaneUuid: state.selectedPaneUuid,
        selectedPaneTarget: state.selectedPaneTarget
      });
      return state.selectedPaneTarget;
    }
    if (state.selectedPaneTarget) {
      logInfo("shellman.target.resolve.hit_selected", {
        selectedTaskId: state.selectedTaskId,
        selectedPaneUuid: state.selectedPaneUuid,
        selectedPaneTarget: state.selectedPaneTarget
      });
      return state.selectedPaneTarget;
    }
    if (!state.selectedTaskId) {
      logInfo("shellman.target.resolve.miss", { reason: "no-selected-task" });
      return "";
    }
    logInfo("shellman.target.resolve.miss", {
      reason: "no-pane-binding",
      selectedTaskId: state.selectedTaskId
    });
    return "";
  }

  function requestTmuxList() {
    send({
      id: nextID(),
      type: "req",
      op: "tmux.list",
      payload: { scope: "all" }
    });
  }

  function buildPaneEndedMessage(reason: string) {
    void reason;
    return "Session ended. Pane is no longer available.";
  }

  const ANSI_ESCAPE_RE =
    /\u001b(?:[@-Z\\-_]|\[[0-?]*[ -/]*[@-~]|\][^\u0007]*(?:\u0007|\u001b\\))/g;

  function stripAnsi(text: string): string {
    return text.replace(ANSI_ESCAPE_RE, "");
  }

  function trimToRecentLines(text: string, maxLines: number): string {
    if (!text || maxLines <= 0) {
      return text;
    }
    const hasTrailingNewline = text.endsWith("\n");
    const body = hasTrailingNewline ? text.slice(0, -1) : text;
    if (!body) {
      return text;
    }
    const lines = body.split("\n");
    if (lines.length <= maxLines) {
      return text;
    }
    const clipped = lines.slice(lines.length - maxLines).join("\n");
    return hasTrailingNewline ? `${clipped}\n` : clipped;
  }

  function trimTrailingBlankLines(text: string): string {
    const normalized = text.replace(/\r/g, "");
    const lines = normalized.split("\n");
    while (lines.length > 0 && lines[lines.length - 1]?.trim() === "") {
      lines.pop();
    }
    return lines.join("\n");
  }

  function toEndedDisplayText(raw: string, endedLine: string): string {
    const cleaned = trimTrailingBlankLines(stripAnsi(raw));
    if (!cleaned) {
      return `${endedLine}\n`;
    }
    if (cleaned.includes(endedLine)) {
      return `${cleaned}\n`;
    }
    return `${cleaned}\n${endedLine}\n`;
  }

  function markPaneEnded(paneUuid: string, reason: string) {
    if (!paneUuid) {
      return;
    }
    endedPaneUuids[paneUuid] = true;
    const taskID = findTaskIDByPaneUUID(paneUuid);
    const persistedCache = persistedSnapshotByPaneUuid[paneUuid];
    if (taskID) {
      const prev = state.taskRuntimeByTaskId[taskID];
      state.taskRuntimeByTaskId[taskID] = {
        status: "ready",
        updatedAt: prev?.updatedAt ?? 0
      };
    }
    const endedLine = buildPaneEndedMessage(reason);
    const prevCache = state.terminalByPaneUuid[paneUuid] ?? persistedCache;
    const prevOutput = String(prevCache?.output ?? "");
    const text = trimToRecentLines(toEndedDisplayText(prevOutput, endedLine), TERMINAL_CACHE_MAX_LINES);
    const cacheEntry = {
      output: text,
      frame: { mode: "reset" as const, data: text },
      cursor: null,
      source: "ended" as const
    };
    state.terminalByPaneUuid[paneUuid] = cacheEntry;
    if (taskID) {
      scheduleRefreshTaskFromServer(taskID);
    }
    if (state.selectedPaneUuid === paneUuid) {
      state.terminalOutput = text;
      state.terminalFrame = { mode: "reset", data: text };
      state.terminalCursor = null;
      state.selectedPaneTarget = "";
      state.terminalEnded = true;
      logInfo("shellman.pane.ended.selected", { paneUuid, reason });
    } else {
      logInfo("shellman.pane.ended.cached", { paneUuid, reason });
    }
  }

  function sendResizeToTarget(target: string, size: { cols: number; rows: number }, reason: string) {
    if (!state.wsConnected) {
      logInfo("shellman.term.resize.skip", {
        reason: "ws-not-connected",
        selectedTaskId: state.selectedTaskId,
        target,
        cols: size.cols,
        rows: size.rows
      });
      return;
    }
    send({
      id: nextID(),
      type: "event",
      op: "term.resize",
      payload: { target, cols: size.cols, rows: size.rows }
    });
    logInfo("shellman.term.resize.sent", {
      target,
      cols: size.cols,
      rows: size.rows,
      trigger: reason
    });
  }

  function touchActivePane(paneUuid: string) {
    const normalized = String(paneUuid ?? "").trim();
    if (!normalized) {
      return "";
    }
    const existingIdx = activePaneOrderByUuid.indexOf(normalized);
    if (existingIdx >= 0) {
      activePaneOrderByUuid.splice(existingIdx, 1);
    }
    activePaneOrderByUuid.push(normalized);
    if (activePaneOrderByUuid.length <= ACTIVE_PANE_CACHE_LIMIT) {
      return "";
    }
    const evicted = activePaneOrderByUuid.shift() ?? "";
    if (evicted) {
      paneGapByPaneUuid[evicted] = true;
    }
    return evicted;
  }

  async function selectTask(taskId: string, options: { forceRefreshNotes?: boolean; skipNotesHydration?: boolean } = {}) {
    logInfo("shellman.select_task.start", { taskId });
    const prevTaskId = state.selectedTaskId;
    const prevPaneTarget = state.selectedPaneTarget;
    state.selectedTaskId = taskId;
    if (!options.skipNotesHydration) {
      await loadTaskMessages(taskId, options.forceRefreshNotes === true);
    }
    try {
      await loadTaskSidecarMode(taskId);
    } catch {
      logInfo("shellman.task.sidecar_mode.load.error", { taskId });
    }

    if (!state.paneByTaskId[taskId]) {
      await loadTaskPane(taskId);
    }
    const selectedBinding = state.paneByTaskId[taskId];
    const target = selectedBinding?.paneTarget ?? "";
    state.selectedPaneUuid = selectedBinding?.paneUuid ?? "";
    state.selectedPaneTarget = target;
    const paneEnded = state.selectedPaneUuid ? Boolean(endedPaneUuids[state.selectedPaneUuid]) : false;
    if (state.selectedPaneUuid && paneEnded) {
      let endedCache = state.terminalByPaneUuid[state.selectedPaneUuid] ?? persistedSnapshotByPaneUuid[state.selectedPaneUuid];
      if (endedCache && endedCache.source !== "ended") {
        const text = toEndedDisplayText(endedCache.output, buildPaneEndedMessage(""));
        endedCache = {
          output: text,
          frame: { mode: "reset", data: text },
          cursor: null,
          source: "ended"
        };
        state.terminalByPaneUuid[state.selectedPaneUuid] = endedCache;
      }
      if (endedCache) {
        state.terminalOutput = endedCache.output;
        state.terminalFrame = endedCache.frame;
        state.terminalCursor = endedCache.cursor;
        state.terminalEnded = true;
      } else {
        markPaneEnded(state.selectedPaneUuid, TMUX_PANE_NOT_FOUND_CODE);
      }
    }
    const cached = state.selectedPaneUuid ? state.terminalByPaneUuid[state.selectedPaneUuid] : null;
    if (cached && (cached.source !== "persisted" || paneEnded)) {
      state.terminalOutput = cached.output;
      state.terminalFrame = cached.frame;
      state.terminalCursor = cached.cursor;
      state.terminalEnded = cached.source === "ended";
    } else if (cached) {
      state.terminalEnded = false;
    } else if (!cached) {
      state.terminalEnded = false;
    }
    if (!target || !state.wsConnected) {
      logInfo("shellman.select_task.skip", {
        taskId,
        target,
        wsConnected: state.wsConnected
      });
      if (!target && paneLookupStatus[taskId] !== "missing") {

      }
      return;
    }
    const sameTaskAndTarget = taskId === prevTaskId && target === prevPaneTarget;
    const alreadySentInCurrentConnection =
      lastSelectSent?.target === target && lastSelectSent?.connectionSeq === wsConnectionSeq;
    if (sameTaskAndTarget && alreadySentInCurrentConnection) {
      logInfo("shellman.select_task.skip", {
        taskId,
        target,
        reason: "duplicate-select-same-target"
      });
      return;
    }
    if (state.selectedPaneUuid) {
      touchActivePane(state.selectedPaneUuid);
    }
    const selectPayload: { target: string; cols?: number; rows?: number; gap_recover?: boolean; history_lines?: number } = { target };
    if (state.selectedPaneUuid && paneGapByPaneUuid[state.selectedPaneUuid]) {
      selectPayload.gap_recover = true;
      selectPayload.history_lines = GAP_RECOVER_HISTORY_LINES;
      delete paneGapByPaneUuid[state.selectedPaneUuid];
    }
    if (latestTerminalSize && latestTerminalSize.cols >= 2 && latestTerminalSize.rows >= 2) {
      selectPayload.cols = latestTerminalSize.cols;
      selectPayload.rows = latestTerminalSize.rows;
    }
    const requestID = nextID();
    if (state.selectedPaneUuid) {
      selectReqPaneUuidByReqId[requestID] = state.selectedPaneUuid;
    }
    send({
      id: requestID,
      type: "req",
      op: "tmux.select_pane",
      payload: selectPayload
    });
    lastSelectSent = { target, connectionSeq: wsConnectionSeq };
    logInfo("shellman.select_task.sent", { taskId, target });
  }

  function sendTerminalInput(text: string) {
    const inputSeq = ++termInputSeq;
    logInfo("shellman.term.input.request", {
      inputSeq,
      selectedTaskId: state.selectedTaskId,
      selectedPaneTarget: state.selectedPaneTarget,
      text,
      textLen: text.length,
      textEscaped: escapeForLog(text)
    });
    const target = resolveSelectedPaneTarget();
    if (!target) {
      logInfo("shellman.term.input.skip", {
        reason: "no-target",
        selectedTaskId: state.selectedTaskId
      });
      return;
    }
    const reqID = nextID();
    pendingTermInputByReqID[reqID] = {
      inputSeq,
      target,
      textLen: text.length,
      sentAtMs: Date.now()
    };
    send({
      id: reqID,
      type: "event",
      op: "term.input",
      payload: { target, text }
    });
    logInfo("shellman.term.input.sent", {
      inputSeq,
      reqID,
      target,
      text,
      textLen: text.length,
      textEscaped: escapeForLog(text)
    });
  }

  async function sendImagePasteToTerminal(file: File) {
    if (!file || !String(file.type).startsWith("image/") || file.size <= 0) {
      throw new Error("INVALID_PASTE_IMAGE");
    }

    const form = new FormData();
    form.append("file", file, file.name || "pasted-image");

    const uploadRes = (await fetchImpl(apiURL("/api/v1/system/uploads/image"), {
      method: "POST",
      headers: apiHeaders(),
      body: form
    }).then((r) => r.json())) as APIResponse<ImageUploadResponse>;

    if (!uploadRes.ok || !uploadRes.data?.path) {
      throw new Error(String(uploadRes.error?.code ?? "UPLOAD_IMAGE_FAILED"));
    }

    sendTerminalInput(`${String(uploadRes.data.path).trim()} `);
  }

  function sendTerminalResize(size: { cols: number; rows: number }) {
    latestTerminalSize = { cols: size.cols, rows: size.rows };
    logInfo("shellman.term.resize.request", {
      selectedTaskId: state.selectedTaskId,
      selectedPaneTarget: state.selectedPaneTarget,
      cols: size.cols,
      rows: size.rows
    });
    const target = resolveSelectedPaneTarget();
    if (!target) {
      logInfo("shellman.term.resize.skip", {
        reason: "no-target",
        selectedTaskId: state.selectedTaskId,
        cols: size.cols,
        rows: size.rows
      });
      return;
    }
    sendResizeToTarget(target, size, "terminal-pane");
  }

  async function loadMorePaneHistory(taskId: string, lines = GAP_RECOVER_HISTORY_LINES) {
    const nextTaskID = String(taskId ?? "").trim();
    if (!nextTaskID) {
      return;
    }
    const nextLines = Math.max(200, Math.min(10000, Math.floor(lines)));
    const res = (await fetchImpl(apiURL(`/api/v1/tasks/${nextTaskID}/pane-history?lines=${encodeURIComponent(String(nextLines))}`), {
      headers: apiHeaders()
    }).then((r) => r.json())) as APIResponse<{
      task_id: string;
      pane_uuid?: string;
      pane_id?: string;
      pane_target?: string;
      snapshot?: PaneSnapshotPayload | null;
    }>;
    if (!res.ok) {
      throw new Error(res.error?.code || "TASK_PANE_HISTORY_LOAD_FAILED");
    }
    const paneUuid =
      String(res.data?.pane_uuid ?? "").trim() ||
      String(state.paneByTaskId[nextTaskID]?.paneUuid ?? "").trim() ||
      String(res.data?.pane_id ?? "").trim();
    const snapshotCache = toTerminalCacheEntry(res.data?.snapshot);
    if (!paneUuid || !snapshotCache) {
      return;
    }
    state.terminalByPaneUuid[paneUuid] = snapshotCache;
    persistedSnapshotByPaneUuid[paneUuid] = snapshotCache;
    if (state.selectedTaskId === nextTaskID && state.selectedPaneUuid === paneUuid) {
      state.terminalOutput = snapshotCache.output;
      state.terminalFrame = snapshotCache.frame;
      state.terminalCursor = snapshotCache.cursor;
      state.terminalEnded = false;
    }
  }

  function clearPendingTermInput(reason: string) {
    const pendingCount = Object.keys(pendingTermInputByReqID).length;
    if (pendingCount <= 0) {
      return;
    }
    for (const reqID of Object.keys(pendingTermInputByReqID)) {
      delete pendingTermInputByReqID[reqID];
    }
    logInfo("shellman.term.input.pending.clear", { reason, pendingCount });
  }

  function connectWS(url?: string) {
    const wsURL =
      url ??
      (() => {
        const base = new URL(ctx.baseOrigin);
        const wsProtocol = base.protocol === "https:" ? "wss:" : "ws:";
        const turnPath = ctx.turnUUID || "local";
        return `${wsProtocol}//${base.host}/ws/client/${turnPath}`;
      })();
    logInfo("shellman.ws.connect.start", { url: wsURL });
    ws = wsFactory(wsURL);
    ws.addEventListener("open", () => {
      state.wsConnected = true;
      wsConnectionSeq += 1;
      lastSelectSent = null;
      clearPendingTermInput("ws-open");
      logInfo("shellman.ws.connect.open", { url: wsURL });
      requestTmuxList();
      if (state.selectedTaskId) {
        void selectTask(state.selectedTaskId, { skipNotesHydration: true });
      }
    });
    ws.addEventListener("close", () => {
      state.wsConnected = false;
      lastSelectSent = null;
      clearPendingTermInput("ws-close");
      logInfo("shellman.ws.connect.close", { url: wsURL });
    });
    ws.addEventListener("message", (event: MessageEvent<string>) => {
      const raw = String(event.data ?? "");
      logInfo("shellman.ws.recv.raw", { len: raw.length });
      try {
        applyProtocolMessage(JSON.parse(raw) as ProtocolMessage);
      } catch {
        logInfo("shellman.ws.recv.parse_error", { raw });
      }
    });
  }

  function disconnectWS() {
    if (ws) {
      logInfo("shellman.ws.disconnect", {});
      ws.close();
      ws = null;
      lastSelectSent = null;
      clearPendingTermInput("ws-disconnect");
    }
  }

  function findProjectIDByTaskStrict(taskId: string): string {
    for (const [projectId, nodes] of Object.entries(state.treesByProject)) {
      if (nodes.some((n) => n.taskId === taskId)) {
        return projectId;
      }
    }
    return "";
  }

  function findProjectIDByTask(taskId: string): string {
    const strict = findProjectIDByTaskStrict(taskId);
    if (strict) {
      return strict;
    }
    return state.projects[0]?.projectId ?? "";
  }

  async function createSiblingPane(taskId: string, title: string) {
    logInfo("shellman.pane.create_sibling.start", { taskId, title });
    const res = (await fetchImpl(apiURL(`/api/v1/tasks/${taskId}/panes/sibling`), {
      method: "POST",
      headers: apiHeaders({ "Content-Type": "application/json" }),
      body: JSON.stringify({ title })
    }).then((r) => r.json())) as APIResponse<{ task_id: string; pane_uuid?: string; pane_id: string; pane_target?: string }>;
    if (!res.ok || !res.data?.task_id || !res.data?.pane_id) {
      throw new Error(String(res.error?.message ?? "CREATE_SIBLING_FAILED"));
    }

    const projectId = findProjectIDByTask(taskId);
    const nodes = state.treesByProject[projectId] ?? [];
    const nowSec = Math.floor(Date.now() / 1000);
    nodes.push({ taskId: res.data.task_id, title, description: "", checked: false, archived: false, status: "pending", updatedAt: nowSec });
    state.treesByProject[projectId] = nodes;
    state.paneByTaskId[res.data.task_id] = {
      paneUuid: String(res.data.pane_uuid ?? res.data.pane_id),
      paneId: res.data.pane_id,
      paneTarget: String(res.data.pane_target ?? res.data.pane_id),
      relation: "sibling"
    };
    logInfo("shellman.pane.create_sibling.done", {
      taskId: res.data.task_id,
      paneTarget: state.paneByTaskId[res.data.task_id]?.paneTarget ?? ""
    });
  }

  async function createChildTask(taskId: string, title: string) {
    logInfo("shellman.task.create_child.start", { taskId, title });
    const projectId = findProjectIDByTask(taskId);
    const res = (await fetchImpl(apiURL(`/api/v1/tasks/${taskId}/derive`), {
      method: "POST",
      headers: apiHeaders({ "Content-Type": "application/json" }),
      body: JSON.stringify({ title })
    }).then((r) => r.json())) as APIResponse<{ task_id: string }>;
    if (!res.ok || !res.data?.task_id) {
      throw new Error(String(res.error?.message ?? "CREATE_CHILD_TASK_FAILED"));
    }

    const nodes = state.treesByProject[projectId] ?? [];
    const nowSec = Math.floor(Date.now() / 1000);
    nodes.push({
      taskId: res.data.task_id,
      parentTaskId: taskId,
      title,
      description: "",
      checked: false,
      archived: false,
      status: "pending",
      updatedAt: nowSec
    });
    state.treesByProject[projectId] = nodes;
    paneLookupStatus[res.data.task_id] = "missing";
    recomputePaneLookupComplete();
    logInfo("shellman.task.create_child.done", { taskId: res.data.task_id });
    await selectTask(res.data.task_id);
  }

  async function createRootTask(projectId: string, title: string) {
    logInfo("shellman.task.create_root.start", { projectId, title });
    const res = (await fetchImpl(apiURL(`/api/v1/tasks`), {
      method: "POST",
      headers: apiHeaders({ "Content-Type": "application/json" }),
      body: JSON.stringify({ project_id: projectId, title })
    }).then((r) => r.json())) as APIResponse<{ task_id: string }>;
    if (!res.ok || !res.data?.task_id) {
      throw new Error(String(res.error?.message ?? "CREATE_ROOT_TASK_FAILED"));
    }

    const nodes = state.treesByProject[projectId] ?? [];
    const nowSec = Math.floor(Date.now() / 1000);
    nodes.push({ taskId: res.data.task_id, title, description: "", checked: false, archived: false, status: "pending", updatedAt: nowSec });
    state.treesByProject[projectId] = nodes;
    paneLookupStatus[res.data.task_id] = "missing";
    recomputePaneLookupComplete();
    logInfo("shellman.task.create_root.done", { taskId: res.data.task_id });
    await selectTask(res.data.task_id);
  }

  async function createChildPane(taskId: string, title: string) {
    await createChildTask(taskId, title);
  }

  async function createRootPane(projectId: string, title: string) {
    await createRootTask(projectId, title);
  }

  function getOrphanPaneItems(): TmuxPaneItem[] {
    if (!state.paneLookupComplete) {
      return [];
    }
    const boundTargets = new Set(
      Object.values(state.paneByTaskId)
        .map((pane) => String(pane?.paneTarget ?? "").trim())
        .filter((target) => target.length > 0)
    );
    return state.tmuxPaneItems.filter((item) => !boundTargets.has(String(item.target ?? "").trim()));
  }

  async function adoptPaneAsChild(parentTaskId: string, paneTarget: string, title = "") {
    const parentID = String(parentTaskId ?? "").trim();
    const target = String(paneTarget ?? "").trim();
    const nextTitle = String(title ?? "").trim();
    if (!parentID || !target) {
      throw new Error("PANE_ADOPT_FAILED");
    }

    const res = (await fetchImpl(apiURL(`/api/v1/tasks/${parentID}/adopt-pane`), {
      method: "POST",
      headers: apiHeaders({ "Content-Type": "application/json" }),
      body: JSON.stringify({ pane_target: target, title: nextTitle })
    }).then((r) => r.json())) as APIResponse<{ task_id: string; title?: string; pane_uuid?: string; pane_id: string; pane_target: string }>;
    if (!res.ok || !res.data?.task_id || !res.data?.pane_id || !res.data?.pane_target) {
      throw new Error(String(res.error?.code ?? "PANE_ADOPT_FAILED"));
    }

    const projectId = findProjectIDByTask(parentID);
    if (!projectId) {
      throw new Error("PROJECT_NOT_FOUND");
    }

    const nodes = [...(state.treesByProject[projectId] ?? [])];
    const nowSec = Math.floor(Date.now() / 1000);
    const createdTitle = String(res.data.title ?? nextTitle ?? "").trim() || `Adopted ${target}`;
    nodes.push({
      taskId: res.data.task_id,
      parentTaskId: parentID,
      title: createdTitle,
      description: "",
      checked: false,
      status: "running",
      updatedAt: nowSec
    });
    state.treesByProject[projectId] = nodes;
    state.paneByTaskId[res.data.task_id] = {
      paneUuid: String(res.data.pane_uuid ?? res.data.pane_id),
      paneId: String(res.data.pane_id),
      paneTarget: String(res.data.pane_target),
      relation: "child"
    };
    paneLookupStatus[res.data.task_id] = "bound";
    recomputePaneLookupComplete();
  }

  async function addActiveProject(projectId: string, repoRoot: string) {
    const id = projectId.trim();
    const root = repoRoot.trim();
    logInfo("shellman.project.add.start", {
      projectIdRaw: projectId,
      repoRootRaw: repoRoot,
      projectId: id,
      repoRoot: root
    });
    const res = (await fetchImpl(apiURL("/api/v1/projects/active"), {
      method: "POST",
      headers: apiHeaders({ "Content-Type": "application/json" }),
      body: JSON.stringify({ project_id: id, repo_root: root })
    }).then((r) => r.json())) as APIResponse<{ project_id: string }>;

    if (!res.ok) {
      logInfo("shellman.project.add.fail", {
        projectId: id,
        code: String(res.error?.code ?? "PROJECT_ADD_FAILED")
      });
      throw new Error(res.error?.code || "PROJECT_ADD_FAILED");
    }

    const existing = state.projects.find((p) => p.projectId === id);
    if (existing) {
      existing.repoRoot = root;
    } else {
      state.projects.push({ projectId: id, repoRoot: root });
    }

    const treeRes = (await fetchImpl(apiURL(`/api/v1/projects/${id}/tree`), {
      headers: apiHeaders()
    }).then((r) => r.json())) as APIResponse<{
      project_id: string;
      nodes: Array<{
        task_id: string;
        parent_task_id?: string;
        title: string;
        description?: string;
      flag?: "success" | "notify" | "error";
      flag_desc?: string;
      checked?: boolean;
      archived?: boolean;
      status: string;
      updated_at?: number | string;
    }>;
    }>;
    const nodes = toTaskNodes(treeRes.data.nodes);
    state.treesByProject[id] = nodes;
    for (const node of nodes) {
      await loadTaskPane(node.taskId);
    }
    logInfo("shellman.project.add.done", {
      projectId: id,
      treeCount: nodes.length
    });
  }

  async function removeActiveProject(projectId: string) {
    const id = projectId.trim();
    logInfo("shellman.project.remove.start", {
      projectIdRaw: projectId,
      projectId: id
    });
    const res = (await fetchImpl(apiURL(`/api/v1/projects/active/${encodeURIComponent(id)}`), {
      method: "DELETE",
      headers: apiHeaders()
    }).then((r) => r.json())) as APIResponse<{ project_id: string }>;
    if (!res.ok) {
      logInfo("shellman.project.remove.fail", {
        projectId: id,
        code: String(res.error?.code ?? "PROJECT_REMOVE_FAILED")
      });
      throw new Error(res.error?.code || "PROJECT_REMOVE_FAILED");
    }

    const removedTasks = new Set((state.treesByProject[id] ?? []).map((node) => node.taskId));
    state.projects = state.projects.filter((project) => project.projectId !== id);
    delete state.treesByProject[id];

    for (const taskId of removedTasks) {
      delete state.paneByTaskId[taskId];
      delete state.taskMessagesByTaskId[taskId];
      delete state.taskRuntimeByTaskId[taskId];
      delete paneLookupStatus[taskId];
      delete messagesHydratedByTaskID[taskId];
    }

    if (removedTasks.has(state.selectedTaskId)) {
      state.selectedTaskId = "";
      state.selectedPaneUuid = "";
      state.selectedPaneTarget = "";
      state.terminalOutput = "";
      state.terminalFrame = { mode: "reset", data: "" };
      state.terminalCursor = null;
      state.terminalEnded = false;
    }

    recomputePaneLookupComplete();
    logInfo("shellman.project.remove.done", {
      projectId: id,
      projectCount: state.projects.length
    });
  }

  async function archiveDoneTasksByProject(projectId: string) {
    const id = projectId.trim();
    logInfo("shellman.project.archive_done.start", {
      projectIdRaw: projectId,
      projectId: id
    });
    const res = (await fetchImpl(apiURL(`/api/v1/projects/${encodeURIComponent(id)}/archive-done`), {
      method: "POST",
      headers: apiHeaders()
    }).then((r) => r.json())) as APIResponse<{ project_id: string; archived_count: number }>;
    if (!res.ok) {
      logInfo("shellman.project.archive_done.fail", {
        projectId: id,
        code: String(res.error?.code ?? "TASK_ARCHIVE_FAILED")
      });
      throw new Error(res.error?.code || "TASK_ARCHIVE_FAILED");
    }

    const prevTaskIds = new Set((state.treesByProject[id] ?? []).map((node) => node.taskId));
    const treeRes = (await fetchImpl(apiURL(`/api/v1/projects/${id}/tree`), {
      headers: apiHeaders()
    }).then((r) => r.json())) as APIResponse<{
      project_id: string;
      nodes: Array<{
        task_id: string;
        parent_task_id?: string;
        title: string;
        description?: string;
        flag?: "success" | "notify" | "error";
        flag_desc?: string;
        checked?: boolean;
        archived?: boolean;
        status: string;
        updated_at?: number | string;
      }>;
    }>;
    const nextNodes = toTaskNodes(treeRes.data.nodes);
    state.treesByProject[id] = nextNodes;
    const aliveTaskIds = new Set(nextNodes.map((node) => node.taskId));
    for (const taskId of prevTaskIds) {
      if (aliveTaskIds.has(taskId)) {
        continue;
      }
      delete state.paneByTaskId[taskId];
      delete state.taskMessagesByTaskId[taskId];
      delete state.taskRuntimeByTaskId[taskId];
      delete paneLookupStatus[taskId];
      delete messagesHydratedByTaskID[taskId];
    }
    if (state.selectedTaskId && !aliveTaskIds.has(state.selectedTaskId)) {
      state.selectedTaskId = "";
      state.selectedPaneUuid = "";
      state.selectedPaneTarget = "";
      state.terminalOutput = "";
      state.terminalFrame = { mode: "reset", data: "" };
      state.terminalCursor = null;
      state.terminalEnded = false;
    }
    recomputePaneLookupComplete();
    logInfo("shellman.project.archive_done.done", {
      projectId: id,
      archivedCount: Number(res.data?.archived_count ?? 0),
      treeCount: nextNodes.length
    });
  }

  async function getFSRoots() {
    const res = (await fetchImpl(apiURL("/api/v1/fs/roots"), {
      headers: apiHeaders()
    }).then((r) => r.json())) as APIResponse<{ roots: string[] }>;
    if (!res.ok) {
      throw new Error(res.error?.code || "FS_ROOTS_FAILED");
    }
    return res.data?.roots ?? [];
  }

  async function listDirectories(path: string): Promise<DirectoryListResult> {
    const p = path.trim();
    const res = (await fetchImpl(apiURL(`/api/v1/fs/list?path=${encodeURIComponent(p)}`), {
      headers: apiHeaders()
    }).then((r) => r.json())) as APIResponse<DirectoryListResult>;
    if (!res.ok || !res.data) {
      throw new Error(res.error?.code || "FS_LIST_FAILED");
    }
    return {
      path: String(res.data.path ?? ""),
      items: (res.data.items ?? []).map((item) => ({
        name: String(item.name ?? ""),
        path: String(item.path ?? ""),
        is_dir: Boolean(item.is_dir)
      }))
    };
  }

  async function resolveDirectory(path: string): Promise<string> {
    const res = (await fetchImpl(apiURL("/api/v1/fs/resolve"), {
      method: "POST",
      headers: apiHeaders({ "Content-Type": "application/json" }),
      body: JSON.stringify({ path: path.trim() })
    }).then((r) => r.json())) as APIResponse<{ path: string }>;
    if (!res.ok || !res.data?.path) {
      throw new Error(res.error?.code || "FS_RESOLVE_FAILED");
    }
    return res.data.path;
  }

  async function searchDirectories(base: string, q: string, limit = 20): Promise<DirectoryItem[]> {
    const params = new URLSearchParams({
      base: base.trim(),
      q: q.trim(),
      limit: String(limit)
    });
    const res = (await fetchImpl(apiURL(`/api/v1/fs/search?${params.toString()}`), {
      headers: apiHeaders()
    }).then((r) => r.json())) as APIResponse<{ items: DirectoryItem[] }>;
    if (!res.ok) {
      throw new Error(res.error?.code || "FS_SEARCH_FAILED");
    }
    return (res.data?.items ?? []).map((item) => ({
      name: String(item.name ?? ""),
      path: String(item.path ?? ""),
      is_dir: Boolean(item.is_dir)
    }));
  }

  async function getDirectoryHistory(limit = 20): Promise<DirectoryHistoryItem[]> {
    const res = (await fetchImpl(apiURL(`/api/v1/fs/history?limit=${encodeURIComponent(String(limit))}`), {
      headers: apiHeaders()
    }).then((r) => r.json())) as APIResponse<{ items: DirectoryHistoryItem[] }>;
    if (!res.ok) {
      throw new Error(res.error?.code || "FS_HISTORY_LIST_FAILED");
    }
    return (res.data?.items ?? []).map((item) => ({
      path: String(item.path ?? ""),
      first_accessed_at: Number(item.first_accessed_at ?? 0),
      last_accessed_at: Number(item.last_accessed_at ?? 0),
      access_count: Number(item.access_count ?? 0)
    }));
  }

  async function recordDirectoryHistory(path: string) {
    const res = (await fetchImpl(apiURL("/api/v1/fs/history"), {
      method: "POST",
      headers: apiHeaders({ "Content-Type": "application/json" }),
      body: JSON.stringify({ path: path.trim() })
    }).then((r) => r.json())) as APIResponse<{ path: string }>;
    if (!res.ok) {
      throw new Error(res.error?.code || "FS_HISTORY_WRITE_FAILED");
    }
  }

  async function loadConfig() {
    const seq = ++configLoadSeq;
    const res = (await fetchImpl(apiURL("/api/v1/config"), {
      headers: apiHeaders()
    }).then((r) => r.json())) as APIResponse<GlobalConfig>;
    if (!res.ok || !res.data) {
      throw new Error(res.error?.code || "CONFIG_LOAD_FAILED");
    }
    // Keep only the latest config response to avoid older in-flight requests clobbering fresh values.
    if (seq !== configLoadSeq) {
      return;
    }
    state.localPort = typeof res.data.local_port === "number" && res.data.local_port > 0 ? res.data.local_port : 4621;
    state.defaultLaunchProgram = normalizeLaunchProgram(res.data.defaults?.session_program);
    state.defaultHelperProgram = normalizeHelperProgram(res.data.defaults?.helper_program);
    if (state.appPrograms.length > 0 && !state.appPrograms.some((item) => item.id === state.defaultHelperProgram)) {
      state.defaultHelperProgram = state.appPrograms[0].id;
    }
    state.helperOpenAIEndpoint = String(res.data.helper_openai?.endpoint ?? "").trim();
    state.helperOpenAIModel = String(res.data.helper_openai?.model ?? "").trim();
    state.helperOpenAIApiKey = "";
    state.taskCompletionMode = normalizeTaskCompletionMode(res.data.task_completion_mode);
    state.taskCompletionCommand = String(res.data.task_completion_command ?? "").trim();
    state.taskCompletionIdleDuration = typeof res.data.task_completion_idle_duration_seconds === "number" &&
      res.data.task_completion_idle_duration_seconds >= 0
      ? Math.floor(res.data.task_completion_idle_duration_seconds)
      : 0;
  }

  async function saveDefaultLaunchProgram(program: LaunchProgram) {
    const nextProgram = normalizeLaunchProgram(program);
    const body = {
      local_port: state.localPort,
      defaults: {
        session_program: nextProgram,
        helper_program: state.defaultHelperProgram
      },
      task_completion_mode: state.taskCompletionMode,
      task_completion_command: state.taskCompletionCommand,
      task_completion_idle_duration_seconds: state.taskCompletionIdleDuration
    };
    return saveConfig(body);
  }

  async function saveTaskCompletionSettings(program: LaunchProgram, helperProgram: HelperProgram, payload: {
    taskCompletionMode: TaskCompletionMode;
    taskCompletionCommand: string;
    taskCompletionIdleDuration: number;
    helperOpenAIEndpoint: string;
    helperOpenAIModel: string;
    helperOpenAIApiKey: string;
  }) {
    const nextProgram = normalizeLaunchProgram(program);
    const nextHelperProgram = normalizeHelperProgram(helperProgram);
    const nextDuration = Number(payload.taskCompletionIdleDuration) >= 0 ? Math.floor(Number(payload.taskCompletionIdleDuration)) : 0;
    const helperOpenAIEndpoint = String(payload.helperOpenAIEndpoint ?? "").trim();
    const helperOpenAIModel = String(payload.helperOpenAIModel ?? "").trim();
    const helperOpenAIApiKey = String(payload.helperOpenAIApiKey ?? "").trim();
    const helperOpenAI: Record<string, string> = {
      endpoint: helperOpenAIEndpoint,
      model: helperOpenAIModel
    };
    if (helperOpenAIApiKey !== "") {
      helperOpenAI.api_key = helperOpenAIApiKey;
    }
    const res = (await fetchImpl(apiURL("/api/v1/config"), {
      method: "PATCH",
      headers: apiHeaders({ "Content-Type": "application/json" }),
      body: JSON.stringify({
        local_port: state.localPort,
        defaults: {
          session_program: nextProgram,
          helper_program: nextHelperProgram
        },
        task_completion_mode: String(payload.taskCompletionMode ?? "none").trim(),
        task_completion_command: String(payload.taskCompletionCommand ?? "").trim(),
        task_completion_idle_duration_seconds: nextDuration,
        helper_openai: helperOpenAI
      })
    }).then((r) => r.json())) as APIResponse<GlobalConfig>;
    if (!res.ok || !res.data) {
      throw new Error(res.error?.code || "CONFIG_SAVE_FAILED");
    }
    state.localPort = typeof res.data.local_port === "number" && res.data.local_port > 0 ? res.data.local_port : 4621;
    state.defaultLaunchProgram = normalizeLaunchProgram(res.data.defaults?.session_program);
    state.defaultHelperProgram = normalizeHelperProgram(res.data.defaults?.helper_program);
    state.helperOpenAIEndpoint = String(res.data.helper_openai?.endpoint ?? "").trim();
    state.helperOpenAIModel = String(res.data.helper_openai?.model ?? "").trim();
    state.helperOpenAIApiKey = "";
    state.taskCompletionMode = normalizeTaskCompletionMode(res.data.task_completion_mode);
    state.taskCompletionCommand = String(res.data.task_completion_command ?? "").trim();
    state.taskCompletionIdleDuration = typeof res.data.task_completion_idle_duration_seconds === "number" &&
      res.data.task_completion_idle_duration_seconds >= 0
      ? Math.floor(res.data.task_completion_idle_duration_seconds)
      : 0;
  }

  async function saveConfig(body: Record<string, unknown>) {
    const res = (await fetchImpl(apiURL("/api/v1/config"), {
      method: "PATCH",
      headers: apiHeaders({ "Content-Type": "application/json" }),
      body: JSON.stringify(body)
    }).then((r) => r.json())) as APIResponse<GlobalConfig>;
    if (!res.ok || !res.data) {
      throw new Error(res.error?.code || "CONFIG_SAVE_FAILED");
    }
    state.localPort = typeof res.data.local_port === "number" && res.data.local_port > 0 ? res.data.local_port : 4621;
    state.defaultLaunchProgram = normalizeLaunchProgram(res.data.defaults?.session_program);
    state.defaultHelperProgram = normalizeHelperProgram(res.data.defaults?.helper_program);
    state.helperOpenAIEndpoint = String(res.data.helper_openai?.endpoint ?? "").trim();
    state.helperOpenAIModel = String(res.data.helper_openai?.model ?? "").trim();
    state.helperOpenAIApiKey = "";
    state.taskCompletionMode = normalizeTaskCompletionMode(res.data.task_completion_mode);
    state.taskCompletionCommand = String(res.data.task_completion_command ?? "").trim();
    state.taskCompletionIdleDuration = typeof res.data.task_completion_idle_duration_seconds === "number" &&
      res.data.task_completion_idle_duration_seconds >= 0
      ? Math.floor(res.data.task_completion_idle_duration_seconds)
      : 0;
  }

  async function selectDirectory() {
    const res = (await fetchImpl(apiURL("/api/v1/system/select-directory"), {
      method: "POST",
      headers: apiHeaders({ "Content-Type": "application/json" })
    }).then((r) => r.json())) as APIResponse<{ repo_root: string }>;
    if (!res.ok || !res.data?.repo_root) {
      throw new Error(res.error?.code || "PICK_DIRECTORY_FAILED");
    }
    return res.data.repo_root;
  }

  async function loadAppPrograms() {
    try {
      const res = (await fetchImpl(apiURL("/api/v1/system/app-programs"), {
        headers: apiHeaders()
      }).then((r) => r.json())) as APIResponse<AppProgramsConfig>;
      if (!res.ok || !res.data) {
        throw new Error(res.error?.code || "APP_PROGRAMS_LOAD_FAILED");
      }
      state.appPrograms = normalizeAppPrograms(res.data.providers);
    } catch {
      state.appPrograms = normalizeAppPrograms([]);
    }
    const validHelper = state.appPrograms.some((p) => p.id === state.defaultHelperProgram);
    if (!validHelper && state.appPrograms.length > 0) {
      state.defaultHelperProgram = state.appPrograms[0].id;
    }
  }

  async function setTaskChecked(taskId: string, checked: boolean) {
    const res = (await fetchImpl(apiURL(`/api/v1/tasks/${taskId}/check`), {
      method: "PATCH",
      headers: apiHeaders({ "Content-Type": "application/json" }),
      body: JSON.stringify({ checked })
    }).then((r) => r.json())) as APIResponse<{ task_id: string; checked: boolean }>;
    if (!res.ok) {
      throw new Error(res.error?.code || "TASK_CHECK_UPDATE_FAILED");
    }
    for (const [projectId, nodes] of Object.entries(state.treesByProject)) {
      const idx = nodes.findIndex((n) => n.taskId === taskId);
      if (idx >= 0) {
        const next = [...nodes];
        next[idx] = {
          ...next[idx],
          checked
        };
        state.treesByProject[projectId] = next;
        break;
      }
    }
  }

  async function setTaskTitle(taskId: string, title: string) {
    const nextTitle = title.trim();
    if (!taskId || !nextTitle) {
      return;
    }
    const res = (await fetchImpl(apiURL(`/api/v1/tasks/${taskId}/title`), {
      method: "PATCH",
      headers: apiHeaders({ "Content-Type": "application/json" }),
      body: JSON.stringify({ title: nextTitle })
    }).then((r) => r.json())) as APIResponse<{ task_id: string; title: string }>;
    if (!res.ok) {
      throw new Error(res.error?.code || "TASK_TITLE_UPDATE_FAILED");
    }
    for (const [projectId, nodes] of Object.entries(state.treesByProject)) {
      const idx = nodes.findIndex((n) => n.taskId === taskId);
      if (idx >= 0) {
        const next = [...nodes];
        next[idx] = {
          ...next[idx],
          title: nextTitle
        };
        state.treesByProject[projectId] = next;
        break;
      }
    }
  }

  async function setTaskDescription(taskId: string, description: string) {
    if (!taskId) {
      return;
    }
    const nextDescription = description.trim();
    const res = (await fetchImpl(apiURL(`/api/v1/tasks/${taskId}/description`), {
      method: "PATCH",
      headers: apiHeaders({ "Content-Type": "application/json" }),
      body: JSON.stringify({ description: nextDescription })
    }).then((r) => r.json())) as APIResponse<{ task_id: string; description: string }>;
    if (!res.ok) {
      throw new Error(res.error?.code || "TASK_DESCRIPTION_UPDATE_FAILED");
    }
    for (const [projectId, nodes] of Object.entries(state.treesByProject)) {
      const idx = nodes.findIndex((n) => n.taskId === taskId);
      if (idx >= 0) {
        const next = [...nodes];
        next[idx] = {
          ...next[idx],
          description: nextDescription
        };
        state.treesByProject[projectId] = next;
        break;
      }
    }
  }

  async function markTaskFlagReaded(taskId: string, flagReaded: boolean) {
    if (!taskId) {
      return;
    }
    const res = (await fetchImpl(apiURL(`/api/v1/tasks/${taskId}/flag-readed`), {
      method: "PATCH",
      headers: apiHeaders({ "Content-Type": "application/json" }),
      body: JSON.stringify({ flag_readed: Boolean(flagReaded) })
    }).then((r) => r.json())) as APIResponse<{ task_id: string; flag_readed: boolean }>;
    if (!res.ok) {
      throw new Error(res.error?.code || "TASK_FLAG_READED_UPDATE_FAILED");
    }
    for (const [projectId, nodes] of Object.entries(state.treesByProject)) {
      const idx = nodes.findIndex((n) => n.taskId === taskId);
      if (idx >= 0) {
        const next = [...nodes];
        next[idx] = {
          ...next[idx],
          flagReaded: Boolean(flagReaded)
        };
        state.treesByProject[projectId] = next;
        break;
      }
    }
  }

  async function sendTaskMessage(taskId: string, content: string) {
    const nextTaskID = String(taskId ?? "").trim();
    const nextContent = String(content ?? "").trim();
    if (!nextTaskID || !nextContent) {
      throw new Error("INVALID_MESSAGE");
    }
    const now = Math.floor(Date.now() / 1000);
    const base = state.taskMessagesByTaskId[nextTaskID] ?? [];
    const tmpUserID = -(Date.now());
    const tmpAssistantID = tmpUserID - 1;
    state.taskMessagesByTaskId[nextTaskID] = [
      ...base,
      {
        id: tmpUserID,
        task_id: nextTaskID,
        role: "user",
        content: nextContent,
        status: "completed",
        error_text: "",
        created_at: now,
        updated_at: now
      },
      {
        id: tmpAssistantID,
        task_id: nextTaskID,
        role: "assistant",
        content: "",
        status: "running",
        error_text: "",
        created_at: now,
        updated_at: now
      }
    ];
    messagesHydratedByTaskID[nextTaskID] = true;

    let stopPolling = false;
    const pollLoop = (async () => {
      while (!stopPolling) {
        await loadTaskMessages(nextTaskID, true);
        if (!stopPolling) {
          await new Promise((resolve) => setTimeout(resolve, 80));
        }
      }
    })();

    let sendRes: APIResponse<{ task_id: string }> = { ok: false, data: { task_id: nextTaskID } };
    let sendErr: Error | null = null;
    try {
      sendRes = (await fetchImpl(apiURL(`/api/v1/tasks/${nextTaskID}/messages`), {
        method: "POST",
        headers: apiHeaders({ "Content-Type": "application/json" }),
        body: JSON.stringify({ content: nextContent })
      }).then((r) => r.json())) as APIResponse<{ task_id: string }>;
    } catch (err) {
      sendErr = err instanceof Error ? err : new Error("TASK_MESSAGE_SEND_FAILED");
    } finally {
      stopPolling = true;
      await pollLoop;
      await loadTaskMessages(nextTaskID, true);
    }
    if (sendErr) {
      throw sendErr;
    }
    if (!sendRes.ok) {
      throw new Error(sendRes.error?.code || "TASK_MESSAGE_SEND_FAILED");
    }
  }

  async function stopTaskMessage(taskId: string): Promise<boolean> {
    const nextTaskID = String(taskId ?? "").trim();
    if (!nextTaskID) {
      throw new Error("INVALID_TASK_ID");
    }
    const res = (await fetchImpl(apiURL(`/api/v1/tasks/${nextTaskID}/messages/stop`), {
      method: "POST",
      headers: apiHeaders()
    }).then((r) => r.json())) as APIResponse<{ task_id?: string; canceled?: boolean }>;
    if (!res.ok) {
      throw new Error(res.error?.code || "TASK_MESSAGE_STOP_FAILED");
    }
    return Boolean(res.data?.canceled);
  }

  async function loadTaskSidecarMode(taskId: string, force = false): Promise<SidecarMode> {
    const nextTaskID = String(taskId ?? "").trim();
    if (!nextTaskID) {
      return "advisor";
    }
    if (!force && typeof state.taskSidecarModeByTaskId[nextTaskID] === "string") {
      return state.taskSidecarModeByTaskId[nextTaskID] as SidecarMode;
    }
    const res = (await fetchImpl(apiURL(`/api/v1/tasks/${nextTaskID}/sidecar-mode`), {
      headers: apiHeaders()
    }).then((r) => r.json())) as APIResponse<{ task_id?: string; sidecar_mode?: string }>;
    if (!res.ok) {
      throw new Error(res.error?.code || "TASK_SIDECAR_MODE_LOAD_FAILED");
    }
    const raw = String(res.data?.sidecar_mode ?? "").trim();
    const mode: SidecarMode = raw === "observer" || raw === "autopilot" ? raw : "advisor";
    state.taskSidecarModeByTaskId[nextTaskID] = mode;
    return mode;
  }

  async function setTaskSidecarMode(taskId: string, mode: SidecarMode): Promise<SidecarMode> {
    const nextTaskID = String(taskId ?? "").trim();
    if (!nextTaskID) {
      throw new Error("INVALID_TASK_ID");
    }
    const nextMode: SidecarMode = mode === "observer" || mode === "autopilot" ? mode : "advisor";
    const res = (await fetchImpl(apiURL(`/api/v1/tasks/${nextTaskID}/sidecar-mode`), {
      method: "PATCH",
      headers: apiHeaders({ "Content-Type": "application/json" }),
      body: JSON.stringify({ sidecar_mode: nextMode })
    }).then((r) => r.json())) as APIResponse<{ task_id?: string; sidecar_mode?: string }>;
    if (!res.ok) {
      throw new Error(res.error?.code || "TASK_SIDECAR_MODE_UPDATE_FAILED");
    }
    const raw = String(res.data?.sidecar_mode ?? "").trim();
    const next: SidecarMode = raw === "observer" || raw === "autopilot" ? raw : "advisor";
    state.taskSidecarModeByTaskId[nextTaskID] = next;
    return next;
  }

  async function submitTaskCommit(taskId: string, message: string) {
    const nextMessage = message.trim();
    if (!taskId || !nextMessage) {
      throw new Error("INVALID_COMMIT_MESSAGE");
    }
    const res = (await fetchImpl(apiURL(`/api/v1/tasks/${taskId}/commit`), {
      method: "POST",
      headers: apiHeaders({ "Content-Type": "application/json" }),
      body: JSON.stringify({ message: nextMessage })
    }).then((r) => r.json())) as APIResponse<{ task_id: string; commit_hash: string; message: string }>;
    if (!res.ok) {
      throw new Error(res.error?.code || "TASK_COMMIT_FAILED");
    }
    return {
      taskId: String(res.data?.task_id ?? taskId),
      commitHash: String(res.data?.commit_hash ?? ""),
      message: String(res.data?.message ?? nextMessage)
    };
  }

  async function reopenPaneForTask(taskId: string, options: ReopenPaneOptions = {}) {
    const res = (await fetchImpl(apiURL(`/api/v1/tasks/${taskId}/panes/reopen`), {
      method: "POST",
      headers: apiHeaders({ "Content-Type": "application/json" })
    }).then((r) => r.json())) as APIResponse<{ task_id: string; pane_uuid?: string; pane_id: string; pane_target?: string }>;
    if (!res.ok || !res.data?.task_id || !res.data?.pane_id) {
      throw new Error(String(res.error?.message ?? "REOPEN_PANE_FAILED"));
    }
    state.paneByTaskId[res.data.task_id] = {
      paneUuid: String(res.data.pane_uuid ?? res.data.pane_id),
      paneId: res.data.pane_id,
      paneTarget: String(res.data.pane_target ?? res.data.pane_id),
      relation: "root"
    };
    paneLookupStatus[res.data.task_id] = "bound";
    await selectTask(res.data.task_id);

    const program = normalizeLaunchProgram(options.program);
    const prompt = String(options.prompt ?? "").trim();
    if (prompt) {
      const inferredTitle = deriveTitleFromPrompt(prompt);
      if (inferredTitle) {
        try {
          await setTaskTitle(taskId, inferredTitle);
        } catch {
          logInfo("shellman.task.title.auto_from_prompt.error", { taskId });
        }
      }
    }
    if (program !== "shell") {
      const command = prompt ? `${program} ${shellQuote(prompt)}\r` : `${program}\r`;
      sendTerminalInput(command);
    }
  }

  async function reportRunResult(runId: string, summary: string) {
    const nextRunID = String(runId ?? "").trim();
    const nextSummary = String(summary ?? "").trim();
    if (!nextRunID) {
      throw new Error("INVALID_RUN_ID");
    }
    const requestID = nextRequestID();
    const res = (await fetchImpl(apiURL(`/api/v1/runs/${encodeURIComponent(nextRunID)}/report-result`), {
      method: "POST",
      headers: apiHeaders({ "Content-Type": "application/json" }),
      body: JSON.stringify({ summary: nextSummary, request_id: requestID })
    }).then((r) => r.json())) as APIResponse<{ run_id?: string; status?: string; idempotent?: boolean; request_id?: string }>;
    if (!res.ok) {
      throw new Error(res.error?.code || "RUN_REPORT_RESULT_FAILED");
    }
    return {
      runId: String(res.data?.run_id ?? nextRunID),
      status: String(res.data?.status ?? "completed"),
      idempotent: Boolean(res.data?.idempotent),
      requestId: String(res.data?.request_id ?? requestID)
    };
  }

  return {
    state,
    load,
    connectWS,
    disconnectWS,
    selectTask,
    sendTerminalInput,
    sendTerminalResize,
    loadMorePaneHistory,
    sendImagePasteToTerminal,
    createRootTask,
    createRootPane,
    createSiblingPane,
    createChildTask,
    createChildPane,
    addActiveProject,
    archiveDoneTasksByProject,
    removeActiveProject,
    selectDirectory,
    getFSRoots,
    listDirectories,
    resolveDirectory,
    searchDirectories,
    getDirectoryHistory,
    recordDirectoryHistory,
    loadAppPrograms,
    loadConfig,
    saveDefaultLaunchProgram,
    saveTaskCompletionSettings,
    setTaskChecked,
    setTaskTitle,
    setTaskDescription,
    markTaskFlagReaded,
    loadTaskMessages,
    loadTaskSidecarMode,
    sendTaskMessage,
    stopTaskMessage,
    setTaskSidecarMode,
    submitTaskCommit,
    reportRunResult,
    getOrphanPaneItems,
    adoptPaneAsChild,
    reopenPaneForTask
  };
}
