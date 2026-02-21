<script setup lang="ts">
import { reactive, ref, watch } from "vue";
import { useI18n } from "vue-i18n";
import { ScrollArea } from "@/components/ui/scroll-area";
import { Accordion, AccordionContent, AccordionItem, AccordionTrigger } from "@/components/ui/accordion";
import { Button } from "@/components/ui/button";
import { Checkbox } from "@/components/ui/checkbox";
import { Toggle } from "@/components/ui/toggle";
import TaskTitleResolver from "./TaskTitleResolver.vue";
import TaskFlagDot from "./TaskFlagDot.vue";
import { 
  DropdownMenu, 
  DropdownMenuContent, 
  DropdownMenuItem, 
  DropdownMenuTrigger 
} from "@/components/ui/dropdown-menu";
import { 
  Folder, 
  Plus, 
  Settings, 
  FolderPlus, 
  ListFilter, 
  MoreHorizontal, 
  SquarePen, 
  ChevronDown, 
  ChevronRight,
  Archive,
  Pencil,
  X,
  LoaderCircle
} from "lucide-vue-next";
const { t } = useI18n();

export type TaskStatus =
  | "pending"
  | "running"
  | "completed"
  | "failed"
  | "waiting_user"
  | "waiting_children"
  | "canceled";

export interface Task {
  taskId: string;
  parentTaskId?: string;
  title: string;
  currentCommand?: string;
  flag?: "success" | "notify" | "error";
  flagDesc?: string;
  flagReaded?: boolean;
  checked?: boolean;
  archived?: boolean;
  status: TaskStatus;
  metadata?: string;
  updatedAt?: number;
  runtimeStatus?: "running" | "ready" | "unknown";
  runtimeUpdatedAt?: number;
}

export interface ProjectSection {
  projectId: string;
  title: string;
  tasks: Task[];
}

type OrphanPaneItem = {
  target: string;
  title: string;
  status: "running" | "ready" | "unknown";
  updatedAt: number;
};

const props = defineProps<{
  projects: ProjectSection[];
  selectedTaskId?: string;
  hideFooter?: boolean;
  orphanPanes?: OrphanPaneItem[];
  showOrphanSection?: boolean;
}>();

const emit = defineEmits<{
  (event: "select-task", taskId: string): void;
  (event: "toggle-task-check", payload: { taskId: string; checked: boolean }): void;
  (event: "create-root-pane", projectId: string): void;
  (event: "create-child-pane", taskId: string): void;
  (event: "add-project"): void;
  (event: "open-settings"): void;
  (event: "edit-project", projectId: string): void;
  (event: "archive-project-done", projectId: string): void;
  (event: "remove-project", projectId: string): void;
  (event: "adopt-pane", payload: { parentTaskId: string; paneTarget: string; title?: string }): void;
}>();

const expandedItems = ref<string[]>([]);
const seenProjectIds = new Set<string>();

// Sync expanded items when NEW projects load
watch(() => props.projects, (newProjects) => {
  newProjects.forEach(project => {
    if (!seenProjectIds.has(project.projectId)) {
      seenProjectIds.add(project.projectId);
      expandedItems.value.push(project.projectId);
    }
  });
}, { immediate: true });

type TaskFilterKey = "done" | "error" | "notice" | "success";

const checkedByTaskId = reactive<Record<string, boolean>>({});
const filterCardOpen = ref(false);
const taskFilterState = reactive<Record<TaskFilterKey, boolean>>({
  done: true,
  error: true,
  notice: true,
  success: true
});
const taskFilterOptions: TaskFilterKey[] = ["done", "error", "notice", "success"];
const orphanOpen = ref(false);
const draggingPane = ref<{ paneTarget: string; title?: string } | null>(null);
const dragOverTaskId = ref("");

watch(
  () => props.projects,
  (projects) => {
    const alive = new Set<string>();
    for (const project of projects) {
      for (const task of project.tasks) {
        alive.add(task.taskId);
        checkedByTaskId[task.taskId] = Boolean(task.checked);
      }
    }
    for (const taskId of Object.keys(checkedByTaskId)) {
      if (!alive.has(taskId)) {
        delete checkedByTaskId[taskId];
      }
    }
  },
  { immediate: true, deep: true }
);

