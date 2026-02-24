import { describe, expect, it } from "vitest";
import { resolvePathLinkInProject } from "./terminal_path_resolver";

describe("resolvePathLinkInProject", () => {
  const root = "/Users/wanglei/Projects/cybersailor/shellman-project";

  it("resolves relative path into project root", () => {
    const r = resolvePathLinkInProject("shellman/webui/src/App.vue:10:2", root);
    expect(r).toMatchObject({ safePath: "shellman/webui/src/App.vue", line: 10, col: 2 });
  });

  it("accepts absolute path inside project", () => {
    const r = resolvePathLinkInProject(`${root}/README.md:3`, root);
    expect(r?.safePath).toBe("README.md");
  });

  it("rejects outside-project absolute path", () => {
    const r = resolvePathLinkInProject("/etc/hosts:1", root);
    expect(r).toBeNull();
  });

  it("rejects traversal path", () => {
    const r = resolvePathLinkInProject("../../secret.txt:1", root);
    expect(r).toBeNull();
  });
});
