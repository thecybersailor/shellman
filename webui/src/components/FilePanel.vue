<script setup lang="ts">
import { computed, ref, watch } from "vue";
import { useI18n } from "vue-i18n";
import { ScrollArea } from "@/components/ui/scroll-area";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { ResizableHandle, ResizablePanel, ResizablePanelGroup } from "@/components/ui/resizable";
import { ContextMenu, ContextMenuContent, ContextMenuItem, ContextMenuSeparator, ContextMenuTrigger } from "@/components/ui/context-menu";
import { AlertDialog, AlertDialogAction, AlertDialogCancel, AlertDialogContent, AlertDialogDescription, AlertDialogFooter, AlertDialogHeader, AlertDialogTitle } from "@/components/ui/alert-dialog";
import { Dialog, DialogContent, DialogDescription, DialogFooter, DialogHeader, DialogTitle } from "@/components/ui/dialog";
import { Copy, FilePenLine, Folder, FolderOpen, File, Loader2 } from "lucide-vue-next";
import { toast } from "vue-sonner";
import { getFilePreviewMode, type FilePreviewMode } from "./file_preview_whitelist";
import { Markdown } from "vue-stream-markdown";
import "vue-stream-markdown/index.css";
import tsIconURL from "file-icon-vectors/dist/icons/vivid/ts.svg?url";
import jsIconURL from "file-icon-vectors/dist/icons/vivid/js.svg?url";
import jsonIconURL from "file-icon-vectors/dist/icons/vivid/json.svg?url";
import mdIconURL from "file-icon-vectors/dist/icons/vivid/md.svg?url";
import ymlIconURL from "file-icon-vectors/dist/icons/vivid/yml.svg?url";
import yamlIconURL from "file-icon-vectors/dist/icons/vivid/yaml.svg?url";
import htmlIconURL from "file-icon-vectors/dist/icons/vivid/html.svg?url";
import cssIconURL from "file-icon-vectors/dist/icons/vivid/css.svg?url";
import shIconURL from "file-icon-vectors/dist/icons/vivid/sh.svg?url";
import goIconURL from "file-icon-vectors/dist/icons/vivid/go.svg?url";
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
const renameDialogOpen = ref(false);
const pathDialogOpen = ref(false);
const deleteDialogOpen = ref(false);
const renameInput = ref("");
const pathTargetInput = ref("");
const pendingEntry = ref<FileTreeEntry | null>(null);
const pendingPathAction = ref<"move" | "copy" | null>(null);
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
  fileIconURL: string | null;
};

const FILE_ICON_BY_EXT: Record<string, string> = {
  ts: tsIconURL,
  js: jsIconURL,
  json: jsonIconURL,
  md: mdIconURL,
  yml: ymlIconURL,
  yaml: yamlIconURL,
  html: htmlIconURL,
  css: cssIconURL,
  sh: shIconURL,
  go: goIconURL
};

function resolveFileIcon(path: string): string | null {
  const normalized = String(path).trim().toLowerCase();
  const dotIndex = normalized.lastIndexOf(".");
  if (dotIndex <= 0 || dotIndex === normalized.length - 1) {
    return null;
  }
  const ext = normalized.slice(dotIndex + 1);
  return FILE_ICON_BY_EXT[ext] ?? null;
}

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
const selectedFileIsMarkdown = computed(() => /\.md$/i.test(String(selectedFilePath.value ?? "").trim()));
const previewRawURL = computed(() => {
  if (!props.taskId || !selectedFilePath.value) {
    return "";
  }
  return `/api/v1/tasks/${props.taskId}/files/raw?path=${encodeURIComponent(selectedFilePath.value)}`;
});

async function selectFile(path: string, options?: { silentOnNotFound?: boolean }) {
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
      const code = String(res.error?.code ?? "TASK_FILE_CONTENT_LOAD_FAILED");
      if (options?.silentOnNotFound && code === "FILE_NOT_FOUND") {
        selectedFilePath.value = "";
        selectedFileContent.value = "";
        return;
      }
      throw new Error(code);
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
      await selectFile(selectedFilePath.value, { silentOnNotFound: true });
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

function onDoubleClickEntry(entry: FileTreeEntry) {
  if (entry.is_dir) {
    return;
  }
  emit("file-open", entry.path);
}

function collectVisible(path: string, depth: number, out: FlatNode[]) {
  const children = treeByPath.value[path] ?? [];
  for (const entry of children) {
    out.push({
      key: `${entry.path}-${depth}`,
      entry,
      depth,
      fileIconURL: entry.is_dir ? null : resolveFileIcon(entry.path)
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
    depth: 0,
    fileIconURL: entry.is_dir ? null : resolveFileIcon(entry.path)
  }));
});

