<script setup lang="ts">
import { computed } from "vue";
import { useI18n } from "vue-i18n";

const { t } = useI18n();

const props = withDefaults(
  defineProps<{
    taskTitle?: string;
    currentCommand?: string;
    fallbackTitle?: string;
  }>(),
  {
    taskTitle: "",
    currentCommand: "",
    fallbackTitle: ""
  }
);

const resolvedTitle = computed(() => {
  const task = String(props.taskTitle ?? "").trim();
  const command = String(props.currentCommand ?? "").trim();
  return task || command || String(props.fallbackTitle ?? "").trim() || t("task.terminalTask");
});
</script>

<template>
  <span>{{ resolvedTitle }}</span>
</template>
