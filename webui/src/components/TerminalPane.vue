<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, ref, watch } from "vue";
import { useI18n } from "vue-i18n";
import { Terminal } from "@xterm/xterm";
import { FitAddon } from "@xterm/addon-fit";
import { CanvasAddon } from "@xterm/addon-canvas";
import { Card, CardContent } from "@/components/ui/card";
import TaskHeader from "@/components/TaskHeader.vue";
import PaneLaunchForm from "@/components/PaneLaunchForm.vue";
const { t } = useI18n();

type LaunchProgram = "shell" | "codex" | "claude" | "cursor";

const props = defineProps<{
  frame?: { mode: "reset" | "append"; data: string } | null;
  cursor?: { x: number; y: number } | null;
  taskId?: string;
  taskTitle?: string;
  taskDescription?: string;
  paneUuid?: string;
  currentCommand?: string;
  isEnded?: boolean;
  showReopenButton?: boolean;
  isNoPaneTask?: boolean;
  defaultLaunchProgram?: LaunchProgram;
  appPrograms?: Array<{ id: "codex" | "claude" | "cursor"; display_name: string; command: string }>;
}>();
const emit = defineEmits<{
  (event: "terminal-input", text: string): void;
  (event: "terminal-resize", size: { cols: number; rows: number }): void;
  (event: "terminal-history-more"): void;
  (event: "terminal-image-paste", file: File): void;
  (event: "reopen-pane", payload: { program: LaunchProgram; prompt?: string }): void;
  (event: "open-session-detail"): void;
}>();

const root = ref<HTMLElement | null>(null);
const term = new Terminal({ convertEol: true, scrollback: 10000 });
const canvasAddon = new CanvasAddon();
const fitAddon = new FitAddon();
const opened = ref(false);
const lastSize = ref<{ cols: number; rows: number } | null>(null);
let resizeObserver: ResizeObserver | null = null;
let resizeRAF = 0;
let scrollRAF = 0;
let fitRetryCount = 0;
let taskSwitchRetryTimer: number | null = null;
let viewWriteSeq = 0;
let onDataSeq = 0;
let cursorMoveSeq = 0;
let terminalInput: HTMLTextAreaElement | null = null;
let terminalInputPasteHandler: ((event: ClipboardEvent) => void) | null = null;
const launchSubmitLabel = computed(() => (props.isNoPaneTask ? t("terminal.start") : t("terminal.reopen")));

