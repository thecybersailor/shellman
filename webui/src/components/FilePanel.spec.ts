import { mount } from "@vue/test-utils";
import { afterEach, describe, expect, it, vi } from "vitest";
import FilePanel from "./FilePanel.vue";
import { toast } from "vue-sonner";

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

  it("renders markdown preview for .md files", async () => {
    const fakeFetch = vi.fn(async (url: string) => {
      if (url.includes("/api/v1/tasks/t1/files/tree?path=.")) {
        return {
          json: async () => ({
            ok: true,
            data: {
              entries: [{ name: "README.md", path: "README.md", is_dir: false }]
            }
          })
        } as Response;
      }
      if (url.includes("/api/v1/tasks/t1/files/content?path=README.md")) {
        return {
          json: async () => ({ ok: true, data: { content: "# Title\n\ncontent" } })
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
    await wrapper.get("[data-test-id='shellman-file-item-README.md']").trigger("click");
    await flushPromises();

    expect(wrapper.find("[data-test-id='shellman-file-preview-markdown']").exists()).toBe(true);
    expect(wrapper.find("pre").exists()).toBe(false);
  });

  it("emits file-open on double click file item", async () => {
    const fakeFetch = vi.fn(async (url: string) => {
      if (url.includes("/api/v1/tasks/t1/files/tree?path=.")) {
        return {
          json: async () => ({
            ok: true,
            data: {
              entries: [{ name: "README.md", path: "README.md", is_dir: false }]
            }
          })
        } as Response;
      }
      if (url.includes("/api/v1/tasks/t1/files/content?path=README.md")) {
        return {
          json: async () => ({ ok: true, data: { content: "# Title" } })
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
    await wrapper.get("[data-test-id='shellman-file-item-README.md']").trigger("dblclick");

    expect(wrapper.emitted("file-open")?.[0]).toEqual(["README.md"]);
  });

  it("does not toast FILE_NOT_FOUND when restoring missing selected file from snapshot", async () => {
    localStorage.setItem(
      "shellman.project-panel.file.project:p1",
      JSON.stringify({
        searchQuery: "",
        expandedPaths: [],
        selectedFilePath: "ghost.txt"
      })
    );
    const toastErrorSpy = vi.spyOn(toast, "error").mockImplementation(() => "toast-id");
    const fakeFetch = vi.fn(async (url: string) => {
      if (url.includes("/api/v1/tasks/t1/files/tree?path=.")) {
        return {
          json: async () => ({
            ok: true,
            data: { entries: [{ name: "README.md", path: "README.md", is_dir: false }] }
          })
        } as Response;
      }
      if (url.includes("/api/v1/tasks/t1/files/content?path=ghost.txt")) {
        return {
          json: async () => ({
            ok: false,
            error: { code: "FILE_NOT_FOUND", message: "file not found" }
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

    expect(toastErrorSpy).not.toHaveBeenCalled();
    expect(wrapper.text()).toContain("未选择文件");
  });

  it("executes rename and refreshes tree", async () => {
    let rootFetchCount = 0;
    const fakeFetch = vi.fn(async (input: string, init?: RequestInit) => {
      const url = String(input);
      const method = String(init?.method || "GET").toUpperCase();
      if (url.includes("/api/v1/tasks/t1/files/tree?path=.") && method === "GET") {
        rootFetchCount += 1;
        return {
          json: async () => ({
            ok: true,
            data: {
              entries: [{ name: "main.ts", path: "main.ts", is_dir: false }]
            }
          })
        } as Response;
      }
      if (url.includes("/api/v1/tasks/t1/files/rename") && method === "POST") {
        return {
          json: async () => ({ ok: true, data: { task_id: "t1", path: "main.ts", target_path: "renamed.ts" } })
        } as Response;
      }
      if (url.includes("/api/v1/tasks/t1/files/content?path=") && method === "GET") {
        return {
          json: async () => ({ ok: true, data: { content: "" } })
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
    (document.body.querySelector("[data-test-id='shellman-file-context-rename']") as HTMLElement).click();
    await flushPromises();
    const renameInput = document.body.querySelector("[data-test-id='shellman-file-rename-input']") as HTMLInputElement;
    renameInput.value = "renamed.ts";
    renameInput.dispatchEvent(new Event("input", { bubbles: true }));
    await flushPromises();
    (document.body.querySelector("[data-test-id='shellman-file-rename-confirm']") as HTMLElement).click();
    await flushPromises();

    const renameCall = fakeFetch.mock.calls.find((call) => String(call[0]).includes("/api/v1/tasks/t1/files/rename"));
    expect(Boolean(renameCall)).toBe(true);
    expect(String(renameCall?.[1]?.method ?? "")).toBe("POST");
    expect(String(renameCall?.[1]?.body ?? "")).toContain("\"path\":\"main.ts\"");
    expect(String(renameCall?.[1]?.body ?? "")).toContain("\"new_name\":\"renamed.ts\"");
    expect(rootFetchCount >= 2).toBe(true);
  });

  it("executes delete and refreshes tree", async () => {
    let rootFetchCount = 0;
    const fakeFetch = vi.fn(async (input: string, init?: RequestInit) => {
      const url = String(input);
      const method = String(init?.method || "GET").toUpperCase();
      if (url.includes("/api/v1/tasks/t1/files/tree?path=.") && method === "GET") {
        rootFetchCount += 1;
        return {
          json: async () => ({
            ok: true,
            data: {
              entries: [{ name: "main.ts", path: "main.ts", is_dir: false }]
            }
          })
        } as Response;
      }
      if (url.includes("/api/v1/tasks/t1/files?path=main.ts") && method === "DELETE") {
        return {
          json: async () => ({ ok: true, data: { task_id: "t1", path: "main.ts" } })
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
    (document.body.querySelector("[data-test-id='shellman-file-context-delete']") as HTMLElement).click();
    await flushPromises();
    (document.body.querySelector("[data-test-id='shellman-file-delete-confirm']") as HTMLElement).click();
    await flushPromises();

    const deleteCall = fakeFetch.mock.calls.find((call) => String(call[0]).includes("/api/v1/tasks/t1/files?path=main.ts"));
    expect(Boolean(deleteCall)).toBe(true);
    expect(String(deleteCall?.[1]?.method ?? "")).toBe("DELETE");
    expect(rootFetchCount >= 2).toBe(true);
  });

  it("copies full path and relative path", async () => {
    const fakeFetch = vi.fn(async (input: string) => {
      const url = String(input);
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
    const writeText = vi.fn(async () => undefined);
    vi.stubGlobal("navigator", { clipboard: { writeText } });

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
    (document.body.querySelector("[data-test-id='shellman-file-context-copy-path']") as HTMLElement).click();
    await flushPromises();

    await wrapper.get("[data-test-id='shellman-file-item-main.ts']").trigger("contextmenu");
    await flushPromises();
    (document.body.querySelector("[data-test-id='shellman-file-context-copy-relative-path']") as HTMLElement).click();
    await flushPromises();

    expect(writeText.mock.calls[0]?.[0]).toBe("/repo/main.ts");
    expect(writeText.mock.calls[1]?.[0]).toBe("main.ts");
  });
});
