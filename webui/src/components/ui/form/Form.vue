<script setup lang="ts">
import type { HTMLAttributes } from "vue";
import { cn } from "@/lib/utils";

const props = defineProps<{
  class?: HTMLAttributes["class"];
}>();

const emit = defineEmits<{
  (e: "submit", event: Event): void;
}>();

function onSubmit(event: Event) {
  emit("submit", event);
}
</script>

<template>
  <form data-slot="form" :class="cn('space-y-2', props.class)" @submit="onSubmit">
    <slot />
  </form>
</template>
