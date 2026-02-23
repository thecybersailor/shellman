import { mount } from "@vue/test-utils";
import { beforeAll, describe, expect, it, vi } from "vitest";
import { nextTick } from "vue";
import ConversationSession from "./ConversationSession.vue";

describe("ConversationSession", () => {
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
    const wrapper = mount(ConversationSession, {
      props: {
        taskId: "t1",
        modelValue: "",
        taskMessages: [
          { id: 1, task_id: "t1", role: "user", content: "hello", status: "completed", created_at: 1, updated_at: 1 },
          { id: 2, task_id: "t1", role: "assistant", content: "world", status: "completed", created_at: 2, updated_at: 2 }
        ]
      }
    });
    await nextTick();
    await nextTick();

    expect(wrapper.findAll("[data-test-id='shellman-shellman-message-user']").length).toBe(1);
    expect(wrapper.findAll("[data-test-id='shellman-shellman-message-assistant']").length).toBe(1);
  });

  it("emits submit-message when prompt is submitted", async () => {
    const wrapper = mount(ConversationSession, {
      props: {
        taskId: "t1",
        modelValue: "",
        taskMessages: []
      }
    });

    await wrapper.get("[data-test-id='shellman-shellman-input']").setValue("Reply exactly: SHELLMAN_E2E_OK");
    await wrapper.get("form").trigger("submit");
    expect(wrapper.emitted("submit-message")?.[0]?.[0]).toEqual({ content: "Reply exactly: SHELLMAN_E2E_OK" });
    expect(wrapper.emitted("update:modelValue")?.at(-1)?.[0]).toBe("");
  });

  it("switches submit button to stop when assistant is running and prompt is empty", async () => {
    const wrapper = mount(ConversationSession, {
      props: {
        taskId: "t1",
        modelValue: "",
        taskMessages: [{ id: 1, task_id: "t1", role: "assistant", content: "", status: "running", created_at: 1, updated_at: 1 }]
      }
    });
    await nextTick();

    expect(wrapper.find("[data-test-id='shellman-shellman-send']").exists()).toBe(false);
    await wrapper.get("[data-test-id='shellman-shellman-stop']").trigger("click");
    expect(wrapper.emitted("stop-running-assistant-message")?.length).toBe(1);
  });
});
