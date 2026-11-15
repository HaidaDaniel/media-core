// Task 0045 E2E suite configuration: one worker against one browser-facing
// origin, because the seeded library state is shared by every test. The
// bootstrap (Go binary build, web bundle build, API process, origin proxy
// and seed) lives in the spec's beforeAll so the whole suite fits in the
// two task-allowed web files. Run artifacts go to the temp dir, never into
// the repo tree.
import { defineConfig } from "@playwright/test";
import { tmpdir } from "node:os";
import path from "node:path";

export default defineConfig({
  testDir: "./e2e",
  outputDir: path.join(tmpdir(), "media-core-e2e-results"),
  timeout: 60_000,
  workers: 1,
  retries: 0,
  reporter: "list",
  use: {
    baseURL: "http://127.0.0.1:8787",
    headless: true,
  },
});
