<script setup lang="ts">
import { Terminal } from "lucide-vue-next";
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
}>();

const emit = defineEmits<{
  (event: "open-session-detail"): void;
}>();

function onOpenSessionDetail() {
  emit("open-session-detail");
}
</script>

<template>
  <CardHeader class="p-3 py-2 border-b border-border/10 bg-muted/5 select-none min-h-[52px] flex flex-col justify-center">
    <CardTitle class="text-xs p-2 w-full font-mono text-muted-foreground flex items-center overflow-hidden">
      <div
        data-test-id="shellman-task-meta-display"
        class="group dark:bg-gray-800 bg-gray-200 w-full cursor-pointer space-y-0.5 rounded-md border border-transparent p-4 transition-all hover:bg-white/5 active:bg-white/10"
        @click="onOpenSessionDetail"
      >
        <span data-test-id="shellman-console-pane-uuid" class="sr-only">{{ props.paneUuid || "" }}</span>
        <span data-test-id="shellman-console-pane-current-command" class="sr-only">{{ props.currentCommand || "" }}</span>
        <div class="hidden md:flex items-center gap-1.5 overflow-hidden">
          <Terminal class="hidden sm:block size-3 shrink-0 text-primary/60" />
          <TaskTitleResolver
            :task-title="props.taskTitle"
            :current-command="props.currentCommand"
            data-test-id="shellman-task-title-display"
            class="truncate text-xs font-semibold text-foreground tracking-tight"
          />
        </div>
        <div
          v-if="props.taskDescription"
          data-test-id="shellman-task-description-display"
          class="max-h-8 overflow-hidden text-[11px] text-foreground/50 line-clamp-1 transition-colors group-hover:text-foreground/70"
        >
          {{ props.taskDescription }}
        </div>
        <div v-else class="text-[10px] text-muted-foreground/30 italic">{{ t("task.noDescription") }}</div>
      </div>
    </CardTitle>
  </CardHeader>
</template>
