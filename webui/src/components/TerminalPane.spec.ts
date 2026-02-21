import { afterEach, describe, expect, it, vi } from "vitest";
import { mount } from "@vue/test-utils";

let terminalOptions: unknown;
let writes: string[] = [];
let resized: Array<{ cols: number; rows: number }> = [];
let resetCalls = 0;
let scrollToBottomCalls = 0;
let customKeyEventHandler: ((ev: KeyboardEvent) => boolean) | null = null;
let deferWriteCallback = false;

vi.mock("@xterm/xterm", () => ({
  Terminal: class {
    cols = 80;
    rows = 24;
    constructor(options?: unknown) {
      terminalOptions = options;
    }
    open(el: HTMLElement) {
      const input = document.createElement("textarea");
      input.className = "xterm-helper-textarea";
      input.style.width = "9px";
      input.style.height = "18px";
      el.appendChild(input);
    }
    write(text: string, callback?: () => void) {
      writes.push(text);
      if (callback) {
        if (deferWriteCallback) {
          setTimeout(callback, 0);
        } else {
          callback();
        }
      }
    }
    loadAddon() {}
    reset() {
      resetCalls += 1;
    }
    scrollToBottom() {
      scrollToBottomCalls += 1;
    }
    resize(cols: number, rows: number) {
      this.cols = cols;
      this.rows = rows;
      resized.push({ cols, rows });
    }
    attachCustomKeyEventHandler(handler: (ev: KeyboardEvent) => boolean) {
      customKeyEventHandler = handler;
    }
    onData() {}
  }
}));

import TerminalPane from "./TerminalPane.vue";

afterEach(() => {
  document.body.innerHTML = "";
  deferWriteCallback = false;
});

