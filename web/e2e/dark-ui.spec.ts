// Task 0079 browser regression for the dark-first library. Its beforeAll
// builds the Go binary and the Vite bundle, starts the API on a private
// loopback port and serves both from one browser-facing origin. It uses
// distinct ports from the other web specs so `playwright test` can run the
// whole suite. Screenshots are written to gitignored tmp/apk, never to git.
import { test, expect, type Page } from "@playwright/test";
import { execFileSync, spawn, type ChildProcess } from "node:child_process";
import {
  createReadStream,
  existsSync,
  mkdirSync,
  mkdtempSync,
  openSync,
  readFileSync,
  rmSync,
  statSync,
} from "node:fs";
import { createServer, request, type IncomingMessage, type Server, type ServerResponse } from "node:http";
import { tmpdir } from "node:os";
import path from "node:path";
import { fileURLToPath } from "node:url";

const here = path.dirname(fileURLToPath(import.meta.url));
const webDir = path.resolve(here, "..");
const repoRoot = path.resolve(webDir, "..");
const apiPort = 18789;
const webPort = 8789;
const webBase = `http://127.0.0.1:${webPort}`;
const apiHost = `127.0.0.1:${apiPort}`;
const screenshotDir = path.join(repoRoot, "tmp", "apk", "0079-screenshots");

const seededTitles = ["Dark UI Fixture One", "Dark UI Fixture Two"];

let workDir: string | undefined;
let apiProc: ChildProcess | undefined;
let proxy: Server | undefined;
let ownerSecret = "";

test.use({ baseURL: webBase, colorScheme: "light" });

function screenshotPath(name: string): string {
  mkdirSync(screenshotDir, { recursive: true });
  return path.join(screenshotDir, name);
}

async function waitForHttp(url: string, timeoutMs: number): Promise<void> {
  const deadline = Date.now() + timeoutMs;
  for (;;) {
    try {
      const res = await fetch(url);
      void (await res.arrayBuffer());
      return;
    } catch {
      if (Date.now() > deadline) throw new Error(`timed out waiting for ${url}`);
      await new Promise((resolve) => setTimeout(resolve, 250));
    }
  }
}

const CONTENT_TYPES: Record<string, string> = {
  ".html": "text/html; charset=utf-8",
  ".js": "text/javascript; charset=utf-8",
  ".css": "text/css; charset=utf-8",
  ".svg": "image/svg+xml",
  ".ico": "image/x-icon",
  ".json": "application/json; charset=utf-8",
  ".map": "application/json; charset=utf-8",
  ".woff2": "font/woff2",
  ".png": "image/png",
};

function proxyApi(req: IncomingMessage, res: ServerResponse, url: URL): void {
  const headers: Record<string, string | string[]> = {};
  for (const [name, value] of Object.entries(req.headers)) {
    if (value !== undefined) headers[name] = value;
  }
  headers.host = apiHost;
  if (req.method !== "GET" && req.method !== "HEAD") {
    headers.origin = `http://${apiHost}`;
  }
  const upstream = request(
    { host: "127.0.0.1", port: apiPort, method: req.method, path: url.pathname + url.search, headers },
    (up) => {
      res.writeHead(up.statusCode ?? 502, up.headers);
      up.pipe(res);
    },
  );
  upstream.on("error", () => {
    if (!res.headersSent) res.writeHead(502, { "content-type": "text/plain" });
    res.end("bad gateway");
  });
  req.pipe(upstream);
}

function serveStatic(req: IncomingMessage, res: ServerResponse, url: URL, dist: string): void {
  let decoded: string;
  try {
    decoded = decodeURIComponent(url.pathname);
  } catch {
    res.writeHead(400, { "content-type": "text/plain" });
    res.end("bad request");
    return;
  }
  let file = path.normalize(path.join(dist, decoded));
  if (file !== dist && !file.startsWith(dist + path.sep)) {
    res.writeHead(403, { "content-type": "text/plain" });
    res.end("forbidden");
    return;
  }
  if (!existsSync(file) || !statSync(file).isFile()) {
    if (path.extname(decoded) === "") file = path.join(dist, "index.html");
    else {
      res.writeHead(404, { "content-type": "text/plain" });
      res.end("not found");
      return;
    }
  }
  res.writeHead(200, {
    "content-type": CONTENT_TYPES[path.extname(file).toLowerCase()] ?? "application/octet-stream",
    "cache-control": "no-store",
  });
  if (req.method === "HEAD") {
    res.end();
    return;
  }
  createReadStream(file).pipe(res);
}

