import { mount } from "@vue/test-utils";
import { afterEach, describe, expect, it, vi } from "vitest";
import FilePanel from "./FilePanel.vue";

function flushPromises() {
  return new Promise((resolve) => setTimeout(resolve, 0));
}

function createLocalStorageMock() {
  const store: Record<string, string> = {};
  return {
    getItem: (key: string) => store[key] ?? null,
    setItem: (key: string, value: string) => {
      store[key] = value;
    },
    removeItem: (key: string) => {
      delete store[key];
    },
    clear: () => {
      for (const key of Object.keys(store)) {
        delete store[key];
      }
    }
  };
}

describe("FilePanel", () => {
  const localStorageMock = createLocalStorageMock();

  vi.stubGlobal("localStorage", localStorageMock);

  afterEach(() => {
    vi.restoreAllMocks();
    localStorage.clear();
    document.body.innerHTML = "";
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

  it("renders mapped file icons with O(1) extension lookup and keeps folder icons unchanged", async () => {
    const fakeFetch = vi.fn(async (url: string) => {
      if (url.includes("/api/v1/tasks/t1/files/tree?path=.")) {
        return {
          json: async () => ({
            ok: true,
            data: {
              entries: [
                { name: "src", path: "src", is_dir: true },
                { name: "main.ts", path: "main.ts", is_dir: false },
                { name: "README.unknownext", path: "README.unknownext", is_dir: false }
              ]
            }
          })
        } as Response;
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

    expect(wrapper.get("[data-test-id='shellman-file-item-main.ts'] [data-test-id='shellman-file-icon-kind']").attributes("data-icon-kind")).toBe("mapped");
    expect(wrapper.get("[data-test-id='shellman-file-item-README.unknownext'] [data-test-id='shellman-file-icon-kind']").attributes("data-icon-kind")).toBe("default");
    expect(wrapper.get("[data-test-id='shellman-file-item-src'] .lucide-folder").exists()).toBe(true);
  });

  it("shows loading state when expanding a directory", async () => {
    let resolveSrcLoad: (() => void) | null = null;
    const fakeFetch = vi.fn(async (url: string) => {
      if (url.includes("/api/v1/tasks/t1/files/tree?path=.")) {
        return {
          json: async () => ({
            ok: true,
            data: {
              entries: [{ name: "src", path: "src", is_dir: true }]
            }
          })
        } as Response;
      }
      if (url.includes("/api/v1/tasks/t1/files/tree?path=src")) {
        return {
          json: () =>
            new Promise((resolve) => {
              resolveSrcLoad = () => resolve({ ok: true, data: { entries: [] } });
            })
        } as Response;
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
    await wrapper.get("[data-test-id='shellman-file-item-src']").trigger("click");
    await flushPromises();

    expect(wrapper.get("[data-test-id='shellman-file-dir-loading-src']").exists()).toBe(true);

    resolveSrcLoad?.();
    await flushPromises();
  });

  it("renders file tree context menu items in english", async () => {
    const fakeFetch = vi.fn(async (url: string) => {
      if (url.includes("/api/v1/tasks/t1/files/tree?path=.")) {
        return {
          json: async () => ({
            ok: true,
            data: {
              entries: [{ name: "main.ts", path: "main.ts", is_dir: false }]
            }
          })
        } as Response;
      }
      return { json: async () => ({ ok: true, data: { entries: [] } }) } as Response;
    });
    vi.stubGlobal("fetch", fakeFetch);

    const wrapper = mount(FilePanel, {
      props: {
        taskId: "t1",
        projectId: "p1",
        repoRoot: "/repo"
      },
      attachTo: document.body
    });
    await flushPromises();
    await wrapper.get("[data-test-id='shellman-file-item-main.ts']").trigger("contextmenu");
    await flushPromises();

    const menuText = document.body.textContent || "";
    expect(menuText.includes("Cut")).toBe(true);
    expect(menuText.includes("Copy")).toBe(true);
    expect(menuText.includes("Copy Path")).toBe(true);
    expect(menuText.includes("Copy Relative Path")).toBe(true);
    expect(menuText.includes("Rename...")).toBe(true);
    expect(menuText.includes("Delete")).toBe(true);
  });
});
