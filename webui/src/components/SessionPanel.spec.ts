import { mount } from "@vue/test-utils";
import { nextTick } from "vue";
import { beforeAll, describe, expect, it, vi } from "vitest";
import SessionPanel from "./SessionPanel.vue";

describe("SessionPanel", () => {
  beforeAll(() => {
    if (typeof globalThis.ResizeObserver === "undefined") {
      vi.stubGlobal("ResizeObserver", class {
        observe() {}
        unobserve() {}
        disconnect() {}
      });
    }
  });

  it("renders user and assistant messages", async () => {
    const wrapper = mount(SessionPanel, {
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
    const wrapper = mount(SessionPanel, {
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
    const wrapper = mount(SessionPanel, {
      props: { taskId: "t1", taskTitle: "Task", taskDescription: "", taskMessages: [] }
    });
    const input = wrapper.get("[data-test-id='shellman-shellman-input']");
    await input.setValue("Reply exactly: SHELLMAN_E2E_OK");
    await wrapper.get("form").trigger("submit");
    expect(wrapper.emitted("send-message")?.[0]?.[0]).toEqual({ content: "Reply exactly: SHELLMAN_E2E_OK" });
  });

  it("emits set-autopilot when switch toggled", async () => {
    const wrapper = mount(SessionPanel, {
      props: { taskId: "t1", taskTitle: "Task", taskDescription: "", taskMessages: [], autopilot: false }
    });
    const autopilotSwitch = wrapper.get("[data-test-id='shellman-shellman-autopilot-switch']");
    await autopilotSwitch.trigger("click");
    const events = wrapper.emitted("set-autopilot") ?? [];
    expect(events.length).toBeGreaterThan(0);
    expect(events[events.length - 1]?.[0]).toEqual({ enabled: true });
  });

  it("renders responding and ai-elements tool blocks for structured assistant content", async () => {
    const wrapper = mount(SessionPanel, {
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
});
