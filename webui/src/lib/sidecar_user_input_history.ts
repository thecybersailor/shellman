export const SIDECAR_USER_INPUT_HISTORY_KEY = "shellman.sidecar-chat.user-history.v1";
export const SIDECAR_USER_INPUT_HISTORY_LIMIT = 200;

export function loadSidecarUserInputHistory(): string[] {
  if (typeof localStorage === "undefined") {
    return [];
  }
  const raw = localStorage.getItem(SIDECAR_USER_INPUT_HISTORY_KEY);
  if (!raw) {
    return [];
  }
  try {
    const parsed = JSON.parse(raw) as unknown;
    if (!Array.isArray(parsed)) {
      return [];
    }
    return parsed.filter((item): item is string => typeof item === "string");
  } catch {
    return [];
  }
}

export function appendSidecarUserInputHistory(content: string): string[] {
  if (typeof localStorage === "undefined") {
    return [];
  }
  const text = String(content ?? "").trim();
  if (!text) {
    return loadSidecarUserInputHistory();
  }
  const next = [...loadSidecarUserInputHistory(), text].slice(-SIDECAR_USER_INPUT_HISTORY_LIMIT);
  localStorage.setItem(SIDECAR_USER_INPUT_HISTORY_KEY, JSON.stringify(next));
  return next;
}
