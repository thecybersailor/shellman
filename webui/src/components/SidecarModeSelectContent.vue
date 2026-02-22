<script setup lang="ts">
import { SelectContent, SelectItem } from "@/components/ui/select";
import SidecarModeOptionContent from "@/components/SidecarModeOptionContent.vue";

const props = withDefaults(defineProps<{
  class?: string;
  bodyLock?: boolean;
  portal?: boolean;
  side?: "top" | "right" | "bottom" | "left";
  align?: "start" | "center" | "end";
  sideOffset?: number;
  avoidCollisions?: boolean;
  advisorOptionTestId?: string;
  observerOptionTestId?: string;
  autopilotOptionTestId?: string;
}>(), {
  portal: true,
  advisorOptionTestId: "shellman-shellman-sidecar-mode-option-advisor",
  observerOptionTestId: "shellman-shellman-sidecar-mode-option-observer",
  autopilotOptionTestId: "shellman-shellman-sidecar-mode-option-autopilot"
});
</script>

<template>
  <SelectContent
    :class="props.class"
    :body-lock="props.bodyLock"
    :portal="props.portal"
    :side="props.side"
    :align="props.align"
    :side-offset="props.sideOffset"
    :avoid-collisions="props.avoidCollisions"
  >
    <SelectItem :data-test-id="props.advisorOptionTestId" value="advisor">
      <SidecarModeOptionContent mode="advisor" />
    </SelectItem>
    <SelectItem :data-test-id="props.observerOptionTestId" value="observer">
      <SidecarModeOptionContent mode="observer" />
    </SelectItem>
    <SelectItem :data-test-id="props.autopilotOptionTestId" value="autopilot">
      <SidecarModeOptionContent mode="autopilot" />
    </SelectItem>
  </SelectContent>
</template>
