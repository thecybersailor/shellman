<script setup lang="ts">
import { useI18n } from "vue-i18n";
import SCMPanel from "./SCMPanel.vue";
import FilePanel from "./FilePanel.vue";
import ThreadPanel from "./ThreadPanel.vue";
import { Tabs, TabsContent, TabsList, TabsTrigger } from "@/components/ui/tabs";
import shellmanIcon from "@/asserts/icon.svg";
import type { TaskMessage } from "@/stores/shellman";
const { t } = useI18n();

type ProjectPanelTab = "diff" | "file" | "thread";

type SCMAIPayload = {
  taskId: string;
  diff: string;
  files: string[];
  selectedFilePath: string;
};

type SCMSubmitPayload = {
  taskId: string;
  message: string;
};

const props = withDefaults(
  defineProps<{
    taskId?: string;
    projectId?: string;
    repoRoot?: string;
    activeTab?: ProjectPanelTab;
    taskTitle?: string;
    taskDescription?: string;
    taskMessages?: TaskMessage[];
    sidecarMode?: "advisor" | "observer" | "autopilot";
    paneUuid?: string;
    currentCommand?: string;
    aiLoading?: boolean;
    submitLoading?: boolean;
  }>(),
  {
    taskId: "",
    projectId: "",
    repoRoot: "",
    activeTab: "thread",
    taskTitle: "",
    taskDescription: "",
    taskMessages: () => [],
    sidecarMode: "advisor",
    paneUuid: "",
    currentCommand: "",
    aiLoading: false,
    submitLoading: false
  }
);

const emit = defineEmits<{
  (e: "update:active-tab", tab: ProjectPanelTab): void;
  (e: "save-task-meta", payload: { title: string; description: string }): void;
  (e: "send-message", payload: { content: string }): void;
  (e: "set-sidecar-mode", payload: { mode: "advisor" | "observer" | "autopilot" }): void;
  (e: "stop-sidecar-chat"): void;
  (e: "restart-sidecar-context", payload: { strategy: "child" | "root" }): void;
  (e: "ai", payload: SCMAIPayload): void;
  (e: "submit", payload: SCMSubmitPayload): void;
  (e: "file-open", path: string): void;
}>();

function onTabChange(next: string | number) {
  const value = String(next);
  if (value === "diff" || value === "file" || value === "thread") {
    emit("update:active-tab", value);
  }
}
</script>

<template>
  <div class="h-full p-2">
    <Tabs :model-value="props.activeTab" class="h-full flex flex-col" @update:model-value="onTabChange">
      <TabsList class="w-full grid grid-cols-3">
        <TabsTrigger value="thread" class="text-xs">{{ t("projectInfo.thread") }}</TabsTrigger>
        <TabsTrigger value="diff" class="text-xs">{{ t("projectInfo.diff") }}</TabsTrigger>
        <TabsTrigger value="file" class="text-xs">{{ t("projectInfo.file") }}</TabsTrigger>
      </TabsList>
      <TabsContent value="thread" force-mount class="flex-1 min-h-0 mt-2">
        <div
          :key="`task:${props.taskId || ''}`"
          data-test-id="shellman-project-tab-thread-body"
          :data-scope-key="`task:${props.taskId || ''}`"
          class="h-full min-h-0"
        >
          <ThreadPanel
            :task-id="props.taskId"
            :task-title="props.taskTitle"
            :task-description="props.taskDescription"
            :task-messages="props.taskMessages"
            :sidecar-mode="props.sidecarMode"
            :pane-uuid="props.paneUuid"
            :current-command="props.currentCommand"
            @save-task-meta="(payload) => emit('save-task-meta', payload)"
            @send-message="(payload) => emit('send-message', { content: payload.content })"
            @set-sidecar-mode="(payload) => emit('set-sidecar-mode', payload)"
            @stop-sidecar-chat="() => emit('stop-sidecar-chat')"
            @restart-sidecar-context="(payload) => emit('restart-sidecar-context', payload)"
          />
        </div>
      </TabsContent>
      <TabsContent value="diff" force-mount class="flex-1 min-h-0 mt-2">
        <div
          :key="`project:${props.projectId || ''}`"
          data-test-id="shellman-project-tab-diff-body"
          :data-scope-key="`project:${props.projectId || ''}`"
          class="h-full min-h-0"
        >
          <SCMPanel
            :task-id="props.taskId"
            :project-id="props.projectId"
            :ai-loading="props.aiLoading"
            :submit-loading="props.submitLoading"
            @ai="(payload) => emit('ai', payload)"
            @submit="(payload) => emit('submit', payload)"
          />
        </div>
      </TabsContent>
      <TabsContent value="file" force-mount class="flex-1 min-h-0 mt-2">
        <div
          :key="`project:${props.projectId || ''}`"
          data-test-id="shellman-project-tab-file-body"
          :data-scope-key="`project:${props.projectId || ''}`"
          class="h-full min-h-0"
        >
          <FilePanel
            :task-id="props.taskId"
            :project-id="props.projectId"
            :repo-root="props.repoRoot"
            @file-open="(path) => emit('file-open', path)"
          />
        </div>
      </TabsContent>
    </Tabs>
  </div>
</template>
