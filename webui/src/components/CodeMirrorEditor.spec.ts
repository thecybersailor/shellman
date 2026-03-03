import { describe, expect, it } from "vitest";
import source from "./CodeMirrorEditor.vue?raw";

describe("CodeMirrorEditor language mapping", () => {
  it("accepts filePath prop for language detection", () => {
    expect(source).toContain("filePath?: string");
    expect(source).toContain("languageCompartment.of(resolveLanguage(props.filePath))");
    expect(source).toContain("githubDark");
  });

  it("covers common language extensions", () => {
    expect(source).toContain("\\.(ts|tsx|js|jsx|mjs|cjs)$");
    expect(source).toContain("\\.(json|jsonc|json5)$");
    expect(source).toContain("\\.(md|markdown)$");
    expect(source).toContain("\\.(css|scss|sass|less)$");
    expect(source).toContain("\\.(html|htm|vue|svelte)$");
    expect(source).toContain("\\.(xml|svg|plist)$");
    expect(source).toContain("\\.(yml|yaml)$");
    expect(source).toContain("\\.(py|pyw|pyi)$");
    expect(source).toContain("\\.(java)$");
    expect(source).toContain("\\.(c|h|cc|cxx|cpp|hpp|hh|hxx)$");
    expect(source).toContain("\\.(go)$");
    expect(source).toContain("\\.(rs)$");
    expect(source).toContain("\\.(php|phtml)$");
    expect(source).toContain("\\.(sql)$");
    expect(source).toContain("\\.(sh|bash|zsh|ksh|fish|env)$");
  });
});
