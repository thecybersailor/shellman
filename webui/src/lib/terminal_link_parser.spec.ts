import { describe, expect, it } from "vitest";
import { parseTerminalLinks } from "./terminal_link_parser";

describe("parseTerminalLinks", () => {
  it("extracts https url", () => {
    const links = parseTerminalLinks("open https://example.com/docs now");
    expect(links[0]).toMatchObject({ type: "url", text: "https://example.com/docs" });
  });

  it("extracts relative file path with line/col", () => {
    const links = parseTerminalLinks("src/App.vue:120:9");
    expect(links[0]).toMatchObject({ type: "path", path: "src/App.vue", line: 120, col: 9 });
  });

  it("extracts absolute file path", () => {
    const links = parseTerminalLinks("/Users/wanglei/repo/a.ts:7");
    expect(links[0]).toMatchObject({ type: "path", path: "/Users/wanglei/repo/a.ts", line: 7, col: null });
  });

  it("caps parsing count for very long line", () => {
    const links = parseTerminalLinks(`${"x ".repeat(5000)}https://a.com`);
    expect(Array.isArray(links)).toBe(true);
  });
});
