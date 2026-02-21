<script setup lang="ts">
import { ref, watch } from "vue";
import { useI18n } from "vue-i18n";
import ProjectDirectoryPicker from "./ProjectDirectoryPicker.vue";
import ResponsiveOverlay from "./ResponsiveOverlay.vue";
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
  (event: "update:show", value: boolean): void;
  (event: "select-directory", path: string): void;
}>();

const internalOpen = ref(props.show);

watch(
  () => props.show,
  (val) => {
    internalOpen.value = val;
  }
);

watch(internalOpen, (val) => {
  emit("update:show", val);
});

function onSelectDirectory(path: string) {
  emit("select-directory", path);
  internalOpen.value = false;
}
</script>

<template>
  <ResponsiveOverlay
    v-model:open="internalOpen"
    :title="t('projectDirectoryPicker.title')"
    :description="t('projectDirectoryPicker.description')"
    dialog-content-class="z-[120] sm:max-w-[720px]"
    sheet-side="bottom"
    sheet-content-class="z-[120] h-[85vh] flex flex-col p-6"
  >
    <div class="flex-1 overflow-y-auto">
      <ProjectDirectoryPicker
        :show="internalOpen"
        :list-directories="props.listDirectories"
        :resolve-directory="props.resolveDirectory"
        :search-directories="props.searchDirectories"
        :get-directory-history="props.getDirectoryHistory"
        :record-directory-history="props.recordDirectoryHistory"
        @select-directory="onSelectDirectory"
        @update:show="(v) => (internalOpen = v)"
      />
    </div>
  </ResponsiveOverlay>
</template>