function indentStyle(depth: number) {
  return { paddingLeft: `${depth * 14 + 8}px` };
}

const pathDialogTitle = computed(() =>
  pendingPathAction.value === "move" ? t("filePanel.pathDialog.moveTitle") : t("filePanel.pathDialog.copyTitle")
);

const pathDialogConfirmLabel = computed(() =>
  pendingPathAction.value === "move" ? t("filePanel.pathDialog.moveConfirm") : t("filePanel.pathDialog.copyConfirm")
);

function clearPendingContextAction() {
  pendingEntry.value = null;
  pendingPathAction.value = null;
  renameInput.value = "";
  pathTargetInput.value = "";
}

function onRenameDialogOpenChange(open: boolean) {
  renameDialogOpen.value = open;
  if (!open) {
    clearPendingContextAction();
  }
}

function onPathDialogOpenChange(open: boolean) {
  pathDialogOpen.value = open;
  if (!open) {
    clearPendingContextAction();
  }
}

function onDeleteDialogOpenChange(open: boolean) {
  deleteDialogOpen.value = open;
  if (!open) {
    requestAnimationFrame(() => {
      if (!deleteDialogOpen.value) {
        clearPendingContextAction();
      }
    });
  }
}

function getParentDir(path: string): string {
  const normalized = String(path).trim().replaceAll("\\", "/").replace(/^\/+/, "").replace(/\/+$/, "");
  if (!normalized || normalized === ".") {
    return ROOT;
  }
  const idx = normalized.lastIndexOf("/");
  if (idx <= 0) {
    return ROOT;
  }
  return normalized.slice(0, idx);
}

function getFullPath(path: string): string {
  const rel = String(path).trim().replaceAll("\\", "/").replace(/^\/+/, "");
  const root = String(props.repoRoot ?? "").trim().replace(/\/+$/, "");
  if (!root) {
    return rel;
  }
  if (!rel) {
    return root;
  }
  return `${root}/${rel}`;
}

async function refreshAffectedDirs(paths: string[]) {
  const dirs = new Set<string>([ROOT]);
  for (const path of paths) {
    const parent = getParentDir(path);
    dirs.add(parent || ROOT);
  }
  for (const dir of dirs) {
    await loadDir(dir, dir !== ROOT);
  }
}

async function requestTaskFileAction(path: string, init?: RequestInit) {
  const res = (await fetch(path, init).then((r) => r.json())) as APIResponse<Record<string, unknown>>;
  if (!res.ok) {
    throw new Error(String(res.error?.code ?? "TASK_FILE_ACTION_FAILED"));
  }
  return res.data ?? {};
}

function onContextRename(entry: FileTreeEntry) {
  pendingEntry.value = entry;
  renameInput.value = entry.name;
  renameDialogOpen.value = true;
}

function onContextDelete(entry: FileTreeEntry) {
  pendingEntry.value = entry;
  deleteDialogOpen.value = true;
}

function onContextCut(entry: FileTreeEntry) {
  pendingEntry.value = entry;
  pendingPathAction.value = "move";
  pathTargetInput.value = entry.path;
  pathDialogOpen.value = true;
}

function onContextCopy(entry: FileTreeEntry) {
  pendingEntry.value = entry;
  pendingPathAction.value = "copy";
  pathTargetInput.value = entry.path;
  pathDialogOpen.value = true;
}

async function submitContextRename() {
  if (!props.taskId || !pendingEntry.value) {
    return;
  }
  const sourceEntry = pendingEntry.value;
  const trimmed = String(renameInput.value ?? "").trim();
  if (!trimmed) {
    return;
  }
  const data = await requestTaskFileAction(`/api/v1/tasks/${props.taskId}/files/rename`, {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({ path: sourceEntry.path, new_name: trimmed })
  });
  const targetPath = String(data.target_path ?? "").trim();
  await refreshAffectedDirs([sourceEntry.path, targetPath]);
  if (selectedFilePath.value === sourceEntry.path && targetPath) {
    await selectFile(targetPath);
  }
  renameDialogOpen.value = false;
  clearPendingContextAction();
}

async function submitContextDelete() {
  if (!props.taskId || !pendingEntry.value) {
    return;
  }
  const sourceEntry = pendingEntry.value;
  await requestTaskFileAction(`/api/v1/tasks/${props.taskId}/files?path=${encodeURIComponent(sourceEntry.path)}`, {
    method: "DELETE"
  });
  await refreshAffectedDirs([sourceEntry.path]);
  if (selectedFilePath.value === sourceEntry.path) {
    selectedFilePath.value = "";
    selectedFileContent.value = "";
  }
  deleteDialogOpen.value = false;
  clearPendingContextAction();
}