function startOriginProxy(dist: string): Promise<Server> {
  const server = createServer((req, res) => {
    const url = new URL(req.url ?? "/", webBase);
    if (url.pathname.startsWith("/api/")) {
      proxyApi(req, res, url);
      return;
    }
    if (req.method !== "GET" && req.method !== "HEAD") {
      res.writeHead(405, { "content-type": "text/plain" });
      res.end("method not allowed");
      return;
    }
    serveStatic(req, res, url, dist);
  });
  return new Promise((resolve, reject) => {
    server.once("error", reject);
    server.listen(webPort, "127.0.0.1", () => resolve(server));
  });
}

async function seedTwoWorks(): Promise<void> {
  const login = await fetch(`${webBase}/api/v1/session`, {
    method: "POST",
    headers: { "content-type": "application/json" },
    body: JSON.stringify({ owner_secret: ownerSecret }),
  });
  if (login.status !== 200) throw new Error(`seed login failed: ${login.status}`);
  const cookie = (login.headers.getSetCookie()[0] ?? "").split(";")[0];
  const body = (await login.json()) as { data: { csrf_token: string } };
  for (const [index, text] of seededTitles.entries()) {
    const res = await fetch(`${webBase}/api/v1/works`, {
      method: "POST",
      headers: {
        "content-type": "application/json",
        cookie,
        "x-csrf-token": body.data.csrf_token,
      },
      body: JSON.stringify({
        request_id: `dark-ui-seed-${index}`,
        titles: [{ text, language: "en", kind: "original", preferred: true, provenance: "manual" }],
        kind: "book",
        language: "en",
      }),
    });
    const parsed = (await res.json()) as { ok: boolean; data?: { work_id?: string } };
    if (res.status !== 200 || parsed.ok !== true || !parsed.data?.work_id) {
      throw new Error(`seed work failed: ${res.status} ${JSON.stringify(parsed)}`);
    }
  }
}

test.beforeAll(async () => {
  test.setTimeout(240_000);
  workDir = mkdtempSync(path.join(tmpdir(), "media-core-dark-ui-"));
  const binary = path.join(workDir, "media-core");
  execFileSync("go", ["build", "-o", binary, "./cmd/media-core"], {
    cwd: repoRoot,
    stdio: "pipe",
  });
  const stateDir = path.join(workDir, "state");
  execFileSync(binary, ["init", "--state-dir", stateDir], { stdio: "pipe" });
  execFileSync(binary, ["owner", "generate", "--state-dir", stateDir], { stdio: "pipe" });
  ownerSecret = readFileSync(path.join(stateDir, "owner-secret")).toString("hex");
  const distDir = path.join(workDir, "dist");
  execFileSync("pnpm", ["exec", "vite", "build", "--outDir", distDir], {
    cwd: webDir,
    stdio: "pipe",
  });
  const log = openSync(path.join(workDir, "api.log"), "w");
  apiProc = spawn(
    binary,
    ["serve", "--state-dir", stateDir, "--listen", `127.0.0.1:${apiPort}`],
    { cwd: workDir, stdio: ["ignore", log, log] },
  );
  await waitForHttp(`http://${apiHost}/api/v1/works`, 30_000);
  proxy = await startOriginProxy(distDir);
  await waitForHttp(`${webBase}/api/v1/works`, 10_000);
  await seedTwoWorks();
}, 240_000);

test.afterAll(async () => {
  if (proxy !== undefined) {
    await new Promise<void>((resolve) => proxy?.close(() => resolve()));
  }
  if (apiProc !== undefined && apiProc.exitCode === null) {
    await new Promise<void>((resolve) => {
      const timer = setTimeout(resolve, 5_000);
      timer.unref();
      apiProc?.once("exit", () => resolve());
      apiProc?.kill("SIGTERM");
    });
  }
  if (workDir !== undefined) rmSync(workDir, { recursive: true, force: true });
});

