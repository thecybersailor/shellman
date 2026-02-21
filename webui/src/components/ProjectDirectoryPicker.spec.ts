import { flushPromises, mount } from "@vue/test-utils";
import { describe, expect, it, vi } from "vitest";
import ProjectDirectoryPicker from "./ProjectDirectoryPicker.vue";

describe("ProjectDirectoryPicker", () => {
  it("enters child directory and emits select", async () => {
    const wrapper = mount(ProjectDirectoryPicker, {
      props: {
        show: true,
        listDirectories: vi
          .fn()
          .mockResolvedValueOnce({ path: "/tmp", items: [{ name: "repo", path: "/tmp/repo", is_dir: true }] })
          .mockResolvedValueOnce({ path: "/tmp/repo", items: [] }),
        resolveDirectory: vi.fn().mockResolvedValue("/tmp/repo"),
        searchDirectories: vi.fn().mockResolvedValue([]),
        getDirectoryHistory: vi.fn().mockResolvedValue([]),
        recordDirectoryHistory: vi.fn().mockResolvedValue(undefined)
      }
    });
    await flushPromises();

    await wrapper.get("[data-test-id='muxt-dir-item-/tmp/repo']").trigger("dblclick");
    await flushPromises();
    await wrapper.get("[data-test-id='muxt-dir-select-current']").trigger("click");

    expect(wrapper.emitted("select-directory")?.[0]).toEqual(["/tmp/repo"]);
  });
});