async function submitContextPathAction() {
  if (!props.taskId || !pendingEntry.value || !pendingPathAction.value) {
    return;
  }
  const sourceEntry = pendingEntry.value;
  const targetPath = String(pathTargetInput.value ?? "").trim();
  if (!targetPath) {
    return;
  }
  const endpoint = pendingPathAction.value === "move" ? "move" : "copy";
  const data = await requestTaskFileAction(`/api/v1/tasks/${props.taskId}/files/${endpoint}`, {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({ source_path: sourceEntry.path, target_path: targetPath })
  });
  const resultPath = String(data.target_path ?? targetPath).trim();
  await refreshAffectedDirs([sourceEntry.path, resultPath]);
  if (pendingPathAction.value === "move" && selectedFilePath.value === sourceEntry.path && resultPath) {
    await selectFile(resultPath);
  }
  pathDialogOpen.value = false;
  clearPendingContextAction();
}

async function onContextCopyPath(entry: FileTreeEntry) {
  try {
    await navigator.clipboard.writeText(getFullPath(entry.path));
    toast.success(t("filePanel.copiedFilePath"));
  } catch {
    toast.error(t("filePanel.copyFailed"));
  }
}

async function onContextCopyRelativePath(entry: FileTreeEntry) {
  try {
    await navigator.clipboard.writeText(entry.path);
    toast.success(t("filePanel.copiedFilePath"));
  } catch {
    toast.error(t("filePanel.copyFailed"));
  }
}

