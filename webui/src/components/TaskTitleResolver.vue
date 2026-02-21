<script setup lang="ts">
import { computed } from "vue";

const props = withDefaults(
  defineProps<{
    taskTitle?: string;
    currentCommand?: string;
    fallbackTitle?: string;
  }>(),
  {
    taskTitle: "",
    currentCommand: "",
    fallbackTitle: "Terminal Task"
  }
);

const resolvedTitle = computed(() => {
  const task = String(props.taskTitle ?? "").trim();
  const command = String(props.currentCommand ?? "").trim();
  return task || command || String(props.fallbackTitle ?? "").trim() || "Terminal Task";
});
</script>

<template>
  <span>{{ resolvedTitle }}</span>
</template>
