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
        hideFooter: { type: Boolean, default: false },
        alwaysShowTaskRowAction: { type: Boolean, default: false }
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
    expect(tree.props("alwaysShowTaskRowAction")).toBe(true);
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

  it("forwards terminal-link-open from terminal pane", async () => {
    const TerminalPaneStub = defineComponent({
      name: "TerminalPane",
      emits: ["terminal-link-open"],
      template: "<button data-test-id='terminal-link-open-btn' @click=\"$emit('terminal-link-open', { type: 'path', raw: 'src/App.vue:12:3' })\">open</button>"
    });
    const wrapper = mount(MobileStackView, {
      props: {
        projects: [{ projectId: "p1", title: "P1", tasks: [{ taskId: "t1", title: "Root", status: "running" }] }],
        selectedTaskId: "t1",
        darkMode: "dark"
      } as any,
      global: {
        stubs: {
          TerminalPane: TerminalPaneStub
        }
      }
    });

    await wrapper.get("[data-test-id='terminal-link-open-btn']").trigger("click");
    expect(wrapper.emitted("terminal-link-open")?.[0]?.[0]).toEqual({
      type: "path",
      raw: "src/App.vue:12:3"
    });
  });

  it("shows virtual keyboard only when terminal is focused", async () => {
    const TerminalPaneStub = defineComponent({
      name: "TerminalPane",
      emits: ["terminal-focus-change"],
      template: `
        <div>
          <button data-test-id="focus-on" @click="$emit('terminal-focus-change', true)">focus</button>
          <button data-test-id="focus-off" @click="$emit('terminal-focus-change', false)">blur</button>
        </div>
      `
    });

    const wrapper = mount(MobileStackView, {
      props: {
        projects: [{ projectId: "p1", title: "P1", tasks: [{ taskId: "t1", title: "Root", status: "running" }] }],
        selectedTaskId: "t1",
        darkMode: "dark"
      },
      global: {
        stubs: {
          TerminalPane: TerminalPaneStub
        }
      }
    });

    expect(wrapper.find("[data-test-id='tt-virtual-keyboard']").exists()).toBe(false);
    await wrapper.get("[data-test-id='focus-on']").trigger("click");
    expect(wrapper.find("[data-test-id='tt-virtual-keyboard']").exists()).toBe(true);
    await wrapper.get("[data-test-id='focus-off']").trigger("click");
    expect(wrapper.find("[data-test-id='tt-virtual-keyboard']").exists()).toBe(false);
  });

  it("forwards virtual keyboard key press as terminal-input", async () => {
    const TerminalPaneStub = defineComponent({
      name: "TerminalPane",
      emits: ["terminal-focus-change"],
      template: "<button data-test-id='focus-on' @click=\"$emit('terminal-focus-change', true)\">focus</button>"
    });

    const wrapper = mount(MobileStackView, {
      props: {
        projects: [{ projectId: "p1", title: "P1", tasks: [{ taskId: "t1", title: "Root", status: "running" }] }],
        selectedTaskId: "t1",
        darkMode: "dark"
      },
      global: {
        stubs: {
          TerminalPane: TerminalPaneStub
        }
      }
    });

    await wrapper.get("[data-test-id='focus-on']").trigger("click");
    await wrapper.get("[data-test-id='tt-vkey-esc']").trigger("click");
    expect(wrapper.emitted("terminal-input")?.[0]).toEqual(["\u001b"]);
  });

  it("applies armed ctrl to next terminal character input", async () => {
    const TerminalPaneStub = defineComponent({
      name: "TerminalPane",
      emits: ["terminal-focus-change", "terminal-input"],
      template: `
        <div>
          <button data-test-id="focus-on" @click="$emit('terminal-focus-change', true)">focus</button>
          <button data-test-id="emit-c" @click="$emit('terminal-input', 'c')">c</button>
        </div>
      `
    });

    const wrapper = mount(MobileStackView, {
      props: {
        projects: [{ projectId: "p1", title: "P1", tasks: [{ taskId: "t1", title: "Root", status: "running" }] }],
        selectedTaskId: "t1",
        darkMode: "dark"
      },
      global: {
        stubs: {
          TerminalPane: TerminalPaneStub
        }
      }
    });

    await wrapper.get("[data-test-id='focus-on']").trigger("click");
    await wrapper.get("[data-test-id='tt-vkey-ctrl']").trigger("click");
    await wrapper.get("[data-test-id='emit-c']").trigger("click");
    expect(wrapper.emitted("terminal-input")?.[0]).toEqual(["\u0003"]);
  });

  it("moves terminal viewport up with visualViewport keyboard inset", async () => {
    const listeners = new Map<string, (event: Event) => void>();
    const visualViewportMock = {
      width: 390,
      height: 700,
      offsetTop: 0,
      offsetLeft: 0,
      pageTop: 0,
      pageLeft: 0,
      scale: 1,
      addEventListener: (type: string, handler: EventListenerOrEventListenerObject) => {
        listeners.set(type, handler as (event: Event) => void);
      },
      removeEventListener: (type: string) => {
        listeners.delete(type);
      }
    };
    Object.defineProperty(window, "innerHeight", { configurable: true, value: 800 });
    Object.defineProperty(window, "visualViewport", { configurable: true, value: visualViewportMock });

    const TerminalPaneStub = defineComponent({
      name: "TerminalPane",
      emits: ["terminal-focus-change"],
      template: "<button data-test-id='focus-on' @click=\"$emit('terminal-focus-change', true)\">focus</button>"
    });

    const wrapper = mount(MobileStackView, {
      props: {
        projects: [{ projectId: "p1", title: "P1", tasks: [{ taskId: "t1", title: "Root", status: "running" }] }],
        selectedTaskId: "t1",
        darkMode: "dark"
      },
      global: {
        stubs: {
          TerminalPane: TerminalPaneStub
        }
      }
    });

    await wrapper.get("[data-test-id='focus-on']").trigger("click");
    listeners.get("resize")?.(new Event("resize"));
    await wrapper.vm.$nextTick();

    expect(wrapper.get("[data-test-id='shellman-mobile-session-main']").attributes("style")).toContain("padding-bottom: 100px;");
    expect(wrapper.get("[data-test-id='tt-virtual-keyboard']").attributes("style")).toContain("top: 16px;");
  });
});
