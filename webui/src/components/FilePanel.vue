<script setup lang="ts">
import { computed, ref, watch } from "vue";
import { useI18n } from "vue-i18n";
import { ScrollArea } from "@/components/ui/scroll-area";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { ResizableHandle, ResizablePanel, ResizablePanelGroup } from "@/components/ui/resizable";
import { Copy, FilePenLine, Folder, FolderOpen, File } from "lucide-vue-next";
import { toast } from "vue-sonner";
import { getFilePreviewMode, type FilePreviewMode } from "./file_preview_whitelist";
const { t } = useI18n();

type FileTreeEntry = {
  name: string;
  path: string;
  is_dir: boolean;
  ignored?: boolean;
};

type APIResponse<T> = {
  ok: boolean;
  data?: T;
  error?: { code?: string; message?: string };
};

const props = defineProps<{
  taskId: string;
  projectId?: string;
  repoRoot?: string;
}>();

const emit = defineEmits<{
  (e: "file-open", path: string): void;
}>();

const ROOT = ".";
const treeByPath = ref<Record<string, FileTreeEntry[]>>({});
const expandedDirs = ref<Record<string, true>>({});
const loadingDirs = ref<Record<string, true>>({});
const loading = ref(false);
const searching = ref(false);
const error = ref("");
const searchQuery = ref("");
const searchEntries = ref<FileTreeEntry[]>([]);
const previewLoading = ref(false);
const selectedFilePath = ref("");
const selectedFileContent = ref("");
const storageKey = computed(() => `shellman.project-panel.file.project:${String(props.projectId ?? "").trim()}`);

type FilePanelDraftSnapshot = {
  searchQuery?: string;
  expandedPaths?: string[];
  selectedFilePath?: string;
};

function readDraftSnapshot(): FilePanelDraftSnapshot | null {
  if (typeof localStorage === "undefined") {
    return null;
  }
  try {
    const raw = localStorage.getItem(storageKey.value);
    if (!raw) {
      return null;
    }
    const parsed = JSON.parse(raw) as Record<string, unknown>;
    const expandedRaw = Array.isArray(parsed.expandedPaths) ? parsed.expandedPaths : [];
    const expandedPaths = expandedRaw.filter((item): item is string => typeof item === "string");
    return {
      searchQuery: typeof parsed.searchQuery === "string" ? parsed.searchQuery : undefined,
      expandedPaths,
      selectedFilePath: typeof parsed.selectedFilePath === "string" ? parsed.selectedFilePath : undefined
    };
  } catch {
    return null;
  }
}

function persistDraftSnapshot() {
  if (typeof localStorage === "undefined") {
    return;
  }
  try {
    localStorage.setItem(
      storageKey.value,
      JSON.stringify({
        searchQuery: searchQuery.value,
        expandedPaths: Object.keys(expandedDirs.value),
        selectedFilePath: selectedFilePath.value
      })
    );
  } catch {
    // ignore storage quota and serialization errors
  }
}

type FlatNode = {
  key: string;
  entry: FileTreeEntry;
  depth: number;
};

function isExpanded(path: string) {
  return Boolean(expandedDirs.value[path]);
}

function setExpanded(path: string, value: boolean) {
  if (value) {
    expandedDirs.value[path] = true;
    return;
  }
  if (expandedDirs.value[path]) {
    delete expandedDirs.value[path];
  }
}

function mapEntry(item: { name?: string; path?: string; is_dir?: boolean; ignored?: boolean }): FileTreeEntry {
  return {
    name: String(item.name ?? ""),
    path: String(item.path ?? ""),
    is_dir: Boolean(item.is_dir),
    ignored: Boolean(item.ignored)
  };
}

async function loadDir(path: string, markLoading = false) {
  if (!props.taskId) {
    return;
  }
  const normalized = path || ROOT;
  if (loadingDirs.value[normalized]) {
    return;
  }
  if (markLoading) {
    loadingDirs.value[normalized] = true;
  }
  try {
    const encoded = encodeURIComponent(normalized);
    const res = (await fetch(`/api/v1/tasks/${props.taskId}/files/tree?path=${encoded}`).then((r) => r.json())) as APIResponse<{
      entries?: Array<{ name?: string; path?: string; is_dir?: boolean; ignored?: boolean }>;
    }>;
    if (!res.ok) {
      throw new Error(String(res.error?.code ?? "TASK_FILE_TREE_LOAD_FAILED"));
    }
    treeByPath.value[normalized] = (res.data?.entries ?? []).map(mapEntry);
  } catch (e: any) {
    const text = String(e?.message ?? "TASK_FILE_TREE_LOAD_FAILED");
    error.value = text;
    toast.error(text);
  } finally {
    if (loadingDirs.value[normalized]) {
      delete loadingDirs.value[normalized];
    }
  }
}

