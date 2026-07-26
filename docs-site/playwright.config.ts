import { defineConfig } from "@playwright/test";

export default defineConfig({
  testDir: ".",
  testMatch: "browser.spec.ts",
  fullyParallel: false,
  forbidOnly: true,
  retries: 0,
  workers: 1,
  reporter: "line",
  use: {
    baseURL: "http://127.0.0.1:45173",
    browserName: "chromium",
    trace: "retain-on-failure",
  },
  webServer: {
    command: "bun run dev --port 45173 --no-watch --no-reload",
    url: "http://127.0.0.1:45173/",
    reuseExistingServer: false,
    timeout: 30_000,
  },
});
