import { describe, expect, it } from "vitest";
import {
  applyPhysicalKey,
  applyVirtualKey,
  createModifierState,
  type ModifierState
} from "./terminal_keys";

describe("terminal key mapping", () => {
  it("arms ctrl as stateful modifier", () => {
    const result = applyVirtualKey("ctrl", createModifierState());
    expect(result.text).toBe("");
    expect(result.state.ctrlArmed).toBe(true);
  });

  it("consumes armed ctrl with next physical character key", () => {
    const armed: ModifierState = { ctrlArmed: true, altArmed: false };
    const result = applyPhysicalKey("a", armed);
    expect(result).not.toBeNull();
    expect(result?.text).toBe("\u0001");
    expect(result?.state).toEqual({ ctrlArmed: false, altArmed: false });
  });

  it("maps ctrl + arrow to CSI modifier sequence", () => {
    const armed: ModifierState = { ctrlArmed: true, altArmed: false };
    const result = applyPhysicalKey("ArrowUp", armed);
    expect(result?.text).toBe("\u001b[1;5A");
    expect(result?.state).toEqual({ ctrlArmed: false, altArmed: false });
  });

  it("maps ctrl + space to NUL", () => {
    const armed: ModifierState = { ctrlArmed: true, altArmed: false };
    const result = applyPhysicalKey(" ", armed);
    expect(result?.text).toBe("\u0000");
    expect(result?.state).toEqual({ ctrlArmed: false, altArmed: false });
  });

  it("maps alt + character to ESC-prefixed text", () => {
    const armed: ModifierState = { ctrlArmed: false, altArmed: true };
    const result = applyPhysicalKey("x", armed);
    expect(result?.text).toBe("\u001bx");
    expect(result?.state).toEqual({ ctrlArmed: false, altArmed: false });
  });

  it("maps ctrl+alt + arrow to CSI 1;7 sequence", () => {
    const armed: ModifierState = { ctrlArmed: true, altArmed: true };
    const result = applyPhysicalKey("ArrowDown", armed);
    expect(result?.text).toBe("\u001b[1;7B");
    expect(result?.state).toEqual({ ctrlArmed: false, altArmed: false });
  });
});
