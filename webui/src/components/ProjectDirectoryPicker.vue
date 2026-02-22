<script setup lang="ts">
import { computed, ref, watch } from "vue";
import { useI18n } from "vue-i18n";
import { Input } from "@/components/ui/input";
import { Button } from "@/components/ui/button";
import type { DirectoryHistoryItem, DirectoryItem, DirectoryListResult } from "@/stores/shellman";
const { t } = useI18n();

const props = defineProps<{
  show: boolean;
  getFSRoots?: () => Promise<string[]>;
  listDirectories: (path: string) => Promise<DirectoryListResult>;
  resolveDirectory: (path: string) => Promise<string>;
  searchDirectories: (base: string, q: string) => Promise<DirectoryItem[]>;
  getDirectoryHistory: (limit?: number) => Promise<DirectoryHistoryItem[]>;
  recordDirectoryHistory: (path: string) => Promise<void>;
}>();

const emit = defineEmits<{
  (e: "update:show", v: boolean): void;
  (e: "select-directory", path: string): void;
}>();

const currentPath = ref("");
const pathInput = ref("");
const items = ref<DirectoryItem[]>([]);
const autocompleteItems = ref<DirectoryItem[]>([]);
const autocompleteIndex = ref(-1);
const autocompleteNavigated = ref(false);
const pathFocused = ref(false);
const history = ref<DirectoryHistoryItem[]>([]);
const loading = ref(false);
const error = ref("");
let autocompleteSeq = 0;
const historyVisible = false;

const showAutocomplete = computed(() => pathFocused.value && autocompleteItems.value.length > 0);

async function loadDirectory(path: string) {
  const target = path.trim() || "/";
  loading.value = true;
  error.value = "";
  try {
    const out = await props.listDirectories(target);
    currentPath.value = out.path;
    pathInput.value = out.path;
    items.value = out.items ?? [];
    autocompleteItems.value = [];
    autocompleteIndex.value = -1;
    autocompleteNavigated.value = false;
  } catch (err) {
    error.value = err instanceof Error ? err.message : "FS_LIST_FAILED";
  } finally {
    loading.value = false;
  }
}

async function loadHistory() {
  try {
    history.value = await props.getDirectoryHistory(20);
  } catch {
    history.value = [];
  }
}

async function loadInitial() {
  if (!props.show) {
    return;
  }
  let initialPath = currentPath.value || "/";
  if (!currentPath.value && props.getFSRoots) {
    try {
      const roots = await props.getFSRoots();
      const first = String(roots?.[0] ?? "").trim();
      if (first) {
        initialPath = first;
      }
    } catch {
      // fall back to "/"
    }
  }
  await loadDirectory(initialPath);
  await loadHistory();
}

watch(
  () => props.show,
  async (show) => {
    if (show) {
      await loadInitial();
      return;
    }
    autocompleteItems.value = [];
    autocompleteIndex.value = -1;
    autocompleteNavigated.value = false;
    pathFocused.value = false;
    error.value = "";
  },
  { immediate: true }
);

function splitAutocompleteInput(raw: string): { base: string; query: string } {
  const value = raw.trim();
  const normalized = value.replace(/\\/g, "/");
  const idx = normalized.lastIndexOf("/");
  if (idx >= 0) {
    const basePart = normalized.slice(0, idx + 1);
    const query = normalized.slice(idx + 1);
    const base = basePart === "" ? "/" : basePart;
    return { base, query };
  }
  return { base: currentPath.value || "/", query: value };
}

watch(pathInput, async (next) => {
  const { base, query } = splitAutocompleteInput(next);
  if (!query) {
    autocompleteItems.value = [];
    autocompleteIndex.value = -1;
    autocompleteNavigated.value = false;
    return;
  }
  const seq = ++autocompleteSeq;
  try {
    const out = await props.searchDirectories(base, query);
    if (seq !== autocompleteSeq) {
      return;
    }
    autocompleteItems.value = out;
    autocompleteIndex.value = out.length > 0 ? 0 : -1;
    autocompleteNavigated.value = false;
  } catch {
    if (seq !== autocompleteSeq) {
      return;
    }
    autocompleteItems.value = [];
    autocompleteIndex.value = -1;
    autocompleteNavigated.value = false;
  }
});

async function onPathEnter() {
  if (showAutocomplete.value && autocompleteItems.value.length > 0 && autocompleteNavigated.value) {
    const idx = autocompleteIndex.value >= 0 ? autocompleteIndex.value : 0;
    await onSelectAutocomplete(autocompleteItems.value[idx]);
    return;
  }
  try {
    const resolved = await props.resolveDirectory(pathInput.value);
    await loadDirectory(resolved);
  } catch (err) {
    error.value = err instanceof Error ? err.message : "FS_RESOLVE_FAILED";
  }
}

