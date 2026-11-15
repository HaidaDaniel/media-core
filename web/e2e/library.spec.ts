// Task 0045 end-to-end suite: Scenario A (a watched anime stays connected
// to its unread saved manga source: note, tag, publication URL, confirmed
// adaptation link, guessed origin) and Scenario C (duplicate and candidate
// adds never merge silently) through the built web UI. The beforeAll
// bootstrap builds the Go binary and the web bundle, starts the API on a
// private loopback port, serves the built UI and the API from one browser-
// facing origin (so the Host/Origin/CSRF boundary is exercised exactly as
// in the browser) and seeds the Scenario A library through that same path.
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

const jpTitle = "星霜の軌跡";
const ruTitle = "След звёзд";
const tag = "manga-translation-candidate";
const noteText = "Defer smartphone reading until a stable reader is chosen.";
const pubUrl = "https://manga.example.org/star-frost";
const newTitle = "Параллельный Веб-Тест";

let workDir: string | undefined;
let apiProc: ChildProcess | undefined;
let proxy: Server | undefined;
let ownerSecret = "";

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

// seedScenarioA creates the watched anime and its unread saved manga source
// through the same browser-facing origin the tests use, with the same
// payloads and revision expectations as the Go acceptance test.
async function seedScenarioA(): Promise<void> {
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

  const jpTitleInput = (preferred: boolean) => ({
    text: jpTitle,
    language: "ja",
    kind: "original",
    preferred,
    provenance: "manual",
  });

  const created = await call("/api/v1/works", {
    request_id: "e2e-create-anime",
    titles: [
      jpTitleInput(true),
      { text: ruTitle, language: "ru", kind: "alias", preferred: false, provenance: "manual" },
    ],
    kind: "anime",
    language: "ja",
  });
  const workId: string = created.work_id;
  const animeId: string = created.edition_id;

  const consumed = await call(`/api/v1/editions/${animeId}/consumption`, {
    request_id: "e2e-anime-completed",
    expected_revision: 0,
    value: "completed",
    reason: "watched through",
  });
  const rated = await call(`/api/v1/editions/${animeId}/ratings`, {
    request_id: "e2e-anime-rating",
    expected_revision: consumed.revision,
    value: 8,
    comment: "steady, no filler",
  });

  const resolved = await call("/api/v1/additions/resolve", {
    request_id: "e2e-resolve-manga",
    titles: [jpTitleInput(false)],
    kind: "manga",
    language: "ja",
  });
  if (resolved.outcome !== "candidates" || resolved.candidate_token === null) {
    throw new Error(`seed resolve: unexpected outcome ${JSON.stringify(resolved)}`);
  }
  const added = await call(`/api/v1/works/${workId}/editions`, {
    request_id: "e2e-add-manga",
    titles: [jpTitleInput(false)],
    kind: "manga",
    language: "ja",
    candidate_token: resolved.candidate_token,
  });
  const mangaId: string = added.edition_id;

  const saved = await call(`/api/v1/editions/${mangaId}/intention`, {
    request_id: "e2e-manga-saved",
    expected_revision: 0,
    value: "saved",
    reason: "unread, kept for later",
  });
  const tagged = await call("/api/v1/tags/assignments", {
    request_id: "e2e-manga-tag",
    expected_revision: saved.revision,
    target: "edition",
    target_id: mangaId,
    tag,
  });
  await call("/api/v1/notes", {
    request_id: "e2e-manga-note",
    target: "edition",
    target_id: mangaId,
    text: noteText,
  });
  await call(`/api/v1/editions/${mangaId}/references`, {
    request_id: "e2e-manga-url",
    expected_revision: tagged.revision,
    provider: "manga.example.org",
    url: pubUrl,
    role: "publication",
    authoritative: true,
    provenance: "manual",
  });
  await call("/api/v1/relations", {
    request_id: "e2e-relation",
    expected_revision: rated.revision,
    scope: "edition",
    type: "adaptation_of",
    from_id: animeId,
    to_id: mangaId,
    state: "confirmed",
    provenance: "user",
  });
}

