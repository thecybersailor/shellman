<script setup lang="ts">
import { ref, watch } from "vue";
import { useI18n } from "vue-i18n";
import { Input } from "@/components/ui/input";
import { Button } from "@/components/ui/button";
import type { DirectoryHistoryItem, DirectoryItem, DirectoryListResult } from "@/stores/shellman";
const { t } = useI18n();

const props = defineProps<{
  show: boolean;
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
const searchQ = ref("");
const items = ref<DirectoryItem[]>([]);
const searchItems = ref<DirectoryItem[]>([]);
const history = ref<DirectoryHistoryItem[]>([]);
const loading = ref(false);
const error = ref("");

async function loadDirectory(path: string) {
  const target = path.trim() || "/";
  loading.value = true;
  error.value = "";
  try {
    const out = await props.listDirectories(target);
    currentPath.value = out.path;
    pathInput.value = out.path;
    items.value = out.items ?? [];
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
  await loadDirectory(currentPath.value || "/");
  await loadHistory();
}

watch(
  () => props.show,
  async (show) => {
    if (show) {
      await loadInitial();
      return;
    }
    searchQ.value = "";
    searchItems.value = [];
    error.value = "";
  },
  { immediate: true }
);

watch(searchQ, async (q) => {
  const query = q.trim();
  if (!query) {
    searchItems.value = [];
    return;
  }
  try {
    searchItems.value = await props.searchDirectories(currentPath.value || pathInput.value || "/", query);
  } catch {
    searchItems.value = [];
  }
});

async function onPathEnter() {
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

async function onOpenSearch(item: DirectoryItem) {
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
</script>

<template>
  <div v-if="show" class="space-y-3">
    <Input
      v-model="pathInput"
      data-test-id="shellman-dir-path-input"
      placeholder="/path/to/project"
      @keydown.enter.prevent="onPathEnter"
    />

    <Input
      v-model="searchQ"
      data-test-id="shellman-dir-search-input"
      :placeholder="t('projectDirectoryPicker.searchPlaceholder')"
    />

    <div class="space-y-2">
      <div data-test-id="shellman-dir-list" class="max-h-40 overflow-auto rounded border border-border p-2">
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

      <div v-if="searchItems.length" class="max-h-28 overflow-auto rounded border border-border p-2">
        <button
          v-for="item in searchItems"
          :key="`search-${item.path}`"
          type="button"
          :data-test-id="`shellman-dir-search-item-${item.path}`"
          class="block w-full rounded px-2 py-1 text-left text-sm hover:bg-muted"
          @click="onOpenSearch(item)"
        >
          {{ item.path }}
        </button>
      </div>

      <div data-test-id="shellman-dir-history" class="max-h-28 overflow-auto rounded border border-border p-2">
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
