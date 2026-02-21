<script setup lang="ts">
import { computed, ref, watch } from "vue";
import { Button } from "@/components/ui/button";
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@/components/ui/select";
import { Textarea } from "@/components/ui/textarea";

type LaunchProgram = "shell" | "codex" | "claude" | "cursor";
type ProviderProgram = "codex" | "claude" | "cursor";

const props = withDefaults(defineProps<{
  submitLabel?: string;
  defaultProgram?: LaunchProgram;
  providers?: Array<{ id: ProviderProgram; display_name: string; command: string }>;
}>(), {
  submitLabel: "Reopen",
  defaultProgram: "shell",
  providers: () => []
});

const emit = defineEmits<{
  (event: "submit", payload: { program: LaunchProgram; prompt?: string }): void;
}>();

const program = ref<LaunchProgram>(props.defaultProgram);
const prompt = ref("");

const requiresPrompt = computed(() => program.value !== "shell");
const canSubmit = computed(() => true);
const programOptions = computed(() => {
  const seen = new Set<LaunchProgram>();
  const options: Array<{ id: LaunchProgram; label: string }> = [{ id: "shell", label: "shell" }];
  seen.add("shell");
  for (const item of props.providers) {
    const id = item.id;
    if (id !== "codex" && id !== "claude" && id !== "cursor") {
      continue;
    }
    if (seen.has(id)) {
      continue;
    }
    seen.add(id);
    options.push({ id, label: String(item.display_name ?? "").trim() || id });
  }
  return options;
});

watch(program, (next) => {
  if (next === "shell") {
    prompt.value = "";
  }
});

watch(
  () => props.defaultProgram,
  (next) => {
    program.value = next;
  }
);

function submitLaunch() {
  if (!canSubmit.value) {
    return;
  }
  const nextPrompt = requiresPrompt.value ? prompt.value.trim() : "";
  emit("submit", {
    program: program.value,
    prompt: nextPrompt || undefined
  });
}
</script>

<template>
  <form data-test-id="muxt-pane-launch-form" class="px-2 space-y-2.5" @submit.prevent="submitLaunch">
    <div class="space-y-1 max-w-sm">
      <label class="text-[11px] uppercase tracking-wide text-muted-foreground/70">Program</label>
      <Select v-model="program" class="">
        <SelectTrigger data-test-id="muxt-pane-launch-program" class="w-full">
          <SelectValue placeholder="Select program" />
        </SelectTrigger>
        <SelectContent>
          <SelectItem
            v-for="item in programOptions"
            :key="item.id"
            :value="item.id"
          >
            {{ item.label }}
          </SelectItem>
        </SelectContent>
      </Select>
    </div>

    <div v-if="requiresPrompt" class="space-y-1">
      <label class="text-[11px] uppercase tracking-wide text-muted-foreground/70">Initial Prompt</label>
      <Textarea
        v-model="prompt"
        data-test-id="muxt-pane-launch-prompt"
        class="resize-y"
        rows="3"
        placeholder="Input initial prompt..."
      />
    </div>

    <Button
      data-test-id="muxt-reopen-pane-button"
      type="submit"
      class="min-w-32"
      :disabled="!canSubmit"
    >
      {{ props.submitLabel }}
    </Button>
  </form>
</template>
