<script setup lang="ts">
import { Select } from "@/components/ui/select";
import SidecarModeSelectContent from "@/components/SidecarModeSelectContent.vue";
import SidecarModeSelectTrigger from "@/components/SidecarModeSelectTrigger.vue";

const props = withDefaults(defineProps<{
  modelValue: "advisor" | "observer" | "autopilot";
  forceOpen?: boolean;
  triggerClass?: string;
  contentClass?: string;
  contentPortal?: boolean;
  contentSide?: "top" | "right" | "bottom" | "left";
  contentAlign?: "start" | "center" | "end";
  contentSideOffset?: number;
  contentAvoidCollisions?: boolean;
  triggerTestId?: string;
  advisorOptionTestId?: string;
  observerOptionTestId?: string;
  autopilotOptionTestId?: string;
}>(), {
  triggerClass: "h-7 min-w-[132px] text-[11px] border-0 bg-transparent! gap-1.5 py-0! cursor-pointer",
  contentClass: "z-[140]",
  contentPortal: true,
  triggerTestId: "shellman-shellman-sidecar-mode-trigger",
  advisorOptionTestId: "shellman-shellman-sidecar-mode-option-advisor",
  observerOptionTestId: "shellman-shellman-sidecar-mode-option-observer",
  autopilotOptionTestId: "shellman-shellman-sidecar-mode-option-autopilot"
});

const emit = defineEmits<{
  (e: "update:modelValue", mode: "advisor" | "observer" | "autopilot"): void;
}>();

function onModeUpdate(next: unknown) {
  if (next === "advisor" || next === "observer" || next === "autopilot") {
    emit("update:modelValue", next);
  }
}

function onOpenUpdate(_next: boolean) {
  // In force-open mode we intentionally keep the dropdown always expanded.
}
</script>

<template>
  <Select
    :model-value="props.modelValue"
    :open="props.forceOpen ? true : undefined"
    @update:model-value="onModeUpdate"
    @update:open="onOpenUpdate"
  >
    <SidecarModeSelectTrigger :mode="props.modelValue" :class="props.triggerClass" :test-id="props.triggerTestId" />
    <SidecarModeSelectContent
      :class="props.contentClass"
      :portal="props.contentPortal"
      :body-lock="false"
      :side="props.contentSide"
      :align="props.contentAlign"
      :side-offset="props.contentSideOffset"
      :avoid-collisions="props.contentAvoidCollisions"
      :advisor-option-test-id="props.advisorOptionTestId"
      :observer-option-test-id="props.observerOptionTestId"
      :autopilot-option-test-id="props.autopilotOptionTestId"
    />
  </Select>
</template>
