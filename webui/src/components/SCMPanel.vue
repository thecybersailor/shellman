<script setup lang="ts">
import { computed, ref, watch } from "vue";
import { ScrollArea } from "@/components/ui/scroll-area";
import { Button } from "@/components/ui/button";
import { InputGroup, InputGroupAddon, InputGroupTextarea } from "@/components/ui/input-group";
import { Loader2 } from "lucide-vue-next";
import { toast } from "vue-sonner";

const props = withDefaults(
  defineProps<{
    taskId: string;
    aiLoading?: boolean;
    submitLoading?: boolean;
  }>(),
  {
    aiLoading: false,
    submitLoading: false
  }
);
const emit = defineEmits<{
  (e: "ai", payload: { taskId: string; diff: string; files: string[]; selectedFilePath: string }): void;
  (e: "submit", payload: { taskId: string; message: string }): void;
}>();

const loading = ref(false);
const aiInternalLoading = ref(false);
const submitInternalLoading = ref(false);
const error = ref("");
const commitMessage = ref("");
const diffText = ref("");

const files = ref<{ path: string; status: string }[]>([]);
const selectedFilePath = ref("");
const selectedFileContent = ref("");

type DiffRowType = "add" | "remove" | "context" | "meta";

type DiffRow = {
  id: string;
  marker: string;
  oldLine: number | null;
  newLine: number | null;
  text: string;
  type: DiffRowType;
};

type APIResponse<T> = {
  ok: boolean;
  data?: T;
  error?: { code?: string; message?: string };
};

async function loadDiff(taskId: string) {
  const res = (await fetch(`/api/v1/tasks/${taskId}/diff`).then((r) => r.json())) as APIResponse<{ diff?: string }>;
  if (!res.ok) {
    throw new Error(String(res.error?.code ?? "TASK_DIFF_LOAD_FAILED"));
  }
  diffText.value = String(res.data?.diff ?? "");
}

async function loadFiles(taskId: string) {
  const res = (await fetch(`/api/v1/tasks/${taskId}/files`).then((r) => r.json())) as APIResponse<{
    files?: Array<{ path?: string; status?: string }>;
  }>;
  if (!res.ok) {
    throw new Error(String(res.error?.code ?? "TASK_FILES_LOAD_FAILED"));
  }
  files.value = (res.data?.files ?? []).map((item) => ({
    path: String(item.path ?? ""),
    status: String(item.status ?? "")
  }));

  if (files.value.length === 0) {
    selectedFilePath.value = "";
    selectedFileContent.value = "";
    return;
  }

  const existing = files.value.find((item) => item.path === selectedFilePath.value);
  const targetPath = existing?.path ?? files.value[0]?.path ?? "";
  if (targetPath) {
    await loadFileContent(taskId, targetPath);
  }
}

async function loadFileContent(taskId: string, path: string) {
  selectedFilePath.value = path;
  const encodedPath = encodeURIComponent(path);
  const res = (await fetch(`/api/v1/tasks/${taskId}/files/content?path=${encodedPath}`).then((r) => r.json())) as APIResponse<{
    content?: string;
  }>;
  if (!res.ok) {
    throw new Error(String(res.error?.code ?? "TASK_FILE_CONTENT_LOAD_FAILED"));
  }
  selectedFileContent.value = String(res.data?.content ?? "");
}

async function refreshTaskData(taskId: string) {
  if (!taskId) {
    return;
  }
  loading.value = true;
  error.value = "";
  try {
    await loadDiff(taskId);
    await loadFiles(taskId);
  } catch (e: any) {
    error.value = String(e?.message ?? "LOAD_FAILED");
    toast.error(error.value);
  } finally {
    loading.value = false;
  }
}

