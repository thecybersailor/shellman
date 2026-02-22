<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, ref, watch } from "vue";
import { useI18n } from "vue-i18n";
import { useRoute, useRouter } from "vue-router";
import TerminalPane from "./components/TerminalPane.vue";
import ProjectInfoPanel from "./components/ProjectInfoPanel.vue";
import ProjectTaskTree, { type ProjectSection } from "./components/ProjectTaskTree.vue";
import MobileStackView from "./components/MobileStackView.vue";
import ActiveProjectEntry from "./components/ActiveProjectEntry.vue";
import SettingsPanel from "./components/SettingsPanel.vue";
import { createShellmanStore } from "./stores/shellman";
import type { TerminalFrame } from "./stores/shellman";
import { ResizableHandle, ResizablePanel, ResizablePanelGroup } from "@/components/ui/resizable";
import { Button } from "@/components/ui/button";
import { Textarea } from "@/components/ui/textarea";
import { Sheet, SheetContent, SheetHeader, SheetTitle } from "@/components/ui/sheet";
import {
  AlertDialog,
  AlertDialogAction,
  AlertDialogCancel,
  AlertDialogContent,
  AlertDialogDescription,
  AlertDialogFooter,
  AlertDialogHeader,
  AlertDialogTitle
} from "@/components/ui/alert-dialog";
import { Toaster } from "@/components/ui/sonner";
import { Sun, Moon } from "lucide-vue-next";
import shellmanIcon from "@/asserts/icon.svg";
import { useColorMode } from "@vueuse/core";
import { toast } from "vue-sonner";
const { t } = useI18n();

type LaunchProgram = "shell" | "codex" | "claude" | "cursor";

const route = useRoute();
const router = useRouter();

const store = createShellmanStore();
const mode = useColorMode();

const showAddProjectDialog = ref(false);
const showSettingsPanel = ref(false);
const settingsSaving = ref(false);
const scmAiLoading = ref(false);
const scmSubmitLoading = ref(false);
const fileViewerOpen = ref(false);
const fileViewerLoading = ref(false);
const fileViewerPath = ref("");
const fileViewerContent = ref("");
const showRemoveProjectDialog = ref(false);
const pendingRemoveProjectId = ref("");
const projectPanelActiveTab = ref<"diff" | "file" | "thread">("thread");
const isMobileLayout = ref(false);
let mobileLayoutMediaQuery: MediaQueryList | null = null;
let mobileLayoutListener: ((event: MediaQueryListEvent) => void) | null = null;

function logInfo(event: string, payload: Record<string, unknown> = {}) {
  if (import.meta.env.MODE === "test") {
    return;
  }
  const record = {
    ts: new Date().toISOString(),
    event,
    ...payload
  };
  try {
    console.info(JSON.stringify(record));
  } catch {
    console.info(
      JSON.stringify({
        ts: new Date().toISOString(),
        event,
        payload: "[unserializable]"
      })
    );
  }
}

function notifyError(message: string) {
  const text = String(message || "UNKNOWN_ERROR");
  toast.error(text);
}

const projects = computed<ProjectSection[]>(() => {
  return store.state.projects.map((project) => ({
    projectId: project.projectId,
    title: project.projectId,
    tasks: (store.state.treesByProject[project.projectId] ?? []).map((node) => {
      const paneCommand = String(
        store.state.paneMetaByTarget[String(store.state.paneByTaskId[node.taskId]?.paneTarget ?? "")]?.currentCommand ?? ""
      ).trim();
      return {
        currentCommand: paneCommand || String(node.currentCommand ?? "").trim(),
        taskId: node.taskId,
        parentTaskId: node.parentTaskId,
        title: node.title,
        checked: Boolean(node.checked),
        archived: Boolean(node.archived),
        flag: node.flag,
        flagDesc: node.flagDesc,
        flagReaded: Boolean(node.flagReaded),
        updatedAt: node.updatedAt,
        status: (node.status as
          | "pending"
          | "running"
          | "waiting_user"
          | "waiting_children"
          | "completed"
          | "failed"
          | "canceled"),
        runtimeStatus: store.state.taskRuntimeByTaskId[node.taskId]?.status,
        runtimeUpdatedAt: store.state.taskRuntimeByTaskId[node.taskId]?.updatedAt
      };
    })
  }));
});

