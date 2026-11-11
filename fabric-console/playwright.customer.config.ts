import { defineConfig } from "@playwright/test";
import base from "./playwright.config";

export default defineConfig({
  ...base,
  testMatch: "customer-pilot.spec.ts",
  use: { ...base.use, baseURL: "http://127.0.0.1:5199" },
  webServer: {
    command: "npm run dev -- --port 5199 --strictPort",
    url: "http://127.0.0.1:5199",
    reuseExistingServer: false,
    env: {
      VITE_APP_ENV: "local-development",
      VITE_LOCAL_AUTH_ENABLED: "true",
    },
  },
});
