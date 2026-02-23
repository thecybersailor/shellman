<script setup lang="ts">
import { computed, ref, watch } from "vue";
import { useI18n } from "vue-i18n";
import { Button } from "@/components/ui/button";
import { Sheet, SheetContent } from "@/components/ui/sheet";
import { X } from "lucide-vue-next";
import type { ProjectSection } from "./ProjectTaskTree.vue";
import ThreadPanel from "./ThreadPanel.vue";
import type { TaskMessage } from "@/stores/shellman";

type SidecarMode = "advisor" | "observer" | "autopilot";

const mockProjects: ProjectSection[] = [
  {
    projectId: "inbox",
    title: "Inbox",
    tasks: [
      { taskId: "inbox-t1", title: "Clarify weekly goals", status: "running" },
      { taskId: "inbox-t2", title: "Collect release checklist", status: "pending" }
    ]
  },
  {
    projectId: "project1",
    title: "project1",
    tasks: [
      { taskId: "p1-t1", title: "Landing page review", status: "waiting_user" },
      { taskId: "p1-t2", title: "Analytics sanity check", status: "completed" }
    ]
  },
  {
    projectId: "project2",
    title: "project2",
    tasks: [
      { taskId: "p2-t1", title: "Define QA scope", status: "running" },
      { taskId: "p2-t2", title: "Track blocker owners", status: "waiting_children" }
    ]
  },
  {
    projectId: "project3",
    title: "project3",
    tasks: [
      { taskId: "p3-t1", title: "Prepare milestone notes", status: "pending" },
      { taskId: "p3-t2", title: "Demo rehearsal", status: "canceled" }
    ]
  }
];

const props = withDefaults(
  defineProps<{
    open: boolean;
    isMobile?: boolean;
    projects?: ProjectSection[];
    overviewProjectId?: string;
    selectedTaskId?: string;
    selectedTaskMessages?: TaskMessage[];
    selectedTaskTitle?: string;
    selectedTaskDescription?: string;
    selectedTaskSidecarMode?: SidecarMode;
    selectedPaneUuid?: string;
    selectedCurrentCommand?: string;
  }>(),
  {
    isMobile: false,
    projects: () => [],
    overviewProjectId: "",
    selectedTaskId: "",
    selectedTaskMessages: () => [],
    selectedTaskTitle: "",
    selectedTaskDescription: "",
    selectedTaskSidecarMode: "advisor",
    selectedPaneUuid: "",
    selectedCurrentCommand: ""
  }
);

const emit = defineEmits<{
  (event: "update:open", open: boolean): void;
  (event: "select-project", projectId: string): void;
  (event: "select-task", taskId: string): void;
  (event: "send-message", payload: { content: string }): void;
  (event: "set-sidecar-mode", payload: { mode: SidecarMode }): void;
  (event: "stop-running-assistant-message"): void;
}>();

const { t } = useI18n();
const mobileActiveTab = ref<"projects" | "tasks" | "chat">("tasks");
const localProjectId = ref("");

const effectiveProjects = computed<ProjectSection[]>(() => {
  if ((props.projects ?? []).length > 0) {
    return props.projects;
  }
  return mockProjects;
});

watch(
  () => [props.open, props.overviewProjectId, effectiveProjects.value.length] as const,
  () => {
    if (!props.open) {
      return;
    }
    const fromProp = String(props.overviewProjectId ?? "").trim();
    const first = effectiveProjects.value[0]?.projectId ?? "";
    const hasFromProp = effectiveProjects.value.some((item) => item.projectId === fromProp);
    const hasInbox = effectiveProjects.value.some((item) => item.projectId === "inbox");
    if (fromProp && hasFromProp) {
      localProjectId.value = fromProp;
    } else if (hasInbox) {
      localProjectId.value = "inbox";
    } else {
      localProjectId.value = first;
    }
    mobileActiveTab.value = "tasks";
  },
  { immediate: true }
);

const activeProjectId = computed(() => localProjectId.value || effectiveProjects.value[0]?.projectId || "");
const activeProject = computed(() => effectiveProjects.value.find((item) => item.projectId === activeProjectId.value) ?? null);
const activeTasks = computed(() => activeProject.value?.tasks ?? []);
const effectiveTaskId = computed(() => {
  const selected = String(props.selectedTaskId ?? "").trim();
  if (selected) {
    return selected;
  }
  return activeTasks.value[0]?.taskId ?? "";
});
const effectiveTaskTitle = computed(() => {
  if (props.selectedTaskTitle) {
    return props.selectedTaskTitle;
  }
  return activeTasks.value.find((task) => task.taskId === effectiveTaskId.value)?.title ?? "";
});

function closeSheet() {
  emit("update:open", false);
}

function selectProject(projectId: string) {
  localProjectId.value = projectId;
  emit("select-project", projectId);
}

function selectTask(taskId: string) {
  emit("select-task", taskId);
}
</script>