async function searchFiles(keyword: string) {
  if (!props.taskId) {
    return;
  }
  const query = keyword.trim();
  if (!query) {
    searchEntries.value = [];
    return;
  }
  searching.value = true;
  try {
    const encoded = encodeURIComponent(query);
    const res = (await fetch(`/api/v1/tasks/${props.taskId}/files/search?q=${encoded}`).then((r) => r.json())) as APIResponse<{
      entries?: Array<{ name?: string; path?: string; is_dir?: boolean; ignored?: boolean }>;
    }>;
    if (!res.ok) {
      throw new Error(String(res.error?.code ?? "TASK_FILE_SEARCH_FAILED"));
    }
    searchEntries.value = (res.data?.entries ?? []).map(mapEntry);
  } catch (e: any) {
    const text = String(e?.message ?? "TASK_FILE_SEARCH_FAILED");
    error.value = text;
    toast.error(text);
  } finally {
    searching.value = false;
  }
}

const previewMode = computed<FilePreviewMode>(() => getFilePreviewMode(selectedFilePath.value));
const previewRawURL = computed(() => {
  if (!props.taskId || !selectedFilePath.value) {
    return "";
  }
  return `/api/v1/tasks/${props.taskId}/files/raw?path=${encodeURIComponent(selectedFilePath.value)}`;
});

async function selectFile(path: string) {
  if (!props.taskId || !path) {
    return;
  }
  selectedFilePath.value = path;
  selectedFileContent.value = "";
  const mode = getFilePreviewMode(path);
  if (mode !== "txt") {
    previewLoading.value = false;
    return;
  }
  previewLoading.value = true;
  try {
    const encodedPath = encodeURIComponent(path);
    const res = (await fetch(`/api/v1/tasks/${props.taskId}/files/content?path=${encodedPath}`).then((r) => r.json())) as APIResponse<{
      content?: string;
    }>;
    if (!res.ok) {
      throw new Error(String(res.error?.code ?? "TASK_FILE_CONTENT_LOAD_FAILED"));
    }
    selectedFileContent.value = String(res.data?.content ?? "");
  } catch (e: any) {
    const text = String(e?.message ?? "TASK_FILE_CONTENT_LOAD_FAILED");
    error.value = text;
    toast.error(text);
  } finally {
    previewLoading.value = false;
  }
}

async function refreshRoot() {
  if (!props.taskId) {
    treeByPath.value = {};
    expandedDirs.value = {};
    error.value = "";
    return;
  }
  loading.value = true;
  error.value = "";
  const snapshot = readDraftSnapshot();
  searchQuery.value = snapshot?.searchQuery ?? "";
  searchEntries.value = [];
  treeByPath.value = {};
  expandedDirs.value = {};
  selectedFilePath.value = snapshot?.selectedFilePath ?? "";
  selectedFileContent.value = "";
  previewLoading.value = false;
  try {
    await loadDir(ROOT);
    for (const path of snapshot?.expandedPaths ?? []) {
      if (!path || path === ROOT) {
        continue;
      }
      setExpanded(path, true);
      await loadDir(path, true);
    }
    if (selectedFilePath.value) {
      await selectFile(selectedFilePath.value);
    }
  } finally {
    loading.value = false;
  }
}

async function onClickEntry(entry: FileTreeEntry) {
  if (!entry.is_dir) {
    await selectFile(entry.path);
    return;
  }
  const next = !isExpanded(entry.path);
  setExpanded(entry.path, next);
  if (!next) {
    return;
  }
  if (!treeByPath.value[entry.path]) {
    await loadDir(entry.path, true);
  }
}

function collectVisible(path: string, depth: number, out: FlatNode[]) {
  const children = treeByPath.value[path] ?? [];
  for (const entry of children) {
    out.push({
      key: `${entry.path}-${depth}`,
      entry,
      depth
    });
    if (entry.is_dir && isExpanded(entry.path)) {
      collectVisible(entry.path, depth + 1, out);
    }
  }
}

const visibleNodes = computed<FlatNode[]>(() => {
  const out: FlatNode[] = [];
  collectVisible(ROOT, 0, out);
  return out;
});

const displayNodes = computed<FlatNode[]>(() => {
  if (!searchQuery.value.trim()) {
    return visibleNodes.value;
  }
  return searchEntries.value.map((entry, idx) => ({
    key: `search-${entry.path}-${idx}`,
    entry,
    depth: 0
  }));
});

function indentStyle(depth: number) {
  return { paddingLeft: `${depth * 14 + 8}px` };
}

async function copyPath() {
  if (!selectedFilePath.value) {
    return;
  }
  try {
    await navigator.clipboard.writeText(selectedFilePath.value);
    toast.success(t("filePanel.copiedFilePath"));
  } catch {
    toast.error(t("filePanel.copyFailed"));
  }
}

function emitEdit() {
  if (!selectedFilePath.value) {
    return;
  }
  emit("file-open", selectedFilePath.value);
}

watch(
  () => `${String(props.projectId ?? "").trim()}|${props.taskId}`,
  async () => {
    await refreshRoot();
  },
  { immediate: true }
);

