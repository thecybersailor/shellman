<script setup lang="ts">
import type { RadioGroupIndicatorProps } from "reka-ui"
import type { HTMLAttributes } from "vue"
import { reactiveOmit } from "@vueuse/core"
import { RadioGroupIndicator } from "reka-ui"
import { cn } from "@/lib/utils"

const props = defineProps<RadioGroupIndicatorProps & { class?: HTMLAttributes["class"] }>()

const delegatedProps = reactiveOmit(props, "class")
</script>

<template>
  <RadioGroupIndicator
    data-slot="radio-group-indicator"
    v-bind="delegatedProps"
    :class="cn(
      'absolute left-0 top-0 z-10 flex h-full w-full items-center justify-center rounded-full data-[state=checked]:opacity-100 data-[state=unchecked]:opacity-0 transition-opacity',
      props.class,
    )"
  >
    <slot />
  </RadioGroupIndicator>
</template>

