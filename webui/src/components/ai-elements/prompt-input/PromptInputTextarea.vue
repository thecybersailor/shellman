<script setup lang="ts">
import type { HTMLAttributes } from 'vue'
import { InputGroupTextarea } from '@/components/ui/input-group'
import { cn } from '@/lib/utils'
import { computed, ref } from 'vue'
import { usePromptInput } from './context'
import { loadSidecarUserInputHistory } from '@/lib/sidecar_user_input_history'

type PromptInputTextareaProps = InstanceType<typeof InputGroupTextarea>['$props']

interface Props extends /* @vue-ignore */ PromptInputTextareaProps {
  class?: HTMLAttributes['class']
  historyEnabled?: boolean
}

const props = defineProps<Props>()

const { textInput, setTextInput, submitForm, addFiles, files, removeFile } = usePromptInput()
const isComposing = ref(false)
const historyCursor = ref<number | null>(null)
const historyDraft = ref("")

function isAtFirstLine(el: HTMLTextAreaElement): boolean {
  const start = Number(el.selectionStart ?? 0)
  const end = Number(el.selectionEnd ?? 0)
  if (start !== end) {
    return false
  }
  return !el.value.slice(0, start).includes('\n')
}

function applyHistoryByCursor(cursor: number, history: string[]) {
  const index = history.length - 1 - cursor
  setTextInput(history[index] ?? "")
}

function handleHistoryKeyDown(e: KeyboardEvent): boolean {
  if (!props.historyEnabled || isComposing.value) {
    return false
  }
  if (e.shiftKey || e.altKey || e.metaKey || e.ctrlKey) {
    return false
  }
  if (e.key !== 'ArrowUp' && e.key !== 'ArrowDown') {
    return false
  }
  const el = e.target as HTMLTextAreaElement | null
  if (!el) {
    return false
  }
  const history = loadSidecarUserInputHistory()
  if (history.length === 0) {
    return false
  }

  if (historyCursor.value === null) {
    if (e.key !== 'ArrowUp' || !isAtFirstLine(el)) {
      return false
    }
    historyDraft.value = textInput.value
    historyCursor.value = 0
    e.preventDefault()
    applyHistoryByCursor(0, history)
    return true
  }

  e.preventDefault()
  if (e.key === 'ArrowUp') {
    historyCursor.value = Math.min(historyCursor.value + 1, history.length - 1)
    applyHistoryByCursor(historyCursor.value, history)
    return true
  }

  if (historyCursor.value === 0) {
    historyCursor.value = null
    setTextInput(historyDraft.value)
    return true
  }

  historyCursor.value -= 1
  applyHistoryByCursor(historyCursor.value, history)
  return true
}

function handleKeyDown(e: KeyboardEvent) {
  if (handleHistoryKeyDown(e)) {
    return
  }

  if (e.key === 'Enter') {
    if (isComposing.value || e.shiftKey)
      return
    e.preventDefault()
    historyCursor.value = null
    submitForm()
  }

  // Remove last attachment on backspace if input is empty
  if (e.key === 'Backspace' && textInput.value === '' && files.value.length > 0) {
    const lastFile = files.value[files.value.length - 1]
    if (lastFile) {
      removeFile(lastFile.id)
    }
  }
}

function handlePaste(e: ClipboardEvent) {
  if (historyCursor.value !== null) {
    historyCursor.value = null
  }
  const items = e.clipboardData?.items
  if (!items)
    return

  const pastedFiles: File[] = []
  for (const item of Array.from(items)) {
    if (item.kind === 'file') {
      const file = item.getAsFile()
      if (file)
        pastedFiles.push(file)
    }
  }

  if (pastedFiles.length > 0) {
    e.preventDefault()
    addFiles(pastedFiles)
  }
}

const modelValue = computed({
  get: () => textInput.value,
  set: val => setTextInput(val),
})
</script>

<template>
  <InputGroupTextarea
    v-model="modelValue"
    placeholder="What would you like to know?"
    name="message"
    :class="cn('field-sizing-content max-h-48 min-h-16', props.class)"
    v-bind="props"
    @keydown="handleKeyDown"
    @paste="handlePaste"
    @compositionstart="isComposing = true"
    @compositionend="isComposing = false"
  />
</template>