function statusLabel(status: TaskStatus) {
  return status.replace("_", " ");
}

function formatRelativeTime(unixSecond: number | undefined) {
  if (!Number.isFinite(unixSecond) || !unixSecond || unixSecond <= 0) {
    return "--";
  }
  const diffMs = Math.max(0, Date.now() - Number(unixSecond) * 1000);
  const minutes = Math.floor(diffMs / 60000);
  if (minutes < 1) {
    return "now";
  }
  if (minutes < 60) {
    return `${minutes}m`;
  }
  const hours = Math.floor(minutes / 60);
  if (hours < 24) {
    return `${hours}h`;
  }
  const days = Math.floor(hours / 24);
  return `${days}d`;
}

function displayTime(task: Task) {
  if (task.runtimeStatus === "running") {
    return "";
  }
  if (task.runtimeUpdatedAt) {
    return formatRelativeTime(task.runtimeUpdatedAt);
  }
  if (task.updatedAt) {
    return formatRelativeTime(task.updatedAt);
  }
  return task.metadata || "--";
}

function isTaskChecked(task: Task) {
  return Boolean(checkedByTaskId[task.taskId]);
}

function onTaskCheckModelUpdate(task: Task, checked: boolean | "indeterminate") {
  const normalized = checked === true;
  checkedByTaskId[task.taskId] = normalized;
  emit("toggle-task-check", { taskId: task.taskId, checked: normalized });
}

function toggleTaskFilter(key: TaskFilterKey) {
  taskFilterState[key] = !taskFilterState[key];
}

function taskMatchesFilter(task: Task, key: TaskFilterKey) {
  if (key === "done") {
    return isTaskChecked(task);
  }
  if (key === "error") {
    return task.flag === "error";
  }
  if (key === "notice") {
    return task.flag === "notify";
  }
  return task.flag === "success";
}

type TaskRow = {
  task: Task;
  depth: number;
};

function buildTaskRows(tasks: Task[]): TaskRow[] {
  const byId = new Map<string, Task>();
  const childrenByParent = new Map<string, Task[]>();
  const roots: Task[] = [];

  for (const task of tasks) {
    byId.set(task.taskId, task);
  }

  for (const task of tasks) {
    const parentId = task.parentTaskId;
    if (parentId && byId.has(parentId)) {
      const bucket = childrenByParent.get(parentId) ?? [];
      bucket.push(task);
      childrenByParent.set(parentId, bucket);
    } else {
      roots.push(task);
    }
  }

  const rows: TaskRow[] = [];
  const visited = new Set<string>();

  const append = (task: Task, depth: number) => {
    if (visited.has(task.taskId)) {
      return;
    }
    visited.add(task.taskId);
    rows.push({ task, depth });
    const children = childrenByParent.get(task.taskId) ?? [];
    for (const child of children) {
      append(child, depth + 1);
    }
  };

  for (const root of roots) {
    append(root, 0);
  }

  for (const task of tasks) {
    if (!visited.has(task.taskId)) {
      append(task, 0);
    }
  }

  return rows;
}

function visibleTaskRows(tasks: Task[]): TaskRow[] {
  const rows = buildTaskRows(tasks);
  if (!filterCardOpen.value) {
    return rows;
  }
  if (taskFilterState.done) {
    return rows;
  }

  // done 未勾选时: 仅展示未完成任务，并且必须命中已勾选的 flag 过滤项
  const flagFilters = (["error", "notice", "success"] as const).filter((key) => taskFilterState[key]);
  if (flagFilters.length === 0) {
    return [];
  }

  return rows.filter((row) => {
    return !isTaskChecked(row.task) && flagFilters.some((key) => taskMatchesFilter(row.task, key));
  });
}

function stableTarget(target: string) {
  return target.replace(/[^a-zA-Z0-9_-]/g, "-");
}

function onOrphanDragStart(event: DragEvent, pane: OrphanPaneItem) {
  draggingPane.value = {
    paneTarget: String(pane.target ?? ""),
    title: String(pane.title ?? "")
  };
  if (event.dataTransfer) {
    event.dataTransfer.effectAllowed = "move";
    event.dataTransfer.setData("text/plain", String(pane.target ?? ""));
  }
}

function onOrphanDragEnd() {
  draggingPane.value = null;
  dragOverTaskId.value = "";
}

