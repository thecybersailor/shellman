import { mount } from "@vue/test-utils";
import { nextTick } from "vue";
import { afterEach, beforeAll, describe, expect, it, vi } from "vitest";
import ThreadPanel from "./ThreadPanel.vue";

describe("ThreadPanel", () => {
  beforeAll(() => {
    if (typeof globalThis.ResizeObserver === "undefined") {
      vi.stubGlobal("ResizeObserver", class {
        observe() {}
        unobserve() {}
        disconnect() {}
      });
    }
    if (
      typeof globalThis.localStorage === "undefined" ||
      typeof globalThis.localStorage.getItem !== "function" ||
      typeof globalThis.localStorage.setItem !== "function" ||
      typeof globalThis.localStorage.clear !== "function"
    ) {
      const storage = new Map<string, string>();
      vi.stubGlobal("localStorage", {
        getItem: (key: string) => (storage.has(key) ? storage.get(key)! : null),
        setItem: (key: string, value: string) => {
          storage.set(key, String(value));
        },
        removeItem: (key: string) => {
          storage.delete(key);
        },
        clear: () => {
          storage.clear();
        }
      });
    }
  });
  afterEach(() => {
    if (typeof localStorage !== "undefined" && typeof localStorage.clear === "function") {
      localStorage.clear();
    }
  });

  it("renders user and assistant messages", async () => {
    const wrapper = mount(ThreadPanel, {
      props: {
        taskId: "t1",
        taskTitle: "Task",
        taskDescription: "",
        taskMessages: [
          { id: 1, task_id: "t1", role: "user", content: "hello", status: "completed", created_at: 1, updated_at: 1 },
          { id: 2, task_id: "t1", role: "assistant", content: "world", status: "completed", created_at: 2, updated_at: 2 }
        ]
      }
    });
    await nextTick();
    await nextTick();

    const userMessage = wrapper.findAll("[data-test-id='shellman-shellman-message-user']");
    const assistantMessage = wrapper.findAll("[data-test-id='shellman-shellman-message-assistant']");

    expect(userMessage.length).toBe(1);
    expect(assistantMessage.length).toBe(1);
  });

  it("renders runtime typed message with runtime test id", async () => {
    const wrapper = mount(ThreadPanel, {
      props: {
        taskId: "t1",
        taskTitle: "Task",
        taskDescription: "",
        taskMessages: [
          {
            id: 10,
            task_id: "t1",
            role: "user",
            status: "completed",
            created_at: 10,
            updated_at: 10,
            content: JSON.stringify({
              text: "auto-complete: pane idle and output stable",
              meta: {
                display_type: "runtime",
                source: "auto_progress",
                event: "auto_progress"
              }
            })
          }
        ]
      }
    });
    await nextTick();
    await nextTick();

    expect(wrapper.findAll("[data-test-id='shellman-shellman-message-runtime']").length).toBe(1);
    expect(wrapper.text()).toContain("runtime");
  });

  it("emits send-message when prompt is submitted", async () => {
    const wrapper = mount(ThreadPanel, {
      props: { taskId: "t1", taskTitle: "Task", taskDescription: "", taskMessages: [] }
    });
    const input = wrapper.get("[data-test-id='shellman-shellman-input']");
    await input.setValue("Reply exactly: SHELLMAN_E2E_OK");
    await wrapper.get("form").trigger("submit");
    expect(wrapper.emitted("send-message")?.[0]?.[0]).toEqual({ content: "Reply exactly: SHELLMAN_E2E_OK" });
  });

  it("restores draft title/description/prompt from localStorage", async () => {
    if (typeof localStorage !== "undefined" && typeof localStorage.setItem === "function") {
      localStorage.setItem(
        "shellman.project-panel.thread.task:t1",
        JSON.stringify({
          titleDraft: "Draft title",
          descriptionDraft: "Draft desc",
          promptDraft: "Draft prompt"
        })
      );
    }
    const wrapper = mount(ThreadPanel, {
      props: { taskId: "t1", taskTitle: "Task", taskDescription: "", taskMessages: [] }
    });
    await nextTick();
    await nextTick();

    expect((wrapper.get("[data-test-id='shellman-task-title-input']").element as HTMLInputElement).value).toBe("Draft title");
    expect((wrapper.get("[data-test-id='shellman-task-description-input']").element as HTMLTextAreaElement).value).toBe("Draft desc");
    expect((wrapper.get("[data-test-id='shellman-shellman-input']").element as HTMLTextAreaElement).value).toBe("Draft prompt");
  });

  it("emits set-sidecar-mode when select value updated", async () => {
    const wrapper = mount(ThreadPanel, {
      props: { taskId: "t1", taskTitle: "Task", taskDescription: "", taskMessages: [], sidecarMode: "advisor" }
    });
    const select = wrapper.findComponent({ name: "Select" });
    select.vm.$emit("update:modelValue", "observer");
    await nextTick();
    const events = wrapper.emitted("set-sidecar-mode") ?? [];
    expect(events.length).toBeGreaterThan(0);
    expect(events[events.length - 1]?.[0]).toEqual({ mode: "observer" });
  });

  it("renders responding and ai-elements tool blocks for structured assistant content", async () => {
    const wrapper = mount(ThreadPanel, {
      props: {
        taskId: "t1",
        taskTitle: "Task",
        taskDescription: "",
        taskMessages: [
          {
            id: 3,
            task_id: "t1",
            role: "assistant",
            status: "running",
            created_at: 3,
            updated_at: 3,
            content: JSON.stringify({
              text: "",
              tools: [
                {
                  type: "dynamic-tool",
                  state: "input-streaming",
                  tool_name: "gateway_http",
                  input: { method: "GET", path: "/healthz" },
                  output: { status: 200, body: "ok" }
                }
              ]
            })
          }
        ]
      }
    });
    await nextTick();
    await nextTick();

    expect(wrapper.find("[data-test-id='shellman-shellman-responding']").exists()).toBe(true);
    expect(wrapper.find("[data-test-id='shellman-shellman-tool']").exists()).toBe(true);
    expect(wrapper.find("[data-test-id='shellman-shellman-tool-header']").exists()).toBe(true);
    expect(wrapper.find("[data-test-id='shellman-shellman-tool-content']").exists()).toBe(true);
    expect(wrapper.find("[data-test-id='shellman-shellman-tool-input']").exists()).toBe(true);
    expect(wrapper.find("[data-test-id='shellman-shellman-tool-output']").exists()).toBe(true);
  });

  it("shows explicit sidecar chat action buttons and emits events", async () => {
    const wrapper = mount(ThreadPanel, {
      props: {
        taskId: "t1",
        taskTitle: "Task",
        taskDescription: "",
        sidecarMode: "autopilot",
        taskMessages: [
          { id: 1, task_id: "t1", role: "assistant", content: "", status: "running", created_at: 1, updated_at: 1 }
        ]
      }
    });
    await nextTick();

    await wrapper.get("[data-test-id='shellman-sidecar-stop-chat']").trigger("click");
    await wrapper.get("[data-test-id='shellman-sidecar-restart-context-child']").trigger("click");
    await wrapper.get("[data-test-id='shellman-sidecar-restart-context-root']").trigger("click");

    expect(wrapper.emitted("stop-sidecar-chat")?.length).toBe(1);
    expect(wrapper.emitted("restart-sidecar-context")?.[0]?.[0]).toEqual({ strategy: "child" });
    expect(wrapper.emitted("restart-sidecar-context")?.[1]?.[0]).toEqual({ strategy: "root" });
  });

  it("uses explicit observer wording to avoid stop confusion", async () => {
    const wrapper = mount(ThreadPanel, {
      props: { taskId: "t1", taskTitle: "Task", taskDescription: "", taskMessages: [], sidecarMode: "advisor" }
    });
    await nextTick();

    expect(wrapper.text()).toContain("Observer");
    expect(wrapper.text()).toContain("no auto-continue");
  });
});
