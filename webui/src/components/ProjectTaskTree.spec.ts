import { mount } from "@vue/test-utils";
import { describe, expect, it, vi } from "vitest";
import ProjectTaskTree from "./ProjectTaskTree.vue";
import { Checkbox } from "@/components/ui/checkbox";

describe("ProjectTaskTree", () => {
  it("renders project sections and task status", () => {
    const wrapper = mount(ProjectTaskTree, {
      props: {
        projects: [
          {
            projectId: "p1",
            title: "Project One",
            tasks: [
              { taskId: "t1", title: "Root", status: "running" },
              { taskId: "t2", title: "Child", status: "waiting_children" }
            ]
          }
        ],
        selectedTaskId: "t1"
      }
    });

    expect(wrapper.text()).toContain("Project One");
    expect(wrapper.text()).toContain("Root");
    expect(wrapper.get("[data-test-id='shellman-task-status-t1']").attributes("data-status")).toBe("running");
  });

  it("shows spinner for running runtime status and relative time otherwise", () => {
    const nowSpy = vi.spyOn(Date, "now").mockReturnValue(new Date("2026-02-18T10:10:00Z").getTime());
    const wrapper = mount(ProjectTaskTree, {
      props: {
        projects: [
          {
            projectId: "p1",
            title: "Project One",
            tasks: [
              { taskId: "t1", title: "Root", status: "running", runtimeStatus: "running", runtimeUpdatedAt: 1771409340 },
              { taskId: "t2", title: "Child", status: "waiting_children", runtimeStatus: "ready", runtimeUpdatedAt: 1771402200 }
            ]
          }
        ],
        selectedTaskId: "t1"
      }
    });

    expect(wrapper.get("[data-test-id='shellman-task-status-t1'] svg").classes()).toContain("animate-spin");
    expect(wrapper.text()).toContain("2h");
    nowSpy.mockRestore();
  });

  it("uses task updatedAt when runtimeUpdatedAt is absent", () => {
    const nowSpy = vi.spyOn(Date, "now").mockReturnValue(new Date("2026-02-18T10:10:00Z").getTime());
    const wrapper = mount(ProjectTaskTree, {
      props: {
        projects: [
          {
            projectId: "p1",
            title: "Project One",
            tasks: [{ taskId: "t1", title: "Root", status: "completed", updatedAt: 1771402200 }]
          }
        ],
        selectedTaskId: "t1"
      }
    });

    expect(wrapper.text()).toContain("2h");
    nowSpy.mockRestore();
  });

  it("renders status message on second line when flagDesc exists", () => {
    const wrapper = mount(ProjectTaskTree, {
      props: {
        projects: [
          {
            projectId: "p1",
            title: "Project One",
            tasks: [{ taskId: "t1", title: "Root", status: "completed", flagDesc: "等待人工确认" }]
          }
        ],
        selectedTaskId: "t1"
      }
    });

    expect(wrapper.get("[data-test-id='shellman-task-title-line-t1']").text()).toContain("Root");
    expect(wrapper.get("[data-test-id='shellman-task-status-message-t1']").text()).toBe("等待人工确认");
  });

  it("renders flag dot at task row right side and keeps slot width when flag is missing", () => {
    const wrapper = mount(ProjectTaskTree, {
      props: {
        projects: [
          {
            projectId: "p1",
            title: "Project One",
            tasks: [
              { taskId: "t_success", title: "Done", status: "completed", flag: "success" },
              { taskId: "t_notify", title: "Need Follow-up", status: "completed", flag: "notify" },
              { taskId: "t_error", title: "Failed", status: "failed", flag: "error" },
              { taskId: "t_plain", title: "No Flag", status: "pending" }
            ]
          }
        ],
        selectedTaskId: "t_success"
      }
    });

    expect(wrapper.get("[data-test-id='shellman-task-flag-dot-t_success']").attributes("data-flag")).toBe("success");
    expect(wrapper.get("[data-test-id='shellman-task-flag-dot-t_notify']").attributes("data-flag")).toBe("notify");
    expect(wrapper.get("[data-test-id='shellman-task-flag-dot-t_error']").attributes("data-flag")).toBe("error");
    expect(wrapper.find("[data-test-id='shellman-task-flag-dot-t_plain']").exists()).toBe(false);
    expect(wrapper.get("[data-test-id='shellman-task-flag-slot-t_success']").exists()).toBe(true);
    expect(wrapper.get("[data-test-id='shellman-task-flag-slot-t_plain']").exists()).toBe(true);
    expect(wrapper.get("[data-test-id='shellman-task-flag-slot-t_plain'] > span > span").classes()).toContain("opacity-0");
  });

  it("hides flag dot when flag_readed is true", () => {
    const wrapper = mount(ProjectTaskTree, {
      props: {
        projects: [
          {
            projectId: "p1",
            title: "Project One",
            tasks: [
              { taskId: "t_readed", title: "Readed", status: "completed", flag: "notify", flagReaded: true },
              { taskId: "t_unread", title: "Unread", status: "completed", flag: "notify", flagReaded: false }
            ]
          }
        ],
        selectedTaskId: "t_unread"
      }
    });

    expect(wrapper.find("[data-test-id='shellman-task-flag-dot-t_readed']").exists()).toBe(false);
    expect(wrapper.get("[data-test-id='shellman-task-flag-dot-t_unread']").attributes("data-flag")).toBe("notify");
  });

  it("emits create-root-pane from project section", async () => {
    const wrapper = mount(ProjectTaskTree, {
      props: {
        projects: [{ projectId: "p1", title: "Project One", tasks: [] }],
        selectedTaskId: ""
      }
    });

    await wrapper.get("[data-test-id='shellman-project-root-pane-p1']").trigger("click");
    expect(wrapper.emitted("create-root-pane")?.[0]).toEqual(["p1"]);
  });

  it("emits open-overview from footer button", async () => {
    const wrapper = mount(ProjectTaskTree, {
      props: {
        projects: [{ projectId: "p1", title: "Project One", tasks: [] }],
        selectedTaskId: ""
      }
    });

    await wrapper.get("[data-test-id='shellman-open-overview']").trigger("click");
    expect(wrapper.emitted("open-overview")?.[0]).toEqual([]);
  });

  it("emits toggle-task-check without selecting task when checkbox is clicked", async () => {
    const wrapper = mount(ProjectTaskTree, {
      props: {
        projects: [
          {
            projectId: "p1",
            title: "Project One",
            tasks: [{ taskId: "t1", title: "Root", status: "running", checked: false }]
          }
        ],
        selectedTaskId: ""
      }
    });

    wrapper.getComponent(Checkbox).vm.$emit("update:modelValue", true);
    expect(wrapper.emitted("toggle-task-check")?.[0]).toEqual([{ taskId: "t1", checked: true }]);
    expect(wrapper.emitted("select-task")).toBeUndefined();
  });

  it("renders orphan tmux section collapsed by default", () => {
    const wrapper = mount(ProjectTaskTree, {
      props: {
        projects: [{ projectId: "p1", title: "Project One", tasks: [{ taskId: "t1", title: "Root", status: "running" }] }],
        selectedTaskId: "",
        orphanPanes: [{ target: "e2e:0.1", title: "pane-a", status: "running", updatedAt: 1771408800 }],
        showOrphanSection: true
      }
    });

    expect(wrapper.find("[data-test-id='shellman-orphan-toggle']").exists()).toBe(true);
    expect(wrapper.find("[data-test-id='shellman-orphan-item-e2e-0-1']").exists()).toBe(false);
  });

  it("expands orphan section and emits adopt-pane on drop to task", async () => {
    const wrapper = mount(ProjectTaskTree, {
      props: {
        projects: [{ projectId: "p1", title: "Project One", tasks: [{ taskId: "t1", title: "Root", status: "running" }] }],
        selectedTaskId: "",
        orphanPanes: [{ target: "e2e:0.1", title: "pane-a", status: "running", updatedAt: 1771408800 }],
        showOrphanSection: true
      }
    });

    await wrapper.get("[data-test-id='shellman-orphan-toggle']").trigger("click");
    const orphanItem = wrapper.get("[data-test-id='shellman-orphan-item-e2e-0-1']");
    await orphanItem.trigger("dragstart", {
      dataTransfer: {
        effectAllowed: "",
        setData: vi.fn()
      }
    });

    const taskRow = wrapper.get("[data-test-id='shellman-task-row-t1']");
    await taskRow.trigger("drop");

    expect(wrapper.emitted("adopt-pane")?.[0]).toEqual([{ parentTaskId: "t1", paneTarget: "e2e:0.1", title: "pane-a" }]);
  });

  it("shows filter card and applies done + flag filter logic", async () => {
    const wrapper = mount(ProjectTaskTree, {
      props: {
        projects: [
          {
            projectId: "p1",
            title: "Project One",
            tasks: [
              { taskId: "checked_only", title: "Checked Task", status: "pending", checked: true },
              { taskId: "checked_error", title: "Checked Error Task", status: "failed", checked: true, flag: "error" },
              { taskId: "error_only", title: "Error Task", status: "failed", flag: "error" },
              { taskId: "notice_only", title: "Notice Task", status: "completed", flag: "notify" },
              { taskId: "success_only", title: "Success Task", status: "completed", flag: "success" },
              { taskId: "plain", title: "Plain Task", status: "pending" }
            ]
          }
        ],
        selectedTaskId: ""
      }
    });

    expect(wrapper.find("[data-test-id='shellman-task-filter-card']").exists()).toBe(false);

    await wrapper.get("[data-test-id='shellman-task-filter-toggle']").trigger("click");
    expect(wrapper.find("[data-test-id='shellman-task-filter-card']").exists()).toBe(true);

    const options = wrapper.findAll("[data-test-id^='shellman-task-filter-option-']");
    const optionLabels = options.map((n) => n.text());
    expect(optionLabels).toEqual(["done", "error", "notice", "success"]);

    // done 选中时，全显示
    expect(wrapper.find("[data-test-id='shellman-task-row-checked_error']").exists()).toBe(true);
    expect(wrapper.find("[data-test-id='shellman-task-row-error_only']").exists()).toBe(true);
    expect(wrapper.find("[data-test-id='shellman-task-row-checked_only']").exists()).toBe(true);
    expect(wrapper.find("[data-test-id='shellman-task-row-notice_only']").exists()).toBe(true);
    expect(wrapper.find("[data-test-id='shellman-task-row-success_only']").exists()).toBe(true);
    expect(wrapper.find("[data-test-id='shellman-task-row-plain']").exists()).toBe(true);

    // done 取消后，仅显示未勾选且命中 flag
    await wrapper.get("[data-test-id='shellman-task-filter-option-done']").trigger("click");
    expect(wrapper.find("[data-test-id='shellman-task-row-checked_error']").exists()).toBe(false);
    expect(wrapper.find("[data-test-id='shellman-task-row-error_only']").exists()).toBe(true);
    expect(wrapper.find("[data-test-id='shellman-task-row-notice_only']").exists()).toBe(true);
    expect(wrapper.find("[data-test-id='shellman-task-row-success_only']").exists()).toBe(true);
    expect(wrapper.find("[data-test-id='shellman-task-row-plain']").exists()).toBe(false);
    expect(wrapper.find("[data-test-id='shellman-task-row-checked_only']").exists()).toBe(false);

    // done 再次选中后恢复全显示
    await wrapper.get("[data-test-id='shellman-task-filter-option-done']").trigger("click");
    await wrapper.get("[data-test-id='shellman-task-filter-option-notice']").trigger("click");
    await wrapper.get("[data-test-id='shellman-task-filter-option-success']").trigger("click");
    expect(wrapper.find("[data-test-id='shellman-task-row-checked_error']").exists()).toBe(true);
    expect(wrapper.find("[data-test-id='shellman-task-row-error_only']").exists()).toBe(true);
    expect(wrapper.find("[data-test-id='shellman-task-row-notice_only']").exists()).toBe(true);
    expect(wrapper.find("[data-test-id='shellman-task-row-success_only']").exists()).toBe(true);
    expect(wrapper.find("[data-test-id='shellman-task-row-plain']").exists()).toBe(true);
  });

  it("restores full task list when filter card is collapsed", async () => {
    const wrapper = mount(ProjectTaskTree, {
      props: {
        projects: [
          {
            projectId: "p1",
            title: "Project One",
            tasks: [
              { taskId: "error_only", title: "Error Task", status: "failed", flag: "error" },
              { taskId: "plain", title: "Plain Task", status: "pending" }
            ]
          }
        ],
        selectedTaskId: ""
      }
    });

    await wrapper.get("[data-test-id='shellman-task-filter-toggle']").trigger("click");
    await wrapper.get("[data-test-id='shellman-task-filter-option-done']").trigger("click");
    await wrapper.get("[data-test-id='shellman-task-filter-option-notice']").trigger("click");
    await wrapper.get("[data-test-id='shellman-task-filter-option-success']").trigger("click");
    expect(wrapper.find("[data-test-id='shellman-task-row-error_only']").exists()).toBe(true);
    expect(wrapper.find("[data-test-id='shellman-task-row-plain']").exists()).toBe(false);

    await wrapper.get("[data-test-id='shellman-task-filter-toggle']").trigger("click");
    expect(wrapper.find("[data-test-id='shellman-task-filter-card']").exists()).toBe(false);
    expect(wrapper.find("[data-test-id='shellman-task-row-error_only']").exists()).toBe(true);
    expect(wrapper.find("[data-test-id='shellman-task-row-plain']").exists()).toBe(true);
  });
});
