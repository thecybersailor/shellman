import { mount } from "@vue/test-utils";
import { beforeEach, describe, expect, it, vi } from "vitest";
import ResponsiveOverlay from "./ResponsiveOverlay.vue";

const mediaQueryMock = vi.fn();

vi.mock("@vueuse/core", () => ({
  useMediaQuery: () => mediaQueryMock()
}));

describe("ResponsiveOverlay", () => {
  beforeEach(() => {
    mediaQueryMock.mockReset();
  });

  function render() {
    return mount(ResponsiveOverlay, {
      props: {
        open: true,
        title: "Add Project",
        description: "Select a project path"
      },
      slots: {
        default: "<div data-test-id='shellman-overlay-slot'>content</div>"
      },
      global: {
        stubs: {
          Dialog: { template: "<div data-test-id='shellman-overlay-dialog'><slot /></div>" },
          DialogContent: {
            template: "<div data-test-id='shellman-overlay-dialog-content' v-bind='$attrs'><slot /></div>"
          },
          DialogHeader: { template: "<div><slot /></div>" },
          DialogTitle: { template: "<div><slot /></div>" },
          DialogDescription: { template: "<div><slot /></div>" },
          Sheet: { template: "<div data-test-id='shellman-overlay-sheet'><slot /></div>" },
          SheetContent: {
            template: "<div data-test-id='shellman-overlay-sheet-content' v-bind='$attrs'><slot /></div>"
          },
          SheetHeader: { template: "<div><slot /></div>" },
          SheetTitle: { template: "<div><slot /></div>" },
          SheetDescription: { template: "<div><slot /></div>" }
        }
      }
    });
  }

  it("renders dialog on desktop", () => {
    mediaQueryMock.mockReturnValue(true);
    const wrapper = render();

    expect(wrapper.find("[data-test-id='shellman-overlay-dialog']").exists()).toBe(true);
    expect(wrapper.find("[data-test-id='shellman-overlay-sheet']").exists()).toBe(false);
  });

  it("renders sheet on mobile", () => {
    mediaQueryMock.mockReturnValue(false);
    const wrapper = render();

    expect(wrapper.find("[data-test-id='shellman-overlay-sheet']").exists()).toBe(true);
    expect(wrapper.find("[data-test-id='shellman-overlay-dialog']").exists()).toBe(false);
  });

  it("uses elevated z-index classes by default", () => {
    mediaQueryMock.mockReturnValue(true);
    const desktop = render();
    expect(desktop.get("[data-test-id='shellman-overlay-dialog-content']").classes()).toContain("z-[120]");

    mediaQueryMock.mockReturnValue(false);
    const mobile = render();
    expect(mobile.get("[data-test-id='shellman-overlay-sheet-content']").classes()).toContain("z-[120]");
  });
});