const selectedTaskId = computed(() => store.state.selectedTaskId);
const selectedPaneUuid = computed(() => store.state.selectedPaneUuid);
const selectedPaneTarget = computed(() => {
  const selected = store.state.selectedPaneTarget;
  if (selected) {
    return selected;
  }
  const taskId = store.state.selectedTaskId;
  if (!taskId) {
    return "";
  }
  return store.state.paneByTaskId[taskId]?.paneTarget ?? "";
});
const selectedPaneMeta = computed(() => {
  const target = selectedPaneTarget.value;
  if (!target) {
    return null;
  }
  return store.state.paneMetaByTarget[target] ?? null;
});
const selectedCurrentCommand = computed(() => String(selectedPaneMeta.value?.currentCommand ?? "").trim());
const selectedTaskNode = computed(() => {
  const taskId = store.state.selectedTaskId;
  if (!taskId) {
    return null;
  }
  for (const nodes of Object.values(store.state.treesByProject)) {
    const found = nodes.find((node) => node.taskId === taskId);
    if (found) {
      return found;
    }
  }
  return null;
});
const selectedTaskProjectRoot = computed(() => {
  const taskId = store.state.selectedTaskId;
  if (!taskId) {
    return "";
  }
  for (const [projectId, nodes] of Object.entries(store.state.treesByProject)) {
    if (nodes.some((node) => node.taskId === taskId)) {
      const project = store.state.projects.find((item) => item.projectId === projectId);
      return String(project?.repoRoot ?? "");
    }
  }
  return "";
});
const selectedTaskTitle = computed(() => String(selectedTaskNode.value?.title ?? ""));
const selectedTaskDescription = computed(() => String(selectedTaskNode.value?.description ?? ""));
const selectedTaskMessages = computed(() => store.state.taskMessagesByTaskId[store.state.selectedTaskId] ?? []);
const selectedTaskNotes = computed(() => []);
const selectedTaskFrame = computed<TerminalFrame>(() => store.state.terminalFrame);
const selectedTaskCursor = computed(() => store.state.terminalCursor);
const selectedTaskEnded = computed(() => store.state.terminalEnded);
const selectedTaskHasPaneBinding = computed(() => {
  const taskId = store.state.selectedTaskId;
  if (!taskId) {
    return false;
  }
  const binding = store.state.paneByTaskId[taskId];
  return Boolean(binding?.paneTarget);
});
const selectedTaskIsNoPane = computed(() => Boolean(store.state.selectedTaskId) && !selectedTaskHasPaneBinding.value);
const orphanPanes = computed(() => store.getOrphanPaneItems());
const appBootstrapped = ref(false);
const showReopenPaneButton = computed(() => {
  if (!store.state.selectedTaskId) {
    return false;
  }
  if (selectedTaskIsNoPane.value) {
    return true;
  }
  return store.state.terminalFrame.mode === "reset" && store.state.terminalFrame.data.includes(t("app.sessionEndedPaneUnavailable"));
});
const projectEntryError = ref("");
const taskActionError = ref("");

async function onSelectTask(taskId: string) {
  if (taskId) {
    void store.markTaskFlagReaded(taskId, true).catch(() => {
      // do not block task selection when readed mark fails
    });
  }
  if (taskId && taskId !== route.params.sessionId) {
    router.push({ name: 'session', params: { sessionId: taskId } });
  } else {
    await store.selectTask(taskId);
  }
}

// Sync route param to store
watch(() => route.params.sessionId, async (newId) => {
  if (!appBootstrapped.value) {
    return;
  }
  const id = Array.isArray(newId) ? newId[0] : newId;
  if (id && id !== store.state.selectedTaskId) {
    await store.selectTask(id);
  }
});

