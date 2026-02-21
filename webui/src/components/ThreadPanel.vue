<script setup lang="ts">
import { computed, ref, watch } from "vue";
import { useI18n } from "vue-i18n";
import { Input } from "@/components/ui/input";
import { Switch } from "@/components/ui/switch";
import { Textarea } from "@/components/ui/textarea";
import { Conversation, ConversationContent, ConversationEmptyState, ConversationScrollButton } from "@/components/ai-elements/conversation";
import { Loader } from "@/components/ai-elements/loader";
import { Message, MessageContent, MessageResponse } from "@/components/ai-elements/message";
import {
  PromptInput,
  PromptInputBody,
  PromptInputFooter,
  PromptInputSubmit,
  PromptInputTextarea,
  PromptInputTools
} from "@/components/ai-elements/prompt-input";
import { Reasoning, ReasoningContent, ReasoningTrigger } from "@/components/ai-elements/reasoning";
import { Tool, ToolContent, ToolHeader, ToolInput, ToolOutput } from "@/components/ai-elements/tool";
import type { TaskMessage } from "@/stores/shellman";
const { t } = useI18n();

const props = defineProps<{
  taskId?: string;
  taskTitle?: string;
  taskDescription?: string;
  paneUuid?: string;
  currentCommand?: string;
  taskMessages?: TaskMessage[];
  autopilot?: boolean;
}>();

const emit = defineEmits<{
  (event: "save-task-meta", payload: { title: string; description: string }): void;
  (event: "send-message", payload: { content: string }): void;
  (event: "set-autopilot", payload: { enabled: boolean }): void;
}>();

