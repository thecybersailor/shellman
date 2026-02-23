import { defineComponent } from "vue";
import { mount } from "@vue/test-utils";
import { describe, expect, it } from "vitest";
import MobileStackView from "./MobileStackView.vue";

describe("MobileStackView", () => {
  it("shows explorer when task is not selected", () => {
    const wrapper = mount(MobileStackView, {
      props: {
        projects: [{ projectId: "p1", title: "P1", tasks: [{ taskId: "t1", title: "Root", status: "running" }] }],
        selectedTaskId: "",
        darkMode: "dark"
      }
    });

    expect(wrapper.text()).toContain("shellman");
    expect(wrapper.text()).not.toContain("Session");
  });

  it("forwards output and cursor to terminal on session view", () => {
    const TerminalPaneStub = defineComponent({
      name: "TerminalPane",
      props: {
        taskId: { type: String, default: "" },
        paneUuid: { type: String, default: "" },
        frame: { type: Object, default: null },
        cursor: { type: Object, default: null }
      },
      template: "<div data-test-id='terminal-pane-stub' />"
    });

    const wrapper = mount(MobileStackView, {
      props: {
        projects: [{ projectId: "p1", title: "P1", tasks: [{ taskId: "t1", title: "Root", status: "running" }] }],
        selectedTaskId: "t1",
        selectedPaneUuid: "pane-uuid-1",
        darkMode: "dark",
        frame: { mode: "append", data: "hello-from-store" },
        cursor: { x: 3, y: 7 }
      } as any,
      global: {
        stubs: {
          TerminalPane: TerminalPaneStub
        }
      }
    });

    const terminal = wrapper.getComponent(TerminalPaneStub);
    expect(terminal.props("taskId")).toBe("t1");
    expect(terminal.props("paneUuid")).toBe("pane-uuid-1");
    expect(terminal.props("frame")).toEqual({ mode: "append", data: "hello-from-store" });
    expect(terminal.props("cursor")).toEqual({ x: 3, y: 7 });
  });

  it("disables orphan tmux section on mobile tree", () => {
    const ProjectTaskTreeStub = defineComponent({
      name: "ProjectTaskTree",
      props: {
        showOrphanSection: { type: Boolean, default: true },
        hideFooter: { type: Boolean, default: false }
      },
      template: "<div data-test-id='project-task-tree-stub' />"
    });

    const wrapper = mount(MobileStackView, {
      props: {
        projects: [{ projectId: "p1", title: "P1", tasks: [{ taskId: "t1", title: "Root", status: "running" }] }],
        selectedTaskId: "",
        darkMode: "dark"
      },
      global: {
        stubs: {
          ProjectTaskTree: ProjectTaskTreeStub
        }
      }
    });

    const tree = wrapper.getComponent(ProjectTaskTreeStub);
    expect(tree.props("hideFooter")).toBe(true);
    expect(tree.props("showOrphanSection")).toBe(false);
  });

  it("opens mobile info panel and switches to thread tab on header click", async () => {
    const TerminalPaneStub = defineComponent({
      name: "TerminalPane",
      emits: ["open-session-detail"],
      template: "<button data-test-id='shellman-task-meta-display' @click=\"$emit('open-session-detail')\">open</button>"
    });

    const ProjectInfoPanelStub = defineComponent({
      name: "ProjectInfoPanel",
      props: {
        activeTab: { type: String, default: "thread" }
      },
      template: "<div data-test-id='project-info-panel-stub' />"
    });

    const wrapper = mount(MobileStackView, {
      props: {
        projects: [{ projectId: "p1", title: "P1", tasks: [{ taskId: "t1", title: "Root", status: "running" }] }],
        selectedTaskId: "t1",
        selectedTaskProjectId: "p1",
        darkMode: "dark"
      } as any,
      global: {
        stubs: {
          TerminalPane: TerminalPaneStub,
          ProjectInfoPanel: ProjectInfoPanelStub
        }
      }
    });

    expect(wrapper.findComponent(ProjectInfoPanelStub).exists()).toBe(false);

    await wrapper.get("[data-test-id='shellman-task-meta-display']").trigger("click");

    const panel = wrapper.getComponent(ProjectInfoPanelStub);
    expect(panel.props("activeTab")).toBe("thread");
  });

  it("forwards remove-project event from mobile tree", async () => {
    const ProjectTaskTreeStub = defineComponent({
      name: "ProjectTaskTree",
      emits: ["remove-project"],
      template: "<button data-test-id='remove-project-btn' @click=\"$emit('remove-project','p1')\">remove</button>"
    });

    const wrapper = mount(MobileStackView, {
      props: {
        projects: [{ projectId: "p1", title: "P1", tasks: [] }],
        selectedTaskId: "",
        darkMode: "dark"
      },
      global: {
        stubs: {
          ProjectTaskTree: ProjectTaskTreeStub
        }
      }
    });

    await wrapper.get("[data-test-id='remove-project-btn']").trigger("click");
    expect(wrapper.emitted("remove-project")?.[0]).toEqual(["p1"]);
  });

  it("forwards archive-project-done event from mobile tree", async () => {
    const ProjectTaskTreeStub = defineComponent({
      name: "ProjectTaskTree",
      emits: ["archive-project-done"],
      template: "<button data-test-id='archive-project-btn' @click=\"$emit('archive-project-done','p1')\">archive</button>"
    });

    const wrapper = mount(MobileStackView, {
      props: {
        projects: [{ projectId: "p1", title: "P1", tasks: [] }],
        selectedTaskId: "",
        darkMode: "dark"
      },
      global: {
        stubs: {
          ProjectTaskTree: ProjectTaskTreeStub
        }
      }
    });

    await wrapper.get("[data-test-id='archive-project-btn']").trigger("click");
    expect(wrapper.emitted("archive-project-done")?.[0]).toEqual(["p1"]);
  });

  it("forwards open-overview event from mobile header", async () => {
    const wrapper = mount(MobileStackView, {
      props: {
        projects: [{ projectId: "p1", title: "P1", tasks: [] }],
        selectedTaskId: "",
        darkMode: "dark"
      }
    });

    await wrapper.get("[data-test-id='shellman-mobile-open-overview']").trigger("click");
    expect(wrapper.emitted("open-overview")?.[0]).toEqual([]);
  });
});
