<script setup lang="ts">
import { computed } from "vue";

type TaskFlag = "success" | "notify" | "error";

const props = withDefaults(defineProps<{
  flag?: TaskFlag;
  flagDesc?: string;
  testId?: string;
  preserveSpace?: boolean;
}>(), {
  flag: undefined,
  flagDesc: "",
  testId: "",
  preserveSpace: true
});

const dotClass = computed(() => {
  switch (props.flag) {
    case "success":
      return "bg-emerald-500/90";
    case "notify":
      return "bg-amber-500/90";
    case "error":
      return "bg-rose-500/90";
    default:
      return "bg-muted-foreground/25";
  }
});
</script>

<template>
  <span class="inline-flex h-2 w-2 shrink-0 items-center justify-center">
    <span
      v-if="props.flag"
      :data-test-id="props.testId || undefined"
      :data-flag="props.flag || ''"
      :title="props.flagDesc || ''"
      class="h-2 w-2 rounded-full"
      :class="dotClass"
    />
    <span
      v-else-if="props.preserveSpace"
      class="h-2 w-2 rounded-full opacity-0"
      aria-hidden="true"
    />
  </span>
</template>
