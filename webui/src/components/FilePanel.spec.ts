import { mount } from "@vue/test-utils";
import { afterEach, describe, expect, it, vi } from "vitest";
import FilePanel from "./FilePanel.vue";

function flushPromises() {
  return new Promise((resolve) => setTimeout(resolve, 0));
}

describe("FilePanel", () => {
  afterEach(() => {
    vi.restoreAllMocks();
    localStorage.clear();
  });

  it("restores and persists draft search text by project scope", async () => {
    localStorage.setItem(
      "shellman.project-panel.file.project:p1",
      JSON.stringify({
        searchQuery: "draft-query",
        expandedPaths: [],
        selectedFilePath: ""
      })
    );
    const fakeFetch = vi.fn(async (url: string) => {
      if (url.includes("/api/v1/tasks/t1/files/tree?path=.")) {
        return { json: async () => ({ ok: true, data: { entries: [] } }) } as Response;
      }
      if (url.includes("/api/v1/tasks/t1/files/search?q=draft-query")) {
        return { json: async () => ({ ok: true, data: { entries: [] } }) } as Response;
      }
      return { json: async () => ({ ok: true, data: { entries: [] } }) } as Response;
    });
    vi.stubGlobal("fetch", fakeFetch);

    const wrapper = mount(FilePanel, {
      props: {
        taskId: "t1",
        projectId: "p1",
        repoRoot: "/repo"
      }
    });
    await flushPromises();

    expect((wrapper.get("[data-test-id='shellman-file-search-input']").element as HTMLInputElement).value).toBe("draft-query");

    await wrapper.get("[data-test-id='shellman-file-search-input']").setValue("next-query");
    await flushPromises();

    const nextSnapshot = JSON.parse(localStorage.getItem("shellman.project-panel.file.project:p1") || "{}") as Record<string, unknown>;
    expect(nextSnapshot.searchQuery).toBe("next-query");
  });
});
