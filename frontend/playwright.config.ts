import { defineConfig } from "@playwright/test";
const baseURL = `http://127.0.0.1:${process.env.EXCHANGE_FRONTEND_PORT ?? 5178}`;
export default defineConfig({
  testDir: "./e2e",
  workers: 1,
  use: {
    baseURL,
    headless: true,
    locale: "zh-CN",
    viewport: { width: 1600, height: 1080 },
  },
  webServer: {
    command: "npm run dev:test",
    url: baseURL,
    reuseExistingServer: false,
    timeout: 120000,
  },
  reporter: "list",
});
