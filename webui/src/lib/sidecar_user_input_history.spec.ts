import { beforeEach, describe, expect, it } from "vitest";
import {
  SIDECAR_USER_INPUT_HISTORY_KEY,
  SIDECAR_USER_INPUT_HISTORY_LIMIT,
  appendSidecarUserInputHistory,
  loadSidecarUserInputHistory
} from "./sidecar_user_input_history";

describe("sidecar user input history", () => {
  beforeEach(() => {
    localStorage.clear();
  });

  it("appends duplicates and trims to limit", () => {
    for (let i = 0; i < SIDECAR_USER_INPUT_HISTORY_LIMIT + 5; i += 1) {
      appendSidecarUserInputHistory(`msg-${i}`);
    }

    appendSidecarUserInputHistory("msg-204");
    appendSidecarUserInputHistory("msg-204");

    const items = loadSidecarUserInputHistory();
    expect(items.length).toBe(SIDECAR_USER_INPUT_HISTORY_LIMIT);
    expect(items.at(-1)).toBe("msg-204");
    expect(items.at(-2)).toBe("msg-204");
  });

  it("returns empty list when storage payload is invalid", () => {
    localStorage.setItem(SIDECAR_USER_INPUT_HISTORY_KEY, "{bad-json");
    expect(loadSidecarUserInputHistory()).toEqual([]);
  });
});