async function handleGenerateCommitMessage() {
  if (props.aiLoading || aiInternalLoading.value || loading.value) {
    return;
  }
  aiInternalLoading.value = true;
  error.value = "";
  try {
    const res = (await fetch(`/api/v1/tasks/${props.taskId}/commit-message/generate`, {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: "{}"
    }).then((r) => r.json())) as APIResponse<{ message?: string }>;
    if (!res.ok) {
      throw new Error(String(res.error?.code ?? "TASK_COMMIT_MESSAGE_GENERATE_FAILED"));
    }
    commitMessage.value = String(res.data?.message ?? "");
    emit("ai", {
      taskId: props.taskId,
      diff: diffText.value,
      files: files.value.map((item) => item.path),
      selectedFilePath: selectedFilePath.value
    });
  } catch (e: any) {
    error.value = String(e?.message ?? "TASK_COMMIT_MESSAGE_GENERATE_FAILED");
    toast.error(error.value);
  } finally {
    aiInternalLoading.value = false;
  }
}

async function handleSubmitCommitMessage() {
  if (props.submitLoading || submitInternalLoading.value || loading.value) {
    return;
  }
  const message = commitMessage.value.trim();
  if (!message) {
    return;
  }
  submitInternalLoading.value = true;
  error.value = "";
  try {
    const res = (await fetch(`/api/v1/tasks/${props.taskId}/commit`, {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ message })
    }).then((r) => r.json())) as APIResponse<{ commit_hash?: string }>;
    if (!res.ok) {
      throw new Error(String(res.error?.code ?? "TASK_COMMIT_FAILED"));
    }
    emit("submit", { taskId: props.taskId, message });
    commitMessage.value = "";
    await loadDiff(props.taskId);
    await loadFiles(props.taskId);
  } catch (e: any) {
    error.value = String(e?.message ?? "TASK_COMMIT_FAILED");
    toast.error(error.value);
  } finally {
    submitInternalLoading.value = false;
  }
}

