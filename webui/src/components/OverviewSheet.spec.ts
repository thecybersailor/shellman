import { mount } from "@vue/test-utils";
import { beforeAll, describe, expect, it, vi } from "vitest";
import OverviewSheet from "./OverviewSheet.vue";

const globalStubs = {
  Sheet: { template: "<div><slot /></div>" },
  SheetContent: { template: "<div><slot /></div>" }
};

describe("OverviewSheet", () => {
  beforeAll(() => {
    if (typeof globalThis.ResizeObserver === "undefined") {
      vi.stubGlobal("ResizeObserver", class {
        observe() {}
        unobserve() {}
        disconnect() {}
      });
    }
  });

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
        stubs: globalStubs
      }
    });

    expect(wrapper.get("[data-test-id='shellman-overview-layout-desktop']").exists()).toBe(true);
    expect(wrapper.get("[data-test-id='shellman-overview-col-projects']").classes()).toContain("w-[220px]");
    expect(wrapper.get("[data-test-id='shellman-overview-col-tasks']").classes()).toContain("flex-1");
    expect(wrapper.get("[data-test-id='shellman-overview-col-chat']").classes()).toContain("w-[400px]");
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
        stubs: globalStubs
      }
    });

    expect(wrapper.get("[data-test-id='shellman-overview-mobile-tasks']").exists()).toBe(true);
    await wrapper.get("[data-test-id='shellman-overview-tab-projects']").trigger("click");
    expect(wrapper.get("[data-test-id='shellman-overview-mobile-projects']").exists()).toBe(true);
  });

  it("renders empty project/task lists when projects prop is empty", () => {
    const wrapper = mount(OverviewSheet, {
      props: {
        open: true,
        isMobile: false,
        projects: [],
        overviewProjectId: "not-exists",
        selectedTaskId: "",
        selectedTaskMessages: [],
        selectedTaskTitle: "",
        selectedTaskDescription: "",
        selectedTaskSidecarMode: "advisor",
        selectedPaneUuid: "",
        selectedCurrentCommand: ""
      },
      global: {
        stubs: globalStubs
      }
    });

    expect(wrapper.findAll("[data-test-id^='shellman-overview-project-']").length).toBe(0);
    expect(wrapper.findAll("[data-test-id^='shellman-overview-task-']").length).toBe(0);
  });

  it("does not render task-level title/desc/footer controls in overview chat", () => {
    const wrapper = mount(OverviewSheet, {
      props: {
        open: true,
        isMobile: false,
        projects: [{ projectId: "p1", title: "P1", tasks: [{ taskId: "t1", title: "Task", status: "running" }] }],
        overviewProjectId: "p1",
        selectedTaskId: "t1",
        selectedTaskMessages: [],
        selectedTaskTitle: "Task",
        selectedTaskDescription: "Desc",
        selectedTaskSidecarMode: "observer",
        selectedPaneUuid: "pane-1",
        selectedCurrentCommand: "echo hi"
      },
      global: {
        stubs: globalStubs
      }
    });

    expect(wrapper.find("[data-test-id='shellman-task-title-input']").exists()).toBe(false);
    expect(wrapper.find("[data-test-id='shellman-task-description-input']").exists()).toBe(false);
    expect(wrapper.find("[data-test-id='shellman-shellman-sidecar-mode-trigger']").exists()).toBe(false);
    expect(wrapper.text()).not.toContain("#pane-1");
  });

  it("reuses task title resolver fallback when task title is empty", () => {
    const wrapper = mount(OverviewSheet, {
      props: {
        open: true,
        isMobile: false,
        projects: [{ projectId: "p1", title: "P1", tasks: [{ taskId: "t1", title: "", currentCommand: "codex (/repo)" as any, status: "running" }] }],
        overviewProjectId: "p1",
        selectedTaskId: "t1",
        selectedTaskMessages: []
      },
      global: {
        stubs: globalStubs
      }
    });

    expect(wrapper.text()).toContain("codex (/repo)");
  });

  it("renders Project Manager title in chat area", async () => {
    const desktop = mount(OverviewSheet, {
      props: {
        open: true,
        isMobile: false,
        projects: [{ projectId: "p1", title: "P1", tasks: [{ taskId: "t1", title: "Task", status: "running" }] }],
        overviewProjectId: "p1",
        selectedTaskId: "t1",
        selectedTaskMessages: []
      },
      global: {
        stubs: globalStubs
      }
    });
    expect(desktop.text()).toContain("Project Manager");

    const mobile = mount(OverviewSheet, {
      props: {
        open: true,
        isMobile: true,
        projects: [{ projectId: "p1", title: "P1", tasks: [{ taskId: "t1", title: "Task", status: "running" }] }],
        overviewProjectId: "p1",
        selectedTaskId: "t1",
        selectedTaskMessages: []
      },
      global: {
        stubs: globalStubs
      }
    });
    await mobile.get("[data-test-id='shellman-overview-tab-chat']").trigger("click");
    expect(mobile.text()).toContain("Project Manager");
  });
});
