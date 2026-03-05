<script setup lang="ts">
import { computed, watch } from "vue";
import { useI18n } from "vue-i18n";
import SCMPanel from "./SCMPanel.vue";
import FilePanel from "./FilePanel.vue";
import ThreadPanel from "./ThreadPanel.vue";
import { Tabs, TabsContent, TabsList, TabsTrigger } from "@/components/ui/tabs";
import { FileText, GitCompare, MessageSquareText } from "lucide-vue-next";
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
    projectIsGitRepo?: boolean;
    activeTab?: ProjectPanelTab;
    taskTitle?: string;
    taskDescription?: string;
    taskMessages?: TaskMessage[];
    sidecarMode?: "advisor" | "observer" | "autopilot";
    paneUuid?: string;
    currentCommand?: string;
    aiLoading?: boolean;
    submitLoading?: boolean;
    hideThreadFooter?: boolean;
  }>(),
  {
    taskId: "",
    projectId: "",
    repoRoot: "",
    projectIsGitRepo: true,
    activeTab: "thread",
    taskTitle: "",
    taskDescription: "",
    taskMessages: () => [],
    sidecarMode: "advisor",
    paneUuid: "",
    currentCommand: "",
    aiLoading: false,
    submitLoading: false,
    hideThreadFooter: false
  }
);

const emit = defineEmits<{
  (e: "update:active-tab", tab: ProjectPanelTab): void;
  (e: "save-task-meta", payload: { title: string; description: string }): void;
  (e: "send-message", payload: { content: string }): void;
  (e: "set-sidecar-mode", payload: { mode: "advisor" | "observer" | "autopilot" }): void;
  (e: "stop-running-assistant-message"): void;
  (e: "ai", payload: SCMAIPayload): void;
  (e: "submit", payload: SCMSubmitPayload): void;
  (e: "file-open", path: string): void;
}>();

function onTabChange(next: string | number) {
  const value = String(next);
  if (value === "diff" && !canShowDiff.value) {
    emit("update:active-tab", "thread");
    return;
  }
  if (value === "diff" || value === "file" || value === "thread") {
    emit("update:active-tab", value);
  }
}

const canShowDiff = computed(() => props.projectIsGitRepo !== false);
const effectiveTab = computed<ProjectPanelTab>(() => {
  if (!canShowDiff.value && props.activeTab === "diff") {
    return "thread";
  }
  return props.activeTab;
});

watch(
  () => [canShowDiff.value, props.activeTab] as const,
  ([showDiff, activeTab]) => {
    if (!showDiff && activeTab === "diff") {
      emit("update:active-tab", "thread");
    }
  },
  { immediate: true }
);
</script>

<template>
  <div class="h-full p-2">
    <Tabs :model-value="effectiveTab" class="h-full flex flex-col" @update:model-value="onTabChange">
      <TabsList class="w-full grid" :class="canShowDiff ? 'grid-cols-3' : 'grid-cols-2'">
        <TabsTrigger value="thread" class="text-xs">
          <span class="inline-flex items-center gap-1.5">
            <MessageSquareText class="h-3.5 w-3.5" />
            <span>{{ t("projectInfo.thread") }}</span>
          </span>
        </TabsTrigger>
        <TabsTrigger v-if="canShowDiff" value="diff" class="text-xs">
          <span class="inline-flex items-center gap-1.5">
            <GitCompare class="h-3.5 w-3.5" />
            <span>{{ t("projectInfo.diff") }}</span>
          </span>
        </TabsTrigger>
        <TabsTrigger value="file" class="text-xs">
          <span class="inline-flex items-center gap-1.5">
            <FileText class="h-3.5 w-3.5" />
            <span>{{ t("projectInfo.file") }}</span>
          </span>
        </TabsTrigger>
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
            :hide-footer="props.hideThreadFooter"
            @save-task-meta="(payload) => emit('save-task-meta', payload)"
            @send-message="(payload) => emit('send-message', { content: payload.content })"
            @set-sidecar-mode="(payload) => emit('set-sidecar-mode', payload)"
            @stop-running-assistant-message="() => emit('stop-running-assistant-message')"
          />
        </div>
      </TabsContent>
      <TabsContent v-if="canShowDiff" value="diff" force-mount class="flex-1 min-h-0 mt-2">
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
