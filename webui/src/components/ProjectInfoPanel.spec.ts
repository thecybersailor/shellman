import { mount } from "@vue/test-utils";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import ProjectInfoPanel from "./ProjectInfoPanel.vue";

function flushPromises() {
  return new Promise((resolve) => setTimeout(resolve, 0));
}

describe("ProjectInfoPanel", () => {
  beforeEach(() => {
    if (typeof globalThis.ResizeObserver === "undefined") {
      vi.stubGlobal("ResizeObserver", class {
        observe() {}
        unobserve() {}
        disconnect() {}
      });
    }
  });

  afterEach(() => {
    vi.restoreAllMocks();
  });

  it("loads real diff/files data and file content", async () => {
    const fakeFetch = vi.fn(async (url: string) => {
      if (url.endsWith("/api/v1/tasks/t1/diff")) {
        return { json: async () => ({ ok: true, data: { task_id: "t1", diff: "diff --git a/a.txt b/a.txt" } }) } as Response;
      }
      if (url.endsWith("/api/v1/tasks/t1/files")) {
        return {
          json: async () => ({ ok: true, data: { task_id: "t1", files: [{ path: "a.txt", status: "M" }, { path: "b.txt", status: "??" }] } })
        } as Response;
      }
      if (url.includes("/api/v1/tasks/t1/files/content?path=a.txt")) {
        return { json: async () => ({ ok: true, data: { task_id: "t1", path: "a.txt", content: "hello world" } }) } as Response;
      }
      return { json: async () => ({ ok: false, error: { code: "NOT_FOUND" } }) } as Response;
    });
    vi.stubGlobal("fetch", fakeFetch);

    const wrapper = mount(ProjectInfoPanel, { props: { taskId: "t1", activeTab: "diff" } });
    await flushPromises();

    expect(wrapper.get("[data-test-id='shellman-scm-commit-message']").exists()).toBe(true);
    expect(wrapper.get("[data-test-id='shellman-addon-file-list']").text()).toContain("a.txt");
    expect(wrapper.get("[data-test-id='shellman-addon-file-content']").text()).toContain("hello world");
    expect(wrapper.text()).not.toContain("placeholder");
  });

  it("shows thread tab and emits save-task-meta on title/desc change (auto-save)", async () => {
    vi.useFakeTimers();
    const wrapper = mount(ProjectInfoPanel, {
      props: {
        taskId: "t1",
        projectId: "p1",
        activeTab: "thread",
        taskTitle: "Old",
        taskDescription: "Old desc",
        taskMessages: [
          { id: 1, task_id: "t1", role: "assistant", content: "first note", status: "completed", created_at: 1735689600000, updated_at: 1735689600000 }
        ]
      } as any
    });

    await wrapper.get("[data-test-id='shellman-task-title-input']").setValue("New");
    await wrapper.get("[data-test-id='shellman-task-description-input']").setValue("New desc");
    await vi.advanceTimersByTimeAsync(500);

    expect(wrapper.emitted("save-task-meta")?.[0]?.[0]).toEqual({
      title: "New",
      description: "New desc"
    });
    expect(wrapper.find("[data-test-id='shellman-shellman-message-assistant']").exists()).toBe(true);
    vi.useRealTimers();
  });

  it("uses different scope keys: diff/file by project, thread by task", async () => {
    const wrapper = mount(ProjectInfoPanel, {
      props: {
        taskId: "t1",
        projectId: "p1",
        activeTab: "diff"
      }
    });

    expect(wrapper.get("[data-test-id='shellman-project-tab-diff-body']").attributes("data-scope-key")).toBe("project:p1");

    const threadWrapper = mount(ProjectInfoPanel, {
      props: {
        taskId: "t2",
        projectId: "p1",
        activeTab: "thread"
      }
    });

    expect(threadWrapper.get("[data-test-id='shellman-project-tab-thread-body']").attributes("data-scope-key")).toBe("task:t2");
  });

  it("hides diff tab for non-git project and falls back to thread tab", async () => {
    const wrapper = mount(ProjectInfoPanel, {
      props: {
        taskId: "t1",
        projectId: "p1",
        activeTab: "diff",
        projectIsGitRepo: false
      }
    });

    await flushPromises();

    expect(wrapper.text()).not.toContain("Diff");
    expect(wrapper.find("[data-test-id='shellman-project-tab-diff-body']").exists()).toBe(false);
    expect(wrapper.emitted("update:active-tab")?.[0]?.[0]).toBe("thread");
  });
});