function extractFilePatch(diff: string, path: string): string[] {
  if (!diff.trim() || !path) {
    return [];
  }

  const lines = diff.replaceAll("\r\n", "\n").split("\n");
  let inTarget = false;
  let collecting = false;
  const patch: string[] = [];

  for (let i = 0; i < lines.length; i += 1) {
    const line = lines[i] ?? "";
    if (line.startsWith("diff --git ")) {
      if (collecting) {
        break;
      }

      const afterPrefix = line.slice("diff --git ".length);
      const parts = afterPrefix.split(" ");
      const aPath = (parts[0] ?? "").replace(/^a\//, "");
      const bPath = (parts[1] ?? "").replace(/^b\//, "");
      inTarget = aPath === path || bPath === path;
      continue;
    }

    if (!inTarget) {
      continue;
    }

    if (line.startsWith("@@ ")) {
      collecting = true;
    }

    if (collecting) {
      patch.push(line);
    }
  }

  return patch;
}

function buildRowsFromPatch(lines: string[]): DiffRow[] {
  const rows: DiffRow[] = [];
  let oldLine = 0;
  let newLine = 0;

  for (let i = 0; i < lines.length; i += 1) {
    const line = lines[i] ?? "";

    if (line.startsWith("@@ ")) {
      const match = line.match(/^@@\s+-(\d+)(?:,\d+)?\s+\+(\d+)(?:,\d+)?\s+@@/);
      if (match) {
        oldLine = Number(match[1]);
        newLine = Number(match[2]);
      }
      rows.push({
        id: `meta-${i}`,
        marker: "@@",
        oldLine: null,
        newLine: null,
        text: line,
        type: "meta"
      });
      continue;
    }

    if (line.startsWith("+")) {
      rows.push({
        id: `add-${i}`,
        marker: "+",
        oldLine: null,
        newLine,
        text: line.slice(1),
        type: "add"
      });
      newLine += 1;
      continue;
    }

    if (line.startsWith("-")) {
      rows.push({
        id: `remove-${i}`,
        marker: "-",
        oldLine,
        newLine: null,
        text: line.slice(1),
        type: "remove"
      });
      oldLine += 1;
      continue;
    }

    if (line.startsWith("\\")) {
      rows.push({
        id: `meta-note-${i}`,
        marker: "\\",
        oldLine: null,
        newLine: null,
        text: line,
        type: "meta"
      });
      continue;
    }

    if (line.startsWith(" ")) {
      rows.push({
        id: `context-${i}`,
        marker: " ",
        oldLine,
        newLine,
        text: line.slice(1),
        type: "context"
      });
      oldLine += 1;
      newLine += 1;
      continue;
    }
  }

  return rows;
}

const selectedFileStatus = computed(() => files.value.find((item) => item.path === selectedFilePath.value)?.status ?? "");

const diffRows = computed<DiffRow[]>(() => {
  if (!selectedFilePath.value) {
    return [];
  }

  const patchLines = extractFilePatch(diffText.value, selectedFilePath.value);
  const patchRows = buildRowsFromPatch(patchLines);
  if (patchRows.length > 0) {
    return patchRows;
  }

  const content = selectedFileContent.value;
  if (!content) {
    return [];
  }

  const status = selectedFileStatus.value.toUpperCase();
  const textLines = content.replaceAll("\r\n", "\n").split("\n");
  if (textLines.length > 0 && textLines[textLines.length - 1] === "") {
    textLines.pop();
  }

  return textLines.map((text, idx) => {
    const lineNo = idx + 1;
    const untrackedOrAdded = status.includes("??") || status.includes("A");
    return {
      id: `content-${lineNo}`,
      marker: untrackedOrAdded ? "+" : " ",
      oldLine: untrackedOrAdded ? null : lineNo,
      newLine: lineNo,
      text,
      type: untrackedOrAdded ? "add" : "context"
    };
  });
});

function formatLineNo(value: number | null): string {
  return value == null ? "" : String(value);
}

watch(
  () => props.taskId,
  async (taskId) => {
    await refreshTaskData(taskId);
  },
  { immediate: true }
);

watch(
  () => props.submitLoading,
  async (next, prev) => {
    if (prev && !next && props.taskId) {
      await refreshTaskData(props.taskId);
    }
  }
);
</script>

<template>
  <div class="h-full min-h-0 flex flex-col gap-2" data-test-id="muxt-addon-panel">
    <section class="flex flex-col gap-2">
      <InputGroup class="h-auto min-h-[120px] flex-col items-stretch">
        <InputGroupTextarea
          v-model="commitMessage"
          class="min-h-[32px] font-mono text-xs!"
          placeholder="Type commit message..."
          data-test-id="muxt-scm-commit-message"
        />
        <InputGroupAddon align="block-end" class="w-full justify-end gap-2 p-2">
          <Button
            type="button"
            variant="outline"
            size="sm"
            data-test-id="muxt-scm-ai-button"
            :disabled="props.aiLoading || aiInternalLoading || loading"
            @click="handleGenerateCommitMessage"
          >
            <Loader2 v-if="props.aiLoading || aiInternalLoading" class="mr-1 h-3.5 w-3.5 animate-spin" />
            {{ props.aiLoading || aiInternalLoading ? "生成中" : "AI" }}
          </Button>
          <Button
            type="button"
            size="sm"
            class="min-w-30"
            data-test-id="muxt-scm-submit-button"
            :disabled="props.submitLoading || submitInternalLoading || loading || !commitMessage.trim()"
            @click="handleSubmitCommitMessage"
          >
            <Loader2 v-if="props.submitLoading || submitInternalLoading" class="mr-1 h-3.5 w-3.5 animate-spin" />
            {{ props.submitLoading || submitInternalLoading ? "提交中" : "提交" }}
          </Button>
        </InputGroupAddon>
      </InputGroup>
    </section>

    <section class="flex flex-col gap-2 min-h-0">
      <ScrollArea class="h-40 border rounded-md">
        <div v-if="loading" class="text-xs text-muted-foreground p-3" data-test-id="muxt-addon-files-loading">loading...</div>
        <div v-else class="p-2 space-y-1" data-test-id="muxt-addon-file-list">
          <Button
            v-for="file in files"
            :key="file.path"
            variant="ghost"
            size="sm"
            class="w-full justify-start text-xs h-7 px-2"
            :class="{ 'bg-accent text-accent-foreground': selectedFilePath === file.path }"
            :data-selected="selectedFilePath === file.path"
            @click="loadFileContent(props.taskId, file.path)"
          >
            <span class="mr-2 opacity-70 font-mono text-[10px]">{{ file.status }}</span>
            <span class="truncate">{{ file.path }}</span>
          </Button>
          <div v-if="files.length === 0" class="text-xs text-muted-foreground px-2 py-1">No changed files.</div>
        </div>
      </ScrollArea>
    </section>

    <section class="flex flex-col gap-2 flex-1 min-h-0">
      <ScrollArea class="flex-1 min-h-0 border rounded-md bg-muted/20" :horizontal="true">
        <div v-if="loading" class="text-xs text-muted-foreground p-3" data-test-id="muxt-addon-diff-loading">loading...</div>
        <div v-else-if="diffRows.length === 0" class="text-xs text-muted-foreground p-3" data-test-id="muxt-addon-file-content">
          No changes.
        </div>
        <div v-else class="muxt-diff" data-test-id="muxt-addon-file-content">
          <div
            v-for="row in diffRows"
            :key="row.id"
            class="muxt-diff-row"
            :class="`muxt-diff-row--${row.type}`"
          >
            <span class="muxt-diff-sign">{{ row.marker }}</span>
            <span class="muxt-diff-ln">{{ formatLineNo(row.oldLine) }}</span>
            <span class="muxt-diff-ln">{{ formatLineNo(row.newLine) }}</span>
            <span class="muxt-diff-code">{{ row.text }}</span>
          </div>
        </div>
      </ScrollArea>
    </section>
  </div>
</template>

<style scoped>
.muxt-diff {
  padding: 8px 0;
  font-family: ui-monospace, SFMono-Regular, Menlo, Monaco, Consolas, "Liberation Mono", "Courier New", monospace;
  font-size: 12px;
  line-height: 1.45;
  width: max-content;
  min-width: 100%;

}

.muxt-diff-row {
  display: grid;
  grid-template-columns: 18px 52px 52px 1fr;
  align-items: center;
  min-height: 20px;
}

.muxt-diff-row--context {
  background: hsl(200 28% 16% / 0.72);
}

.muxt-diff-row--add {
  background: hsl(155 44% 22% / 0.8);
}

.muxt-diff-row--remove {
  background: hsl(5 45% 22% / 0.82);
}

.muxt-diff-row--meta {
  background: hsl(210 18% 24% / 0.7);
  color: hsl(var(--muted-foreground));
}

.muxt-diff-sign,
.muxt-diff-ln {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  height: 100%;
  color: hsl(var(--muted-foreground));
}

.muxt-diff-sign {
  font-weight: 700;
}

.muxt-diff-code {
  white-space: pre;
  word-break: normal;
  overflow-wrap: normal;
  padding: 2px 8px;
}

.muxt-diff-row--add .muxt-diff-sign,
.muxt-diff-row--add .muxt-diff-ln {
  color: hsl(145 64% 74%);
}

.muxt-diff-row--remove .muxt-diff-sign,
.muxt-diff-row--remove .muxt-diff-ln {
  color: hsl(2 68% 76%);
}

.muxt-diff-row--context .muxt-diff-sign,
.muxt-diff-row--context .muxt-diff-ln {
  color: hsl(190 22% 72% / 0.9);
}

.muxt-diff-row--meta .muxt-diff-sign,
.muxt-diff-row--meta .muxt-diff-ln {
  color: hsl(var(--muted-foreground));
}
</style>