async function onCreateChild(taskId: string) {
  try {
    taskActionError.value = "";
    await store.createChildTask(taskId, t("app.childOfTask", { taskId }));
  } catch (err) {
    taskActionError.value = err instanceof Error ? err.message : "CHILD_CREATE_FAILED";
    notifyError(taskActionError.value);
  }
}

async function onCreateRoot(projectId: string) {
  await store.createRootTask(projectId, "");
}

async function onToggleTaskCheck(payload: { taskId: string; checked: boolean }) {
  await store.setTaskChecked(payload.taskId, payload.checked);
}

async function onAdoptPane(payload: { parentTaskId: string; paneTarget: string; title?: string }) {
  try {
    taskActionError.value = "";
    await store.adoptPaneAsChild(payload.parentTaskId, payload.paneTarget, payload.title ?? "");
  } catch (err) {
    taskActionError.value = err instanceof Error ? err.message : "PANE_ADOPT_FAILED";
    notifyError(taskActionError.value);
  }
}

async function onDirectorySelected(path: string) {
  try {
    projectEntryError.value = "";
    const id = path.split(/[\\/]/).filter(Boolean).pop() || "project";
    await store.addActiveProject(id, path);
    try {
      await store.recordDirectoryHistory(path);
    } catch {
      // keep add-project success even if history write fails
    }
    showAddProjectDialog.value = false;
  } catch (err) {
    projectEntryError.value = err instanceof Error ? err.message : "PROJECT_ADD_FAILED";
    notifyError(projectEntryError.value);
  }
}

function onOpenAddProject(source: "desktop" | "mobile") {
  logInfo("shellman.add_project.open.request", { source, selectedTaskId: store.state.selectedTaskId });
  showAddProjectDialog.value = true;
}

function onRequestRemoveProject(projectId: string) {
  pendingRemoveProjectId.value = String(projectId ?? "").trim();
  showRemoveProjectDialog.value = Boolean(pendingRemoveProjectId.value);
}

async function onArchiveProjectDone(projectId: string) {
  const id = String(projectId ?? "").trim();
  if (!id) {
    return;
  }
  try {
    taskActionError.value = "";
    await store.archiveDoneTasksByProject(id);
  } catch (err) {
    taskActionError.value = err instanceof Error ? err.message : "TASK_ARCHIVE_FAILED";
    notifyError(taskActionError.value);
  }
}

async function onConfirmRemoveProject() {
  const projectId = pendingRemoveProjectId.value;
  if (!projectId) {
    showRemoveProjectDialog.value = false;
    return;
  }
  try {
    projectEntryError.value = "";
    await store.removeActiveProject(projectId);
    showRemoveProjectDialog.value = false;
    pendingRemoveProjectId.value = "";
  } catch (err) {
    projectEntryError.value = err instanceof Error ? err.message : "PROJECT_REMOVE_FAILED";
    notifyError(projectEntryError.value);
  }
}

watch(showAddProjectDialog, (open) => {
  logInfo("shellman.add_project.open.state", { open });
});

async function onOpenSettings(source: "desktop" | "mobile") {
  logInfo("shellman.settings.open.request", { source });
  try {
    await store.loadConfig();
    logInfo("shellman.settings.config.reload.done", {
      helperOpenAIEndpoint: store.state.helperOpenAIEndpoint,
      helperOpenAIModel: store.state.helperOpenAIModel
    });
  } catch (err) {
    const message = err instanceof Error ? err.message : "CONFIG_LOAD_FAILED";
    notifyError(message);
    logInfo("shellman.settings.config.reload.failed", { message });
  }
  showSettingsPanel.value = true;
}

watch(showSettingsPanel, (open) => {
  logInfo("shellman.settings.open.state", { open });
});

