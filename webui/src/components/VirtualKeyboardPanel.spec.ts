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
    expect(wrapper.get("[data-test-id='tt-vkey-status']").text()).toContain("Ctrl armed");
  });
});
