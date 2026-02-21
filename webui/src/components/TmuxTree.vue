<template>
  <ul data-test-id="tt-pane-list" class="space-y-2">
    <li
      v-for="item in items"
      :key="item.target"
      class="list-none"
    >
      <button
        :data-test-id="`tt-pane-item-${toStableTestId(item.target)}`"
        :aria-selected="selectedPane === item.target ? 'true' : 'false'"
        class="w-full rounded-lg border px-3 py-2 text-left text-sm font-medium transition hover:bg-accent hover:text-accent-foreground focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring"
        :class="
          selectedPane === item.target
            ? 'border-primary bg-primary text-primary-foreground shadow-sm'
            : 'border-border bg-card text-card-foreground'
        "
        @click="$emit('select-pane', item.target)"
      >
        <div class="flex items-center justify-between gap-2">
          <span class="truncate font-semibold">{{ item.title }}</span>
          <span
            :data-test-id="`tt-pane-status-${toStableTestId(item.target)}`"
            :data-status="item.status"
            class="rounded px-1.5 py-0.5 text-xs uppercase tracking-wide"
          >
            {{ item.status }}
          </span>
        </div>
        <div class="mt-1 truncate text-xs opacity-70">{{ item.target }}</div>
      </button>
    </li>
  </ul>
</template>

<script setup lang="ts">
import { toStableTestId } from "../utils/test_id";

defineProps<{
  items: Array<{ target: string; title: string; status: "running" | "ready" | "unknown"; updatedAt: number }>;
  selectedPane: string;
}>();
defineEmits<{ (e: "select-pane", pane: string): void }>();
</script>
