<script setup lang="ts">
import { computed } from "vue";
import { useI18n } from "vue-i18n";
import { Eye, MessageCircle, Plane } from "lucide-vue-next";
import { SelectTrigger } from "@/components/ui/select";

const props = withDefaults(defineProps<{
  mode: "advisor" | "observer" | "autopilot";
  class?: string;
  testId?: string;
}>(), {
  class: "h-7 min-w-[132px] text-[11px] border-0 bg-transparent! gap-1.5 py-0! cursor-pointer",
  testId: "shellman-shellman-sidecar-mode-trigger"
});

const { t } = useI18n();

const triggerLabel = computed(() => {
  if (props.mode === "advisor") {
    return t("thread.sidecarModeAdvisor");
  }
  if (props.mode === "observer") {
    return t("thread.sidecarModeObserver");
  }
  return t("thread.sidecarModeAutopilot");
});
</script>

<template>
  <SelectTrigger :data-test-id="props.testId" :class="props.class">
    <MessageCircle v-if="props.mode === 'advisor'" class="size-3.5 shrink-0 text-muted-foreground" />
    <Eye v-else-if="props.mode === 'observer'" class="size-3.5 shrink-0 text-muted-foreground" />
    <Plane v-else class="size-3.5 shrink-0 text-muted-foreground" />
    <span>{{ triggerLabel }}</span>
  </SelectTrigger>
</template>
