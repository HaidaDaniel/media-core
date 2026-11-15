// Task 0045 E2E suite configuration: one worker against one browser-facing
// origin, because the seeded library state is shared by every test. The
// bootstrap (Go binary build, web bundle build, API process, origin proxy
// and seed) lives in the spec's beforeAll so the whole suite fits in the
// two task-allowed web files.
import { defineConfig } from "@playwright/test";

export default defineConfig({
  testDir: "./e2e",
  timeout: 60_000,
  workers: 1,
  retries: 0,
  reporter: "list",
  use: {
    baseURL: "http://127.0.0.1:8787",
    headless: true,
  },
});