interface BufferSnapshot {
  cursorX: number | null;
  cursorY: number | null;
  viewportY: number | null;
  baseY: number | null;
  length: number | null;
  cols: number | null;
  rows: number | null;
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

function readBufferSnapshot(): BufferSnapshot {
  const view = term as unknown as {
    cols?: number;
    rows?: number;
    buffer?: {
      active?: {
        cursorX?: number;
        cursorY?: number;
        viewportY?: number;
        baseY?: number;
        length?: number;
      };
    };
  };
  const active = view.buffer?.active;
  return {
    cursorX: typeof active?.cursorX === "number" ? active.cursorX : null,
    cursorY: typeof active?.cursorY === "number" ? active.cursorY : null,
    viewportY: typeof active?.viewportY === "number" ? active.viewportY : null,
    baseY: typeof active?.baseY === "number" ? active.baseY : null,
    length: typeof active?.length === "number" ? active.length : null,
    cols: typeof view.cols === "number" ? view.cols : null,
    rows: typeof view.rows === "number" ? view.rows : null
  };
}

function withSnapshot(prefix: string, s: BufferSnapshot): Record<string, number | null> {
  return {
    [`${prefix}CursorX`]: s.cursorX,
    [`${prefix}CursorY`]: s.cursorY,
    [`${prefix}ViewportY`]: s.viewportY,
    [`${prefix}BaseY`]: s.baseY,
    [`${prefix}Length`]: s.length,
    [`${prefix}Cols`]: s.cols,
    [`${prefix}Rows`]: s.rows
  };
}

function logInfo(event: string, payload: Record<string, unknown> = {}) {
  const g = globalThis as typeof globalThis & {
    __SHELLMAN_TERM_DEBUG__?: boolean;
    __SHELLMAN_TERM_DEBUG_LOGS__?: Array<{ event: string; payload: Record<string, unknown> }>;
  };
  if (g.__SHELLMAN_TERM_DEBUG__ !== true) {
    return;
  }
  if (!Array.isArray(g.__SHELLMAN_TERM_DEBUG_LOGS__)) {
    g.__SHELLMAN_TERM_DEBUG_LOGS__ = [];
  }
  g.__SHELLMAN_TERM_DEBUG_LOGS__.push({ event, payload });
  if (g.__SHELLMAN_TERM_DEBUG_LOGS__.length > 600) {
    g.__SHELLMAN_TERM_DEBUG_LOGS__.shift();
  }
  console.info("[shellman-term]", event, payload);
}

function readRendererState() {
  const core = term as unknown as {
    _core?: {
      _renderService?: {
        _renderer?: {
          value?: { constructor?: { name?: string } };
        };
      };
    };
  };
  const rendererName = core._core?._renderService?._renderer?.value?.constructor?.name ?? null;
  const canvasCount = root.value?.querySelectorAll(".xterm-screen canvas").length ?? 0;
  return { rendererName, canvasCount };
}

function tryEnableCanvasRenderer(addon: CanvasAddon, attempt: "initial" | "retry") {
  try {
    term.loadAddon(addon);
  } catch (error) {
    console.warn("[shellman] failed to enable xterm canvas renderer, fallback to DOM renderer", { attempt, error });
    return false;
  }
  const state = readRendererState();
  const enabled = state.rendererName === "CanvasRenderer" || state.canvasCount > 0;
  if (!enabled) {
    console.warn("[shellman] xterm canvas addon loaded but renderer is not canvas", { attempt, ...state });
  }
  return enabled;
}

function scheduleTerminalSizeSync(forceEmit = false) {
  if (resizeRAF) {
    cancelAnimationFrame(resizeRAF);
  }
  resizeRAF = requestAnimationFrame(() => {
    resizeRAF = 0;
    if (forceEmit) {
      lastSize.value = null;
    }
    syncTerminalSize();
  });
}

const handleWindowResize = () => {
  scheduleTerminalSizeSync();
};

function scheduleScrollToBottom(reason: string) {
  if (scrollRAF) {
    cancelAnimationFrame(scrollRAF);
  }
  scrollRAF = requestAnimationFrame(() => {
    scrollRAF = 0;
    if (!opened.value) {
      logInfo("shellman.term.view.scroll_bottom.skip", { reason: "terminal-not-opened", trigger: reason });
      return;
    }
    const terminal = term as unknown as { scrollToBottom?: () => void };
    terminal.scrollToBottom?.();
    moveCursor(props.cursor ?? null);
    logInfo("shellman.term.view.scroll_bottom", { trigger: reason });
  });
}

function isTerminalVisible() {
  return Boolean(root.value && root.value.offsetParent !== null);
}

function focusTerminal(reason: string) {
  if (!opened.value) {
    logInfo("shellman.term.view.focus.skip", { reason: "terminal-not-opened", trigger: reason });
    return;
  }
  if (!isTerminalVisible()) {
    logInfo("shellman.term.view.focus.skip", { reason: "terminal-hidden", trigger: reason });
    return;
  }
  if (props.isNoPaneTask || props.isEnded) {
    logInfo("shellman.term.view.focus.skip", {
      reason: props.isNoPaneTask ? "no-pane-task" : "pane-ended",
      trigger: reason
    });
    return;
  }
  term.focus();
  const input = terminalInput ?? (root.value?.querySelector("textarea.xterm-helper-textarea") as HTMLTextAreaElement | null);
  if (input && document.activeElement !== input) {
    input.focus({ preventScroll: true });
  }
  logInfo("shellman.term.view.focus.apply", { trigger: reason });
}

function syncTerminalSize() {
  if (!opened.value || !root.value) {
    logInfo("shellman.term.view.resize.skip", { reason: "not-opened-or-no-root" });
    return;
  }

  const width = root.value.clientWidth;
  const height = root.value.clientHeight;
  if (width <= 0 || height <= 0) {
    logInfo("shellman.term.view.resize.skip", { reason: "invalid-viewport", width, height });
    return;
  }

  try {
    fitAddon.fit();
  } catch {
    logInfo("shellman.term.view.resize.skip", { reason: "fit-failed", width, height });
    if (fitRetryCount < 20) {
      fitRetryCount += 1;
      scheduleTerminalSizeSync();
    }
    return;
  }
  fitRetryCount = 0;

  const actualSize = { cols: Math.max(2, term.cols), rows: Math.max(2, term.rows) };
  if (lastSize.value && lastSize.value.cols === actualSize.cols && lastSize.value.rows === actualSize.rows) {
    logInfo("shellman.term.view.resize.skip", { reason: "same-size", cols: actualSize.cols, rows: actualSize.rows });
    return;
  }
  lastSize.value = actualSize;
  logInfo("shellman.term.view.resize.emit", {
    width,
    height,
    cols: actualSize.cols,
    rows: actualSize.rows
  });
  emit("terminal-resize", actualSize);
  moveCursor(props.cursor ?? null);
}

function moveCursor(cursor: { x: number; y: number } | null | undefined) {
  if (!cursor || !opened.value) {
    logInfo("shellman.term.view.cursor.skip", {
      reason: !cursor ? "cursor-empty" : "terminal-not-opened"
    });
    return;
  }
  const maxRows = Math.max(1, term.rows || 1);
  const maxCols = Math.max(1, term.cols || 1);
  const row = Math.max(1, Math.min(maxRows, cursor.y + 1));
  const col = Math.max(1, Math.min(maxCols, cursor.x + 1));
  const seq = ++cursorMoveSeq;
  const before = readBufferSnapshot();
  logInfo("shellman.term.view.cursor.move.before", {
    seq,
    x: cursor.x,
    y: cursor.y,
    row,
    col,
    ...withSnapshot("bufferBefore", before)
  });
  term.write(`\u001b[${row};${col}H`, () => {
    const after = readBufferSnapshot();
    logInfo("shellman.term.view.cursor.move.after", {
      seq,
      x: cursor.x,
      y: cursor.y,
      row,
      col,
      ...withSnapshot("bufferAfter", after)
    });
  });
}

function writeOutput(text: string, onDone?: () => void) {
  const seq = ++viewWriteSeq;
  const before = readBufferSnapshot();
  logInfo("shellman.term.view.output.write.before", {
    seq,
    dataLen: text.length,
    dataEscaped: escapeForLog(text),
    ...withSnapshot("bufferBefore", before)
  });
  term.write(text, () => {
    const after = readBufferSnapshot();
    logInfo("shellman.term.view.output.write.after", {
      seq,
      dataLen: text.length,
      dataEscaped: escapeForLog(text),
      ...withSnapshot("bufferAfter", after)
    });
    onDone?.();
  });
}

function normalizeResetFrameData(text: string) {
  // tmux capture payload usually ends with a terminal newline marker; rendering it
  // as content shifts viewport by one row before cursor restore on task switch.
  return text.endsWith("\n") ? text.slice(0, -1) : text;
}

function onPaste(ev: ClipboardEvent) {
  if (!ev.clipboardData) {
    return;
  }
  const items = Array.from(ev.clipboardData.items ?? []);
  const imageItem = items.find((item) => item.kind === "file" && item.type.startsWith("image/"));
  if (!imageItem) {
    return;
  }
  const imageFile = imageItem.getAsFile();
  if (!imageFile) {
    return;
  }
  ev.preventDefault();
  emit("terminal-image-paste", imageFile);
}

function bindPasteHandler(input: HTMLTextAreaElement | null) {
  if (!input) {
    return;
  }
  const handler = onPaste;
  terminalInput = input;
  terminalInputPasteHandler = handler;
  input.addEventListener("paste", handler);
}

function syncTerminalInputDisabled() {
  if (!terminalInput) {
    return;
  }
  const disabled = Boolean(props.isEnded);
  terminalInput.disabled = disabled;
  terminalInput.readOnly = disabled;
  terminalInput.setAttribute("aria-disabled", disabled ? "true" : "false");
}

function unbindPasteHandler() {
  if (!terminalInput || !terminalInputPasteHandler) {
    return;
  }
  terminalInput.removeEventListener("paste", terminalInputPasteHandler);
  terminalInput = null;
  terminalInputPasteHandler = null;
}

defineExpose({ writeOutput });

watch(
  () => props.frame ?? null,
  (next) => {
    if (!next) {
      return;
    }
    const rawText = next.data ?? "";
    const text = next.mode === "reset" ? normalizeResetFrameData(rawText) : rawText;
    logInfo("shellman.term.view.frame.watch", { mode: next.mode, dataLen: text.length, rawDataLen: rawText.length });
    if (next.mode === "reset") {
      const beforeReset = readBufferSnapshot();
      (term as unknown as { reset?: () => void }).reset?.();
      const afterReset = readBufferSnapshot();
      logInfo("shellman.term.view.frame.reset", {
        dataLen: text.length,
        ...withSnapshot("bufferBefore", beforeReset),
        ...withSnapshot("bufferAfter", afterReset)
      });
      if (text) {
        writeOutput(text, () => {
          moveCursor(props.cursor ?? null);
          scheduleScrollToBottom("frame-reset");
        });
      } else {
        moveCursor(props.cursor ?? null);
        scheduleScrollToBottom("frame-reset-empty");
      }
      return;
    }

    if (text) {
      writeOutput(text, () => {
        moveCursor(props.cursor ?? null);
      });
    } else {
      moveCursor(props.cursor ?? null);
    }
    logInfo("shellman.term.view.frame.append", { dataLen: text.length });
  }
);

watch(
  () => props.cursor ?? null,
  (next) => {
    moveCursor(next);
  }
);

watch(
  () => Boolean(props.isEnded),
  (next) => {
    logInfo("shellman.term.view.ended.watch", { ended: next });
    syncTerminalInputDisabled();
  },
  { immediate: true }
);

watch(
  () => props.taskId ?? "",
  (next, prev) => {
    if (!opened.value || !next || next === prev) {
      return;
    }
    logInfo("shellman.term.view.task.switch", { fromTaskId: prev, toTaskId: next });
    scheduleTerminalSizeSync(true);
    scheduleScrollToBottom("task-switch");
    focusTerminal("task-switch");
    if (taskSwitchRetryTimer !== null) {
      window.clearTimeout(taskSwitchRetryTimer);
    }
    taskSwitchRetryTimer = window.setTimeout(() => {
      taskSwitchRetryTimer = null;
      scheduleTerminalSizeSync(true);
      scheduleScrollToBottom("task-switch-retry");
      focusTerminal("task-switch-retry");
    }, 120);
  }
);

watch(
  () => Boolean(props.isNoPaneTask),
  (next, prev) => {
    if (prev && !next && opened.value) {
      scheduleTerminalSizeSync(true);
    }
  }
);

onMounted(() => {
  logInfo("shellman.term.view.mounted.start");
  if (root.value) {
    if (typeof window.matchMedia !== "function") {
      logInfo("shellman.term.view.mounted.skip", { reason: "matchMedia-missing" });
      return;
    }
    term.open(root.value);
    {
      const g = globalThis as typeof globalThis & {
        __SHELLMAN_TERM_DEBUG__?: boolean;
        __SHELLMAN_TERM_INSTANCES__?: unknown[];
      };
      if (g.__SHELLMAN_TERM_DEBUG__ === true) {
        if (!Array.isArray(g.__SHELLMAN_TERM_INSTANCES__)) {
          g.__SHELLMAN_TERM_INSTANCES__ = [];
        }
        g.__SHELLMAN_TERM_INSTANCES__.push(term);
      }
    }
    let canvasEnabled = tryEnableCanvasRenderer(canvasAddon, "initial");
    if (!canvasEnabled) {
      canvasEnabled = tryEnableCanvasRenderer(new CanvasAddon(), "retry");
    }
    const finalRendererState = readRendererState();
    root.value.dataset.renderer = canvasEnabled ? "canvas" : "dom";
    if (!canvasEnabled) {
      console.warn("[shellman] xterm renderer remains non-canvas after retry", finalRendererState);
    }
    term.loadAddon(fitAddon);
    opened.value = true;
    logInfo("shellman.term.view.opened");
    const input = root.value?.querySelector("textarea.xterm-helper-textarea") as HTMLTextAreaElement | null;
    if (input) {
      input.setAttribute("data-test-id", "tt-terminal-input");
      logInfo("shellman.term.view.input.ready");
      bindPasteHandler(input);
      syncTerminalInputDisabled();
    }
    const terminal = term as unknown as {
      onData?: (handler: (data: string) => void) => void;
      onScroll?: (handler: (y: number) => void) => void;
      attachCustomKeyEventHandler?: (handler: (ev: KeyboardEvent) => boolean) => void;
    };
    terminal.attachCustomKeyEventHandler?.((ev: KeyboardEvent) => {
      if (
        ev.type === "keydown" &&
        ev.key === "Enter" &&
        ev.shiftKey &&
        !ev.altKey &&
        !ev.ctrlKey &&
        !ev.metaKey &&
        !ev.isComposing
      ) {
        ev.preventDefault();
        logInfo("shellman.term.view.key.shift_enter", { action: "emit-lf" });
        emit("terminal-input", "\n");
        return false;
      }
      return true;
    });
    terminal.onData?.((data: string) => {
      const seq = ++onDataSeq;
      logInfo("shellman.term.view.on_data", {
        seq,
        text: data,
        textLen: data.length,
        textEscaped: escapeForLog(data)
      });
      emit("terminal-input", data);
    });
    terminal.onScroll?.((y: number) => {
      if (typeof y === "number" && y <= 0) {
        emit("terminal-history-more");
      }
    });
    scheduleTerminalSizeSync(true);
    window.addEventListener("resize", handleWindowResize);
    logInfo("shellman.term.view.resize_listener.added");
    if (typeof ResizeObserver !== "undefined") {
      resizeObserver = new ResizeObserver(() => scheduleTerminalSizeSync());
      resizeObserver.observe(root.value);
      logInfo("shellman.term.view.resize_observer.added");
    }
  }
});

onBeforeUnmount(() => {
  logInfo("shellman.term.view.before_unmount");
  unbindPasteHandler();
  window.removeEventListener("resize", handleWindowResize);
  if (resizeRAF) {
    cancelAnimationFrame(resizeRAF);
    resizeRAF = 0;
  }
  if (scrollRAF) {
    cancelAnimationFrame(scrollRAF);
    scrollRAF = 0;
  }
  if (taskSwitchRetryTimer !== null) {
    window.clearTimeout(taskSwitchRetryTimer);
    taskSwitchRetryTimer = null;
  }
  if (resizeObserver) {
    resizeObserver.disconnect();
    resizeObserver = null;
    logInfo("shellman.term.view.resize_observer.removed");
  }
});
</script>

<template>
  <Card :class="['h-full border-none shadow-none bg-black/95 flex flex-col', { 'terminal-ended': Boolean(props.isEnded) }]">
    <TaskHeader
      :task-id="taskId"
      :task-title="taskTitle"
      :task-description="taskDescription"
      :pane-uuid="paneUuid"
      :current-command="currentCommand"
      @open-session-detail="() => emit('open-session-detail')"
    />
    <CardContent v-show="!props.isNoPaneTask" class="p-1 flex-1 min-h-0 relative">
       <div data-test-id="tt-terminal-root" ref="root" class="w-full h-full" />
    </CardContent>
    <div
      v-if="props.showReopenButton && !props.isNoPaneTask"
      class="px-3 py-2 border-t border-border/10 bg-muted/5"
    >
      <PaneLaunchForm
        :submit-label="launchSubmitLabel"
        :default-program="props.defaultLaunchProgram ?? 'shell'"
        :providers="props.appPrograms ?? []"
        @submit="(payload) => emit('reopen-pane', payload)"
      />
    </div>
    <div
      v-if="props.showReopenButton && props.isNoPaneTask"
      class="px-3 py-3 border-t border-border/10 bg-muted/5"
      >
      <PaneLaunchForm
        :submit-label="launchSubmitLabel"
        :default-program="props.defaultLaunchProgram ?? 'shell'"
        :providers="props.appPrograms ?? []"
        @submit="(payload) => emit('reopen-pane', payload)"
      />
    </div>
  </Card>
</template>

<style scoped>
.terminal-ended :deep(.xterm-rows) {
  color: #9ca3af !important;
}

.terminal-ended :deep(.xterm-rows span) {
  color: #9ca3af !important;
  background-color: transparent !important;
}

.terminal-ended :deep(.xterm-cursor) {
  display: none !important;
}


.terminal-ended :deep(.xterm-cursor) {
  display: none !important;
}
</style>