describe("TerminalPane", () => {
  it("forwards open-session-detail from header", async () => {
    terminalOptions = undefined;
    writes = [];
    resized = [];
    resetCalls = 0;
    Object.defineProperty(window, "matchMedia", {
      writable: true,
      value: () => ({ matches: false, addEventListener() {}, removeEventListener() {} })
    });
    const wrapper = mount(TerminalPane, {
      props: {
        taskTitle: "T"
      }
    });

    await wrapper.get("[data-test-id='muxt-task-meta-display']").trigger("click");
    expect(wrapper.emitted("open-session-detail")?.length).toBe(1);
  });

  it("renders terminal root", () => {
    terminalOptions = undefined;
    writes = [];
    resized = [];
    resetCalls = 0;
    customKeyEventHandler = null;
    Object.defineProperty(window, "matchMedia", {
      writable: true,
      value: () => ({ matches: false, addEventListener() {}, removeEventListener() {} })
    });
    const wrapper = mount(TerminalPane);
    expect(wrapper.find("[data-test-id='tt-terminal-root']").exists()).toBe(true);
    expect(wrapper.find("[data-test-id='tt-terminal-input']").exists()).toBe(true);
    expect(wrapper.find("[data-test-id='tt-terminal-output']").exists()).toBe(false);
  });

  it("shows pane uuid in console header", () => {
    terminalOptions = undefined;
    writes = [];
    resized = [];
    resetCalls = 0;
    Object.defineProperty(window, "matchMedia", {
      writable: true,
      value: () => ({ matches: false, addEventListener() {}, removeEventListener() {} })
    });
    const wrapper = mount(TerminalPane, {
      props: {
        paneUuid: "pane-uuid-1"
      }
    });
    expect(wrapper.get("[data-test-id='muxt-console-pane-uuid']").text()).toBe("pane-uuid-1");
  });

  it("shows current command in console header", () => {
    terminalOptions = undefined;
    writes = [];
    resized = [];
    resetCalls = 0;
    Object.defineProperty(window, "matchMedia", {
      writable: true,
      value: () => ({ matches: false, addEventListener() {}, removeEventListener() {} })
    });
    const wrapper = mount(TerminalPane, {
      props: {
        currentCommand: "toolkit"
      }
    });
    expect(wrapper.get("[data-test-id='muxt-console-pane-current-command']").text()).toBe("toolkit");
  });

  it("shows reopen button and emits reopen-pane with launch options", async () => {
    terminalOptions = undefined;
    writes = [];
    resized = [];
    resetCalls = 0;
    Object.defineProperty(window, "matchMedia", {
      writable: true,
      value: () => ({ matches: false, addEventListener() {}, removeEventListener() {} })
    });
    const wrapper = mount(TerminalPane, {
      props: {
        showReopenButton: true
      }
    });
    await wrapper.get("[data-test-id='muxt-pane-launch-form']").trigger("submit");
    const emitted = wrapper.emitted("reopen-pane") ?? [];
    expect(emitted.length).toBe(1);
    expect((emitted[0] ?? [])[0]).toMatchObject({ program: "shell" });
  });

  it("hides xterm and shows Start button for no-pane task", () => {
    terminalOptions = undefined;
    writes = [];
    resized = [];
    resetCalls = 0;
    Object.defineProperty(window, "matchMedia", {
      writable: true,
      value: () => ({ matches: false, addEventListener() {}, removeEventListener() {} })
    });
    const wrapper = mount(TerminalPane, {
      props: {
        showReopenButton: true,
        isNoPaneTask: true
      }
    });
    const root = wrapper.get("[data-test-id='tt-terminal-root']");
    expect(root.exists()).toBe(true);
    const container = root.element.parentElement as HTMLElement;
    expect(container.style.display).toBe("none");
    expect(wrapper.get("[data-test-id='muxt-reopen-pane-button']").text()).toBe("Start");
  });

  it("enables convertEol so plain \\n output wraps correctly", () => {
    terminalOptions = undefined;
    writes = [];
    resized = [];
    resetCalls = 0;
    Object.defineProperty(window, "matchMedia", {
      writable: true,
      value: () => ({ matches: false, addEventListener() {}, removeEventListener() {} })
    });
    mount(TerminalPane);
    expect(terminalOptions).toMatchObject({ convertEol: true });
  });

  it("repositions cursor from cursor prop", async () => {
    terminalOptions = undefined;
    writes = [];
    resized = [];
    resetCalls = 0;
    Object.defineProperty(window, "matchMedia", {
      writable: true,
      value: () => ({ matches: false, addEventListener() {}, removeEventListener() {} })
    });
    const wrapper = mount(TerminalPane);
    await wrapper.setProps({ frame: { mode: "reset", data: "bash-5.3$ " }, cursor: { x: 2, y: 1 } });
    expect(writes).toContain("\u001b[2;3H");
  });

  it("applies cursor after append output with trailing spaces", async () => {
    terminalOptions = undefined;
    writes = [];
    resized = [];
    resetCalls = 0;
    Object.defineProperty(window, "matchMedia", {
      writable: true,
      value: () => ({ matches: false, addEventListener() {}, removeEventListener() {} })
    });
    const wrapper = mount(TerminalPane);
    const prompt = `\u001b[1m\u001b[32m➜\u001b[0m prompt${" ".repeat(16)}`;

    await wrapper.setProps({ frame: { mode: "reset", data: prompt }, cursor: { x: 7, y: 0 } });
    await wrapper.setProps({
      frame: { mode: "append", data: `\n${prompt}` },
      cursor: { x: 7, y: 1 }
    });

    expect(writes).toContain(prompt);
    expect(writes).toContain(`\n${prompt}`);
    expect(writes).toContain("\u001b[1;8H");
    expect(writes).toContain("\u001b[2;8H");
    expect(writes[writes.length - 1]).toBe("\u001b[2;8H");
  });

  it("applies reset frame once then appends without full rewrite reset", async () => {
    terminalOptions = undefined;
    writes = [];
    resized = [];
    resetCalls = 0;
    Object.defineProperty(window, "matchMedia", {
      writable: true,
      value: () => ({ matches: false, addEventListener() {}, removeEventListener() {} })
    });
    const wrapper = mount(TerminalPane);

    await wrapper.setProps({ frame: { mode: "reset", data: "bash-5.3$ " } });
    await wrapper.setProps({ frame: { mode: "append", data: "ls\r\n" } });

    expect(writes).toContain("bash-5.3$ ");
    expect(writes).toContain("ls\r\n");
    expect(resetCalls).toBe(1);
  });

  it("re-applies cursor after reset even when cursor value does not change", async () => {
    terminalOptions = undefined;
    writes = [];
    resized = [];
    resetCalls = 0;
    deferWriteCallback = true;
    Object.defineProperty(window, "matchMedia", {
      writable: true,
      value: () => ({ matches: false, addEventListener() {}, removeEventListener() {} })
    });
    const wrapper = mount(TerminalPane);

    await wrapper.setProps({ cursor: { x: 2, y: 1 } });
    await wrapper.setProps({ frame: { mode: "reset", data: "bash-5.3$ " } });
    await new Promise((resolve) => setTimeout(resolve, 10));

    expect(writes[writes.length - 1]).toBe("\u001b[2;3H");
  });

  it("emits terminal-resize when viewport size changes", async () => {
    terminalOptions = undefined;
    writes = [];
    resized = [];
    resetCalls = 0;
    Object.defineProperty(window, "matchMedia", {
      writable: true,
      value: () => ({ matches: false, addEventListener() {}, removeEventListener() {} })
    });
    const wrapper = mount(TerminalPane);
    const root = wrapper.get("[data-test-id='tt-terminal-root']").element as HTMLElement;
    Object.defineProperty(root, "clientWidth", { configurable: true, value: 720 });
    Object.defineProperty(root, "clientHeight", { configurable: true, value: 360 });

    window.dispatchEvent(new Event("resize"));
    await new Promise((resolve) => setTimeout(resolve, 30));
    await wrapper.vm.$nextTick();

    const emitted = wrapper.emitted("terminal-resize") ?? [];
    expect(emitted.length).toBeGreaterThan(0);
  });

  it("emits terminal-image-paste when clipboard has image", async () => {
    terminalOptions = undefined;
    writes = [];
    resized = [];
    resetCalls = 0;
    Object.defineProperty(window, "matchMedia", {
      writable: true,
      value: () => ({ matches: false, addEventListener() {}, removeEventListener() {} })
    });
    const wrapper = mount(TerminalPane, { attachTo: document.body });
    const input = wrapper.get("textarea.xterm-helper-textarea").element as HTMLTextAreaElement;
    const file = new File([new Uint8Array([1, 2, 3])], "x.png", { type: "image/png" });
    const dt = {
      items: [{ kind: "file", type: "image/png", getAsFile: () => file }]
    };

    const ev = new Event("paste", { bubbles: true, cancelable: true }) as ClipboardEvent & { clipboardData: typeof dt };
    Object.defineProperty(ev, "clipboardData", { value: dt });
    input.dispatchEvent(ev);

    const emitted = wrapper.emitted("terminal-image-paste") ?? [];
    expect(emitted.length).toBe(1);
    expect(emitted[0]?.[0]).toBeInstanceOf(File);
    expect(emitted[0]?.[0]?.type).toBe("image/png");
  });

  it("does not intercept text paste", async () => {
    terminalOptions = undefined;
    writes = [];
    resized = [];
    resetCalls = 0;
    Object.defineProperty(window, "matchMedia", {
      writable: true,
      value: () => ({ matches: false, addEventListener() {}, removeEventListener() {} })
    });
    const preventDefault = vi.fn();
    const wrapper = mount(TerminalPane, { attachTo: document.body });
    const input = wrapper.get("textarea.xterm-helper-textarea").element as HTMLTextAreaElement;
    const file = new File(["hello"], "note.txt", { type: "text/plain" });
    const dt = {
      items: [{ kind: "file", type: "text/plain", getAsFile: () => file }]
    };

    const ev = new Event("paste", { bubbles: true, cancelable: true }) as ClipboardEvent & { clipboardData: typeof dt };
    Object.defineProperty(ev, "clipboardData", { value: dt });
    Object.defineProperty(ev, "preventDefault", { value: preventDefault });
    input.dispatchEvent(ev);

    const emitted = wrapper.emitted("terminal-image-paste") ?? [];
    expect(emitted.length).toBe(0);
    expect(preventDefault).not.toHaveBeenCalled();
  });

  it("maps Shift+Enter to LF in terminal input", () => {
    terminalOptions = undefined;
    writes = [];
    resized = [];
    resetCalls = 0;
    customKeyEventHandler = null;
    Object.defineProperty(window, "matchMedia", {
      writable: true,
      value: () => ({ matches: false, addEventListener() {}, removeEventListener() {} })
    });
    const wrapper = mount(TerminalPane);
    expect(customKeyEventHandler).toBeTruthy();

    const ev = new KeyboardEvent("keydown", {
      key: "Enter",
      shiftKey: true,
      bubbles: true,
      cancelable: true
    });
    const allowed = customKeyEventHandler?.(ev as KeyboardEvent);
    const emitted = wrapper.emitted("terminal-input") ?? [];

    expect(allowed).toBe(false);
    expect(ev.defaultPrevented).toBe(true);
    expect(emitted.length).toBe(1);
    expect((emitted[0] ?? [])[0]).toBe("\n");
  });

  it("disables terminal input when pane is ended", async () => {
    terminalOptions = undefined;
    writes = [];
    resized = [];
    resetCalls = 0;
    Object.defineProperty(window, "matchMedia", {
      writable: true,
      value: () => ({ matches: false, addEventListener() {}, removeEventListener() {} })
    });
    const wrapper = mount(TerminalPane, {
      props: {
        isEnded: true
      }
    });
    const input = wrapper.get("textarea.xterm-helper-textarea").element as HTMLTextAreaElement;
    expect(input.disabled).toBe(true);

    await wrapper.setProps({ isEnded: false });
    expect(input.disabled).toBe(false);
  });

  it("scrolls to bottom when switching task", async () => {
    terminalOptions = undefined;
    writes = [];
    resized = [];
    resetCalls = 0;
    scrollToBottomCalls = 0;
    Object.defineProperty(window, "matchMedia", {
      writable: true,
      value: () => ({ matches: false, addEventListener() {}, removeEventListener() {} })
    });
    const wrapper = mount(TerminalPane, {
      props: {
        taskId: "task-1"
      }
    });

    await wrapper.setProps({ taskId: "task-2" });
    await new Promise((resolve) => setTimeout(resolve, 180));

    expect(scrollToBottomCalls).toBeGreaterThan(0);
  });
});