async function onSaveSettings(payload: {
  defaultLaunchProgram: "shell" | "codex" | "claude" | "cursor";
  defaultHelperProgram: "codex" | "claude" | "cursor";
  taskCompletionMode: "none" | "command";
  taskCompletionCommand: string;
  taskCompletionIdleDuration: number;
  helperOpenAIEndpoint: string;
  helperOpenAIModel: string;
  helperOpenAIApiKey: string;
}) {
  try {
    settingsSaving.value = true;
    await store.saveTaskCompletionSettings(payload.defaultLaunchProgram, payload.defaultHelperProgram, {
      taskCompletionMode: payload.taskCompletionMode,
      taskCompletionCommand: payload.taskCompletionCommand,
      taskCompletionIdleDuration: payload.taskCompletionIdleDuration,
      helperOpenAIEndpoint: payload.helperOpenAIEndpoint,
      helperOpenAIModel: payload.helperOpenAIModel,
      helperOpenAIApiKey: payload.helperOpenAIApiKey
    });
    showSettingsPanel.value = false;
  } catch (err) {
    taskActionError.value = err instanceof Error ? err.message : "SETTINGS_SAVE_FAILED";
    notifyError(taskActionError.value);
  } finally {
    settingsSaving.value = false;
  }
}

function onTerminalInput(text: string) {
  store.sendTerminalInput(text);
}

function onTerminalImagePaste(file: File) {
  void store.sendImagePasteToTerminal(file).catch((err) => {
    console.error("upload image for terminal input failed", err);
    notifyError(err instanceof Error ? err.message : "IMAGE_UPLOAD_FAILED");
  });
}

function onTerminalResize(size: { cols: number; rows: number }) {
  store.sendTerminalResize(size);
}

async function onTerminalHistoryMore() {
  const taskId = store.state.selectedTaskId;
  if (!taskId) {
    return;
  }
  try {
    await store.loadMorePaneHistory(taskId);
  } catch (err) {
    const code = err instanceof Error ? err.message : "TASK_PANE_HISTORY_LOAD_FAILED";
    logInfo("shellman.term.history_more.error", { taskId, code });
  }
}

function onOpenSessionDetail() {
  projectPanelActiveTab.value = "thread";
}

function onProjectPanelActiveTabChange(next: string) {
  if (next === "diff" || next === "file" || next === "thread") {
    projectPanelActiveTab.value = next;
  }
}

function findProjectIdByTask(taskId: string) {
  if (!taskId) {
    return "";
  }
  for (const [projectId, nodes] of Object.entries(store.state.treesByProject)) {
    if (nodes.some((node) => node.taskId === taskId)) {
      return projectId;
    }
  }
  return "";
}

async function onReopenPane(payload: { program: LaunchProgram; prompt?: string }) {
  if (!store.state.selectedTaskId) {
    return;
  }
  try {
    taskActionError.value = "";
    await store.reopenPaneForTask(store.state.selectedTaskId, payload);
  } catch (err) {
    taskActionError.value = err instanceof Error ? err.message : "REOPEN_PANE_FAILED";
    notifyError(taskActionError.value);
  }
}

async function onSaveTaskMeta(payload: { title: string; description: string }) {
  const taskId = store.state.selectedTaskId;
  if (!taskId) {
    return;
  }
  try {
    taskActionError.value = "";
    await store.setTaskTitle(taskId, payload.title);
    await store.setTaskDescription(taskId, payload.description);
  } catch (err) {
    taskActionError.value = err instanceof Error ? err.message : "TASK_META_UPDATE_FAILED";
    notifyError(taskActionError.value);
  }
}

async function onSendTaskMessage(payload: { content: string }) {
  const taskId = String(store.state.selectedTaskId ?? "").trim();
  const content = String(payload.content ?? "").trim();
  if (!taskId || !content) {
    return;
  }
  try {
    await store.sendTaskMessage(taskId, content);
  } catch (err) {
    notifyError(err instanceof Error ? err.message : "TASK_MESSAGE_SEND_FAILED");
  }
}

