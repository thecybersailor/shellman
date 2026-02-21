import { afterEach, describe, expect, it } from "vitest";
import { mount } from "@vue/test-utils";

import TaskHeader from "./TaskHeader.vue";

afterEach(() => {
  document.body.innerHTML = "";
});

describe("TaskHeader", () => {
  it("emits open-session-detail when title area is clicked", async () => {
    const wrapper = mount(TaskHeader, {
      props: {
        taskTitle: "T"
      }
    });

    await wrapper.get("[data-test-id='muxt-task-meta-display']").trigger("click");
    expect(wrapper.emitted("open-session-detail")?.length).toBe(1);
  });

  it("keeps title row hidden on mobile layout", () => {
    const wrapper = mount(TaskHeader, {
      props: {
        taskTitle: "Mobile title"
      }
    });

    const titleRow = wrapper
      .get("[data-test-id='muxt-task-title-display']")
      .element.parentElement;

    expect(titleRow).not.toBeNull();
    expect(Array.from(titleRow!.classList)).toContain("hidden");
    expect(Array.from(titleRow!.classList)).toContain("md:flex");
  });
});
