<script setup lang="ts">
import { useMediaQuery } from "@vueuse/core";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import {
  Sheet,
  SheetContent,
  SheetDescription,
  SheetHeader,
  SheetTitle,
} from "@/components/ui/sheet";

const props = withDefaults(
  defineProps<{
    open: boolean;
    title: string;
    description?: string;
    dialogContentClass?: string;
    sheetSide?: "top" | "bottom" | "left" | "right";
    sheetContentClass?: string;
  }>(),
  {
    description: "",
    dialogContentClass: "z-[120] sm:max-w-[720px]",
    sheetSide: "bottom",
    sheetContentClass: "z-[120] h-[85vh] flex flex-col p-6 overflow-y-auto",
  }
);

const emit = defineEmits<{
  (event: "update:open", value: boolean): void;
}>();

const isDesktop = useMediaQuery("(min-width: 768px)");
</script>

<template>
  <template v-if="isDesktop">
    <Dialog :open="props.open" @update:open="(value) => emit('update:open', value)">
      <DialogContent :class="props.dialogContentClass">
        <DialogHeader>
          <DialogTitle>{{ props.title }}</DialogTitle>
          <DialogDescription v-if="props.description">
            {{ props.description }}
          </DialogDescription>
        </DialogHeader>
        <slot />
      </DialogContent>
    </Dialog>
  </template>

  <template v-else>
    <Sheet :open="props.open" @update:open="(value) => emit('update:open', value)">
      <SheetContent :side="props.sheetSide" :class="props.sheetContentClass">
        <SheetHeader class="text-left mb-2">
          <SheetTitle>{{ props.title }}</SheetTitle>
          <SheetDescription v-if="props.description">
            {{ props.description }}
          </SheetDescription>
        </SheetHeader>
        <slot />
      </SheetContent>
    </Sheet>
  </template>
</template>
