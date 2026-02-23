<script setup lang="ts">
import { computed } from "vue";
import { useI18n } from "vue-i18n";
import { InputGroupButton } from "@/components/ui/input-group";
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
import { SquareIcon } from "lucide-vue-next";
import type { TaskMessage } from "@/stores/shellman";

const { t } = useI18n();

const props = withDefaults(
  defineProps<{
    taskId?: string;
    taskMessages?: TaskMessage[];
    modelValue?: string;
  }>(),
  {
    taskId: "",
    taskMessages: () => [],
    modelValue: ""
  }
);

const emit = defineEmits<{
  (event: "update:modelValue", value: string): void;
  (event: "submit-message", payload: { content: string }): void;
  (event: "stop-running-assistant-message"): void;
}>();

const scopeKey = computed(() => `task:${String(props.taskId ?? "").trim()}`);
const hasRunningAssistantMessage = computed(() =>
  (props.taskMessages ?? []).some((message) => message.role === "assistant" && message.status === "running")
);
const shouldShowStopSubmit = computed(() => hasRunningAssistantMessage.value && String(props.modelValue ?? "").trim() === "");

function onPromptSubmit(payload: { text?: string }) {
  const content = String(payload?.text ?? "").trim();
  if (!content) {
    return;
  }
  emit("submit-message", { content });
  emit("update:modelValue", "");
}

function onPromptInput(event: Event) {
  emit("update:modelValue", String((event.target as HTMLTextAreaElement | null)?.value ?? ""));
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
  <div data-test-id="shellman-conversation-session" class="min-h-0 h-full flex flex-col">
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

    <PromptInput :key="`prompt-${scopeKey}`" class="mt-2" :initial-input="props.modelValue" @submit="onPromptSubmit">
      <PromptInputBody>
        <PromptInputTextarea data-test-id="shellman-shellman-input" :placeholder="t('thread.talkPlaceholder')" @input="onPromptInput" />
      </PromptInputBody>
      <PromptInputFooter>
        <PromptInputTools />
        <InputGroupButton
          v-if="shouldShowStopSubmit"
          type="button"
          data-test-id="shellman-shellman-stop"
          :aria-label="t('thread.stop')"
          size="icon-sm"
          @click="emit('stop-running-assistant-message')"
        >
          <SquareIcon class="size-4" />
        </InputGroupButton>
        <PromptInputSubmit v-else data-test-id="shellman-shellman-send" />
      </PromptInputFooter>
    </PromptInput>
  </div>
</template>
