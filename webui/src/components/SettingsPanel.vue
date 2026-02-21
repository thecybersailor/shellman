<script setup lang="ts">
import { computed, ref, watch } from "vue";
import { useI18n } from "vue-i18n";
import ResponsiveOverlay from "./ResponsiveOverlay.vue";
import { Button } from "@/components/ui/button";
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@/components/ui/select";
import { Checkbox } from "@/components/ui/checkbox";
import { Input } from "@/components/ui/input";
import { InputGroup, InputGroupAddon, InputGroupTextarea } from "@/components/ui/input-group";
const { t } = useI18n();

type LaunchProgram = "shell" | "codex" | "claude" | "cursor";
type HelperProgram = "codex" | "claude" | "cursor";
type TaskCompletionMode = "none" | "command" | "webhook";
type DelayPreset = "0" | "60" | "300";

const props = withDefaults(defineProps<{
  show: boolean;
  defaultLaunchProgram?: LaunchProgram;
  defaultHelperProgram?: HelperProgram;
  providers?: Array<{ id: HelperProgram; display_name: string; command: string }>;
  taskCompletionMode?: TaskCompletionMode;
  taskCompletionCommand?: string;
  taskCompletionWebhook?: string;
  taskCompletionIdleDuration?: number;
  helperOpenAIEndpoint?: string;
  helperOpenAIModel?: string;
  helperOpenAIApiKey?: string;
  saving?: boolean;
}>(), {
  defaultLaunchProgram: "shell",
  defaultHelperProgram: "codex",
  providers: () => [],
  taskCompletionMode: "none",
  taskCompletionCommand: "",
  taskCompletionWebhook: "",
  taskCompletionIdleDuration: 0,
  helperOpenAIEndpoint: "",
  helperOpenAIModel: "",
  helperOpenAIApiKey: "",
  saving: false
});

const emit = defineEmits<{
  (event: "update:show", value: boolean): void;
  (event: "save", payload: {
    defaultLaunchProgram: LaunchProgram;
    defaultHelperProgram: HelperProgram;
    taskCompletionMode: TaskCompletionMode;
    taskCompletionCommand: string;
    taskCompletionWebhook: string;
    taskCompletionIdleDuration: number;
    helperOpenAIEndpoint: string;
    helperOpenAIModel: string;
    helperOpenAIApiKey: string;
  }): void;
}>();

const localDefaultProgram = ref<LaunchProgram>(props.defaultLaunchProgram);
const localDefaultHelperProgram = ref<HelperProgram>(props.defaultHelperProgram);

const enableTaskCompletionCommand = ref(props.taskCompletionMode === "command");
const localTaskCompletionCommand = ref(props.taskCompletionCommand);
const localTaskCompletionIdleDuration = ref(Math.max(0, props.taskCompletionIdleDuration ?? 0));
const selectedDelayPreset = ref<DelayPreset>(resolveDelayPreset(props.taskCompletionIdleDuration));
const localHelperOpenAIEndpoint = ref(props.helperOpenAIEndpoint);
const localHelperOpenAIModel = ref(props.helperOpenAIModel);
const localHelperOpenAIApiKey = ref(props.helperOpenAIApiKey);

const actionInputPlaceholder = computed(() => t("settings.actionInputPlaceholder"));
const sessionProgramOptions = computed(() => {
  const options: Array<{ id: LaunchProgram; label: string }> = [{ id: "shell", label: "shell" }];
  const seen = new Set<string>(["shell"]);
  for (const item of props.providers) {
    if (seen.has(item.id)) {
      continue;
    }
    seen.add(item.id);
    options.push({ id: item.id, label: String(item.display_name ?? "").trim() || item.id });
  }
  return options;
});
const helperProgramOptions = computed(() => {
  const options: Array<{ id: HelperProgram; label: string }> = [];
  const seen = new Set<string>();
  for (const item of props.providers) {
    if (seen.has(item.id)) {
      continue;
    }
    seen.add(item.id);
    options.push({ id: item.id, label: String(item.display_name ?? "").trim() || item.id });
  }
  return options;
});

watch(
  () => props.defaultLaunchProgram,
  (value) => {
    localDefaultProgram.value = value;
  }
);
watch(
  () => props.defaultHelperProgram,
  (value) => {
    localDefaultHelperProgram.value = value;
  }
);
watch(
  () => props.taskCompletionMode,
  (value) => {
    enableTaskCompletionCommand.value = value === "command";
  }
);
watch(
  () => props.taskCompletionCommand,
  (value) => {
    localTaskCompletionCommand.value = value;
  }
);
watch(
  () => props.taskCompletionIdleDuration,
  (value) => {
    const duration = Math.max(0, value ?? 0);
    localTaskCompletionIdleDuration.value = duration;
    selectedDelayPreset.value = resolveDelayPreset(duration);
  }
);
watch(
  () => props.helperOpenAIEndpoint,
  (value) => {
    localHelperOpenAIEndpoint.value = value;
  }
);
watch(
  () => props.helperOpenAIModel,
  (value) => {
    localHelperOpenAIModel.value = value;
  }
);
watch(
  () => props.helperOpenAIApiKey,
  (value) => {
    localHelperOpenAIApiKey.value = value;
  }
);
watch(enableTaskCompletionCommand, (value) => {
  if (!value) {
    localTaskCompletionCommand.value = "";
  }
});
watch(selectedDelayPreset, (value) => {
  localTaskCompletionIdleDuration.value = Number(value);
});

