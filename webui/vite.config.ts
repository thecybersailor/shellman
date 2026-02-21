import path from "node:path";
import { defineConfig } from "vite";
import tailwindcss from "@tailwindcss/vite";
import vue from "@vitejs/plugin-vue";
import { configDefaults } from "vitest/config";

export default defineConfig({
  plugins: [vue(), tailwindcss()],
  server: {
    host: "127.0.0.1",
    port: 15173,
    strictPort: true,
    allowedHosts: ["cli", "webui", "localhost", "127.0.0.1"]
  },
  resolve: {
    alias: {
      "@": path.resolve(__dirname, "./src")
    }
  },
  test: {
    environment: "jsdom",
    exclude: [...configDefaults.exclude, "e2e/**", "e2e-ws/**"]
  }
});
