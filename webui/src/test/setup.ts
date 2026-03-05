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

if (
  typeof globalThis.localStorage === "undefined" ||
  typeof globalThis.localStorage.getItem !== "function" ||
  typeof globalThis.localStorage.setItem !== "function" ||
  typeof globalThis.localStorage.removeItem !== "function" ||
  typeof globalThis.localStorage.clear !== "function"
) {
  const store = new Map<string, string>();
  vi.stubGlobal("localStorage", {
    getItem(key: string) {
      const normalized = String(key);
      return store.has(normalized) ? String(store.get(normalized)) : null;
    },
    setItem(key: string, value: string) {
      store.set(String(key), String(value));
    },
    removeItem(key: string) {
      store.delete(String(key));
    },
    clear() {
      store.clear();
    }
  });
}
