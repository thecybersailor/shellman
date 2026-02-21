import { describe, expect, it } from "vitest";
import mainSource from "./main.ts?raw";

describe("main entry", () => {
  it("loads xterm base css", () => {
    expect(mainSource).toContain('import "@xterm/xterm/css/xterm.css";');
  });
});
