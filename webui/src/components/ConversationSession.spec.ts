import { mount } from "@vue/test-utils";
import { beforeAll, describe, expect, it, vi } from "vitest";
import { nextTick } from "vue";
import { SIDECAR_USER_INPUT_HISTORY_KEY } from "@/lib/sidecar_user_input_history";
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

  it("records submitted content into sidecar user history when enabled", async () => {
    localStorage.clear();
    const wrapper = mount(ConversationSession, {
      props: {
        taskId: "t1",
        modelValue: "",
        taskMessages: [],
        inputHistoryEnabled: true
      }
    });

    await wrapper.get("[data-test-id='shellman-shellman-input']").setValue("hello sidecar");
    await wrapper.get("form").trigger("submit");

    const stored = JSON.parse(localStorage.getItem(SIDECAR_USER_INPUT_HISTORY_KEY) || "[]") as string[];
    expect(stored.at(-1)).toBe("hello sidecar");
  });

  it("does not record history when input history is disabled", async () => {
    localStorage.clear();
    const wrapper = mount(ConversationSession, {
      props: {
        taskId: "t1",
        modelValue: "",
        taskMessages: [],
        inputHistoryEnabled: false
      }
    });

    await wrapper.get("[data-test-id='shellman-shellman-input']").setValue("pm-like message");
    await wrapper.get("form").trigger("submit");

    expect(localStorage.getItem(SIDECAR_USER_INPUT_HISTORY_KEY)).toBeNull();
  });

  it("navigates sidecar history with ArrowUp/ArrowDown and restores draft", async () => {
    localStorage.clear();
    localStorage.setItem(SIDECAR_USER_INPUT_HISTORY_KEY, JSON.stringify(["older", "newer"]));
    const wrapper = mount(ConversationSession, {
      props: {
        taskId: "t1",
        modelValue: "",
        taskMessages: [],
        inputHistoryEnabled: true
      }
    });

    const input = wrapper.get("[data-test-id='shellman-shellman-input']");
    const el = input.element as HTMLTextAreaElement;

    await input.setValue("draft-not-empty");
    el.setSelectionRange(0, 0);

    await input.trigger("keydown", { key: "ArrowUp" });
    await nextTick();
    expect(el.value).toBe("newer");

    await input.trigger("keydown", { key: "ArrowUp" });
    await nextTick();
    expect(el.value).toBe("older");

    await input.trigger("keydown", { key: "ArrowDown" });
    await nextTick();
    expect(el.value).toBe("newer");

    await input.trigger("keydown", { key: "ArrowDown" });
    await nextTick();
    expect(el.value).toBe("draft-not-empty");
  });
});
