<script setup lang="ts">
import { ref } from "vue";
import { useI18n } from "vue-i18n";
import type { ProjectSection } from "./ProjectTaskTree.vue";
import type { TaskMessage, TerminalFrame } from "@/stores/shellman";
import ProjectInfoPanel from "./ProjectInfoPanel.vue";
import ProjectTaskTree from "./ProjectTaskTree.vue";
import TerminalPane from "./TerminalPane.vue";
import TaskTitleResolver from "./TaskTitleResolver.vue";
import { 
  Settings,
  ChevronLeft,
  Info,
  Sun,
  Moon,
  LayoutDashboard
} from "lucide-vue-next";
import { Button } from "@/components/ui/button";
const { t } = useI18n();

type LaunchProgram = "shell" | "codex" | "claude" | "cursor";

const props = defineProps<{
  projects: ProjectSection[];
  selectedTaskId: string;
  selectedPaneUuid?: string;
  selectedTaskTitle?: string;
  selectedTaskDescription?: string;
  selectedTaskMessages?: TaskMessage[];
  selectedTaskNotes?: Array<{ task_id: string; created_at: number; flag?: "success" | "notify" | "error"; notes: string }>;
  selectedCurrentCommand?: string;
  selectedTaskSidecarMode?: "advisor" | "observer" | "autopilot";
  selectedTaskProjectId?: string;
  selectedTaskRepoRoot?: string;
  selectedTaskProjectIsGitRepo?: boolean;
  darkMode: "light" | "dark" | "auto";
  frame?: TerminalFrame | null;
  cursor?: { x: number; y: number } | null;
  isEnded?: boolean;
  showManualLaunchPaneButton?: boolean;
  isNoPaneTask?: boolean;
  defaultLaunchProgram?: LaunchProgram;
  appPrograms?: Array<{ id: "codex" | "claude" | "cursor"; display_name: string; command: string }>;
  scmAiLoading?: boolean;
  scmSubmitLoading?: boolean;
}>();

const emit = defineEmits<{
  (event: "select-task", taskId: string): void;
  (event: "toggle-task-check", payload: { taskId: string; checked: boolean }): void;
  (event: "terminal-input", text: string): void;
  (event: "terminal-image-paste", file: File): void;
  (event: "terminal-resize", size: { cols: number; rows: number }): void;
  (event: "terminal-history-more"): void;
  (event: "manual-launch-pane", payload: { program: LaunchProgram; prompt?: string }): void;
  (event: "save-task-meta", payload: { title: string; description: string }): void;
  (event: "send-message", payload: { content: string }): void;
  (event: "set-sidecar-mode", payload: { mode: "advisor" | "observer" | "autopilot" }): void;
  (event: "stop-running-assistant-message"): void;
  (event: "add-project"): void;
  (event: "open-overview"): void;
  (event: "open-settings"): void;
  (event: "edit-project", projectId: string): void;
  (event: "create-root-pane", projectId: string): void;
  (event: "create-child-pane", taskId: string): void;
  (event: "archive-project-done", projectId: string): void;
  (event: "remove-project", projectId: string): void;
  (event: "toggle-dark"): void;
  (event: "scm-ai", payload: { taskId: string; diff: string; files: string[]; selectedFilePath: string }): void;
  (event: "scm-submit", payload: { taskId: string; message: string }): void;
  (event: "file-open", path: string): void;
}>();

const showInfoPanel = ref(false);
const projectPanelActiveTab = ref<"diff" | "file" | "thread">("thread");

function onSelectTask(taskId: string) {
  emit("select-task", taskId);
}

function goBack() {
  showInfoPanel.value = false;
  emit("select-task", "");
}

function resolveSelectedTaskTitle() {
  if (!props.selectedTaskId) {
    return "";
  }
  for (const project of props.projects) {
    const task = project.tasks.find((item) => item.taskId === props.selectedTaskId);
    if (task) {
      return task.title;
    }
  }
  return props.selectedTaskId;
}