test.beforeAll(async () => {
  test.setTimeout(240_000);
  workDir = mkdtempSync(path.join(tmpdir(), "media-core-e2e-"));
  const binary = path.join(workDir, "media-core");
  execFileSync("go", ["build", "-o", binary, "./cmd/media-core"], {
    cwd: repoRoot,
    stdio: "pipe",
  });

  const stateDir = path.join(workDir, "state");
  execFileSync(binary, ["init", "--state-dir", stateDir], { stdio: "pipe" });
  execFileSync(binary, ["owner", "generate", "--state-dir", stateDir], { stdio: "pipe" });
  // The file holds the raw 32 secret bytes; the login accepts their hex.
  ownerSecret = readFileSync(path.join(stateDir, "owner-secret")).toString("hex");

  execFileSync("pnpm", ["run", "build"], { cwd: webDir, stdio: "pipe" });

  const log = openSync(path.join(workDir, "api.log"), "w");
  apiProc = spawn(
    binary,
    ["serve", "--state-dir", stateDir, "--listen", `127.0.0.1:${apiPort}`],
    { cwd: workDir, stdio: ["ignore", log, log] },
  );
  await waitForHttp(`http://${apiHost}/api/v1/works`, 30_000);

  proxy = await startOriginProxy(path.join(webDir, "dist"));
  await waitForHttp(`${webBase}/api/v1/works`, 10_000);

  await seedScenarioA();
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

test("scenario A: a watched anime stays connected to its unread manga source", async ({ page }) => {
  await signIn(page);

  // The seeded work is found by its original title with the alias and both
  // edition states visible on the card.
  const search = page.getByRole("searchbox", { name: "Search" });
  await search.fill(jpTitle);
  await expect(page.getByRole("status")).toHaveText("1 work");
  const card = page.locator("article.work-card").first();
  await expect(card).toContainText(ruTitle);
  await expect(card).toContainText("Completed");
  await expect(card).toContainText("Not started");

  // The alias title finds the same single work, and the tag filter keeps
  // exactly the tagged work.
  await search.fill(ruTitle);
  await expect(page.getByRole("status")).toHaveText("1 work");
  await search.fill("");
  await page.getByLabel("Tag").fill(tag);
  await expect(page.getByRole("status")).toHaveText("1 work");
  await expect(card).toBeVisible();
  await page.getByLabel("Tag").fill("");

  // The work page lists both editions with the manga note.
  await card.getByRole("link", { name: jpTitle }).click();
  await page.waitForURL(/\/works\//);
  await expect(page.getByRole("heading", { name: jpTitle })).toBeVisible();
  await expect(page.getByText(noteText)).toBeVisible();

  // The anime edition carries the confirmed adaptation of the manga; the
  // relation row links to the manga edition.
  // The relation row appears on both editions (from their own perspective);
  // the first is the anime's, and its link points to the manga edition.
  const relationRow = page
    .locator("li.work-edition", { hasText: "is an adaptation of" })
    .first();
  await expect(relationRow).toContainText("Confirmed");
  await relationRow.getByRole("link").first().click();
  await page.waitForURL(/\/editions\//);

  // The manga edition page shows its full personal state.
  await expect(page.getByText("Status: not started")).toBeVisible();
  await expect(page.getByText("Intention: saved — unread, kept for later")).toBeVisible();
  await expect(page.getByText(tag, { exact: true }).first()).toBeVisible();
  await expect(page.getByText(noteText).first()).toBeVisible();
  await expect(page.locator(`a[href="${pubUrl}"]`)).toBeVisible();

  // The work back link returns to the work page, where a guessed origin is
  // recorded as proposed.
  await page.getByRole("link", { name: jpTitle }).click();
  await page.waitForURL(/\/works\//);
  const originForm = page.locator("form", {
    has: page.getByRole("button", { name: "Record origin" }),
  });
  await originForm.getByLabel("Edition").selectOption({ label: "manga (ja)" });
  await originForm.getByLabel("Kind").selectOption({ label: "original" });
  await originForm.getByLabel("State").selectOption({ label: "proposed" });
  await originForm.getByLabel("Provenance").fill("guessed");
  await originForm.getByRole("button", { name: "Record origin" }).click();
  await expect(page.getByText("Source: original")).toBeVisible();
  await expect(page.locator("li.work-edition", { hasText: "Source: original" })).toContainText(
    "Proposed",
  );

  // Signing out returns to the login screen.
  await page.getByRole("button", { name: "Sign out" }).click();
  await page.waitForURL(/\/login/);
});

test("scenario C: duplicate adds show candidates and never merge silently", async ({ page }) => {
  await signIn(page);
  const nav = page.getByRole("navigation", { name: "Main" });

  // One seeded work before any add.
  await expect(page.getByRole("status")).toHaveText("1 work");

  // Re-adding the seeded title alone surfaces candidates, never a silent
  // duplicate; adding an edition under a candidate needs a medium.
  await nav.getByRole("link", { name: "Quick add" }).click();
  await page.waitForURL(/\/add/);
  await page.getByPlaceholder("The title you want to add").fill(jpTitle);
  await page.getByRole("button", { name: "Add", exact: true }).click();
  await expect(
    page.getByText("Possible matches for this title. Nothing is merged or created until you choose."),
  ).toBeVisible();
  // The seeded title matches several candidates; any of them leads to the
  // same existing work.
  await expect(page.getByRole("button", { name: "Use existing" }).first()).toBeVisible();
  await expect(
    page.getByRole("button", { name: "Add edition under this work" }).first(),
  ).toBeDisabled();

  // Choosing the existing record creates nothing and opens the work
  // directly.
  await page.getByRole("button", { name: "Use existing" }).first().click();
  await page.waitForURL(/\/works\//);
  await expect(page.getByRole("heading", { name: jpTitle })).toBeVisible();

  // The library still holds exactly one work.
  await nav.getByRole("link", { name: "Library" }).click();
  await expect(page.getByRole("status")).toHaveText("1 work");

  // A genuinely new title creates its own edition.
  await nav.getByRole("link", { name: "Quick add" }).click();
  await page.getByPlaceholder("The title you want to add").fill(newTitle);
  await page.getByLabel("Medium (optional)").selectOption("manga");
  await page.getByRole("button", { name: "Add", exact: true }).click();
  await page.waitForURL(/\/editions\//);
  await expect(page.getByRole("heading", { level: 2, name: newTitle })).toBeVisible();
  await expect(page.getByText("Status: not started")).toBeVisible();

  // The library now holds two works, and re-adding the new title surfaces
  // candidates again instead of creating a third record.
  await nav.getByRole("link", { name: "Library" }).click();
  await expect(page.getByRole("status")).toHaveText("2 works");
  await nav.getByRole("link", { name: "Quick add" }).click();
  await page.getByPlaceholder("The title you want to add").fill(newTitle);
  await page.getByLabel("Medium (optional)").selectOption("manga");
  await page.getByRole("button", { name: "Add", exact: true }).click();
  await expect(
    page.getByText("Possible matches for this title. Nothing is merged or created until you choose."),
  ).toBeVisible();
  await nav.getByRole("link", { name: "Library" }).click();
  await expect(page.getByRole("status")).toHaveText("2 works");
});

test("a wrong owner secret is rejected with a distinguishable error", async ({ page }) => {
  await page.goto("/login");
  await page.getByLabel("Owner secret").fill("not-the-owner-secret");
  await page.getByRole("button", { name: "Sign in" }).click();
  await expect(page.getByRole("alert")).toContainText("does not match");
});
