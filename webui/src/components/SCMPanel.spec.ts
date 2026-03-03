import { mount } from "@vue/test-utils";
import { afterEach, describe, expect, it, vi } from "vitest";
import SCMPanel from "./SCMPanel.vue";
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

describe("SCMPanel", () => {
  const localStorageMock = createLocalStorageMock();

  vi.stubGlobal("localStorage", localStorageMock);

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

  it("does not toast FILE_NOT_FOUND when auto-selecting deleted file on refresh", async () => {
    const toastErrorSpy = vi.spyOn(toast, "error").mockImplementation(() => "toast-id");
    const fakeFetch = vi.fn(async (url: string) => {
      if (url.endsWith("/api/v1/tasks/t1/diff")) {
        return {
          json: async () => ({
            ok: true,
            data: {
              diff: "diff --git a/old.txt b/old.txt\nindex 1111111..0000000 100644\n--- a/old.txt\n+++ /dev/null\n@@ -1 +0,0 @@\n-legacy line\n"
            }
          })
        } as Response;
      }
      if (url.endsWith("/api/v1/tasks/t1/files")) {
        return {
          json: async () => ({
            ok: true,
            data: { files: [{ path: "old.txt", status: "D" }] }
          })
        } as Response;
      }
      if (url.includes("/api/v1/tasks/t1/files/content?path=old.txt")) {
        return {
          json: async () => ({
            ok: false,
            error: { code: "FILE_NOT_FOUND", message: "file not found" }
          })
        } as Response;
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

    expect(toastErrorSpy).not.toHaveBeenCalled();
    expect(wrapper.get("[data-test-id='shellman-addon-file-list']").text()).toContain("old.txt");
  });

  it("does not toast FILE_READ_FAILED when changed path is not a readable file", async () => {
    const toastErrorSpy = vi.spyOn(toast, "error").mockImplementation(() => "toast-id");
    const fakeFetch = vi.fn(async (url: string) => {
      if (url.endsWith("/api/v1/tasks/t1/diff")) {
        return {
          json: async () => ({
            ok: true,
            data: { diff: "diff --git a/shellman b/shellman\nindex 1111111..2222222 160000\n--- a/shellman\n+++ b/shellman\n@@ -1 +1 @@\n-Subproject commit 1111111\n+Subproject commit 2222222\n" }
          })
        } as Response;
      }
      if (url.endsWith("/api/v1/tasks/t1/files")) {
        return {
          json: async () => ({
            ok: true,
            data: { files: [{ path: "shellman", status: "M" }] }
          })
        } as Response;
      }
      if (url.includes("/api/v1/tasks/t1/files/content?path=shellman")) {
        return {
          json: async () => ({
            ok: false,
            error: { code: "FILE_READ_FAILED", message: "is a directory" }
          })
        } as Response;
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

    expect(toastErrorSpy).not.toHaveBeenCalled();
    expect(wrapper.get("[data-test-id='shellman-addon-file-list']").text()).toContain("shellman");
  });
});
