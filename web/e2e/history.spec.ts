// Task 0046 end-to-end suite: Scenario B (progress 42 to 48, a process
// restart, a lower reread, and a supersedes_id correction where both
// versions stay visible) and Scenario D (a dropped anime keeps its negative
// note, revision and rating, stays findable by alias and dropped filter and
// in the activity feed, while a saved manga stays independent) through the
// built web UI. The beforeAll bootstrap builds the Go binary and the web
// bundle, starts the API on a private loopback port, serves the built UI and
// the API from one browser-facing origin (so the Host/Origin/CSRF boundary
// is exercised exactly as in the browser) and seeds the three works through
// that same path. Scenario B kills and respawns the API process mid-test to
// prove the state survives a restart; the in-memory session does not, so the
// test signs in again afterwards.
import { test, expect, type Page } from "@playwright/test";
import { execFileSync, spawn, type ChildProcess } from "node:child_process";
import {
  createReadStream,
  existsSync,
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
const apiPort = 18787;
const webPort = 8787;
const webBase = `http://127.0.0.1:${webPort}`;
const apiHost = `127.0.0.1:${apiPort}`;

const webNovelTitle = "Тихий Портал";
const animeTitleRu = "Мрачный Рассвет";
const animeTitleEn = "Grim Dawn";
const animeRefUrl = "https://stream.example.org/grim-dawn";
const mangaTitle = "Полночь";
const noteOld = "Slow pacing and a disappointing ending; not for me.";
const noteNew = "Even less enjoyable on a second look; skipping the sequel.";

let workDir: string | undefined;
let stateDir: string | undefined;
let binaryPath: string | undefined;
let apiProc: ChildProcess | undefined;
let proxy: Server | undefined;
let ownerSecret = "";
let webNovelEditionId = "";
let animeEditionId = "";
let mangaEditionId = "";

async function waitForHttp(url: string, timeoutMs: number): Promise<void> {
  const deadline = Date.now() + timeoutMs;
  for (;;) {
    try {
      const res = await fetch(url);
      void (await res.arrayBuffer());
      return;
    } catch {
      if (Date.now() > deadline) {
        throw new Error(`timed out waiting for ${url}`);
      }
      await new Promise((r) => setTimeout(r, 250));
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

// proxyApi forwards one request to the private API listener and rewrites
// the Host (always) and Origin (unsafe methods) headers to the listener's
// own spelling, which is the only spelling its security boundary accepts.
function proxyApi(req: IncomingMessage, res: ServerResponse, url: URL): void {
  const headers: Record<string, string | string[]> = {};
  for (const [name, value] of Object.entries(req.headers)) {
    if (value !== undefined) {
      headers[name] = value;
    }
  }
  headers.host = apiHost;
  if (req.method !== "GET" && req.method !== "HEAD") {
    headers.origin = `http://${apiHost}`;
  }
  const upstream = request(
    {
      host: "127.0.0.1",
      port: apiPort,
      method: req.method,
      path: url.pathname + url.search,
      headers,
    },
    (up) => {
      res.writeHead(up.statusCode ?? 502, up.headers);
      up.pipe(res);
    },
  );
  upstream.on("error", () => {
    if (!res.headersSent) {
      res.writeHead(502, { "content-type": "text/plain" });
    }
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
    if (path.extname(decoded) === "") {
      file = path.join(dist, "index.html");
    } else {
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

// startOriginProxy serves the built UI and the API from the single
// browser-facing origin.
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

function startApi(): ChildProcess {
  const log = openSync(path.join(workDir!, "api.log"), "a");
  const proc = spawn(
    binaryPath!,
    ["serve", "--state-dir", stateDir!, "--listen", `127.0.0.1:${apiPort}`],
    { cwd: workDir!, stdio: ["ignore", log, log] },
  );
  return proc;
}

// restartApi kills the running API process and starts a new one over the
// same state dir: the data must survive, the in-memory session does not.
async function restartApi(): Promise<void> {
  const old = apiProc;
  if (old !== undefined && old.exitCode === null) {
    await new Promise<void>((resolve) => {
      const timer = setTimeout(resolve, 5_000);
      timer.unref();
      old.once("exit", () => resolve());
      old.kill("SIGTERM");
    });
  }
  apiProc = startApi();
  await waitForHttp(`http://${apiHost}/api/v1/works`, 30_000);
}

// seedHistory creates the web novel, the aliased anime with its source
// link, and the saved manga through the same browser-facing origin the
// tests use.
async function seedHistory(): Promise<void> {
  const login = await fetch(`${webBase}/api/v1/session`, {
    method: "POST",
    headers: { "content-type": "application/json" },
    body: JSON.stringify({ owner_secret: ownerSecret }),
  });
  if (login.status !== 200) {
    throw new Error(`seed login failed: ${login.status} ${await login.text()}`);
  }
  const cookie = (login.headers.getSetCookie()[0] ?? "").split(";")[0];
  const loginBody = (await login.json()) as { data: { csrf_token: string } };

  async function call(p: string, body: unknown): Promise<Record<string, any>> {
    const res = await fetch(`${webBase}${p}`, {
      method: "POST",
      headers: {
        "content-type": "application/json",
        cookie,
        "x-csrf-token": loginBody.data.csrf_token,
      },
      body: JSON.stringify(body),
    });
    const text = await res.text();
    let parsed: { ok?: boolean };
    try {
      parsed = JSON.parse(text) as { ok?: boolean };
    } catch {
      throw new Error(`seed POST ${p}: non-JSON ${res.status}: ${text}`);
    }
    if (res.status !== 200 || parsed.ok !== true) {
      throw new Error(`seed POST ${p}: ${res.status} ${text}`);
    }
    return (parsed as { data: Record<string, any> }).data;
  }

  const novel = await call("/api/v1/works", {
    request_id: "e2e-history-webnovel",
    titles: [
      { text: webNovelTitle, language: "ru", kind: "original", preferred: true, provenance: "manual" },
    ],
    kind: "web_novel",
    language: "ru",
  });
  webNovelEditionId = novel.edition_id;

  const anime = await call("/api/v1/works", {
    request_id: "e2e-history-anime",
    titles: [
      { text: animeTitleRu, language: "ru", kind: "original", preferred: true, provenance: "manual" },
      { text: animeTitleEn, language: "en", kind: "localized", preferred: false, provenance: "manual" },
    ],
    kind: "anime",
    language: "ru",
  });
  animeEditionId = anime.edition_id;
  await call(`/api/v1/editions/${animeEditionId}/references`, {
    request_id: "e2e-history-anime-ref",
    expected_revision: 0,
    provider: "stream.example.org",
    url: animeRefUrl,
    role: "publication",
    authoritative: true,
    provenance: "manual",
  });

  const manga = await call("/api/v1/works", {
    request_id: "e2e-history-manga",
    titles: [
      { text: mangaTitle, language: "ru", kind: "original", preferred: true, provenance: "manual" },
    ],
    kind: "manga",
    language: "ru",
  });
  mangaEditionId = manga.edition_id;
  await call(`/api/v1/editions/${mangaEditionId}/intention`, {
    request_id: "e2e-history-manga-saved",
    expected_revision: 0,
    value: "saved",
    reason: "kept for later",
  });
}

test.beforeAll(async () => {
  test.setTimeout(240_000);
  workDir = mkdtempSync(path.join(tmpdir(), "media-core-e2e-history-"));
  binaryPath = path.join(workDir, "media-core");
  execFileSync("go", ["build", "-o", binaryPath, "./cmd/media-core"], {
    cwd: repoRoot,
    stdio: "pipe",
  });

  stateDir = path.join(workDir, "state");
  execFileSync(binaryPath!, ["init", "--state-dir", stateDir], { stdio: "pipe" });
  execFileSync(binaryPath!, ["owner", "generate", "--state-dir", stateDir], { stdio: "pipe" });
  // The file holds the raw 32 secret bytes; the login accepts their hex.
  ownerSecret = readFileSync(path.join(stateDir, "owner-secret")).toString("hex");

  // The bundle is built into the temp work dir, never into the repo tree,
  // so the suite leaves no untracked artifacts behind.
  const distDir = path.join(workDir, "dist");
  execFileSync("pnpm", ["exec", "vite", "build", "--outDir", distDir], {
    cwd: webDir,
    stdio: "pipe",
  });

  apiProc = startApi();
  await waitForHttp(`http://${apiHost}/api/v1/works`, 30_000);

  proxy = await startOriginProxy(distDir);
  await waitForHttp(`${webBase}/api/v1/works`, 10_000);

  await seedHistory();
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
  if (workDir !== undefined) {
    rmSync(workDir, { recursive: true, force: true });
  }
});

async function signIn(page: Page): Promise<void> {
  await page.goto("/");
  await page.getByLabel("Owner secret").fill(ownerSecret);
  await page.getByRole("button", { name: "Sign in" }).click();
  await page.getByRole("navigation", { name: "Main" }).waitFor();
}

const progressList = (page: Page) =>
  page.getByRole("heading", { name: "Progress", exact: true }).locator("xpath=following-sibling::ul[1]");

const progressForm = (page: Page) =>
  page.locator("form").filter({ has: page.getByLabel("Update kind") });

const historyList = (page: Page) =>
  page.getByRole("heading", { name: "History", exact: true }).locator("xpath=following-sibling::ul[1]");

const statusForm = (page: Page) =>
  page.locator("form").filter({ has: page.getByRole("button", { name: "Set status" }) });

const notesForm = (page: Page) =>
  page.locator("form").filter({ has: page.getByLabel("Note action") });

// A wrapping label's text content includes the select's option texts, so a
// label-matching locator cannot hit the bare "Note" or "Rating" controls;
// the label's own span is the exact-text anchor instead.
const noteSelect = (page: Page) =>
  notesForm(page).locator('label:has(> span:text-is("Note")) select');

const ratingSelect = (page: Page) =>
  page.locator("form").filter({ has: page.getByLabel("Rating action") }).locator(
    'label:has(> span:text-is("Rating")) select',
  );

const ratingForm = (page: Page) =>
  page.locator("form").filter({ has: page.getByLabel("Rating action") });

// The library also renders a transient role="status" while a filter
// refetch is in flight; the count element is the stable one.
const workCount = (page: Page) => page.locator("p.library-count");

test("scenario B: progress survives restart; reread and correction keep history", async ({ page }) => {
  test.setTimeout(120_000);
  await signIn(page);
  await page.goto(`/editions/${webNovelEditionId}`);
  await expect(page.getByRole("heading", { level: 2, name: webNovelTitle })).toBeVisible();

  // Record chapter 42, then 48; the live list shows only the latest record.
  await progressForm(page)
    .getByLabel("Unit")
    .selectOption("chapter");
  await progressForm(page).getByLabel("Position").fill("42");
  await progressForm(page).getByRole("button", { name: "Record update" }).click();
  await expect(progressList(page).getByText("chapter: 42", { exact: true })).toBeVisible();

  await progressForm(page).getByLabel("Position").fill("48");
  await progressForm(page).getByRole("button", { name: "Record update" }).click();
  await expect(progressList(page).getByText("chapter: 48", { exact: true })).toBeVisible();
  await expect(progressList(page).getByText("chapter: 42", { exact: true })).toHaveCount(0);

  // Reopen the process: the data survives, the session does not.
  await restartApi();
  await signIn(page);
  await page.goto(`/editions/${webNovelEditionId}`);

  // The latest value is 48 with an unknown total, and both historical rows
  // are still there: 42 effective, 48 current.
  await expect(progressList(page).getByText("chapter: 48", { exact: true })).toBeVisible();
  await expect(progressList(page).getByText(/chapter: 48\//)).toHaveCount(0);
  const history = historyList(page);
  await expect(history.getByRole("listitem").filter({ hasText: "chapter 42" })).toContainText(
    "effective",
  );
  await expect(history.getByRole("listitem").filter({ hasText: "chapter 48" })).toContainText(
    "current",
  );

  // A lower reread position is recorded without deleting history.
  await progressForm(page).getByLabel("Position").fill("40");
  await progressForm(page).getByRole("button", { name: "Record update" }).click();
  await expect(progressList(page).getByText("chapter: 40", { exact: true })).toBeVisible();

  // A mistaken row is corrected with supersedes_id; both versions remain
  // visible, the mistaken one superseded.
  await progressForm(page).getByLabel("Position").fill("49");
  await progressForm(page).getByRole("button", { name: "Record update" }).click();
  await expect(progressList(page).getByText("chapter: 49", { exact: true })).toBeVisible();

  await progressForm(page).getByLabel("Update kind").selectOption({ label: "correct mistake" });
  await progressForm(page).getByLabel("Record to correct").selectOption({ index: 1 });
  await progressForm(page).getByLabel("Position").fill("48");
  await progressForm(page).getByRole("button", { name: "Correct mistake" }).click();
  await expect(progressList(page).getByText("chapter: 48", { exact: true })).toBeVisible();
  await expect(progressList(page).getByText("chapter: 49", { exact: true })).toHaveCount(0);

  const rows48 = history.getByRole("listitem").filter({ hasText: "chapter 48" });
  await expect(rows48).toHaveCount(2);
  await expect(rows48.filter({ hasText: "current" })).toHaveCount(1);
  await expect(rows48.filter({ hasText: "effective" })).toHaveCount(1);
  await expect(history.getByRole("listitem").filter({ hasText: "chapter 49" })).toContainText(
    "superseded",
  );
  await expect(history.getByRole("listitem").filter({ hasText: "chapter 42" })).toContainText(
    "effective",
  );
  await expect(history.getByRole("listitem").filter({ hasText: "chapter 40" })).toContainText(
    "effective",
  );

  // Invalid positions are rejected with a distinguishable error and change
  // nothing.
  await progressForm(page).getByLabel("Update kind").selectOption({ label: "record new update" });
  await progressForm(page).getByLabel("Unit").selectOption("chapter");
  await progressForm(page).getByLabel("Position").fill("-1");
  await progressForm(page).getByRole("button", { name: "Record update" }).click();
  await expect(page.getByRole("alert").first()).toContainText("non-negative decimal");

  await progressForm(page).getByLabel("Unit").selectOption("page");
  await progressForm(page).getByLabel("Position").fill("12.5");
  await progressForm(page).getByRole("button", { name: "Record update" }).click();
  await expect(page.getByRole("alert").first()).toContainText("must be integral");
  await expect(progressList(page).getByText("chapter: 48", { exact: true })).toBeVisible();
});

test("scenario D: a dropped anime keeps its negative opinions and stays findable", async ({ page }) => {
  test.setTimeout(120_000);
  await signIn(page);
  await page.goto(`/editions/${animeEditionId}`);
  await expect(page.getByRole("heading", { level: 2, name: animeTitleRu })).toBeVisible();

  // In progress, then dropped with a reason.
  await statusForm(page).getByLabel("Status").selectOption("in_progress");
  await statusForm(page).getByLabel("Reason").fill("started");
  await statusForm(page).getByRole("button", { name: "Set status" }).click();
  await expect(page.getByText("Status: in progress — started")).toBeVisible();

  await statusForm(page).getByLabel("Status").selectOption("dropped");
  await statusForm(page).getByLabel("Reason").fill("abandoned after episode 5");
  await statusForm(page).getByRole("button", { name: "Set status" }).click();
  await expect(page.getByText("Status: dropped — abandoned after episode 5")).toBeVisible();

  // A negative note, then an explicit revision that keeps the old text.
  await notesForm(page).getByLabel("Note text").fill(noteOld);
  await notesForm(page).getByRole("button", { name: "Add note" }).click();
  await expect(page.getByText(noteOld, { exact: true })).toBeVisible();

  await notesForm(page).getByLabel("Note action").selectOption({ label: "edit note" });
  await noteSelect(page).selectOption({ index: 1 });
  await notesForm(page).getByLabel("Note text").fill(noteNew);
  await notesForm(page).getByRole("button", { name: "Save revision" }).click();
  await expect(page.getByText(noteNew, { exact: true })).toBeVisible();

  // The negative rating.
  await ratingSelect(page).selectOption("2");
  await ratingForm(page).getByLabel("Rating comment").fill("waste of time");
  await ratingForm(page).getByRole("button", { name: "Set rating" }).click();
  await expect(page.getByText("Rating: 2/10 — waste of time")).toBeVisible();

  // The history keeps every value: both consumption states, both note
  // revisions (the old text still readable) and the rating.
  const history = historyList(page);
  await expect(history.getByRole("listitem").filter({ hasText: "in_progress" })).toContainText(
    "effective",
  );
  await expect(history.getByRole("listitem").filter({ hasText: "dropped" })).toContainText(
    "effective",
  );
  await expect(history.getByRole("listitem").filter({ hasText: "revision 1" })).toContainText(
    "superseded",
  );
  await expect(history.getByRole("listitem").filter({ hasText: "revision 2" })).toContainText(
    "current",
  );
  await expect(history.getByText(noteOld)).toHaveCount(1);
  await expect(history.getByRole("listitem").filter({ hasText: "2: waste of time" })).toContainText(
    "effective",
  );

  // The drop does not erase the source link.
  await expect(page.locator(`a[href="${animeRefUrl}"]`)).toBeVisible();

  // Later lookup: the alias finds the entry, the dropped filter finds it,
  // and the library still counts it.
  const nav = page.getByRole("navigation", { name: "Main" });
  await nav.getByRole("link", { name: "Library" }).click();
  const search = page.getByRole("searchbox", { name: "Search" });
  await search.fill(animeTitleEn);
  await expect(workCount(page)).toHaveText("1 work");
  await expect(page.locator("article.work-card").first()).toContainText("Dropped");
  await search.fill("");
  await page.getByLabel("Status").selectOption("dropped");
  await expect(workCount(page)).toHaveText("1 work");
  await page.getByLabel("Status").selectOption("");
  await expect(workCount(page)).toHaveText("3 works");

  // The activity feed carries the dropped observation.
  await nav.getByRole("link", { name: "Activity" }).click();
  await page.waitForURL(/\/activity/);
  await expect(page.getByRole("heading", { name: "Recent activity" })).toBeVisible();
  await expect(
    page
      .locator("li.activity-row")
      .filter({ hasText: animeTitleRu })
      .filter({ hasText: "dropped" })
      .first(),
  ).toContainText("effective");

  // The saved manga is fully independent of the dropped anime.
  await page.goto(`/editions/${mangaEditionId}`);
  await expect(page.getByText("Status: not started")).toBeVisible();
  await expect(page.getByText("Intention: saved — kept for later")).toBeVisible();
});
