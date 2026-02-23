import { mount } from "@vue/test-utils";
import { describe, expect, it } from "vitest";
import OverviewSheet from "./OverviewSheet.vue";

describe("OverviewSheet", () => {
  it("renders desktop three columns with 20/45/35 layout", () => {
    const wrapper = mount(OverviewSheet, {
      props: {
        open: true,
        isMobile: false,
        projects: [{ projectId: "p1", title: "P1", tasks: [{ taskId: "t1", title: "Task", status: "running" }] }],
        overviewProjectId: "p1",
        selectedTaskId: "t1",
        selectedTaskMessages: [],
        selectedTaskTitle: "Task",
        selectedTaskDescription: "",
        selectedTaskSidecarMode: "advisor",
        selectedPaneUuid: "",
        selectedCurrentCommand: ""
      },
      global: {
        stubs: {
          Sheet: { template: "<div><slot /></div>" },
          SheetContent: { template: "<div><slot /></div>" }
        }
      }
    });

    expect(wrapper.get("[data-test-id='shellman-overview-layout-desktop']").exists()).toBe(true);
    expect(wrapper.get("[data-test-id='shellman-overview-col-projects']").attributes("style")).toContain("20%");
    expect(wrapper.get("[data-test-id='shellman-overview-col-tasks']").attributes("style")).toContain("45%");
    expect(wrapper.get("[data-test-id='shellman-overview-col-chat']").attributes("style")).toContain("35%");
  });

  it("defaults to tasks tab on mobile and can switch tabs", async () => {
    const wrapper = mount(OverviewSheet, {
      props: {
        open: true,
        isMobile: true,
        projects: [{ projectId: "p1", title: "P1", tasks: [{ taskId: "t1", title: "Task", status: "running" }] }],
        overviewProjectId: "p1",
        selectedTaskId: "t1",
        selectedTaskMessages: [],
        selectedTaskTitle: "Task",
        selectedTaskDescription: "",
        selectedTaskSidecarMode: "advisor",
        selectedPaneUuid: "",
        selectedCurrentCommand: ""
      },
      global: {
        stubs: {
          Sheet: { template: "<div><slot /></div>" },
          SheetContent: { template: "<div><slot /></div>" }
        }
      }
    });

    expect(wrapper.get("[data-test-id='shellman-overview-mobile-tasks']").exists()).toBe(true);
    await wrapper.get("[data-test-id='shellman-overview-tab-projects']").trigger("click");
    expect(wrapper.get("[data-test-id='shellman-overview-mobile-projects']").exists()).toBe(true);
  });
});
