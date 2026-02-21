import { defineConfig } from "@playwright/test";

export default defineConfig({
  testDir: "./e2e",
  outputDir: "../logs/playwright-artifacts",
  use: {
    testIdAttribute: "data-test-id",
    video: "on",
    screenshot: "only-on-failure",
    trace: "retain-on-failure"
  }
});
