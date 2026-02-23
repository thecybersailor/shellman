<script setup lang="ts">
import { computed, ref, watch } from "vue";
import { useI18n } from "vue-i18n";
import { Button } from "@/components/ui/button";
import { Sheet, SheetContent } from "@/components/ui/sheet";
import type { ProjectSection } from "./ProjectTaskTree.vue";
import ConversationSession from "./ConversationSession.vue";
import TaskTitleResolver from "./TaskTitleResolver.vue";
import type { TaskMessage } from "@/stores/shellman";

type SidecarMode = "advisor" | "observer" | "autopilot";

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
const promptDraft = ref("");

const effectiveProjects = computed<ProjectSection[]>(() => {
  return props.projects ?? [];
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
    if (fromProp && hasFromProp) {
      localProjectId.value = fromProp;
    } else {
      localProjectId.value = first;
    }
    mobileActiveTab.value = "tasks";
    promptDraft.value = "";
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
      class="h-[90vh] top-[10vh] max-w-none p-0 rounded-t-xl border-none bg-transparent outline-none"
      @escape-key-down="closeSheet"
      @pointer-down-outside="closeSheet"
    >
      <div class="h-full flex flex-col bg-gray-200 dark:bg-gray-800 rounded-t-xl overflow-hidden">
        <header class="h-10 border-b border-border/60 px-3 flex items-center justify-between shrink-0">
          <h2 class="text-sm font-semibold">{{ t("overview.title") }}</h2>
        </header>

        <template v-if="!props.isMobile">
          <div data-test-id="shellman-overview-layout-desktop" class="h-full min-h-0 flex">
            <section
              data-test-id="shellman-overview-col-projects"
              class="h-full min-h-0 w-[220px] shrink-0 border-r border-border/60 p-3 overflow-y-auto"
            >
              <div class="space-y-1">
                <button
                  v-for="project in effectiveProjects"
                  :key="project.projectId"
                  type="button"
                  class="w-full text-left px-3 py-2 rounded-md text-sm transition-colors"
                  :class="project.projectId === activeProjectId ? 'bg-primary/10 text-primary font-medium' : 'text-muted-foreground hover:bg-muted hover:text-foreground'"
                  :data-test-id="`shellman-overview-project-${project.projectId}`"
                  @click="selectProject(project.projectId)"
                >
                  {{ project.title }}
                </button>
              </div>
            </section>
            <section
              data-test-id="shellman-overview-col-tasks"
              class="h-full min-h-0 flex-1 border-r border-border/60 p-4 overflow-y-auto"
            >
              <div class="space-y-2">
                <button
                  v-for="task in activeTasks"
                  :key="task.taskId"
                  type="button"
                  class="w-full text-left px-4 py-3 rounded-lg border transition-all duration-200"
                  :class="task.taskId === effectiveTaskId ? 'border-border/50 bg-primary/5' : 'border-border/50 bg-card hover:border-primary/30 hover:shadow-sm'"
                  :data-test-id="`shellman-overview-task-${task.taskId}`"
                  @click="selectTask(task.taskId)"
                >
                  <TaskTitleResolver
                    :task-title="task.title"
                    :current-command="task.currentCommand"
                    class="text-sm font-medium block truncate"
                    :class="task.taskId === effectiveTaskId ? 'text-primary' : 'text-foreground'"
                  />
                  <div class="text-[11px] mt-1.5 flex items-center gap-1.5" :class="task.taskId === effectiveTaskId ? 'text-primary/70' : 'text-muted-foreground'">
                    <span class="w-1.5 h-1.5 rounded-full" :class="{
                      'bg-green-500': task.status === 'completed',
                      'bg-yellow-500': task.status === 'running',
                      'bg-blue-500': task.status === 'waiting_user',
                      'bg-gray-400': !['completed', 'running', 'waiting_user'].includes(task.status)
                    }"></span>
                    {{ task.status }}
                  </div>
                </button>
              </div>
            </section>
            <section
              data-test-id="shellman-overview-col-chat"
              class="h-full min-h-0 w-[400px] shrink-0 p-2 bg-gray-200 dark:bg-gray-800"
            >
              <div class="h-full min-h-0 overflow-hidden border border-border/60 rounded-md p-1.5 bg-background/40 flex flex-col">
                <div class="px-1.5 pb-1 text-xs font-semibold text-muted-foreground">{{ t("overview.projectManager") }}</div>
                <ConversationSession
                  :task-id="effectiveTaskId"
                  :task-messages="props.selectedTaskMessages"
                  :model-value="promptDraft"
                  @update:model-value="(value) => (promptDraft = value)"
                  @submit-message="(payload) => emit('send-message', payload)"
                  @stop-running-assistant-message="() => emit('stop-running-assistant-message')"
                />
              </div>
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

            <div v-if="mobileActiveTab === 'projects'" data-test-id="shellman-overview-mobile-projects" class="flex-1 min-h-0 overflow-y-auto p-3 space-y-1">
              <button
                v-for="project in effectiveProjects"
                :key="project.projectId"
                type="button"
                class="w-full text-left px-3 py-2 rounded-md text-sm transition-colors"
                :class="project.projectId === activeProjectId ? 'bg-primary/10 text-primary font-medium' : 'text-muted-foreground hover:bg-muted hover:text-foreground'"
                @click="selectProject(project.projectId)"
              >
                {{ project.title }}
              </button>
            </div>

            <div v-else-if="mobileActiveTab === 'tasks'" data-test-id="shellman-overview-mobile-tasks" class="flex-1 min-h-0 overflow-y-auto p-4 space-y-2">
              <button
                v-for="task in activeTasks"
                :key="task.taskId"
                type="button"
                class="w-full text-left px-4 py-3 rounded-lg border transition-all duration-200"
                :class="task.taskId === effectiveTaskId ? 'border-border/50 bg-primary/5' : 'border-border/50 bg-card hover:border-primary/30 hover:shadow-sm'"
                @click="selectTask(task.taskId)"
              >
                <TaskTitleResolver
                  :task-title="task.title"
                  :current-command="task.currentCommand"
                  class="text-sm font-medium block truncate"
                  :class="task.taskId === effectiveTaskId ? 'text-primary' : 'text-foreground'"
                />
                <div class="text-[11px] mt-1.5 flex items-center gap-1.5" :class="task.taskId === effectiveTaskId ? 'text-primary/70' : 'text-muted-foreground'">
                    <span class="w-1.5 h-1.5 rounded-full" :class="{
                      'bg-green-500': task.status === 'completed',
                      'bg-yellow-500': task.status === 'running',
                      'bg-blue-500': task.status === 'waiting_user',
                      'bg-gray-400': !['completed', 'running', 'waiting_user'].includes(task.status)
                    }"></span>
                    {{ task.status }}
                </div>
              </button>
            </div>

            <div v-else data-test-id="shellman-overview-mobile-chat" class="flex-1 min-h-0 p-2 flex flex-col">
              <div class="h-full min-h-0 overflow-hidden border border-border/60 rounded-md p-1.5 bg-background/40 flex flex-col">
                <div class="px-1.5 pb-1 text-xs font-semibold text-muted-foreground">{{ t("overview.projectManager") }}</div>
                <ConversationSession
                  :task-id="effectiveTaskId"
                  :task-messages="props.selectedTaskMessages"
                  :model-value="promptDraft"
                  @update:model-value="(value) => (promptDraft = value)"
                  @submit-message="(payload) => emit('send-message', payload)"
                  @stop-running-assistant-message="() => emit('stop-running-assistant-message')"
                />
              </div>
            </div>
          </div>
        </template>
      </div>
    </SheetContent>
  </Sheet>
</template>