async function onSetTaskSidecarMode(payload: { mode: "advisor" | "observer" | "autopilot" }) {
  const taskId = String(store.state.selectedTaskId ?? "").trim();
  if (!taskId) {
    return;
  }
  try {
    await store.setTaskSidecarMode(taskId, payload.mode);
  } catch (err) {
    notifyError(err instanceof Error ? err.message : "TASK_SIDECAR_MODE_UPDATE_FAILED");
  }
}

async function onStopRunningAssistantMessage() {
  const taskId = String(store.state.selectedTaskId ?? "").trim();
  if (!taskId) {
    return;
  }
  try {
    await store.stopTaskMessage(taskId);
  } catch (err) {
    notifyError(err instanceof Error ? err.message : "TASK_MESSAGE_STOP_FAILED");
  }
}

async function onSCMAI(payload: { taskId: string; diff: string; files: string[]; selectedFilePath: string }) {
  if (scmAiLoading.value) {
    return;
  }
  try {
    scmAiLoading.value = true;
    logInfo("shellman.scm.ai.request", {
      taskId: payload.taskId,
      files: payload.files.length,
      selectedFilePath: payload.selectedFilePath
    });
  } finally {
    scmAiLoading.value = false;
  }
}

async function onSCMSubmit(payload: { taskId: string; message: string }) {
  if (scmSubmitLoading.value) {
    return;
  }
  try {
    scmSubmitLoading.value = true;
    taskActionError.value = "";
    logInfo("shellman.scm.submit.request", {
      taskId: payload.taskId,
      messageLength: payload.message.length
    });
    await store.submitTaskCommit(payload.taskId, payload.message);
  } catch (err) {
    taskActionError.value = err instanceof Error ? err.message : "SCM_SUBMIT_FAILED";
    notifyError(taskActionError.value);
  } finally {
    scmSubmitLoading.value = false;
  }
}

async function onFileOpen(path: string) {
  const taskId = store.state.selectedTaskId;
  if (!taskId || !path) {
    return;
  }
  fileViewerOpen.value = true;
  fileViewerLoading.value = true;
  fileViewerPath.value = path;
  fileViewerContent.value = "";
  try {
    const encodedPath = encodeURIComponent(path);
    const res = (await fetch(`/api/v1/tasks/${taskId}/files/content?path=${encodedPath}`).then((r) => r.json())) as {
      ok: boolean;
      data?: { content?: string };
      error?: { code?: string; message?: string };
    };
    if (!res.ok) {
      throw new Error(String(res.error?.code ?? "TASK_FILE_CONTENT_LOAD_FAILED"));
    }
    fileViewerContent.value = String(res.data?.content ?? "");
  } catch (err) {
    notifyError(err instanceof Error ? err.message : "TASK_FILE_CONTENT_LOAD_FAILED");
  } finally {
    fileViewerLoading.value = false;
  }
}

onMounted(async () => {
  if (typeof window !== "undefined" && typeof window.matchMedia === "function") {
    mobileLayoutMediaQuery = window.matchMedia("(max-width: 980px)");
    isMobileLayout.value = mobileLayoutMediaQuery.matches;
    mobileLayoutListener = (event: MediaQueryListEvent) => {
      isMobileLayout.value = event.matches;
    };
    mobileLayoutMediaQuery.addEventListener("change", mobileLayoutListener);
  }
  logInfo("shellman.app.mounted.start");
  const routeSessionId = Array.isArray(route.params.sessionId) ? route.params.sessionId[0] : route.params.sessionId;
  try {
    await store.load({
      preferredTaskId: routeSessionId,
      prefetchAllTaskPanes: false
    });
  } catch (err) {
    notifyError(err instanceof Error ? err.message : "APP_LOAD_FAILED");
  }
  logInfo("shellman.app.load.done", { selectedTaskId: store.state.selectedTaskId });
  try {
    await store.loadAppPrograms();
    logInfo("shellman.app.app_programs.load.done", { count: store.state.appPrograms.length });
  } catch (err) {
    notifyError(err instanceof Error ? err.message : "APP_PROGRAMS_LOAD_FAILED");
    logInfo("shellman.app.app_programs.load.failed");
  }
  try {
    await store.loadConfig();
    logInfo("shellman.app.config.load.done", {
      defaultLaunchProgram: store.state.defaultLaunchProgram,
      defaultHelperProgram: store.state.defaultHelperProgram
    });
  } catch (err) {
    notifyError(err instanceof Error ? err.message : "CONFIG_LOAD_FAILED");
    logInfo("shellman.app.config.load.failed");
  }
  store.connectWS();
  logInfo("shellman.app.ws.connect.called");

  const initialTaskId = routeSessionId || store.state.selectedTaskId;
  if (initialTaskId) {
    await store.selectTask(initialTaskId);
  }
  if (!routeSessionId && store.state.selectedTaskId) {
    router.replace({ name: 'session', params: { sessionId: store.state.selectedTaskId } });
  }
  appBootstrapped.value = true;
});

