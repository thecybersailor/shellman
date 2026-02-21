<script setup lang="ts">
import type { VirtualKey } from "@/lib/terminal_keys";

defineProps<{
  ctrlArmed: boolean;
  altArmed: boolean;
}>();

const emit = defineEmits<{
  (event: "press-key", key: VirtualKey): void;
}>();

const specialKeys: Array<{ key: VirtualKey; label: string }> = [
  { key: "esc", label: "Esc" },
  { key: "tab", label: "Tab" },
  { key: "enter", label: "Enter" },
  { key: "backspace", label: "Backspace" },
  { key: "up", label: "Up" },
  { key: "down", label: "Down" },
  { key: "left", label: "Left" },
  { key: "right", label: "Right" }
];

function onPress(key: VirtualKey) {
  emit("press-key", key);
}
</script>

<template>
  <section
    data-test-id="tt-virtual-keyboard"
    class="fixed bottom-4 right-4 z-30 max-w-[calc(100vw-2rem)] rounded-xl border border-border bg-card/95 p-3 shadow-lg backdrop-blur"
  >
    <div class="mb-2 flex flex-wrap gap-2">
      <button
        data-test-id="tt-vkey-ctrl"
        type="button"
        class="rounded-md border px-3 py-1.5 text-xs font-semibold transition"
        :class="
          ctrlArmed
            ? 'border-emerald-600 bg-emerald-600 text-white'
            : 'border-border bg-background text-foreground hover:bg-accent'
        "
        :aria-pressed="ctrlArmed ? 'true' : 'false'"
        @click="onPress('ctrl')"
      >
        Ctrl
      </button>
      <button
        data-test-id="tt-vkey-alt"
        type="button"
        class="rounded-md border px-3 py-1.5 text-xs font-semibold transition"
        :class="
          altArmed
            ? 'border-sky-600 bg-sky-600 text-white'
            : 'border-border bg-background text-foreground hover:bg-accent'
        "
        :aria-pressed="altArmed ? 'true' : 'false'"
        @click="onPress('alt')"
      >
        Alt
      </button>
      <span
        data-test-id="tt-vkey-status"
        class="inline-flex items-center rounded-md border border-border bg-background px-2 py-1 text-[11px] text-muted-foreground"
      >
        {{ ctrlArmed ? "Ctrl armed" : altArmed ? "Alt armed" : "Ready" }}
      </span>
    </div>

    <div class="grid grid-cols-4 gap-2">
      <button
        v-for="item in specialKeys"
        :key="item.key"
        :data-test-id="`tt-vkey-${item.key}`"
        type="button"
        class="rounded-md border border-border bg-background px-2 py-1.5 text-xs font-medium text-foreground transition hover:bg-accent"
        @click="onPress(item.key)"
      >
        {{ item.label }}
      </button>
    </div>
  </section>
</template>