function resolveDelayPreset(value?: number): DelayPreset {
  if (value === 60) {
    return "60";
  }
  if (value === 300) {
    return "300";
  }
  return "0";
}

function setDelayPreset(value: DelayPreset) {
  selectedDelayPreset.value = value;
}

function setTaskCompletionEnabled(value: boolean | "indeterminate") {
  enableTaskCompletionCommand.value = value === true;
}

function closePanel() {
  emit("update:show", false);
}

function saveSettings() {
  const mode: TaskCompletionMode = enableTaskCompletionCommand.value ? "command" : "none";
  const commandValue = mode === "command" ? localTaskCompletionCommand.value : "";
  emit("save", {
    defaultLaunchProgram: localDefaultProgram.value,
    defaultHelperProgram: localDefaultHelperProgram.value,
    taskCompletionMode: mode,
    taskCompletionCommand: commandValue,
    taskCompletionWebhook: "",
    taskCompletionIdleDuration: Number(localTaskCompletionIdleDuration.value),
    helperOpenAIEndpoint: String(localHelperOpenAIEndpoint.value ?? "").trim(),
    helperOpenAIModel: String(localHelperOpenAIModel.value ?? "").trim(),
    helperOpenAIApiKey: String(localHelperOpenAIApiKey.value ?? "").trim()
  });
}
</script>

<template>
  <ResponsiveOverlay
    :open="props.show"
    :title="t('settings.title')"
    :description="t('settings.description')"
    dialog-content-class="z-[120] sm:max-w-[560px]"
    sheet-side="bottom"
    sheet-content-class="z-[120] h-[60vh] flex flex-col p-6"
    @update:open="(v) => emit('update:show', v)"
  >
    <div class="space-y-4">
      <div class="space-y-1.5">
        <label class="text-xs font-medium text-muted-foreground">{{ t("settings.defaultProgramForNewSession") }}</label>
        <Select v-model="localDefaultProgram" class="w-full">
          <SelectTrigger data-test-id="shellman-settings-default-program" class="w-full">
            <SelectValue :placeholder="t('settings.selectDefaultProgram')" />
          </SelectTrigger>
          <SelectContent class="z-[130] w-full">
            <SelectItem
              v-for="item in sessionProgramOptions"
              :key="`session-${item.id}`"
              :value="item.id"
            >
              {{ item.label }}
            </SelectItem>
          </SelectContent>
        </Select>
      </div>
      <div class="space-y-2">
        <label class="text-xs font-medium text-muted-foreground">{{ t("settings.helperOpenaiApi") }}</label>
        <Input
          v-model="localHelperOpenAIEndpoint"
          data-test-id="shellman-settings-helper-openai-endpoint"
          placeholder="https://api.openai.com/v1"
        />
        <Input
          v-model="localHelperOpenAIModel"
          data-test-id="shellman-settings-helper-openai-model"
          placeholder="gpt-5"
        />
        <Input
          v-model="localHelperOpenAIApiKey"
          data-test-id="shellman-settings-helper-openai-apikey"
          type="password"
          placeholder="sk-..."
          autocomplete="off"
        />
      </div>

      <div class="space-y-2">
        <div class="flex item-center gap-4">
        <label class="flex items-center gap-2 text-sm text-foreground">
          <Checkbox
            :model-value="enableTaskCompletionCommand"
            data-test-id="shellman-settings-task-completion-enable"
            @update:model-value="setTaskCompletionEnabled"
          />
          {{ t("settings.runCommandAfterTaskCompleted") }}
        </label>

          <Select v-model="selectedDelayPreset">
            <SelectTrigger
              data-test-id="shellman-settings-task-completion-delay"
            >
              <SelectValue :placeholder="t('settings.delaySelector')" />
            </SelectTrigger>
            <SelectContent class="z-[130]">
              <SelectItem
                value="0"
                data-test-id="shellman-settings-task-completion-delay-0"
                @click="setDelayPreset('0')"
              >
                {{ t("settings.sendImmediately") }}
              </SelectItem>
              <SelectItem
                value="60"
                data-test-id="shellman-settings-task-completion-delay-60"
                @click="setDelayPreset('60')"
              >
                {{ t("settings.idle60s") }}
              </SelectItem>
              <SelectItem
                value="300"
                data-test-id="shellman-settings-task-completion-delay-300"
                @click="setDelayPreset('300')"
              >
                {{ t("settings.idle300s") }}
              </SelectItem>
            </SelectContent>
          </Select>
        </div>

        <div v-if="enableTaskCompletionCommand" class="space-y-1.5">
          <InputGroup class="h-auto min-h-[124px] flex-col items-stretch px-2 py-2">
            <InputGroupTextarea
              v-model="localTaskCompletionCommand"
              data-test-id="shellman-settings-task-completion-action-input"
              :placeholder="actionInputPlaceholder"
              class="min-h-[56px] rounded-none border-0 px-2 py-1.5 text-sm"
          /></InputGroup>
        </div>
      </div>

      <div class="flex items-center justify-end gap-2 pt-2">
        <Button variant="ghost" :disabled="props.saving" @click="closePanel">{{ t("common.cancel") }}</Button>
        <Button data-test-id="shellman-settings-save" :disabled="props.saving" @click="saveSettings">
          {{ props.saving ? t("common.saving") : t("common.save") }}
        </Button>
      </div>
    </div>
  </ResponsiveOverlay>
</template>
