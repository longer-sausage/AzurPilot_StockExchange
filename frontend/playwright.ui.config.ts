import { defineConfig } from "@playwright/test";
const port = process.env.EXCHANGE_FRONTEND_PORT ?? "5178";
export default defineConfig({
  testDir: "./ui-tests",
  testMatch: "*.spec.ts",
  workers: 1,
  use: {
    baseURL: `http://127.0.0.1:${port}`,
    headless: true,
    locale: "zh-CN",
    viewport: { width: 1280, height: 1000 },
  },
  webServer: {
    command: "npm run preview:demo",
    url: `http://127.0.0.1:${port}`,
    reuseExistingServer: false,
    timeout: 30000,
  },
  reporter: "list",
});
