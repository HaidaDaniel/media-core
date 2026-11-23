// Task 0077 browser regression on an insecure HTTP LAN origin. The suite
// builds the real release binary (embedded React assets), starts it with
// --trusted-lan on a private interface, and drives it directly at
// http://media.home.arpa:<port> with no proxy or Node server. Chromium
// resolves the home.arpa name to the discovered LAN IP via a browser-only
// host-resolver rule; no system DNS or router configuration is changed.
import { test, expect, type Page } from "@playwright/test";
import { execFileSync, spawn, type ChildProcess } from "node:child_process";
import { mkdtempSync, readFileSync, rmSync } from "node:fs";
import { networkInterfaces, tmpdir } from "node:os";
import path from "node:path";
import { fileURLToPath } from "node:url";

const here = path.dirname(fileURLToPath(import.meta.url));
const webDir = path.resolve(here, "..");
const repoRoot = path.resolve(webDir, "..");
const port = 18788;
const lanHost = "media.home.arpa";

function privateIPv4(): string | undefined {
  for (const addresses of Object.values(networkInterfaces())) {
    for (const address of addresses ?? []) {
      if (address.family !== "IPv4" || address.internal) continue;
      const parts = address.address.split(".").map(Number);
      const rfc1918 =
        parts[0] === 10 ||
        (parts[0] === 172 && parts[1] >= 16 && parts[1] <= 31) ||
        (parts[0] === 192 && parts[1] === 168);
      if (rfc1918) return address.address;
    }
  }
  return undefined;
}

const lanIP = privateIPv4();
const origin = `http://${lanHost}:${port}`;

test.use({
  baseURL: origin,
  launchOptions: {
    args: lanIP ? [`--host-resolver-rules=MAP ${lanHost} ${lanIP}`] : [],
  },
});

let workDir: string;
let server: ChildProcess | undefined;
let ownerSecret = "";
let seededTitle = "";
let seededWorkID = "";
let pageErrors: string[] = [];

async function waitForHTTP(url: string, timeoutMs: number): Promise<void> {
  const deadline = Date.now() + timeoutMs;
  for (;;) {
    try {
      const res = await fetch(url);
      void (await res.arrayBuffer());
      return;
    } catch {
      if (Date.now() > deadline) throw new Error(`timed out waiting for ${url}`);
      await new Promise((resolve) => setTimeout(resolve, 200));
    }
  }
}

async function api(pathname: string, body: unknown, cookie?: string, csrf?: string): Promise<{ status: number; data: Record<string, any> }> {
  const headers: Record<string, string> = {
    "content-type": "application/json",
    origin: `http://${lanIP}:${port}`,
  };
  if (cookie) headers.cookie = cookie;
  if (csrf) headers["x-csrf-token"] = csrf;
  const res = await fetch(`http://${lanIP}:${port}${pathname}`, {
    method: "POST",
    headers,
    body: JSON.stringify(body),
  });
  const parsed = (await res.json()) as { ok: boolean; data: Record<string, any> };
  return { status: res.status, data: parsed.data ?? {} };
}

test.beforeAll(async () => {
  test.skip(!lanIP, "no RFC1918 IPv4 interface available; HTTP LAN browser context is unverified here");
  test.setTimeout(360_000);
  workDir = mkdtempSync(path.join(tmpdir(), "media-core-lan-e2e-"));

  // The release build embeds the real React assets, so the Go binary can
  // serve the UI directly; no origin proxy is used anywhere in this suite.
  execFileSync("bash", ["scripts/build-release.sh"], {
    cwd: repoRoot,
    stdio: "pipe",
    timeout: 300_000,
  });
  const binary = path.join(repoRoot, "release", process.arch === "arm64" ? "media-core-linux-arm64" : "media-core-linux-amd64");
  const stateDir = path.join(workDir, "state");
  execFileSync(binary, ["init", "--state-dir", stateDir], { stdio: "pipe" });
  execFileSync(binary, ["owner", "generate", "--state-dir", stateDir], { stdio: "pipe" });
  ownerSecret = readFileSync(path.join(stateDir, "owner-secret")).toString("hex");

  server = spawn(binary, [
    "serve", "--state-dir", stateDir, "--trusted-lan",
    "--listen", `${lanIP}:${port}`, "--lan-host", lanHost,
  ], { cwd: workDir, stdio: ["ignore", "pipe", "pipe"] });
  await waitForHTTP(`http://${lanIP}:${port}/`, 30_000);

  // Seed one work through the IP spelling so browser tests have a
  // deterministic library entry and SPA route to refresh.
  const login = await fetch(`http://${lanIP}:${port}/api/v1/session`, {
    method: "POST",
    headers: {
      "content-type": "application/json",
      origin: `http://${lanIP}:${port}`,
    },
    body: JSON.stringify({ owner_secret: ownerSecret }),
  });
  const loginBody = (await login.json()) as { data: { csrf_token: string } };
  const cookie = (login.headers.getSetCookie()[0] ?? "").split(";")[0];
  seededTitle = `LAN fixture ${Date.now()}`;
  const created = await api("/api/v1/works", {
    request_id: "lan-e2e-seed",
    titles: [{ text: seededTitle, language: "en", kind: "original", preferred: true, provenance: "manual" }],
    kind: "book",
    language: "en",
  }, cookie, loginBody.data.csrf_token);
  seededWorkID = created.data.work_id as string;
  if (!seededWorkID) throw new Error(`seed failed: ${JSON.stringify(created)}`);
}, 360_000);

