<script setup lang="ts">
import type { RadioGroupItemEmits, RadioGroupItemProps } from "reka-ui"
import type { HTMLAttributes } from "vue"
import { reactiveOmit } from "@vueuse/core"
import { RadioGroupItem, useForwardPropsEmits } from "reka-ui"
import { cn } from "@/lib/utils"

const props = defineProps<RadioGroupItemProps & { class?: HTMLAttributes["class"] }>()
const emits = defineEmits<RadioGroupItemEmits>()

const delegatedProps = reactiveOmit(props, "class")

const forwarded = useForwardPropsEmits(delegatedProps, emits)
</script>

<template>
  <RadioGroupItem
    data-slot="radio-group-item"
    v-bind="forwarded"
    :class="cn(
      'relative h-4 w-4 rounded-full border border-input text-muted-foreground transition-all focus-visible:outline-hidden focus-visible:ring-2 focus-visible:ring-ring focus-visible:ring-offset-2 focus-visible:border-ring data-[state=checked]:border-primary data-[state=checked]:text-primary disabled:cursor-not-allowed disabled:opacity-50',
      props.class,
    )"
  >
    <slot />
  </RadioGroupItem>
</template>

