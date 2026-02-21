<script setup lang="ts">
import SCMPanel from "./SCMPanel.vue";
import FilePanel from "./FilePanel.vue";
import SessionPanel from "./SessionPanel.vue";
import { Tabs, TabsContent, TabsList, TabsTrigger } from "@/components/ui/tabs";
import shellmanIcon from "@/asserts/icon.svg";

type ProjectPanelTab = "diff" | "file" | "session";

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
    taskMessages?: Array<{ id: number; task_id: string; role: "user" | "assistant"; content: string; status: "running" | "completed" | "error"; error_text?: string }>;
    autopilot?: boolean;
    paneUuid?: string;
    currentCommand?: string;
    aiLoading?: boolean;
    submitLoading?: boolean;
  }>(),
  {
    taskId: "",
    projectId: "",
    repoRoot: "",
    activeTab: "session",
    taskTitle: "",
    taskDescription: "",
    taskMessages: () => [],
    autopilot: false,
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
  (e: "set-autopilot", payload: { enabled: boolean }): void;
  (e: "ai", payload: SCMAIPayload): void;
  (e: "submit", payload: SCMSubmitPayload): void;
  (e: "file-open", path: string): void;
}>();

function onTabChange(next: string) {
  if (next === "diff" || next === "file" || next === "session") {
    emit("update:active-tab", next);
  }
}
</script>

<template>
  <div class="h-full p-2">
    <Tabs :model-value="props.activeTab" class="h-full flex flex-col" @update:model-value="onTabChange">
      <TabsList class="w-full grid grid-cols-3">
        <TabsTrigger value="session" class="text-xs">SideCar</TabsTrigger>
        <TabsTrigger value="diff" class="text-xs">Diff</TabsTrigger>
        <TabsTrigger value="file" class="text-xs">File</TabsTrigger>
      </TabsList>
      <TabsContent value="session" class="flex-1 min-h-0 mt-2">
        <div
          :key="`task:${props.taskId || ''}`"
          data-test-id="shellman-project-tab-session-body"
          :data-scope-key="`task:${props.taskId || ''}`"
          class="h-full min-h-0"
        >
          <SessionPanel
            :task-id="props.taskId"
            :task-title="props.taskTitle"
            :task-description="props.taskDescription"
            :task-messages="props.taskMessages"
            :autopilot="props.autopilot"
            :pane-uuid="props.paneUuid"
            :current-command="props.currentCommand"
            @save-task-meta="(payload) => emit('save-task-meta', payload)"
            @send-message="(payload) => emit('send-message', { content: payload.content })"
            @set-autopilot="(payload) => emit('set-autopilot', payload)"
          />
        </div>
      </TabsContent>
      <TabsContent value="diff" class="flex-1 min-h-0 mt-2">
        <div
          data-test-id="shellman-project-tab-diff-body"
          :data-scope-key="`project:${props.projectId || ''}`"
          class="h-full min-h-0"
        >
          <SCMPanel
            :task-id="props.taskId"
            :ai-loading="props.aiLoading"
            :submit-loading="props.submitLoading"
            @ai="(payload) => emit('ai', payload)"
            @submit="(payload) => emit('submit', payload)"
          />
        </div>
      </TabsContent>
      <TabsContent value="file" class="flex-1 min-h-0 mt-2">
        <div
          data-test-id="shellman-project-tab-file-body"
          :data-scope-key="`project:${props.projectId || ''}`"
          class="h-full min-h-0"
        >
          <FilePanel
            :task-id="props.taskId"
            :repo-root="props.repoRoot"
            @file-open="(path) => emit('file-open', path)"
          />
        </div>
      </TabsContent>
    </Tabs>
  </div>
</template>