function onTaskDragOver(taskId: string) {
  if (!draggingPane.value) {
    return;
  }
  dragOverTaskId.value = taskId;
}

function onTaskDragLeave(taskId: string) {
  if (dragOverTaskId.value === taskId) {
    dragOverTaskId.value = "";
  }
}

function onTaskDrop(taskId: string) {
  if (!draggingPane.value) {
    return;
  }
  emit("adopt-pane", {
    parentTaskId: taskId,
    paneTarget: draggingPane.value.paneTarget,
    title: draggingPane.value.title
  });
  draggingPane.value = null;
  dragOverTaskId.value = "";
}
</script>

<template>
  <div class="flex flex-col h-full bg-sidebar overflow-hidden">
    <!-- Header: Fixed Height -->
    <div class="shrink-0 flex items-center justify-between px-4 py-3">
      <h2 class="text-xs font-bold text-muted-foreground/60 uppercase tracking-widest">{{ t("projectTaskTree.projects") }}</h2>
      <div class="flex items-center gap-0.5">
        <Button 
          variant="ghost" 
          size="icon" 
          class="h-7 w-7 text-muted-foreground hover:text-foreground hover:bg-accent/50"
          @click="emit('add-project')"
          :title="t('projectTaskTree.addNewProject')"
        >
          <FolderPlus class="h-4 w-4" />
        </Button>
        <Toggle
          size="sm"
          class="h-7 w-7 p-0 text-muted-foreground hover:text-foreground"
          :class="filterCardOpen ? 'bg-accent/50 text-foreground' : ''"
          data-test-id="shellman-task-filter-toggle"
          :aria-label="t('projectTaskTree.toggleTaskFilters')"
          @click="filterCardOpen = !filterCardOpen"
        >
          <ListFilter class="h-4 w-4" />
        </Toggle>
      </div>
    </div>

    <!-- Scrollable Content: Flex-1 -->
    <div class="flex-1 min-h-0 overflow-hidden">
      <ScrollArea class="h-full">
        <div class="px-2 py-2">
          <div
            v-if="filterCardOpen"
            class="mb-2 rounded-md bg-accent/35 px-2 py-2"
            data-test-id="shellman-task-filter-card"
          >
            <button
              v-for="option in taskFilterOptions"
              :key="option"
              type="button"
              class="flex w-full items-center gap-2 rounded px-1.5 py-1 text-left text-[12px] text-muted-foreground/90 hover:bg-accent/40"
              :data-test-id="`shellman-task-filter-option-${option}`"
              @click="toggleTaskFilter(option)"
            >
              <Checkbox
                class="h-3.5 w-3.5 rounded border-border/70 pointer-events-none"
                :model-value="taskFilterState[option]"
              />
              <span>{{ option }}</span>
            </button>
          </div>

          <Accordion type="multiple" v-model="expandedItems" class="w-full space-y-0.5">
            <AccordionItem
              v-for="project in props.projects"
              :key="project.projectId"
              :value="project.projectId"
              class="border-none"
            >
              <AccordionTrigger 
                class="w-full py-1 hover:no-underline rounded-md px-2 hover:bg-accent/40 group/header transition-all [&[data-state=open]>svg]:rotate-0 [&>svg]:hidden"
              >
                <div class="flex flex-1 items-center justify-between gap-2.5 min-w-0">
                  <div class="flex items-center gap-2.5 min-w-0">
                    <!-- Icon Switch Logic: Folder normally, Chevron on hover -->
                    <div class="relative w-4 h-4 flex items-center justify-center shrink-0">
                      <Folder class="h-3.5 w-3.5 text-muted-foreground/50 group-hover/header:opacity-0 transition-opacity" />
                      <div class="absolute inset-0 flex items-center justify-center opacity-0 group-hover/header:opacity-100 transition-opacity">
                         <ChevronDown v-if="expandedItems.includes(project.projectId)" class="h-3.5 w-3.5 text-muted-foreground/80" />
                         <ChevronRight v-else class="h-3.5 w-3.5 text-muted-foreground/80" />
                      </div>
                    </div>
                    <span class="truncate text-[13px] font-medium text-foreground/80 group-hover/header:text-foreground">
                      {{ project.title }}
                    </span>
                  </div>

                  <div class="flex items-center gap-1 opacity-0 group-hover/header:opacity-100 transition-opacity pr-1" @click.stop>
                    <!-- More Actions Menu -->
                    <DropdownMenu>
                      <DropdownMenuTrigger as-child>
                        <Button variant="ghost" size="icon" class="h-5 w-5 text-muted-foreground/60 hover:text-foreground hover:bg-accent">
                          <MoreHorizontal class="h-3.5 w-3.5" />
                        </Button>
                      </DropdownMenuTrigger>
                      <DropdownMenuContent align="end" class="w-40 bg-popover border-border/50 text-foreground">
                        <DropdownMenuItem @click="emit('edit-project', project.projectId)" class="flex items-center gap-2.5 cursor-pointer py-2">
                          <Pencil class="h-3.5 w-3.5 opacity-70" />
                          <span class="text-sm font-medium">{{ t("projectTaskTree.editName") }}</span>
                        </DropdownMenuItem>
                        <DropdownMenuItem @click="emit('archive-project-done', project.projectId)" class="flex items-center gap-2.5 cursor-pointer py-2">
                          <Archive class="h-3.5 w-3.5 opacity-70" />
                          <span class="text-sm font-medium">{{ t("projectTaskTree.archiveAllDone") }}</span>
                        </DropdownMenuItem>
                        <DropdownMenuItem @click="emit('remove-project', project.projectId)" class="flex items-center gap-2.5 text-destructive focus:text-destructive cursor-pointer py-2">
                          <X class="h-4 w-4" />
                          <span class="text-sm font-medium">{{ t("projectTaskTree.remove") }}</span>
                        </DropdownMenuItem>
                      </DropdownMenuContent>
                    </DropdownMenu>

                    <!-- Start New Thread Button -->
                    <Button
                      variant="ghost"
                      size="icon"
                      class="h-5 w-5 text-muted-foreground/60 hover:text-foreground hover:bg-accent"
                      @click.stop="emit('create-root-pane', project.projectId)"
                      :title="t('projectTaskTree.startNewThreadIn', { title: project.title })"
                      :data-test-id="`shellman-project-root-pane-${project.projectId}`"
                    >
                      <SquarePen class="h-3.5 w-3.5" />
                    </Button>
                  </div>
                </div>
                <!-- Optional: Keep the template just in case, but the CSS handles it -->
                <template #icon />
              </AccordionTrigger>
              
              <AccordionContent class="pb-1 pt-0.5">
                <div class="flex flex-col ml-[16px] space-y-px">
                  <div
                      v-for="row in visibleTaskRows(project.tasks)"
                      :key="row.task.taskId"
                      class="flex items-center justify-between group rounded-md px-2 py-1.5 text-[13px] cursor-pointer hover:bg-accent/30 transition-all"
                      :class="{
                        'bg-accent/50 text-foreground font-medium': selectedTaskId === row.task.taskId,
                        'text-muted-foreground/80': selectedTaskId !== row.task.taskId,
                        'ring-1 ring-border/80 bg-accent/40': dragOverTaskId === row.task.taskId
                      }"
                      :data-test-id="`shellman-task-row-${row.task.taskId}`"
                      :style="{ paddingLeft: `${8 + row.depth * 14}px` }"
                      @click="emit('select-task', row.task.taskId)"
                      @dragover.prevent="onTaskDragOver(row.task.taskId)"
                      @dragleave="onTaskDragLeave(row.task.taskId)"
                      @drop.stop.prevent="onTaskDrop(row.task.taskId)"
                  >
                      <div class="flex items-center min-w-0 flex-1">
                        <span v-if="row.depth > 0" class="h-4 w-px bg-border/70 shrink-0" />
                        <span 
                          class="task-check-slot shrink-0 inline-flex items-center justify-center py-1 px-2 cursor-pointer"
                          @click.stop="onTaskCheckModelUpdate(row.task, !isTaskChecked(row.task))"
                        >
                          <Checkbox
                            class="task-check-input h-3.5 w-3.5 rounded border-border/70 accent-foreground/80 transition-opacity pointer-events-none"
                            :model-value="isTaskChecked(row.task)"
                            :data-checked="isTaskChecked(row.task)"
                            :data-test-id="`shellman-task-check-${row.task.taskId}`"
                            @update:model-value="(checked) => onTaskCheckModelUpdate(row.task, checked)"
                          />
                        </span>
                        <TaskTitleResolver
                          :task-title="row.task.title"
                          :current-command="row.task.currentCommand"
                          data-test-id="shellman-task-row-title"
                          class="truncate flex-1"
                        />
                      </div>
                      
                      <div class="flex items-center gap-2 ml-2">
                        <div>
                          <span
                            :data-test-id="`shellman-task-status-${row.task.taskId}`"
                            :data-status="row.task.runtimeStatus || row.task.status"
                            class="inline-flex"
                          >
                            <LoaderCircle
                              v-if="row.task.runtimeStatus === 'running'"
                              class="h-3.5 w-3.5 text-muted-foreground/60 animate-spin"
                            />
                          </span>

                          <span class="text-[11px] text-muted-foreground/40 tabular-nums">
                            {{ displayTime(row.task) }}
                          </span>
                        </div>

                        <!-- Sibling Action on Hover -->
                        <Button
                            variant="ghost"
                            size="icon"
                            class="h-4 w-4 p-0 opacity-0 group-hover:opacity-100 transition-opacity text-muted-foreground/50 hover:text-foreground"
                            @click.stop="emit('create-child-pane', row.task.taskId)"
                        >
                            <Plus class="h-3 w-3" />
                        </Button>

                        <span :data-test-id="`shellman-task-flag-slot-${row.task.taskId}`">
                          <TaskFlagDot
                            :flag="row.task.flag && !row.task.flagReaded ? row.task.flag : undefined"
                            :flag-desc="row.task.flagDesc"
                            :test-id="`shellman-task-flag-dot-${row.task.taskId}`"
                          />
                        </span>
                      </div>
                  </div>
                  <div v-if="visibleTaskRows(project.tasks).length === 0" class="text-[11px] text-muted-foreground/30 italic px-2 py-2">
                      {{ t("projectTaskTree.noThreads") }}
                  </div>
                </div>
              </AccordionContent>
            </AccordionItem>
          </Accordion>

          <div
            v-if="props.showOrphanSection !== false"
            class="mt-3 border-t border-border/40 pt-2 px-1"
          >
            <button
              type="button"
              class="w-full flex items-center justify-between rounded-md px-2 py-1.5 text-xs uppercase tracking-wide text-muted-foreground/70 hover:bg-accent/40"
              data-test-id="shellman-orphan-toggle"
              @click="orphanOpen = !orphanOpen"
            >
              <span>{{ t("projectTaskTree.otherTmux") }}</span>
              <ChevronDown v-if="orphanOpen" class="h-3.5 w-3.5" />
              <ChevronRight v-else class="h-3.5 w-3.5" />
            </button>

            <div v-if="orphanOpen" class="mt-1 space-y-1">
              <div
                v-for="pane in (props.orphanPanes ?? [])"
                :key="pane.target"
                class="rounded-md border border-border/60 px-2 py-1.5 text-[12px] text-muted-foreground/90 hover:bg-accent/30 cursor-grab active:cursor-grabbing"
                :data-test-id="`shellman-orphan-item-${stableTarget(pane.target)}`"
                draggable="true"
                @dragstart="onOrphanDragStart($event, pane)"
                @dragend="onOrphanDragEnd"
              >
                <div class="truncate">{{ pane.title || pane.target }}</div>
              </div>
              <div
                v-if="(props.orphanPanes ?? []).length === 0"
                class="px-2 py-1 text-[11px] text-muted-foreground/50 italic"
              >
                {{ t("projectTaskTree.noOrphanPanes") }}
              </div>
            </div>
          </div>
        </div>
      </ScrollArea>
    </div>

    <!-- Footer: Fixed Height -->
    <div v-if="!props.hideFooter" class="shrink-0 p-2 bg-sidebar/50">
      <Button 
        variant="ghost" 
        size="sm" 
        class="w-full justify-start px-2 h-9 text-muted-foreground/70 hover:text-foreground hover:bg-accent/50 transition-colors gap-2.5"
        @click="emit('open-settings')"
      >
        <Settings class="h-4 w-4 opacity-70" />
        <span class="text-sm font-medium">{{ t("projectTaskTree.settings") }}</span>
      </Button>
    </div>
  </div>
</template>