onBeforeUnmount(() => {
  if (mobileLayoutMediaQuery && mobileLayoutListener) {
    mobileLayoutMediaQuery.removeEventListener("change", mobileLayoutListener);
  }
  mobileLayoutMediaQuery = null;
  mobileLayoutListener = null;
  logInfo("shellman.app.before_unmount");
  store.disconnectWS();
});
</script>

<template>
  <div v-if="!isMobileLayout" class="shellman-desktop h-screen w-full bg-background text-foreground">
    <ResizablePanelGroup direction="horizontal" class="h-full items-stretch">
      <!-- Left Panel -->
      <ResizablePanel :default-size="20" :min-size="15" :max-size="30" class="min-w-[340px]">
        <aside class="h-full flex flex-col bg-sidebar">
          <!-- Top Global Actions (Theme Toggle for Shellman) -->
          <header class="flex items-center justify-between px-4 py-4 opacity-50 hover:opacity-100 transition-opacity">
            <span class="flex items-center gap-1">
              <img :src="shellmanIcon" alt="" class="h-6 max-h-full dark:invert" />
              <span>shellman</span>
            </span>
            <Button variant="ghost" size="icon" @click="mode = mode === 'dark' ? 'light' : 'dark'" class="h-6 w-6 rounded-md">
              <Sun v-if="mode === 'dark'" class="h-3.5 w-3.5" />
              <Moon v-else class="h-3.5 w-3.5" />
            </Button>
          </header>

          <!-- The main content: Projects / Tree / Settings -->
          <div class="flex-1 overflow-hidden">
            <ProjectTaskTree
              :projects="projects"
              :selected-task-id="selectedTaskId"
              :orphan-panes="orphanPanes"
              :show-orphan-section="store.state.paneLookupComplete"
              @select-task="onSelectTask"
              @toggle-task-check="onToggleTaskCheck"
              @create-root-pane="onCreateRoot"
              @create-child-pane="onCreateChild"
              @adopt-pane="onAdoptPane"
              @add-project="onOpenAddProject('desktop')"
              @archive-project-done="onArchiveProjectDone"
              @remove-project="onRequestRemoveProject"
              @open-settings="onOpenSettings('desktop')"
            />
          </div>
          
          <p v-if="projectEntryError" class="absolute bottom-16 left-4 text-xs text-destructive bg-background/80 px-2 py-1 rounded shadow-sm z-50">
            {{ projectEntryError }}
          </p>
          <p v-if="taskActionError" class="absolute bottom-10 left-4 text-xs text-destructive bg-background/80 px-2 py-1 rounded shadow-sm z-50">
            {{ taskActionError }}
          </p>
        </aside>
      </ResizablePanel>

      <ResizableHandle />

      <!-- Center Panel (Terminal) -->
      <ResizablePanel :default-size="48" :min-size="30">
        <section class="h-full p-2 bg-background">
          <slot name="terminal" v-bind="{ 
            selectedTaskId, 
            selectedTaskTitle, 
            selectedTaskDescription, 
            selectedTaskMessages,
            selectedPaneUuid,
            selectedCurrentCommand,
            selectedTaskFrame,
            selectedTaskCursor,
            selectedTaskEnded,
            showReopenPaneButton,
            selectedTaskIsNoPane,
            defaultLaunchProgram: store.state.defaultLaunchProgram,
            appPrograms: store.state.appPrograms
          }">
            <TerminalPane
              :task-id="selectedTaskId"
              :task-title="selectedTaskTitle"
              :task-description="selectedTaskDescription"
              :task-messages="selectedTaskMessages"
              :pane-uuid="selectedPaneUuid"
              :current-command="selectedCurrentCommand"
              :frame="selectedTaskFrame"
              :cursor="selectedTaskCursor"
              :is-ended="selectedTaskEnded"
              :show-reopen-button="showReopenPaneButton"
              :is-no-pane-task="selectedTaskIsNoPane"
              :default-launch-program="store.state.defaultLaunchProgram"
              :app-programs="store.state.appPrograms"
              @terminal-input="onTerminalInput"
              @terminal-image-paste="onTerminalImagePaste"
              @terminal-resize="onTerminalResize"
              @terminal-history-more="onTerminalHistoryMore"
              @reopen-pane="onReopenPane"
              @open-session-detail="onOpenSessionDetail"
            />
          </slot>
        </section>
      </ResizablePanel>

      <ResizableHandle />

      <!-- Right Panel (ProjectInfo) -->
      <ResizablePanel :default-size="32" :min-size="15" :max-size="40" class="min-w-[260px]">
        <aside class="h-full bg-sidebar border-l border-border">
          <ProjectInfoPanel
            :task-id="selectedTaskId"
            :project-id="findProjectIdByTask(selectedTaskId)"
            :repo-root="selectedTaskProjectRoot"
            :active-tab="projectPanelActiveTab"
            :task-title="selectedTaskTitle"
            :task-description="selectedTaskDescription"
            :task-messages="selectedTaskMessages"
            :sidecar-mode="store.state.taskSidecarModeByTaskId[selectedTaskId] || 'advisor'"
            :pane-uuid="selectedPaneUuid"
            :current-command="selectedCurrentCommand"
            :ai-loading="scmAiLoading"
            :submit-loading="scmSubmitLoading"
            @update:active-tab="onProjectPanelActiveTabChange"
            @save-task-meta="onSaveTaskMeta"
            @send-message="onSendTaskMessage"
            @set-sidecar-mode="onSetTaskSidecarMode"
            @stop-running-assistant-message="onStopRunningAssistantMessage"
            @ai="onSCMAI"
            @submit="onSCMSubmit"
            @file-open="onFileOpen"
          />
        </aside>
      </ResizablePanel>
    </ResizablePanelGroup>
  </div>

  <main v-else class="shellman-mobile-only">
    <MobileStackView 
      :projects="projects" 
      :selected-task-id="selectedTaskId" 
      :selected-pane-uuid="selectedPaneUuid"
      :selected-task-title="selectedTaskTitle"
      :selected-task-description="selectedTaskDescription"
      :selected-task-messages="selectedTaskMessages"
      :selected-task-notes="selectedTaskNotes"
      :selected-current-command="selectedCurrentCommand"
      :selected-task-sidecar-mode="store.state.taskSidecarModeByTaskId[selectedTaskId] || 'advisor'"
      :selected-task-project-id="findProjectIdByTask(selectedTaskId)"
      :selected-task-repo-root="selectedTaskProjectRoot"
      :dark-mode="mode"
      :frame="selectedTaskFrame"
      :cursor="selectedTaskCursor"
      :is-ended="selectedTaskEnded"
      :show-reopen-pane-button="showReopenPaneButton"
      :is-no-pane-task="selectedTaskIsNoPane"
      :default-launch-program="store.state.defaultLaunchProgram"
      :app-programs="store.state.appPrograms"
      :scm-ai-loading="scmAiLoading"
      :scm-submit-loading="scmSubmitLoading"
      @select-task="onSelectTask" 
      @toggle-task-check="onToggleTaskCheck"
      @terminal-input="onTerminalInput"
      @terminal-image-paste="onTerminalImagePaste"
      @terminal-resize="onTerminalResize"
      @terminal-history-more="onTerminalHistoryMore"
      @reopen-pane="onReopenPane"
      @save-task-meta="onSaveTaskMeta"
      @send-message="onSendTaskMessage"
      @set-sidecar-mode="onSetTaskSidecarMode"
      @stop-running-assistant-message="onStopRunningAssistantMessage"
      @add-project="onOpenAddProject('mobile')"
      @open-settings="onOpenSettings('mobile')"
      @create-root-pane="onCreateRoot"
      @create-child-pane="onCreateChild"
      @archive-project-done="onArchiveProjectDone"
      @remove-project="onRequestRemoveProject"
      @toggle-dark="mode = mode === 'dark' ? 'light' : 'dark'"
      @scm-ai="onSCMAI"
      @scm-submit="onSCMSubmit"
      @file-open="onFileOpen"
    />
  </main>

  <Sheet v-model:open="fileViewerOpen">
    <SheetContent side="right" class="w-full sm:max-w-2xl flex flex-col gap-3">
      <SheetHeader class="text-left">
        <SheetTitle class="text-xs font-mono break-all">{{ fileViewerPath || "File Viewer" }}</SheetTitle>
      </SheetHeader>
      <Textarea
        v-model="fileViewerContent"
        data-test-id="shellman-file-viewer-textarea"
        class="flex-1 min-h-[70vh] resize-none font-mono text-xs"
        :placeholder="fileViewerLoading ? t('common.loading') : t('common.noContent')"
      />
    </SheetContent>
  </Sheet>

  <ActiveProjectEntry
    v-model:show="showAddProjectDialog"
    :get-f-s-roots="store.getFSRoots"
    :list-directories="store.listDirectories"
    :resolve-directory="store.resolveDirectory"
    :search-directories="store.searchDirectories"
    :get-directory-history="store.getDirectoryHistory"
    :record-directory-history="store.recordDirectoryHistory"
    @select-directory="onDirectorySelected"
  />
  <SettingsPanel
    v-model:show="showSettingsPanel"
    :default-launch-program="store.state.defaultLaunchProgram"
    :default-helper-program="store.state.defaultHelperProgram"
    :providers="store.state.appPrograms"
    :task-completion-command="store.state.taskCompletionCommand"
    :task-completion-mode="store.state.taskCompletionMode"
    :task-completion-idle-duration="store.state.taskCompletionIdleDuration"
    :helper-openai-endpoint="store.state.helperOpenAIEndpoint"
    :helper-openai-model="store.state.helperOpenAIModel"
    :helper-openai-api-key="store.state.helperOpenAIApiKey"
    :saving="settingsSaving"
    @save="onSaveSettings"
  />

  <AlertDialog v-model:open="showRemoveProjectDialog">
    <AlertDialogContent data-test-id="shellman-remove-project-confirm">
      <AlertDialogHeader>
        <AlertDialogTitle>{{ t("app.removeProjectTitle") }}</AlertDialogTitle>
        <AlertDialogDescription>
          {{ t("app.removeProjectDescriptionPrefix") }} <span class="font-mono">{{ pendingRemoveProjectId }}</span> {{ t("app.removeProjectDescriptionSuffix") }}
        </AlertDialogDescription>
      </AlertDialogHeader>
      <AlertDialogFooter>
        <AlertDialogCancel>{{ t("common.cancel") }}</AlertDialogCancel>
        <AlertDialogAction class="bg-destructive text-destructive-foreground hover:bg-destructive/90" @click="onConfirmRemoveProject">
          {{ t("common.remove") }}
        </AlertDialogAction>
      </AlertDialogFooter>
    </AlertDialogContent>
  </AlertDialog>

  <Toaster />
</template>

<style scoped>
.shellman-mobile-only {
  display: none;
}

@media (max-width: 980px) {
  .shellman-desktop {
    display: none;
  }

  .shellman-mobile-only {
    display: block;
  }
}
</style>