function openSessionDetailPanel() {
  projectPanelActiveTab.value = "thread";
  showInfoPanel.value = true;
}

function onProjectPanelActiveTabChange(next: string) {
  if (next === "diff" || next === "file" || next === "thread") {
    projectPanelActiveTab.value = next;
  }
}
</script>

<template>
  <div class="shellman-mobile h-screen flex flex-col bg-background text-foreground overflow-hidden">
    <!-- Explorer Screen -->
    <template v-if="!props.selectedTaskId">
      <header class="h-14 shrink-0 flex items-center justify-between px-4 border-b border-border/50 bg-background/80 backdrop-blur-md z-10">
        <div class="flex items-center gap-2">
          <span class="text-xs font-mono font-bold uppercase tracking-[0.2em] text-primary/80">shellman</span>
        </div>
        <div class="flex items-center gap-1">
          <Button variant="ghost" size="icon" @click="emit('toggle-dark')" class="h-8 w-8 text-muted-foreground">
            <Sun v-if="props.darkMode === 'dark'" class="h-4 w-4" />
            <Moon v-else class="h-4 w-4" />
          </Button>
          <Button
            variant="ghost"
            size="icon"
            class="h-8 w-8 text-muted-foreground"
            data-test-id="shellman-mobile-open-overview"
            @click="emit('open-overview')"
          >
            <LayoutDashboard class="h-5 w-5" />
          </Button>
          <Button variant="ghost" size="icon" @click="emit('open-settings')" class="h-8 w-8 text-muted-foreground">
            <Settings class="h-5 w-5" />
          </Button>
        </div>
      </header>

      <main class="flex-1 min-h-0 relative overflow-hidden bg-sidebar">
        <ProjectTaskTree 
          :projects="props.projects" 
          :selected-task-id="props.selectedTaskId"
          :hide-footer="true"
          :show-orphan-section="false"
          @select-task="onSelectTask"
          @toggle-task-check="(payload) => emit('toggle-task-check', payload)"
          @add-project="emit('add-project')"
          @edit-project="(id) => emit('edit-project', id)"
          @create-root-pane="(id) => emit('create-root-pane', id)"
          @create-child-pane="(id) => emit('create-child-pane', id)"
          @archive-project-done="(id) => emit('archive-project-done', id)"
          @remove-project="(id) => emit('remove-project', id)"
          class="bg-transparent"
        />
      </main>
    </template>

    <!-- Console (Session) Screen -->
    <template v-else>
      <header class="h-14 shrink-0 flex items-center justify-between px-4 border-b border-border/50 bg-background/80 backdrop-blur-md z-10">
        <div class="flex items-center gap-1 -ml-2">
          <Button variant="ghost" size="icon" @click="goBack" class="h-9 w-9 text-muted-foreground mr-1">
            <ChevronLeft class="h-6 w-6" />
          </Button>
          <div class="flex flex-col">
            <span class="text-[10px] font-mono leading-none text-muted-foreground/50 uppercase tracking-widest mb-0.5">{{ t("common.session") }}</span>
            <TaskTitleResolver
              :task-title="props.selectedTaskTitle ?? resolveSelectedTaskTitle()"
              :current-command="props.selectedCurrentCommand ?? ''"
              class="text-xs font-medium leading-none text-foreground/90 max-w-[180px] truncate block"
            />
          </div>
        </div>
        <Button variant="ghost" size="icon" @click="showInfoPanel = !showInfoPanel" :class="['h-8 w-8', showInfoPanel ? 'text-primary bg-primary/10' : 'text-muted-foreground']">
          <Info class="h-5 w-5" />
        </Button>
      </header>

      <main class="flex-1 min-h-0 relative bg-black overflow-hidden">
        <div class="h-full w-full" :class="{ 'opacity-20 blur-sm pointer-events-none': showInfoPanel }">
          <slot name="terminal" v-bind="{
            taskId: props.selectedTaskId,
            taskTitle: props.selectedTaskTitle ?? resolveSelectedTaskTitle(),
            taskDescription: props.selectedTaskDescription ?? '',
            frame: props.frame ?? null,
            cursor: props.cursor ?? null,
            isEnded: Boolean(props.isEnded),
            showManualLaunchButton: Boolean(props.showManualLaunchPaneButton)
          }">
            <TerminalPane
              :task-id="props.selectedTaskId"
              :task-title="props.selectedTaskTitle ?? resolveSelectedTaskTitle()"
              :task-description="props.selectedTaskDescription ?? ''"
              :pane-uuid="props.selectedPaneUuid ?? ''"
              :current-command="props.selectedCurrentCommand ?? ''"
              :frame="props.frame ?? null"
              :cursor="props.cursor ?? null"
              :is-ended="Boolean(props.isEnded)"
              :show-manual-launch-button="Boolean(props.showManualLaunchPaneButton)"
              :is-no-pane-task="Boolean(props.isNoPaneTask)"
              :default-launch-program="props.defaultLaunchProgram ?? 'shell'"
              :app-programs="props.appPrograms ?? []"
              @terminal-input="(text) => emit('terminal-input', text)"
              @terminal-image-paste="(file) => emit('terminal-image-paste', file)"
              @terminal-resize="(size) => emit('terminal-resize', size)"
              @terminal-history-more="() => emit('terminal-history-more')"
              @manual-launch-pane="(payload) => emit('manual-launch-pane', payload)"
              @open-session-detail="openSessionDetailPanel"
            />
          </slot>
        </div>

        <!-- Overlaid Side Panel -->
        <transition 
          enter-active-class="transform transition ease-out duration-300" 
          enter-from-class="translate-x-full" 
          enter-to-class="translate-x-0" 
          leave-active-class="transform transition ease-in duration-200" 
          leave-from-class="translate-x-0" 
          leave-to-class="translate-x-full"
        >
          <aside v-if="showInfoPanel" class="absolute inset-y-0 right-0 w-[85%] bg-sidebar border-l border-border/50 shadow-2xl z-20">
             <ProjectInfoPanel
               :task-id="props.selectedTaskId"
               :project-id="props.selectedTaskProjectId ?? ''"
               :repo-root="props.selectedTaskRepoRoot ?? ''"
               :project-is-git-repo="props.selectedTaskProjectIsGitRepo !== false"
               :active-tab="projectPanelActiveTab"
               :task-title="props.selectedTaskTitle ?? resolveSelectedTaskTitle()"
               :task-description="props.selectedTaskDescription ?? ''"
               :task-messages="props.selectedTaskMessages ?? []"
               :sidecar-mode="props.selectedTaskSidecarMode ?? 'advisor'"
               :pane-uuid="props.selectedPaneUuid ?? ''"
               :current-command="props.selectedCurrentCommand ?? ''"
               :ai-loading="Boolean(props.scmAiLoading)"
               :submit-loading="Boolean(props.scmSubmitLoading)"
               @update:active-tab="onProjectPanelActiveTabChange"
               @save-task-meta="(payload) => emit('save-task-meta', payload)"
               @send-message="(payload) => emit('send-message', payload)"
               @set-sidecar-mode="(payload) => emit('set-sidecar-mode', payload)"
               @stop-running-assistant-message="() => emit('stop-running-assistant-message')"
               @ai="(payload) => emit('scm-ai', payload)"
               @submit="(payload) => emit('scm-submit', payload)"
               @file-open="(path) => emit('file-open', path)"
             />
          </aside>
        </transition>

        <!-- Tap to dismiss overlay -->
        <div v-if="showInfoPanel" @click="showInfoPanel = false" class="absolute inset-x-0 inset-y-0 z-10" />
      </main>
    </template>
  </div>
</template>

<style scoped>
.shellman-mobile {
  position: fixed;
  top: 0;
  left: 0;
  right: 0;
  bottom: 0;
  z-index: 100;
}
</style>
