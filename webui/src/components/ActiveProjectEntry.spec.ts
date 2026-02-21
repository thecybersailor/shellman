import { mount } from "@vue/test-utils";
import { beforeEach, describe, expect, it, vi } from "vitest";
import ActiveProjectEntry from "./ActiveProjectEntry.vue";

const mediaQueryMock = vi.fn();

vi.mock("@vueuse/core", () => ({
  useMediaQuery: () => mediaQueryMock()
}));

describe("ActiveProjectEntry", () => {
  beforeEach(() => {
    mediaQueryMock.mockReset();
    mediaQueryMock.mockReturnValue(true);
  });

  function render(props: Record<string, unknown> = {}) {
    return mount(ActiveProjectEntry, {
      props: {
        show: true,
        listDirectories: vi.fn().mockResolvedValue({ path: "/tmp", items: [] }),
        resolveDirectory: vi.fn().mockResolvedValue("/tmp"),
        searchDirectories: vi.fn().mockResolvedValue([]),
        getDirectoryHistory: vi.fn().mockResolvedValue([]),
        recordDirectoryHistory: vi.fn().mockResolvedValue(undefined),
        ...props
      },
      global: {
        stubs: {
          ResponsiveOverlay: {
            template:
              "<div data-test-id='shellman-responsive-overlay'><slot /></div>"
          },
          ProjectDirectoryPicker: {
            template:
              "<div data-test-id='shellman-project-directory-picker'><button data-test-id='shellman-picker-select' @click=\"$emit('select-directory','/tmp/demo')\" /></div>"
          }
        }
      }
    });
  }

  it("uses responsive overlay wrapper", () => {
    const wrapper = render();
    expect(wrapper.find("[data-test-id='shellman-responsive-overlay']").exists()).toBe(true);
  });

  it("emits select-directory when picker selects path", async () => {
    const wrapper = render();
    await wrapper.get("[data-test-id='shellman-picker-select']").trigger("click");

    expect(wrapper.emitted("select-directory")?.[0]).toEqual(["/tmp/demo"]);
    expect(wrapper.emitted("update:show")?.at(-1)).toEqual([false]);
  });

  it("does not render manual repo-root input in picker mode", () => {
    const wrapper = render();
    expect(wrapper.find("[data-test-id='shellman-project-root-input']").exists()).toBe(false);
  });
});
