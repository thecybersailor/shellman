<script setup lang="ts">
import { computed, ref, watch } from "vue";
import { useI18n } from "vue-i18n";
import { Input } from "@/components/ui/input";
import { Textarea } from "@/components/ui/textarea";
import SidecarModeSelect from "@/components/SidecarModeSelect.vue";
import ConversationSession from "@/components/ConversationSession.vue";
import type { TaskMessage } from "@/stores/shellman";
const { t } = useI18n();

const props = defineProps<{
  taskId?: string;
  taskTitle?: string;
  taskDescription?: string;
  paneUuid?: string;
  currentCommand?: string;
  taskMessages?: TaskMessage[];
  sidecarMode?: "advisor" | "observer" | "autopilot";
}>();

const emit = defineEmits<{
  (event: "save-task-meta", payload: { title: string; description: string }): void;
  (event: "send-message", payload: { content: string }): void;
  (event: "set-sidecar-mode", payload: { mode: "advisor" | "observer" | "autopilot" }): void;
  (event: "stop-running-assistant-message"): void;
}>();

const draftTitle = ref(String(props.taskTitle ?? ""));
const draftDescription = ref(String(props.taskDescription ?? ""));
const promptDraft = ref("");
const sidecarMode = ref<"advisor" | "observer" | "autopilot">(props.sidecarMode ?? "advisor");
const scopeKey = computed(() => `task:${String(props.taskId ?? "").trim()}`);
const storageKey = computed(() => `shellman.project-panel.thread.${scopeKey.value}`);
const prevTaskTitle = ref(String(props.taskTitle ?? ""));
const prevTaskDescription = ref(String(props.taskDescription ?? ""));

type ThreadDraftSnapshot = {
  titleDraft?: string;
  descriptionDraft?: string;
  promptDraft?: string;
};

function readDraftSnapshot(): ThreadDraftSnapshot | null {
  if (typeof localStorage === "undefined") {
    return null;
  }
  try {
    const raw = localStorage.getItem(storageKey.value);
    if (!raw) {
      return null;
    }
    const parsed = JSON.parse(raw) as Record<string, unknown>;
    return {
      titleDraft: typeof parsed.titleDraft === "string" ? parsed.titleDraft : undefined,
      descriptionDraft: typeof parsed.descriptionDraft === "string" ? parsed.descriptionDraft : undefined,
      promptDraft: typeof parsed.promptDraft === "string" ? parsed.promptDraft : undefined
    };
  } catch {
    return null;
  }
}

function persistDraftSnapshot() {
  if (typeof localStorage === "undefined") {
    return;
  }
  try {
    localStorage.setItem(
      storageKey.value,
      JSON.stringify({
        titleDraft: draftTitle.value,
        descriptionDraft: draftDescription.value,
        promptDraft: promptDraft.value
      })
    );
  } catch {
    // ignore storage quota and serialization errors
  }
}

watch(
  scopeKey,
  () => {
    const snapshot = readDraftSnapshot();
    draftTitle.value = snapshot?.titleDraft ?? String(props.taskTitle ?? "");
    draftDescription.value = snapshot?.descriptionDraft ?? String(props.taskDescription ?? "");
    promptDraft.value = snapshot?.promptDraft ?? "";
    prevTaskTitle.value = String(props.taskTitle ?? "");
    prevTaskDescription.value = String(props.taskDescription ?? "");
  },
  { immediate: true }
);

watch(
  () => props.taskTitle,
  (next) => {
    const prev = prevTaskTitle.value;
    const nextText = String(next ?? "");
    if (draftTitle.value === prev) {
      draftTitle.value = nextText;
    }
    prevTaskTitle.value = nextText;
  }
);

watch(
  () => props.taskDescription,
  (next) => {
    const prev = prevTaskDescription.value;
    const nextText = String(next ?? "");
    if (draftDescription.value === prev) {
      draftDescription.value = nextText;
    }
    prevTaskDescription.value = nextText;
  }
);

watch(
  () => props.sidecarMode,
  (value) => {
    sidecarMode.value = (value ?? "advisor") as "advisor" | "observer" | "autopilot";
  }
);

let saveTimer: ReturnType<typeof setTimeout> | null = null;
const DEBOUNCE_MS = 400;

function scheduleSave() {
  if (saveTimer) clearTimeout(saveTimer);
  saveTimer = setTimeout(() => {
    saveTimer = null;
    if (!props.taskId || !draftTitle.value.trim()) return;
    const title = draftTitle.value.trim();
    const desc = draftDescription.value.trim();
    if (title === (props.taskTitle ?? "").trim() && desc === (props.taskDescription ?? "").trim()) return;
    emit("save-task-meta", { title, description: desc });
  }, DEBOUNCE_MS);
}

watch([draftTitle, draftDescription], scheduleSave, { deep: true });
watch([draftTitle, draftDescription, promptDraft], persistDraftSnapshot, { deep: true });

function onSessionSubmit(payload: { content?: string }) {
  const content = String(payload?.content ?? "").trim();
  if (!content) {
    return;
  }
  emit("send-message", { content });
}

function onSidecarModeUpdate(value: string | number) {
  const next = String(value ?? "advisor");
  if (next !== "advisor" && next !== "observer" && next !== "autopilot") {
    return;
  }
  sidecarMode.value = next;
  emit("set-sidecar-mode", { mode: next });
}

</script>

<template>
  <div data-test-id="shellman-task-meta-form" class="h-full min-h-0 flex flex-col gap-1">
    <div>
      <Input
        v-model="draftTitle"
        data-test-id="shellman-task-title-input"
        :placeholder="t('thread.taskTitlePlaceholder')"
        class="border-0 bg-transparent dark:bg-transparent focus-visible:border focus-visible:border-ring px-2 py-0.5 h-7"
      />
    </div>

    <div class="flex-1 min-h-0 flex flex-col">
      <Textarea
        v-model="draftDescription"
        data-test-id="shellman-task-description-input"
        :placeholder="t('thread.taskDescriptionPlaceholder')"
        class="border-0 bg-transparent dark:bg-transparent focus-visible:border focus-visible:border-ring px-2 py-1 min-h-12"
      />
      <div class="mt-1 flex-1 min-h-0 overflow-hidden border border-border/60 rounded-md p-1.5 bg-background/40 flex flex-col">
        <ConversationSession
          :task-id="props.taskId"
          :task-messages="props.taskMessages"
          :model-value="promptDraft"
          @update:model-value="(value) => (promptDraft = value)"
          @submit-message="onSessionSubmit"
          @stop-running-assistant-message="emit('stop-running-assistant-message')"
        />
      </div>
    </div>

    <div class="px-2 text-[11px] text-muted-foreground/80 font-mono mt-0.5 flex items-center justify-between gap-2">
      <div class="flex items-center gap-2 flex-wrap">
        <SidecarModeSelect
          :model-value="sidecarMode"
          content-side="top"
          content-align="start"
          :content-side-offset="6"
          @update:model-value="(next) => onSidecarModeUpdate(next)"
        />
      </div>
      <div class="text-xs">#{{ props.paneUuid ? props.paneUuid.slice(0, 7) : "-" }}</div>
    </div>
  </div>
</template>
