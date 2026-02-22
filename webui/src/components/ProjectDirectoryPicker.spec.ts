import { flushPromises, mount } from "@vue/test-utils";
import { describe, expect, it, vi } from "vitest";
import ProjectDirectoryPicker from "./ProjectDirectoryPicker.vue";

describe("ProjectDirectoryPicker", () => {
  it("removes standalone search input", async () => {
    const wrapper = mount(ProjectDirectoryPicker, {
      props: {
        show: true,
        listDirectories: vi.fn().mockResolvedValue({ path: "/tmp", items: [] }),
        resolveDirectory: vi.fn().mockResolvedValue("/tmp"),
        searchDirectories: vi.fn().mockResolvedValue([]),
        getDirectoryHistory: vi.fn().mockResolvedValue([]),
        recordDirectoryHistory: vi.fn().mockResolvedValue(undefined)
      }
    });
    await flushPromises();

    expect(wrapper.find("[data-test-id='shellman-dir-search-input']").exists()).toBe(false);
  });

  it("loads home root first when roots are available", async () => {
    const listDirectories = vi.fn().mockResolvedValue({ path: "/Users/demo", items: [] });
    const getFSRoots = vi.fn().mockResolvedValue(["/Users/demo", "/"]);
    mount(ProjectDirectoryPicker, {
      props: {
        show: true,
        listDirectories,
        resolveDirectory: vi.fn().mockResolvedValue("/Users/demo"),
        searchDirectories: vi.fn().mockResolvedValue([]),
        getDirectoryHistory: vi.fn().mockResolvedValue([]),
        recordDirectoryHistory: vi.fn().mockResolvedValue(undefined),
        getFSRoots
      }
    });
    await flushPromises();

    expect(getFSRoots).toHaveBeenCalledTimes(1);
    expect(listDirectories).toHaveBeenCalledWith("/Users/demo");
  });

  it("shows autocomplete dropdown and selects by Enter", async () => {
    const wrapper = mount(ProjectDirectoryPicker, {
      props: {
        show: true,
        listDirectories: vi
          .fn()
          .mockResolvedValueOnce({ path: "/Users/demo", items: [] })
          .mockResolvedValueOnce({ path: "/Users/demo/Products", items: [] }),
        resolveDirectory: vi.fn().mockResolvedValue("/Users/demo"),
        searchDirectories: vi.fn().mockResolvedValue([{ name: "Products", path: "/Users/demo/Products", is_dir: true }]),
        getDirectoryHistory: vi.fn().mockResolvedValue([]),
        recordDirectoryHistory: vi.fn().mockResolvedValue(undefined),
        getFSRoots: vi.fn().mockResolvedValue(["/Users/demo"])
      }
    });
    await flushPromises();

    const pathInput = wrapper.get("[data-test-id='shellman-dir-path-input']");
    await pathInput.trigger("focus");
    await pathInput.setValue("/Users/demo/Pro");
    await flushPromises();

    expect(wrapper.find("[data-test-id='shellman-dir-autocomplete']").exists()).toBe(true);
    await pathInput.trigger("keydown.down");
    await pathInput.trigger("keydown.enter");
    await flushPromises();

    expect((pathInput.element as HTMLInputElement).value).toBe("/Users/demo/Products");
  });

  it("supports Tab to accept autocomplete item", async () => {
    const wrapper = mount(ProjectDirectoryPicker, {
      props: {
        show: true,
        listDirectories: vi
          .fn()
          .mockResolvedValueOnce({ path: "/Users/demo", items: [] })
          .mockResolvedValueOnce({ path: "/Users/demo/Projects", items: [] }),
        resolveDirectory: vi.fn().mockResolvedValue("/Users/demo"),
        searchDirectories: vi.fn().mockResolvedValue([{ name: "Projects", path: "/Users/demo/Projects", is_dir: true }]),
        getDirectoryHistory: vi.fn().mockResolvedValue([]),
        recordDirectoryHistory: vi.fn().mockResolvedValue(undefined),
        getFSRoots: vi.fn().mockResolvedValue(["/Users/demo"])
      }
    });
    await flushPromises();

    const pathInput = wrapper.get("[data-test-id='shellman-dir-path-input']");
    await pathInput.trigger("focus");
    await pathInput.setValue("/Users/demo/Pro");
    await flushPromises();
    await pathInput.trigger("keydown.tab");
    await flushPromises();

    expect((pathInput.element as HTMLInputElement).value).toBe("/Users/demo/Projects");
  });

  it("keeps typed directory on Enter unless autocomplete list is explicitly navigated", async () => {
    const listDirectories = vi
      .fn()
      .mockResolvedValueOnce({ path: "/Users/demo", items: [] })
      .mockResolvedValueOnce({ path: "/Users/demo/Documents", items: [] });
    const wrapper = mount(ProjectDirectoryPicker, {
      props: {
        show: true,
        listDirectories,
        resolveDirectory: vi.fn().mockResolvedValue("/Users/demo"),
        searchDirectories: vi.fn().mockResolvedValue([{ name: "Documents", path: "/Users/demo/Documents", is_dir: true }]),
        getDirectoryHistory: vi.fn().mockResolvedValue([]),
        recordDirectoryHistory: vi.fn().mockResolvedValue(undefined),
        getFSRoots: vi.fn().mockResolvedValue(["/Users/demo"])
      }
    });
    await flushPromises();

    const pathInput = wrapper.get("[data-test-id='shellman-dir-path-input']");
    await pathInput.trigger("focus");
    await pathInput.setValue("/Users/demo");
    await flushPromises();
    await pathInput.trigger("keydown.enter");
    await flushPromises();

    expect(listDirectories).toHaveBeenLastCalledWith("/Users/demo");
  });

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

    await wrapper.get("[data-test-id='shellman-dir-item-/tmp/repo']").trigger("dblclick");
    await flushPromises();
    await wrapper.get("[data-test-id='shellman-dir-select-current']").trigger("click");

    expect(wrapper.emitted("select-directory")?.[0]).toEqual(["/tmp/repo"]);
  });
});
