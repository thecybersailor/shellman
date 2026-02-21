<script setup lang="ts">
import type { ListboxItemProps } from "reka-ui"
import type { HTMLAttributes } from "vue"
import { CommandItem } from '@/components/ui/command'

const props = defineProps<ListboxItemProps & { class?: HTMLAttributes["class"] }>()
</script>

<template>
  <CommandItem v-bind="props" v-bind="$attrs">
    <slot />
  </CommandItem>
</template>
