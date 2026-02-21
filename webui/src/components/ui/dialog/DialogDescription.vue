<script setup lang="ts">
import { DialogDescription, type DialogDescriptionProps, useForwardProps } from "reka-ui"
import { computed, type HTMLAttributes } from "vue"
import { cn } from "@/lib/utils"

const props = defineProps<DialogDescriptionProps & { class?: HTMLAttributes["class"] }>()

const forwardedProps = useForwardProps(props)
</script>

<template>
  <DialogDescription
    v-bind="forwardedProps"
    :class="cn('text-sm text-muted-foreground', props.class)"
  >
    <slot />
  </DialogDescription>
</template>
