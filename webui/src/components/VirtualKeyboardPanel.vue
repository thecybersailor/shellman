<script setup lang="ts">
import type { VirtualKey } from "@/lib/terminal_keys";
import { Paperclip } from "lucide-vue-next";

defineProps<{
  ctrlArmed: boolean;
  altArmed: boolean;
  topOffsetPx?: number;
}>();

const emit = defineEmits<{
  (event: "press-key", key: VirtualKey): void;
  (event: "request-image-pick"): void;
}>();

const specialKeys: Array<{ key: VirtualKey; label: string; ariaLabel: string }> = [
  { key: "esc", label: "Esc", ariaLabel: "Esc" },
  { key: "tab", label: "Tab", ariaLabel: "Tab" },
  { key: "up", label: "↑", ariaLabel: "Up" },
  { key: "down", label: "↓", ariaLabel: "Down" },
  { key: "left", label: "←", ariaLabel: "Left" },
  { key: "right", label: "→", ariaLabel: "Right" }
];
function onPress(key: VirtualKey) {
  emit("press-key", key);
}

function triggerImagePicker() {
  emit("request-image-pick");
}
</script>

<template>
  <section
    data-test-id="tt-virtual-keyboard"
    class="fixed left-1/2 z-30 w-[calc(100vw-40px)] -translate-x-1/2 rounded-xl border border-border bg-card/95 p-2 shadow-lg backdrop-blur"
    :style="{ top: `${topOffsetPx ?? 16}px` }"
  >
    <div data-test-id="tt-virtual-keyboard-main" class="flex items-center gap-2">
      <div
        data-test-id="tt-virtual-keyboard-row"
        class="flex min-w-0 flex-1 items-center gap-2 overflow-x-auto overscroll-x-contain whitespace-nowrap"
        style="-webkit-overflow-scrolling: touch; touch-action: pan-x;"
        @mousedown.prevent
      >
        <button
          data-test-id="tt-vkey-ctrl"
          type="button"
          tabindex="-1"
          class="h-8 shrink-0 rounded-md border px-2 text-sm font-semibold transition"
          :class="
            ctrlArmed
              ? 'border-emerald-600 bg-emerald-600 text-white'
              : 'border-border bg-background text-foreground hover:bg-accent'
          "
          :aria-pressed="ctrlArmed ? 'true' : 'false'"
          aria-label="Ctrl"
          @click="onPress('ctrl')"
        >
          Ctrl
        </button>
        <button
          data-test-id="tt-vkey-alt"
          type="button"
          tabindex="-1"
          class="h-8 shrink-0 rounded-md border px-2 text-sm font-semibold transition"
          :class="
            altArmed
              ? 'border-sky-600 bg-sky-600 text-white'
              : 'border-border bg-background text-foreground hover:bg-accent'
          "
          :aria-pressed="altArmed ? 'true' : 'false'"
          aria-label="Alt"
          @click="onPress('alt')"
        >
          Alt
        </button>
        <button
          v-for="item in specialKeys"
          :key="item.key"
          :data-test-id="`tt-vkey-${item.key}`"
          type="button"
          tabindex="-1"
          class="h-8 shrink-0 rounded-md border border-border bg-background px-2 text-sm font-medium text-foreground transition hover:bg-accent"
          :aria-label="item.ariaLabel"
          @click="onPress(item.key)"
        >
          {{ item.label }}
        </button>
      </div>
      <button
        data-test-id="tt-vkey-upload"
        type="button"
        tabindex="-1"
        class="h-8 w-8 shrink-0 rounded-md border border-border bg-background text-sm font-medium leading-none text-foreground transition hover:bg-accent"
        aria-label="Upload image"
        @mousedown.prevent
        @click="triggerImagePicker"
      >
        <Paperclip class="mx-auto h-4 w-4" />
      </button>
    </div>
  </section>
</template>

<style scoped>
[data-test-id="tt-virtual-keyboard-row"] {
  scrollbar-width: none;
}

[data-test-id="tt-virtual-keyboard-row"]::-webkit-scrollbar {
  display: none;
}
</style>
