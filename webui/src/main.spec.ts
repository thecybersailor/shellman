import { describe, expect, it } from "vitest";
import mainSource from "./main.ts?raw";

describe("main entry", () => {
  it("loads xterm base css", () => {
    expect(mainSource).toContain('import "@xterm/xterm/css/xterm.css";');
  });

  it("registers i18n plugin", () => {
    expect(mainSource).toContain('import i18n from "./i18n";');
    expect(mainSource).toContain("app.use(i18n);");
  });

  it("waits router ready before mounting app", () => {
    expect(mainSource).toContain("router.isReady()");
  });
});