test.afterAll(async () => {
  if (server && server.exitCode === null) {
    await new Promise<void>((resolve) => {
      const timer = setTimeout(resolve, 5_000);
      timer.unref();
      server?.once("exit", () => resolve());
      server?.kill("SIGTERM");
    });
  }
  if (workDir) rmSync(workDir, { recursive: true, force: true });
});

test.beforeEach(async ({ page }) => {
  pageErrors = [];
  page.on("pageerror", (error) => pageErrors.push(error.message));
  // Force the insecure-context condition even on browsers that still
  // expose the API: the UI must never depend on crypto.randomUUID.
  await page.addInitScript(() => {
    Object.defineProperty(globalThis.crypto, "randomUUID", { value: undefined, configurable: true });
  });
});

test.afterEach(() => {
  expect(pageErrors).toEqual([]);
});

async function signIn(page: Page): Promise<void> {
  await page.goto("/login");
  await page.getByLabel("Owner secret").fill(ownerSecret);
  await page.getByRole("button", { name: "Sign in" }).click();
  await page.getByRole("navigation", { name: "Main" }).waitFor();
}

test("HTTP LAN is not a secure context and Quick Add still works", async ({ page }) => {
  await page.goto("/");
  expect(await page.evaluate(() => isSecureContext)).toBe(false);
  expect(await page.evaluate(() => typeof crypto.randomUUID)).toBe("undefined");

  await signIn(page);
  await page.getByRole("navigation", { name: "Main" }).getByRole("link", { name: "Quick add" }).click();
  const title = `LAN quick add ${Date.now()}`;
  await page.getByPlaceholder("The title you want to add").fill(title);
  await page.getByLabel("Medium (optional)").selectOption("book");
  await page.getByRole("button", { name: "Add", exact: true }).click();
  await page.waitForURL(/\/editions\//);
  await expect(page.getByRole("heading", { level: 2, name: title })).toBeVisible();
  await expect(page.getByText("Status: not started")).toBeVisible();
});

test("library reading, SPA refresh and session handling work on HTTP LAN", async ({ page }) => {
  await signIn(page);

  // A hard navigation to a client route is answered by the Go binary's
  // embedded SPA shell without any proxy.
  await page.goto(`/works/${seededWorkID}`);
  await expect(page.getByRole("heading", { name: seededTitle })).toBeVisible();

  await page.goto("/");
  await page.getByRole("searchbox", { name: "Search" }).fill(seededTitle);
  await expect(page.getByRole("status")).toHaveText("1 work");
  await expect(page.locator("article.work-card").first()).toContainText(seededTitle);

  await page.getByRole("button", { name: "Sign out" }).click();
  await page.waitForURL(/\/login/);
  await page.goto("/");
  await page.waitForURL(/\/login/);
});

test("a lost mutation response retries with the same request id on HTTP LAN", async ({ page }) => {
  await signIn(page);
  await page.getByRole("navigation", { name: "Main" }).getByRole("link", { name: "Quick add" }).click();

  const bodies: string[] = [];
  await page.route("**/api/v1/works", async (route) => {
    if (route.request().method() !== "POST") return route.continue();
    bodies.push(route.request().postData() ?? "");
    if (bodies.length === 1) await route.abort("failed");
    else await route.continue();
  });

  const title = `LAN retry ${Date.now()}`;
  await page.getByPlaceholder("The title you want to add").fill(title);
  await page.getByLabel("Medium (optional)").selectOption("book");
  await page.getByRole("button", { name: "Add", exact: true }).click();
  await expect(page.getByRole("alert")).toContainText("unreachable");
  await page.getByRole("button", { name: "Retry" }).click();
  await page.waitForURL(/\/editions\//);
  await expect(page.getByRole("heading", { level: 2, name: title })).toBeVisible();

  expect(bodies).toHaveLength(2);
  const first = JSON.parse(bodies[0]) as { request_id: string };
  const second = JSON.parse(bodies[1]) as { request_id: string };
  expect(first.request_id).toMatch(/^[0-9a-f]{8}-[0-9a-f]{4}-4[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$/);
  expect(second.request_id).toBe(first.request_id);
  expect(JSON.parse(bodies[0])).toEqual(JSON.parse(bodies[1]));
});