async function signIn(page: Page): Promise<void> {
  await page.goto("/login");
  await page.getByLabel("Owner secret").fill(ownerSecret);
  await page.getByRole("button", { name: "Sign in" }).click();
  await page.getByRole("navigation", { name: "Main" }).waitFor();
}

// Two sign-ins only: the API rate-limits logins to five per 15 seconds, and
// the seed login already consumes one.
test("desktop is dark by default with a sidebar, compact header and visible focus", async ({
  page,
}) => {
  await signIn(page);
  const background = await page.evaluate(() => getComputedStyle(document.body).backgroundColor);
  expect(background).toBe("rgb(17, 19, 25)");

  const nav = page.getByRole("navigation", { name: "Main" });
  await expect(nav.getByRole("link", { name: "Library" })).toBeVisible();
  await expect(nav.getByRole("link", { name: "Quick add" })).toBeVisible();
  await expect(nav.getByRole("link", { name: "Activity" })).toBeVisible();
  await expect(page.getByRole("searchbox", { name: "Find in library" })).toBeVisible();
  await expect(page.getByRole("button", { name: "Add work" })).toBeVisible();
  await expect(page.getByRole("button", { name: "Sign out" })).toBeVisible();
  await expect(page.locator("ul.library-cards")).toBeVisible();
  await page.screenshot({ path: screenshotPath("desktop-library.png"), fullPage: true });

  let focusedInteractive = false;
  for (let attempt = 0; attempt < 12; attempt += 1) {
    await page.keyboard.press("Tab");
    focusedInteractive = await page.evaluate(() => {
      const active = document.activeElement;
      return active instanceof HTMLElement && ["A", "BUTTON", "INPUT"].includes(active.tagName);
    });
    if (focusedInteractive) break;
  }
  expect(focusedInteractive).toBe(true);
  const styled = await page.evaluate(() => {
    const active = document.activeElement;
    if (!(active instanceof HTMLElement)) return { visible: false, width: "0px" };
    return {
      visible: active.matches(":focus-visible"),
      width: getComputedStyle(active).outlineWidth,
    };
  });
  expect(styled.visible).toBe(true);
  expect(styled.width).toBe("2px");
});

test("grid/list toggle, 360px layout and empty state work in one session", async ({ page }) => {
  await signIn(page);
  await expect(page.locator("ul.library-cards")).toBeVisible();
  await page.getByRole("button", { name: "List view" }).click();
  await expect(page).toHaveURL(/\?view=list$/);
  await expect(page.locator("ul.library-cards")).toHaveCount(0);
  await expect(page.locator("article.work-card").first()).toBeVisible();
  await page.screenshot({ path: screenshotPath("desktop-list.png"), fullPage: true });
  await page.getByRole("button", { name: "Grid view" }).click();
  await expect(page).toHaveURL(/\/$/);
  await expect(page.locator("ul.library-cards")).toBeVisible();

  await page.setViewportSize({ width: 360, height: 740 });
  await expect(page.getByRole("navigation", { name: "Main" })).toBeVisible();
  await expect(page.getByRole("searchbox", { name: "Find in library" })).toBeVisible();
  const noOverflow = await page.evaluate(
    () => document.documentElement.scrollWidth <= window.innerWidth + 1,
  );
  expect(noOverflow).toBe(true);
  const columns = await page.evaluate(() => {
    const grid = document.querySelector("ul.library-cards");
    return grid === null ? 0 : getComputedStyle(grid).gridTemplateColumns.split(" ").length;
  });
  expect(columns).toBe(1);
  await page.screenshot({ path: screenshotPath("mobile-library-360.png"), fullPage: true });

  await page.getByRole("searchbox", { name: "Search" }).fill("zzz-no-such-work");
  await expect(page.getByText("No works match the current filters.")).toBeVisible();
  await expect(page.getByRole("button", { name: "Reset filters" })).toBeVisible();
  await page.screenshot({ path: screenshotPath("mobile-empty-360.png"), fullPage: true });
});
