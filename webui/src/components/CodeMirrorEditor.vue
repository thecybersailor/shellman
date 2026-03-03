<script setup lang="ts">
import { Compartment, EditorState } from "@codemirror/state";
import { EditorView, placeholder } from "@codemirror/view";
import { basicSetup } from "codemirror";
import { StreamLanguage } from "@codemirror/language";
import { shell } from "@codemirror/legacy-modes/mode/shell";
import { cpp } from "@codemirror/lang-cpp";
import { css } from "@codemirror/lang-css";
import { go } from "@codemirror/lang-go";
import { html } from "@codemirror/lang-html";
import { javascript } from "@codemirror/lang-javascript";
import { java } from "@codemirror/lang-java";
import { json } from "@codemirror/lang-json";
import { markdown } from "@codemirror/lang-markdown";
import { php } from "@codemirror/lang-php";
import { python } from "@codemirror/lang-python";
import { rust } from "@codemirror/lang-rust";
import { sql } from "@codemirror/lang-sql";
import { xml } from "@codemirror/lang-xml";
import { yaml } from "@codemirror/lang-yaml";
import { onBeforeUnmount, onMounted, ref, watch } from "vue";
import { githubDark } from "@uiw/codemirror-theme-github";

const props = withDefaults(defineProps<{
  modelValue: string;
  filePath?: string;
  placeholder?: string;
  readonly?: boolean;
}>(), {
  filePath: "",
  placeholder: "",
  readonly: false
});

const emit = defineEmits<{
  (event: "update:modelValue", value: string): void;
}>();

const rootEl = ref<HTMLElement | null>(null);
let editorView: EditorView | null = null;
let syncingFromProps = false;
const editableCompartment = new Compartment();
const readOnlyCompartment = new Compartment();
const placeholderCompartment = new Compartment();
const languageCompartment = new Compartment();

function resolveLanguage(path: string) {
  const normalized = String(path ?? "").trim().toLowerCase();

  if (/\.(ts|tsx|js|jsx|mjs|cjs)$/.test(normalized)) {
    return javascript({ typescript: true, jsx: /\.(tsx|jsx)$/.test(normalized) });
  }
  if (/\.(json|jsonc|json5)$/.test(normalized)) {
    return json();
  }
  if (/\.(md|markdown)$/.test(normalized)) {
    return markdown();
  }
  if (/\.(css|scss|sass|less)$/.test(normalized)) {
    return css();
  }
  if (/\.(html|htm|vue|svelte)$/.test(normalized)) {
    return html();
  }
  if (/\.(xml|svg|plist)$/.test(normalized)) {
    return xml();
  }
  if (/\.(yml|yaml)$/.test(normalized)) {
    return yaml();
  }
  if (/\.(py|pyw|pyi)$/.test(normalized)) {
    return python();
  }
  if (/\.(java)$/.test(normalized)) {
    return java();
  }
  if (/\.(c|h|cc|cxx|cpp|hpp|hh|hxx)$/.test(normalized)) {
    return cpp();
  }
  if (/\.(go)$/.test(normalized)) {
    return go();
  }
  if (/\.(rs)$/.test(normalized)) {
    return rust();
  }
  if (/\.(php|phtml)$/.test(normalized)) {
    return php();
  }
  if (/\.(sql)$/.test(normalized)) {
    return sql();
  }
  if (/\.(sh|bash|zsh|ksh|fish|env)$/.test(normalized)) {
    return StreamLanguage.define(shell);
  }

  return [];
}

function updateDoc(nextValue: string) {
  if (!editorView) {
    return;
  }
  const current = editorView.state.doc.toString();
  if (current === nextValue) {
    return;
  }
  editorView.dispatch({
    changes: {
      from: 0,
      to: current.length,
      insert: nextValue
    }
  });
}

function setCursor(line: number, col: number) {
  if (!editorView) {
    return;
  }
  const safeLine = Math.max(1, Math.min(line, editorView.state.doc.lines));
  const lineInfo = editorView.state.doc.line(safeLine);
  const safeCol = Math.max(1, Math.min(col, lineInfo.length + 1));
  const pos = lineInfo.from + safeCol - 1;
  editorView.dispatch({
    selection: { anchor: pos, head: pos },
    scrollIntoView: true
  });
  editorView.focus();
}

onMounted(() => {
  if (!rootEl.value) {
    return;
  }
  editorView = new EditorView({
    parent: rootEl.value,
    state: EditorState.create({
      doc: props.modelValue,
      extensions: [
        basicSetup,
        editableCompartment.of(EditorView.editable.of(!props.readonly)),
        readOnlyCompartment.of(EditorState.readOnly.of(Boolean(props.readonly))),
        placeholderCompartment.of(placeholder(String(props.placeholder ?? ""))),
        languageCompartment.of(resolveLanguage(props.filePath)),
        githubDark,
        EditorView.updateListener.of((update) => {
          if (!update.docChanged || syncingFromProps) {
            return;
          }
          emit("update:modelValue", update.state.doc.toString());
        }),
        EditorView.theme({
          "&": {
            height: "100%"
          },
          ".cm-focused": {
            outline: "none"
          },
          ".cm-scroller": {
            overflow: "auto",
            fontFamily: "ui-monospace, SFMono-Regular, Menlo, Monaco, Consolas, Liberation Mono, monospace",
            fontSize: "12px"
          }
        })
      ]
    })
  });
});

watch(
  () => props.modelValue,
  (nextValue) => {
    if (!editorView) {
      return;
    }
    syncingFromProps = true;
    updateDoc(nextValue);
    syncingFromProps = false;
  }
);

watch(
  () => props.readonly,
  (readonly) => {
    if (!editorView) {
      return;
    }
    editorView.dispatch({
      effects: [
        editableCompartment.reconfigure(EditorView.editable.of(!readonly)),
        readOnlyCompartment.reconfigure(EditorState.readOnly.of(Boolean(readonly)))
      ]
    });
  }
);

watch(
  () => props.placeholder,
  (value) => {
    if (!editorView) {
      return;
    }
    editorView.dispatch({
      effects: placeholderCompartment.reconfigure(placeholder(String(value ?? "")))
    });
  }
);

watch(
  () => props.filePath,
  (path) => {
    if (!editorView) {
      return;
    }
    editorView.dispatch({
      effects: languageCompartment.reconfigure(resolveLanguage(path))
    });
  }
);

onBeforeUnmount(() => {
  editorView?.destroy();
  editorView = null;
});

defineExpose({
  setCursor
});
</script>

<template>
  <div ref="rootEl" class="h-full min-h-[70vh] w-full" />
</template>