const draftTitle = ref(String(props.taskTitle ?? ""));
const draftDescription = ref(String(props.taskDescription ?? ""));
const promptDraft = ref("");
const autopilotEnabled = ref(Boolean(props.autopilot));
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
  () => props.autopilot,
  (value) => {
    autopilotEnabled.value = Boolean(value);
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

function onPromptSubmit(payload: { text?: string }) {
  const content = String(payload?.text ?? "").trim();
  if (!content) {
    return;
  }
  emit("send-message", { content });
  promptDraft.value = "";
}

function onPromptInput(event: Event) {
  promptDraft.value = String((event.target as HTMLTextAreaElement | null)?.value ?? "");
}

function onAutopilotUpdate(enabled: boolean | "indeterminate") {
  const next = enabled === true;
  autopilotEnabled.value = next;
  emit("set-autopilot", { enabled: next });
}

type ParsedToolCall = {
  type: string;
  state: "input-streaming" | "input-available" | "output-available" | "output-error";
  toolName?: string;
  input?: unknown;
  output?: unknown;
  errorText?: string;
};

type ParsedMessageMeta = {
  displayType?: string;
  source?: string;
  event?: string;
};

type ParsedMessageContent = {
  text: string;
  reasoning?: string;
  toolCalls?: ParsedToolCall[];
  meta?: ParsedMessageMeta;
};

function mapToolState(state?: string): ParsedToolCall["state"] {
  if (state === "output-available" || state === "output-error" || state === "input-available" || state === "input-streaming") {
    return state;
  }
  return "output-available";
}

function parseStructuredContent(content: string): ParsedMessageContent {
  const raw = String(content ?? "").trim();
  if (!raw.startsWith("{") && !raw.startsWith("[")) {
    return { text: String(content ?? "") };
  }
  try {
    const parsed = JSON.parse(raw) as Record<string, unknown>;
    const text = typeof parsed.text === "string" ? parsed.text : String(content ?? "");
    const reasoning = typeof parsed.reasoning === "string" ? parsed.reasoning : undefined;
    let meta: ParsedMessageMeta | undefined;
    if (parsed.meta && typeof parsed.meta === "object") {
      const rawMeta = parsed.meta as Record<string, unknown>;
      meta = {
        displayType: typeof rawMeta.display_type === "string" ? rawMeta.display_type : undefined,
        source: typeof rawMeta.source === "string" ? rawMeta.source : undefined,
        event: typeof rawMeta.event === "string" ? rawMeta.event : undefined
      };
    }
    const toolCalls = Array.isArray(parsed.tools)
      ? parsed.tools
          .filter((item): item is Record<string, unknown> => !!item && typeof item === "object")
          .map((item) => ({
            type: typeof item.type === "string" ? item.type : "tool-call",
            state: mapToolState(typeof item.state === "string" ? item.state : undefined),
            toolName: typeof item.tool_name === "string" ? item.tool_name : undefined,
            input: item.input,
            output: item.output,
            errorText: typeof item.error_text === "string" ? item.error_text : undefined
          }))
      : undefined;
    return { text, reasoning, toolCalls, meta };
  } catch {
    return { text: String(content ?? "") };
  }
}

function messageParsedContent(m: TaskMessage): ParsedMessageContent {
  return parseStructuredContent(m.content);
}

function messageFrom(m: TaskMessage): "user" | "assistant" {
  const displayType = messageParsedContent(m).meta?.displayType;
  if (displayType === "runtime" || displayType === "system") {
    return "assistant";
  }
  return m.role === "user" ? "user" : "assistant";
}

function messageTestId(m: TaskMessage): string {
  const displayType = messageParsedContent(m).meta?.displayType;
  if (displayType === "runtime") {
    return "shellman-shellman-message-runtime";
  }
  if (displayType === "system") {
    return "shellman-shellman-message-system";
  }
  return m.role === "user" ? "shellman-shellman-message-user" : "shellman-shellman-message-assistant";
}

function messageDisplayTypeLabel(m: TaskMessage): string {
  const displayType = messageParsedContent(m).meta?.displayType;
  if (displayType === "runtime") {
    return "runtime";
  }
  if (displayType === "system") {
    return "system";
  }
  return "";
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
        <Conversation class="min-h-0 h-full">
          <ConversationContent>
            <ConversationEmptyState v-if="(props.taskMessages ?? []).length === 0" class="text-[11px]" />
            <Message
              v-for="m in props.taskMessages ?? []"
              :key="m.id"
              :from="messageFrom(m)"
              :data-test-id="messageTestId(m)"
            >
              <MessageContent>
                <div
                  v-if="messageDisplayTypeLabel(m)"
                  class="mb-1 text-[10px] font-mono uppercase tracking-wide text-muted-foreground"
                >
                  {{ messageDisplayTypeLabel(m) }}
                </div>
                <template v-if="m.role === 'assistant' && messageParsedContent(m).reasoning">
                  <Reasoning :is-streaming="m.status === 'running'" :default-open="true">
                    <ReasoningTrigger />
                    <ReasoningContent :content="messageParsedContent(m).reasoning || ''" />
                  </Reasoning>
                </template>
                <template v-for="(tool, idx) in messageParsedContent(m).toolCalls || []" :key="`${m.id}-tool-${idx}`">
                  <Tool data-test-id="shellman-shellman-tool" :default-open="tool.state === 'input-streaming'">
                    <ToolHeader
                      data-test-id="shellman-shellman-tool-header"
                      :type="tool.type as any"
                      :state="tool.state as any"
                      :tool-name="tool.toolName || 'tool'"
                    />
                    <ToolContent data-test-id="shellman-shellman-tool-content">
                      <ToolInput data-test-id="shellman-shellman-tool-input" :input="tool.input as any" />
                      <ToolOutput data-test-id="shellman-shellman-tool-output" :output="tool.output as any" :error-text="tool.errorText as any" />
                    </ToolContent>
                  </Tool>
                </template>
                <Loader v-if="m.role === 'assistant' && m.status === 'running' && !messageParsedContent(m).text" data-test-id="shellman-shellman-responding" />
                <MessageResponse
                  v-if="messageParsedContent(m).text || m.status !== 'running'"
                  :content="messageParsedContent(m).text || m.error_text || ''"
                  class="whitespace-pre-wrap break-words text-sm"
                />
              </MessageContent>
            </Message>
          </ConversationContent>
          <ConversationScrollButton />
        </Conversation>
        <PromptInput :key="`prompt-${scopeKey}`" class="mt-2" :initial-input="promptDraft" @submit="onPromptSubmit">
          <PromptInputBody>
            <PromptInputTextarea data-test-id="shellman-shellman-input" :placeholder="t('thread.talkPlaceholder')" @input="onPromptInput" />
          </PromptInputBody>
          <PromptInputFooter>
            <PromptInputTools />
            <PromptInputSubmit data-test-id="shellman-shellman-send" />
          </PromptInputFooter>
        </PromptInput>
      </div>
    </div>

    <div class="px-2 text-[11px] text-muted-foreground/80 font-mono mt-0.5 flex items-center justify-between gap-2">
      <div class="flex items-center gap-2">
        <Switch
          :model-value="autopilotEnabled"
          data-test-id="shellman-shellman-autopilot-switch"
          @update:model-value="onAutopilotUpdate"
        />
        <label class="text-[11px] text-muted-foreground/80">{{ t("thread.autopilot") }}</label>
      </div>
      <div>Pane: {{ props.paneUuid || "-" }}</div>
    </div>
  </div>
</template>