watch(
  () => searchQuery.value,
  async (next) => {
    await searchFiles(next);
  }
);

watch([searchQuery, expandedDirs, selectedFilePath], persistDraftSnapshot, { deep: true });
</script>

<template>
  <div class="h-full min-h-0 flex flex-col gap-2">
    <section class="flex flex-col gap-2">
      <Input
        v-model="searchQuery"
        data-test-id="shellman-file-search-input"
        :placeholder="t('filePanel.searchByFileName')"
      />
    </section>

    <section class="flex-1 min-h-0">
      <ResizablePanelGroup direction="vertical" class="h-full">
        <ResizablePanel :default-size="50" :min-size="20" class="mb-4">
          <div class="h-full min-h-0 flex flex-col gap-2">
            <div class="text-xs text-muted-foreground px-1 truncate" :title="props.repoRoot || ''">
              {{ props.repoRoot || "Repository" }}
            </div>
            <ScrollArea class="flex-1 min-h-0 border rounded-md">
              <div v-if="loading" class="text-xs text-muted-foreground p-3" data-test-id="shellman-file-tree-loading">loading...</div>
              <div v-else class="p-1.5 space-y-1" data-test-id="shellman-file-tree-list">
                <div v-if="searching" class="text-xs text-muted-foreground px-2 py-1">searching...</div>
                <Button
                  v-for="node in displayNodes"
                  :key="node.key"
                  type="button"
                  variant="ghost"
                  size="sm"
                  class="w-full justify-start h-7 px-1 text-xs font-normal"
                  :class="{ 'opacity-45': node.entry.ignored }"
                  :style="indentStyle(node.depth)"
                  :data-test-id="`shellman-file-item-${node.entry.path}`"
                  :data-ignored="node.entry.ignored ? 'true' : 'false'"
                  @click="onClickEntry(node.entry)"
                >
                  <FolderOpen v-if="node.entry.is_dir && isExpanded(node.entry.path)" class="mr-1.5 h-3.5 w-3.5 opacity-70" />
                  <Folder v-else-if="node.entry.is_dir" class="mr-1.5 h-3.5 w-3.5 opacity-70" />
                  <File v-else class="mr-1.5 h-3.5 w-3.5 opacity-70" />
                  <span class="truncate">{{ node.entry.name }}</span>
                </Button>
                <div v-if="displayNodes.length === 0" class="text-xs text-muted-foreground px-2 py-1">
                  {{ t("filePanel.noFiles") }}
                </div>
              </div>
            </ScrollArea>
          </div>
        </ResizablePanel>

        <ResizableHandle class="opacity-0" />
        <ResizablePanel :default-size="50" :min-size="20">
          <div class="h-full min-h-0 flex flex-col gap-2">
            <div class="h-8 border rounded-md px-2 flex items-center justify-between text-xs">
              <span class="truncate font-mono" :title="selectedFilePath">{{ selectedFilePath || "未选择文件" }}</span>
              <div class="flex items-center gap-1">
                <Button
                  type="button"
                  variant="ghost"
                  size="icon"
                  class="h-6 w-6"
                  :disabled="!selectedFilePath"
                  data-test-id="shellman-file-preview-copy"
                  @click="copyPath"
                >
                  <Copy class="h-3.5 w-3.5" />
                </Button>
                <Button
                  type="button"
                  variant="ghost"
                  size="icon"
                  class="h-6 w-6"
                  :disabled="!selectedFilePath"
                  data-test-id="shellman-file-preview-edit"
                  @click="emitEdit"
                >
                  <FilePenLine class="h-3.5 w-3.5" />
                </Button>
              </div>
            </div>
            <ScrollArea class="flex-1 min-h-0 border rounded-md bg-muted/20" :horizontal="true">
              <div v-if="previewLoading" class="text-xs text-muted-foreground p-3">loading...</div>
              <div v-else-if="!selectedFilePath" class="text-xs text-muted-foreground p-3">{{ t("filePanel.clickTreeToPreview") }}</div>
              <pre v-else-if="previewMode === 'txt'" class="p-3 text-xs font-mono whitespace-pre">{{ selectedFileContent }}</pre>
              <div v-else-if="previewMode === 'image'" class="p-3">
                <img :src="previewRawURL" :alt="t('filePanel.previewImage')" class="max-w-full max-h-[52vh] object-contain" />
              </div>
              <div v-else-if="previewMode === 'video'" class="p-3">
                <video :src="previewRawURL" controls class="max-w-full max-h-[52vh]" />
              </div>
              <div v-else class="text-xs text-muted-foreground p-3">{{ t("filePanel.previewNotSupported") }}</div>
            </ScrollArea>
          </div>
        </ResizablePanel>
      </ResizablePanelGroup>
    </section>

    <p v-if="error" class="text-xs text-destructive px-1">{{ error }}</p>
  </div>
</template>
