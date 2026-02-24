import { describe, expect, it } from "vitest";
import { mount } from "@vue/test-utils";
import VirtualKeyboardPanel from "./VirtualKeyboardPanel.vue";

describe("VirtualKeyboardPanel", () => {
  it("emits key press for ctrl", async () => {
    const wrapper = mount(VirtualKeyboardPanel, {
      props: { ctrlArmed: false, altArmed: false }
    });
    await wrapper.get("[data-test-id='tt-vkey-ctrl']").trigger("click");
    expect(wrapper.emitted("press-key")?.[0]).toEqual(["ctrl"]);
  });

  it("renders armed status for ctrl", () => {
    const wrapper = mount(VirtualKeyboardPanel, {
      props: { ctrlArmed: true, altArmed: false }
    });
    expect(wrapper.get("[data-test-id='tt-vkey-ctrl']").attributes("aria-pressed")).toBe("true");
    expect(wrapper.get("[data-test-id='tt-vkey-alt']").attributes("aria-pressed")).toBe("false");
  });

  it("uses bottom-centered layout with 20px horizontal gap", () => {
    const wrapper = mount(VirtualKeyboardPanel, {
      props: { ctrlArmed: false, altArmed: false }
    });
    const classes = wrapper.get("[data-test-id='tt-virtual-keyboard']").classes();
    expect(classes).toContain("left-1/2");
    expect(classes).toContain("-translate-x-1/2");
    expect(classes).toContain("w-[calc(100vw-40px)]");
  });

  it("uses dynamic top offset style", () => {
    const wrapper = mount(VirtualKeyboardPanel, {
      props: { ctrlArmed: false, altArmed: false, topOffsetPx: 44 }
    });
    expect(wrapper.get("[data-test-id='tt-virtual-keyboard']").attributes("style")).toContain("top: 44px;");
  });

  it("renders one horizontal scroll row for all keys", () => {
    const wrapper = mount(VirtualKeyboardPanel, {
      props: { ctrlArmed: false, altArmed: false }
    });
    const row = wrapper.get("[data-test-id='tt-virtual-keyboard-row']");
    const classes = row.classes();
    expect(classes).toContain("overflow-x-auto");
    expect(classes).toContain("whitespace-nowrap");
    expect(wrapper.findAll("button").length).toBe(8);
  });

  it("hides scrollbar while keeping horizontal scroll", () => {
    const wrapper = mount(VirtualKeyboardPanel, {
      props: { ctrlArmed: false, altArmed: false }
    });
    const row = wrapper.get("[data-test-id='tt-virtual-keyboard-row']");
    expect(row.classes()).toContain("overflow-x-auto");
    expect(row.attributes("style")).toContain("-webkit-overflow-scrolling: touch");
  });

  it("prevents default on mousedown to avoid stealing terminal focus", () => {
    const wrapper = mount(VirtualKeyboardPanel, {
      props: { ctrlArmed: false, altArmed: false }
    });
    const button = wrapper.get("[data-test-id='tt-vkey-esc']");
    const event = new MouseEvent("mousedown", { bubbles: true, cancelable: true });
    button.element.dispatchEvent(event);
    expect(event.defaultPrevented).toBe(true);
  });

  it("keeps virtual keys unfocusable to avoid stealing xterm focus", () => {
    const wrapper = mount(VirtualKeyboardPanel, {
      props: { ctrlArmed: false, altArmed: false }
    });
    expect(wrapper.get("[data-test-id='tt-vkey-ctrl']").attributes("tabindex")).toBe("-1");
    expect(wrapper.get("[data-test-id='tt-vkey-esc']").attributes("tabindex")).toBe("-1");
  });
});