<template>
  <Sheet :open="props.open" @update:open="(next) => emit('update:open', Boolean(next))">
    <SheetContent
      side="bottom"
      class="h-[90vh] top-[10vh] max-w-none p-0 rounded-t-xl border-x-0 border-b-0"
      @escape-key-down="closeSheet"
      @pointer-down-outside="closeSheet"
    >
      <div class="h-full flex flex-col bg-background">
        <header class="h-10 border-b border-border/60 px-3 flex items-center justify-between shrink-0">
          <h2 class="text-sm font-semibold">{{ t("overview.title") }}</h2>
          <Button
            variant="ghost"
            size="icon"
            class="h-7 w-7 text-muted-foreground"
            data-test-id="shellman-overview-close"
            @click="closeSheet"
          >
            <X class="h-4 w-4" />
          </Button>
        </header>

        <template v-if="!props.isMobile">
          <div data-test-id="shellman-overview-layout-desktop" class="h-full min-h-0 flex">
            <section
              data-test-id="shellman-overview-col-projects"
              style="width: 20%;"
              class="h-full min-h-0 border-r border-border/60 p-2 overflow-y-auto"
            >
              <div class="space-y-1">
                <button
                  v-for="project in effectiveProjects"
                  :key="project.projectId"
                  type="button"
                  class="w-full text-left px-2 py-1.5 rounded text-sm hover:bg-accent/60"
                  :class="project.projectId === activeProjectId ? 'bg-accent' : ''"
                  :data-test-id="`shellman-overview-project-${project.projectId}`"
                  @click="selectProject(project.projectId)"
                >
                  {{ project.title }}
                </button>
              </div>
            </section>
            <section
              data-test-id="shellman-overview-col-tasks"
              style="width: 45%;"
              class="h-full min-h-0 border-r border-border/60 p-2 overflow-y-auto"
            >
              <div class="space-y-1.5">
                <button
                  v-for="task in activeTasks"
                  :key="task.taskId"
                  type="button"
                  class="w-full text-left px-2 py-2 rounded border border-border/50 hover:bg-accent/40"
                  :data-test-id="`shellman-overview-task-${task.taskId}`"
                  @click="selectTask(task.taskId)"
                >
                  <div class="text-sm">{{ task.title }}</div>
                  <div class="text-[11px] text-muted-foreground mt-0.5">{{ task.status }}</div>
                </button>
              </div>
            </section>
            <section
              data-test-id="shellman-overview-col-chat"
              style="width: 35%;"
              class="h-full min-h-0 p-2"
            >
              <ThreadPanel
                :task-id="effectiveTaskId"
                :task-title="effectiveTaskTitle"
                :task-description="props.selectedTaskDescription"
                :task-messages="props.selectedTaskMessages"
                :sidecar-mode="props.selectedTaskSidecarMode"
                :pane-uuid="props.selectedPaneUuid"
                :current-command="props.selectedCurrentCommand"
                @send-message="(payload) => emit('send-message', payload)"
                @set-sidecar-mode="(payload) => emit('set-sidecar-mode', payload)"
                @stop-running-assistant-message="() => emit('stop-running-assistant-message')"
              />
            </section>
          </div>
        </template>

        <template v-else>
          <div class="h-full min-h-0 flex flex-col">
            <div class="h-10 px-2 border-b border-border/60 grid grid-cols-3 gap-1">
              <Button
                data-test-id="shellman-overview-tab-projects"
                variant="ghost"
                size="sm"
                :class="mobileActiveTab === 'projects' ? 'bg-accent' : ''"
                @click="mobileActiveTab = 'projects'"
              >
                {{ t("overview.projects") }}
              </Button>
              <Button
                data-test-id="shellman-overview-tab-tasks"
                variant="ghost"
                size="sm"
                :class="mobileActiveTab === 'tasks' ? 'bg-accent' : ''"
                @click="mobileActiveTab = 'tasks'"
              >
                {{ t("overview.tasks") }}
              </Button>
              <Button
                data-test-id="shellman-overview-tab-chat"
                variant="ghost"
                size="sm"
                :class="mobileActiveTab === 'chat' ? 'bg-accent' : ''"
                @click="mobileActiveTab = 'chat'"
              >
                {{ t("overview.chat") }}
              </Button>
            </div>

            <div v-if="mobileActiveTab === 'projects'" data-test-id="shellman-overview-mobile-projects" class="flex-1 min-h-0 overflow-y-auto p-2 space-y-1">
              <button
                v-for="project in effectiveProjects"
                :key="project.projectId"
                type="button"
                class="w-full text-left px-2 py-1.5 rounded text-sm hover:bg-accent/60"
                :class="project.projectId === activeProjectId ? 'bg-accent' : ''"
                @click="selectProject(project.projectId)"
              >
                {{ project.title }}
              </button>
            </div>

            <div v-else-if="mobileActiveTab === 'tasks'" data-test-id="shellman-overview-mobile-tasks" class="flex-1 min-h-0 overflow-y-auto p-2 space-y-1.5">
              <button
                v-for="task in activeTasks"
                :key="task.taskId"
                type="button"
                class="w-full text-left px-2 py-2 rounded border border-border/50 hover:bg-accent/40"
                @click="selectTask(task.taskId)"
              >
                <div class="text-sm">{{ task.title }}</div>
                <div class="text-[11px] text-muted-foreground mt-0.5">{{ task.status }}</div>
              </button>
            </div>

            <div v-else data-test-id="shellman-overview-mobile-chat" class="flex-1 min-h-0 p-2 flex flex-col">
              <ThreadPanel
                :task-id="effectiveTaskId"
                :task-title="effectiveTaskTitle"
                :task-description="props.selectedTaskDescription"
                :task-messages="props.selectedTaskMessages"
                :sidecar-mode="props.selectedTaskSidecarMode"
                :pane-uuid="props.selectedPaneUuid"
                :current-command="props.selectedCurrentCommand"
                @send-message="(payload) => emit('send-message', payload)"
                @set-sidecar-mode="(payload) => emit('set-sidecar-mode', payload)"
                @stop-running-assistant-message="() => emit('stop-running-assistant-message')"
              />
            </div>
          </div>
        </template>
      </div>
    </SheetContent>
  </Sheet>
</template>
