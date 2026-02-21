import { mount } from "@vue/test-utils";
import { describe, expect, it } from "vitest";
import TmuxTree from "./TmuxTree.vue";

describe("TmuxTree", () => {
  it("emits select-pane when clicking pane item", async () => {
    const wrapper = mount(TmuxTree, {
      props: {
        items: [{ target: "s1:0.0", title: "s1", status: "unknown", updatedAt: 0 }],
        selectedPane: ""
      }
    });
    await wrapper.get("[data-test-id='tt-pane-item-s1_0_0']").trigger("click");
    expect(wrapper.emitted("select-pane")?.[0]).toEqual(["s1:0.0"]);
  });

  it("renders pane item with normalized data-test-id", () => {
    const wrapper = mount(TmuxTree, {
      props: {
        items: [{ target: "e2e:0.0", title: "e2e", status: "unknown", updatedAt: 0 }],
        selectedPane: ""
      }
    });
    expect(wrapper.find("[data-test-id='tt-pane-item-e2e_0_0']").exists()).toBe(true);
  });

  it("renders title and status for each pane item", () => {
    const wrapper = mount(TmuxTree, {
      props: {
        items: [{ target: "e2e:0.0", title: "e2e", status: "running", updatedAt: 0 }],
        selectedPane: ""
      }
    });

    expect(wrapper.get("[data-test-id='tt-pane-item-e2e_0_0']").text()).toContain("e2e");
    expect(wrapper.get("[data-test-id='tt-pane-status-e2e_0_0']").attributes("data-status")).toBe("running");
  });
});
