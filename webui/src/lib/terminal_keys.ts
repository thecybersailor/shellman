export type ModifierState = {
  ctrlArmed: boolean;
  altArmed: boolean;
};

export type VirtualKey =
  | "ctrl"
  | "alt"
  | "esc"
  | "tab"
  | "enter"
  | "backspace"
  | "up"
  | "down"
  | "left"
  | "right";

export type KeyApplyResult = {
  text: string;
  state: ModifierState;
};

const EMPTY_MODIFIERS: ModifierState = { ctrlArmed: false, altArmed: false };

function clearModifiers(): ModifierState {
  return { ...EMPTY_MODIFIERS };
}

function modifierNumber(state: ModifierState) {
  if (state.ctrlArmed && state.altArmed) {
    return 7;
  }
  if (state.ctrlArmed) {
    return 5;
  }
  if (state.altArmed) {
    return 3;
  }
  return 1;
}

function arrowSuffix(key: string) {
  if (key === "ArrowUp") {
    return "A";
  }
  if (key === "ArrowDown") {
    return "B";
  }
  if (key === "ArrowRight") {
    return "C";
  }
  if (key === "ArrowLeft") {
    return "D";
  }
  return "";
}

function toCtrlChar(key: string) {
  if (key.length !== 1) {
    return null;
  }
  const ch = key.toUpperCase();
  if (ch >= "A" && ch <= "Z") {
    return String.fromCharCode(ch.charCodeAt(0) - 64);
  }
  if (ch === " " || ch === "@" || ch === "2") {
    return "\u0000";
  }
  if (ch === "[") {
    return "\u001b";
  }
  if (ch === "\\") {
    return "\u001c";
  }
  if (ch === "]") {
    return "\u001d";
  }
  if (ch === "^") {
    return "\u001e";
  }
  if (ch === "_") {
    return "\u001f";
  }
  if (ch === "?") {
    return "\u007f";
  }
  return null;
}

function resolveText(key: string, state: ModifierState) {
  const arrow = arrowSuffix(key);
  if (arrow) {
    const m = modifierNumber(state);
    return m === 1 ? `\u001b[${arrow}` : `\u001b[1;${m}${arrow}`;
  }

  let text = "";
  if (key === "Escape") {
    text = "\u001b";
  } else if (key === "Tab") {
    text = "\t";
  } else if (key === "Enter") {
    text = "\r";
  } else if (key === "Backspace") {
    text = "\u007f";
  } else if (key.length === 1) {
    text = key;
  } else {
    return null;
  }

  if (state.ctrlArmed && key.length === 1) {
    const ctrlText = toCtrlChar(key);
    if (ctrlText !== null) {
      text = ctrlText;
    }
  }

  if (state.altArmed) {
    text = `\u001b${text}`;
  }

  return text;
}

function virtualKeyToPhysicalKey(key: Exclude<VirtualKey, "ctrl" | "alt">) {
  if (key === "esc") {
    return "Escape";
  }
  if (key === "tab") {
    return "Tab";
  }
  if (key === "enter") {
    return "Enter";
  }
  if (key === "backspace") {
    return "Backspace";
  }
  if (key === "up") {
    return "ArrowUp";
  }
  if (key === "down") {
    return "ArrowDown";
  }
  if (key === "left") {
    return "ArrowLeft";
  }
  return "ArrowRight";
}

export function createModifierState(): ModifierState {
  return { ...EMPTY_MODIFIERS };
}

export function applyVirtualKey(key: VirtualKey, state: ModifierState): KeyApplyResult {
  if (key === "ctrl") {
    return {
      text: "",
      state: { ...state, ctrlArmed: !state.ctrlArmed }
    };
  }

  if (key === "alt") {
    return {
      text: "",
      state: { ...state, altArmed: !state.altArmed }
    };
  }

  const text = resolveText(virtualKeyToPhysicalKey(key), state);
  return {
    text: text ?? "",
    state: clearModifiers()
  };
}

export function applyPhysicalKey(key: string, state: ModifierState): KeyApplyResult | null {
  if (!state.ctrlArmed && !state.altArmed) {
    return null;
  }

  const text = resolveText(key, state);
  if (text === null) {
    return null;
  }

  return {
    text,
    state: clearModifiers()
  };
}
