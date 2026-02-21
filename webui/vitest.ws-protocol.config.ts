import { defineConfig } from "vitest/config";

export default defineConfig({
  test: {
    environment: "node",
    include: ["e2e-ws/**/*.spec.ts"],
    testTimeout: 120000
  }
});