async function onOpenItem(item: DirectoryItem) {
  await loadDirectory(item.path);
}

async function onOpenHistory(path: string) {
  await loadDirectory(path);
}

async function onSelectAutocomplete(item: DirectoryItem) {
  pathInput.value = item.path;
  await loadDirectory(item.path);
}

async function onSelectCurrent() {
  if (!currentPath.value) {
    return;
  }
  try {
    await props.recordDirectoryHistory(currentPath.value);
  } catch {
    // ignore history write failures for selection flow
  }
  emit("select-directory", currentPath.value);
  emit("update:show", false);
}

async function onPathTab(event: KeyboardEvent) {
  event.preventDefault();
  if (!autocompleteItems.value.length) {
    await onPathEnter();
    return;
  }
  const idx = autocompleteIndex.value >= 0 ? autocompleteIndex.value : 0;
  await onSelectAutocomplete(autocompleteItems.value[idx]);
}

function onPathArrowDown() {
  if (!autocompleteItems.value.length) {
    return;
  }
  autocompleteNavigated.value = true;
  autocompleteIndex.value = (autocompleteIndex.value + 1 + autocompleteItems.value.length) % autocompleteItems.value.length;
}

function onPathArrowUp() {
  if (!autocompleteItems.value.length) {
    return;
  }
  autocompleteNavigated.value = true;
  autocompleteIndex.value = (autocompleteIndex.value - 1 + autocompleteItems.value.length) % autocompleteItems.value.length;
}

function onPathEscape() {
  autocompleteItems.value = [];
  autocompleteIndex.value = -1;
  autocompleteNavigated.value = false;
}
</script>

<template>
  <div v-if="show" class="space-y-3">
    <div class="relative">
      <Input
        v-model="pathInput"
        data-test-id="shellman-dir-path-input"
        placeholder="/path/to/project"
        @focus="pathFocused = true"
        @blur="pathFocused = false"
        @keydown.enter.prevent="onPathEnter"
        @keydown.tab="onPathTab"
        @keydown.down.prevent="onPathArrowDown"
        @keydown.up.prevent="onPathArrowUp"
        @keydown.esc.prevent="onPathEscape"
      />

      <div
        v-if="showAutocomplete"
        data-test-id="shellman-dir-autocomplete"
        class="absolute left-0 right-0 top-full z-30 mt-1 max-h-44 overflow-auto rounded border border-border bg-popover p-2 shadow-lg"
      >
        <button
          v-for="(item, idx) in autocompleteItems"
          :key="`autocomplete-${item.path}`"
          type="button"
          :data-test-id="`shellman-dir-autocomplete-item-${item.path}`"
          class="block w-full rounded px-2 py-1 text-left text-sm hover:bg-muted"
          :class="idx === autocompleteIndex ? 'bg-muted' : ''"
          @mousedown.prevent
          @click="onSelectAutocomplete(item)"
        >
          {{ item.path }}
        </button>
      </div>
    </div>

    <div class="space-y-2">
      <div data-test-id="shellman-dir-list" class="h-56 overflow-auto rounded border border-border p-2">
        <button
          v-for="item in items"
          :key="item.path"
          type="button"
          :data-test-id="`shellman-dir-item-${item.path}`"
          class="block w-full rounded px-2 py-1 text-left text-sm hover:bg-muted"
          @dblclick="onOpenItem(item)"
        >
          {{ item.name }}
        </button>
      </div>

      <div
        v-if="historyVisible"
        data-test-id="shellman-dir-history"
        class="max-h-28 overflow-auto rounded border border-border p-2"
      >
        <button
          v-for="item in history"
          :key="`history-${item.path}`"
          type="button"
          :data-test-id="`shellman-dir-history-item-${item.path}`"
          class="block w-full rounded px-2 py-1 text-left text-sm hover:bg-muted"
          @click="onOpenHistory(item.path)"
        >
          {{ item.path }}
        </button>
      </div>
    </div>

    <div class="flex items-center justify-between gap-3">
      <span class="truncate text-xs text-muted-foreground">{{ currentPath }}</span>
      <Button
        type="button"
        data-test-id="shellman-dir-select-current"
        :disabled="!currentPath || loading"
        @click="onSelectCurrent"
      >
        {{ t("projectDirectoryPicker.selectCurrent") }}
      </Button>
    </div>

    <p v-if="error" class="text-xs text-destructive">{{ error }}</p>
  </div>
</template>