async function runContextAction(action: () => Promise<void> | void) {
  try {
    await action();
  } catch (e: any) {
    const text = String(e?.message ?? "TASK_FILE_ACTION_FAILED");
    error.value = text;
    toast.error(text);
  }
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
                <ContextMenu
                  v-for="node in displayNodes"
                  :key="node.key"
                >
                  <ContextMenuTrigger as-child>
                    <Button
                      type="button"
                      variant="ghost"
                      size="sm"
                      class="w-full justify-start h-7 px-1 text-xs font-normal"
                      :class="{ 'opacity-45': node.entry.ignored }"
                      :style="indentStyle(node.depth)"
                      :data-test-id="`shellman-file-item-${node.entry.path}`"
                      :data-ignored="node.entry.ignored ? 'true' : 'false'"
                      @click="onClickEntry(node.entry)"
                      @dblclick.stop="onDoubleClickEntry(node.entry)"
                    >
                      <Loader2
                        v-if="node.entry.is_dir && loadingDirs[node.entry.path]"
                        class="mr-1.5 h-3.5 w-3.5 animate-spin opacity-70"
                        :data-test-id="`shellman-file-dir-loading-${node.entry.path}`"
                      />
                      <FolderOpen v-else-if="node.entry.is_dir && isExpanded(node.entry.path)" class="mr-1.5 h-3.5 w-3.5 opacity-70" />
                      <Folder v-else-if="node.entry.is_dir" class="mr-1.5 h-3.5 w-3.5 opacity-70" />
                      <template v-else>
                        <img
                          v-if="node.fileIconURL"
                          :src="node.fileIconURL"
                          :alt="`${node.entry.name} icon`"
                          class="mr-1.5 h-3.5 w-3.5 opacity-80 shrink-0"
                          data-test-id="shellman-file-icon-kind"
                          data-icon-kind="mapped"
                        />
                        <File v-else class="mr-1.5 h-3.5 w-3.5 opacity-70" data-test-id="shellman-file-icon-kind" data-icon-kind="default" />
                      </template>
                      <span class="truncate">{{ node.entry.name }}</span>
                    </Button>
                  </ContextMenuTrigger>
                  <ContextMenuContent align="start" class="w-48">
                    <ContextMenuItem
                      data-test-id="shellman-file-context-cut"
                      @select.prevent="runContextAction(() => onContextCut(node.entry))"
                    >
                      {{ t("filePanel.contextMenu.cut") }}
                    </ContextMenuItem>
                    <ContextMenuItem
                      data-test-id="shellman-file-context-copy"
                      @select.prevent="runContextAction(() => onContextCopy(node.entry))"
                    >
                      {{ t("filePanel.contextMenu.copy") }}
                    </ContextMenuItem>
                    <ContextMenuSeparator />
                    <ContextMenuItem
                      data-test-id="shellman-file-context-copy-path"
                      @select.prevent="runContextAction(() => onContextCopyPath(node.entry))"
                    >
                      {{ t("filePanel.contextMenu.copyPath") }}
                    </ContextMenuItem>
                    <ContextMenuItem
                      data-test-id="shellman-file-context-copy-relative-path"
                      @select.prevent="runContextAction(() => onContextCopyRelativePath(node.entry))"
                    >
                      {{ t("filePanel.contextMenu.copyRelativePath") }}
                    </ContextMenuItem>
                    <ContextMenuSeparator />
                    <ContextMenuItem
                      data-test-id="shellman-file-context-rename"
                      @select.prevent="runContextAction(() => onContextRename(node.entry))"
                    >
                      {{ t("filePanel.contextMenu.rename") }}
                    </ContextMenuItem>
                    <ContextMenuItem
                      data-test-id="shellman-file-context-delete"
                      variant="destructive"
                      @select.prevent="runContextAction(() => onContextDelete(node.entry))"
                    >
                      {{ t("filePanel.contextMenu.delete") }}
                    </ContextMenuItem>
                  </ContextMenuContent>
                </ContextMenu>
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
              <div
                v-else-if="previewMode === 'txt' && selectedFileIsMarkdown"
                class="shellman-markdown-compact p-3 text-sm leading-6"
                data-test-id="shellman-file-preview-markdown"
              >
                <Markdown :content="selectedFileContent" />
              </div>
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

    <Dialog :open="renameDialogOpen" @update:open="(open) => onRenameDialogOpenChange(Boolean(open))">
      <DialogContent class="sm:max-w-md">
        <DialogHeader>
          <DialogTitle>{{ t("filePanel.renameDialog.title") }}</DialogTitle>
          <DialogDescription>{{ t("filePanel.renameDialog.description") }}</DialogDescription>
        </DialogHeader>
        <Input
          v-model="renameInput"
          data-test-id="shellman-file-rename-input"
          :placeholder="t('filePanel.renameDialog.placeholder')"
          @keyup.enter="runContextAction(() => submitContextRename())"
        />
        <DialogFooter>
          <Button
            type="button"
            variant="outline"
            data-test-id="shellman-file-rename-cancel"
            @click="onRenameDialogOpenChange(false)"
          >
            {{ t("common.cancel") }}
          </Button>
          <Button type="button" data-test-id="shellman-file-rename-confirm" @click="runContextAction(() => submitContextRename())">
            {{ t("common.save") }}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>

    <Dialog :open="pathDialogOpen" @update:open="(open) => onPathDialogOpenChange(Boolean(open))">
      <DialogContent class="sm:max-w-md">
        <DialogHeader>
          <DialogTitle>{{ pathDialogTitle }}</DialogTitle>
          <DialogDescription>{{ t("filePanel.pathDialog.description") }}</DialogDescription>
        </DialogHeader>
        <Input
          v-model="pathTargetInput"
          data-test-id="shellman-file-path-action-input"
          :placeholder="t('filePanel.pathDialog.placeholder')"
          @keyup.enter="runContextAction(() => submitContextPathAction())"
        />
        <DialogFooter>
          <Button
            type="button"
            variant="outline"
            data-test-id="shellman-file-path-action-cancel"
            @click="onPathDialogOpenChange(false)"
          >
            {{ t("common.cancel") }}
          </Button>
          <Button type="button" data-test-id="shellman-file-path-action-confirm" @click="runContextAction(() => submitContextPathAction())">
            {{ pathDialogConfirmLabel }}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>

    <AlertDialog :open="deleteDialogOpen" @update:open="(open) => onDeleteDialogOpenChange(Boolean(open))">
      <AlertDialogContent>
        <AlertDialogHeader>
          <AlertDialogTitle>{{ t("filePanel.deleteDialog.title") }}</AlertDialogTitle>
          <AlertDialogDescription>
            {{ t("filePanel.deleteDialog.description", { name: pendingEntry?.name || "" }) }}
          </AlertDialogDescription>
        </AlertDialogHeader>
        <AlertDialogFooter>
          <AlertDialogCancel data-test-id="shellman-file-delete-cancel">
            {{ t("common.cancel") }}
          </AlertDialogCancel>
          <AlertDialogAction
            data-test-id="shellman-file-delete-confirm"
            class="bg-destructive text-destructive-foreground hover:bg-destructive/90"
            @click.prevent="runContextAction(() => submitContextDelete())"
          >
            {{ t("filePanel.contextMenu.delete") }}
          </AlertDialogAction>
        </AlertDialogFooter>
      </AlertDialogContent>
    </AlertDialog>

    <p v-if="error" class="text-xs text-destructive px-1">{{ error }}</p>
  </div>
</template>
