<script setup lang="ts">
import { Terminal, History, Loader2 } from "lucide-vue-next";
import { useI18n } from "vue-i18n";
import { CardHeader, CardTitle } from "@/components/ui/card";
import TaskTitleResolver from "@/components/TaskTitleResolver.vue";
const { t } = useI18n();

const props = defineProps<{
  taskId?: string;
  taskTitle?: string;
  taskDescription?: string;
  paneUuid?: string;
  currentCommand?: string;
  historyMoreLoading?: boolean;
}>();

const emit = defineEmits<{
  (event: "open-session-detail"): void;
  (event: "history-more"): void;
}>();

function onOpenSessionDetail() {
  emit("open-session-detail");
}

function onHistoryMore(event: MouseEvent) {
  event.stopPropagation();
  emit("history-more");
}
</script>

<template>
  <CardHeader class="p-2 border-b border-border/10 bg-muted/5 select-none min-h-[44px] flex flex-row items-center justify-between gap-2 space-y-0">
    <div
      data-test-id="shellman-task-meta-display"
      class="group flex flex-1 items-center gap-3 overflow-hidden cursor-pointer rounded-md border border-transparent px-2 py-1.5 transition-all hover:bg-foreground/5 active:bg-foreground/10"
      @click="onOpenSessionDetail"
    >
      <span data-test-id="shellman-console-pane-uuid" class="sr-only">{{ props.paneUuid || "" }}</span>
      <span data-test-id="shellman-console-pane-current-command" class="sr-only">{{ props.currentCommand || "" }}</span>
      <div class="flex items-center gap-1.5 overflow-hidden shrink-0">
        <Terminal class="size-3.5 shrink-0 text-primary/70" />
        <TaskTitleResolver
          :task-title="props.taskTitle"
          :current-command="props.currentCommand"
          data-test-id="shellman-task-title-display"
          class="truncate text-[13px] font-medium text-foreground tracking-tight"
        />
      </div>
      <div class="w-px h-3.5 bg-border/50 shrink-0"></div>
      <div
        v-if="props.taskDescription"
        data-test-id="shellman-task-description-display"
        class="truncate text-[12px] text-muted-foreground transition-colors group-hover:text-foreground/80 flex-1"
      >
        {{ props.taskDescription }}
      </div>
      <div v-else class="text-[12px] text-muted-foreground/50 italic truncate flex-1">{{ t("task.noDescription") }}</div>
    </div>
    <button
      data-test-id="shellman-task-history-more"
      type="button"
      :disabled="Boolean(props.historyMoreLoading)"
      :aria-busy="props.historyMoreLoading ? 'true' : 'false'"
      :class="[
        'flex items-center gap-1.5 shrink-0 rounded-md border border-border bg-background px-2.5 py-1 text-[11px] font-medium text-muted-foreground transition-colors hover:bg-muted hover:text-foreground shadow-sm',
        props.historyMoreLoading ? 'cursor-not-allowed opacity-60' : ''
      ]"
      @click="onHistoryMore"
    >
      <Loader2 v-if="props.historyMoreLoading" class="size-3 animate-spin shrink-0" />
      <History v-else class="size-3 shrink-0" />
      <span>{{ props.historyMoreLoading ? t("common.loading") : t("terminal.loadPreviousPage") }}</span>
    </button>
  </CardHeader>
</template>
