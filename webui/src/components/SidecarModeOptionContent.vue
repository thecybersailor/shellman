<script setup lang="ts">
import { Eye, MessageCircle, Plane } from "lucide-vue-next";
import { computed } from "vue";
import { useI18n } from "vue-i18n";

const props = defineProps<{
  mode: "advisor" | "observer" | "autopilot";
  label?: string;
  description?: string;
  titleClass?: string;
  descriptionClass?: string;
  iconClass?: string;
  indentClass?: string;
}>();

const { t } = useI18n();
const resolvedLabel = computed(() => {
  if (props.label) {
    return props.label;
  }
  if (props.mode === "advisor") {
    return t("thread.sidecarModeAdvisor");
  }
  if (props.mode === "observer") {
    return t("thread.sidecarModeObserver");
  }
  return t("thread.sidecarModeAutopilot");
});

const resolvedDescription = computed(() => {
  if (props.description) {
    return props.description;
  }
  if (props.mode === "advisor") {
    return t("thread.sidecarModeAdvisorDescription");
  }
  if (props.mode === "observer") {
    return t("thread.sidecarModeObserverDescription");
  }
  return t("thread.sidecarModeAutopilotDescription");
});
</script>

<template>
  <div>
    <div :class="props.titleClass || 'flex items-center gap-2 mb-1'">
      <MessageCircle v-if="props.mode === 'advisor'" :class="props.iconClass || 'size-3.5 shrink-0'" />
      <Eye v-else-if="props.mode === 'observer'" :class="props.iconClass || 'size-3.5 shrink-0'" />
      <Plane v-else :class="props.iconClass || 'size-3.5 shrink-0'" />
      {{ resolvedLabel }}
    </div>
    <div :class="props.descriptionClass || `${props.indentClass || 'ml-5'} text-xs text-muted-foreground/70`">
      {{ resolvedDescription }}
    </div>
  </div>
</template>
