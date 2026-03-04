import { config } from "@vue/test-utils";
import i18n from "@/i18n";
import { vi } from "vitest";

const existingPlugins = Array.isArray(config.global.plugins) ? config.global.plugins : [];

if (!existingPlugins.includes(i18n)) {
  config.global.plugins = [...existingPlugins, i18n];
}

if (typeof globalThis.ResizeObserver === "undefined") {
  vi.stubGlobal(
    "ResizeObserver",
    class {
      observe() {}
      unobserve() {}
      disconnect() {}
    }
  );
}
