import { mount } from "@vue/test-utils";
import { afterEach, describe, expect, it, vi } from "vitest";
import SCMPanel from "./SCMPanel.vue";

function flushPromises() {
  return new Promise((resolve) => setTimeout(resolve, 0));
}

describe("SCMPanel", () => {
  afterEach(() => {
    vi.restoreAllMocks();
    localStorage.clear();
  });

  it("restores and persists commit message by project scope", async () => {
    localStorage.setItem(
      "shellman.project-panel.diff.project:p1",
      JSON.stringify({
        commitMessage: "draft commit message",
        selectedFilePath: ""
      })
    );
    const fakeFetch = vi.fn(async (url: string) => {
      if (url.endsWith("/api/v1/tasks/t1/diff")) {
        return { json: async () => ({ ok: true, data: { diff: "" } }) } as Response;
      }
      if (url.endsWith("/api/v1/tasks/t1/files")) {
        return { json: async () => ({ ok: true, data: { files: [] } }) } as Response;
      }
      return { json: async () => ({ ok: true, data: {} }) } as Response;
    });
    vi.stubGlobal("fetch", fakeFetch);

    const wrapper = mount(SCMPanel, {
      props: {
        taskId: "t1",
        projectId: "p1"
      }
    });
    await flushPromises();

    expect((wrapper.get("[data-test-id='shellman-scm-commit-message']").element as HTMLTextAreaElement).value).toBe("draft commit message");

    await wrapper.get("[data-test-id='shellman-scm-commit-message']").setValue("next commit");
    await flushPromises();

    const nextSnapshot = JSON.parse(localStorage.getItem("shellman.project-panel.diff.project:p1") || "{}") as Record<string, unknown>;
    expect(nextSnapshot.commitMessage).toBe("next commit");
  });
});
