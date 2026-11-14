// The recent-activity screen: the server's deterministic (recorded_at, id)
// descending order rendered as-is, the correction labels shown rather than
// hidden, the target links, the observed and recorded times, the
// distinct imported-release rendering without fabricated release data (a
// contract fixture only — no table produces that type before task 0042),
// the pagination, and the empty, first-load and page-load failure states.
import { cleanup, fireEvent, render, screen, waitFor, within } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { MemoryRouter } from "react-router-dom";
import { ApiError } from "../api/client";
import { ActivityPage } from "./ActivityPage";
import type { ActivityItemWire, ActivityListWire } from "./activityApi";

const { mockListActivity } = vi.hoisted(() => ({
  mockListActivity: vi.fn(),
}));

vi.mock("./activityApi", () => ({
  listActivity: (...args: unknown[]) => mockListActivity(...args),
}));

function item(
  overrides: Partial<ActivityItemWire> & { id: string; type: string },
): ActivityItemWire {
  return {
    work_id: "w-1",
    edition_id: "e-1",
    title: "Manga Title",
    value: null,
    detail: null,
    label: "effective",
    reason: null,
    supersedes_id: null,
    replaces_id: null,
    observed_at: "2026-01-02T00:00:00Z",
    recorded_at: "2026-01-02T00:00:01Z",
    actor: null,
    notes: [],
    references: [],
    ...overrides,
  };
}

function payload(
  items: ActivityItemWire[],
  nextCursor: string | null = null,
  total?: number,
): ActivityListWire {
  return { items, next_cursor: nextCursor, total: total ?? items.length };
}

function renderPage(): void {
  render(
    <MemoryRouter>
      <ActivityPage />
    </MemoryRouter>,
  );
}

function rowTexts(): string[] {
  return screen.getAllByRole("listitem").map((row) => row.textContent ?? "");
}

beforeEach(() => {
  vi.clearAllMocks();
  mockListActivity.mockResolvedValue(payload([]));
});

afterEach(() => {
  cleanup();
});

describe("activity feed", () => {
  it("renders the server's deterministic order as-is, with dates and links", async () => {
    // The payload order is (recorded_at, id) descending from the server;
    // the screen must not re-sort it.
    mockListActivity.mockResolvedValueOnce(
      payload([
        item({ id: "a-new", type: "progress", value: "chapter", detail: "48/100", recorded_at: "2026-01-04T00:00:00Z" }),
        item({ id: "a-old", type: "rating", value: "8", detail: "great", recorded_at: "2026-01-03T00:00:00Z" }),
        item({ id: "a-mid", type: "consumption", value: "completed", recorded_at: "2026-01-03T00:00:00Z", observed_at: "2026-01-01T00:00:00Z" }),
      ]),
    );
    renderPage();
    expect(await screen.findByRole("list")).not.toBeNull();
    const rows = rowTexts();
    expect(rows.length).toBe(3);
    // Row 1 is the newest (chapter 48), row 3 the oldest.
    expect(rows[0]).toContain("chapter 48/100");
    expect(rows[1]).toContain("8/10 — great");
    expect(rows[2]).toContain("completed");
    // The target link and the recorded time are useful, not analytics.
    const first = screen.getAllByRole("listitem")[0];
    const link = within(first).getByRole("link", { name: "Manga Title" });
    expect(link.getAttribute("href")).toBe("/editions/e-1");
    expect(within(first).getByText(/recorded/)).not.toBeNull();
    expect(mockListActivity).toHaveBeenCalledWith({ limit: 30 });
  });

  it("shows superseded and retracted rows with their correction labels", async () => {
    mockListActivity.mockResolvedValueOnce(
      payload([
        item({ id: "a", type: "rating", value: "8", label: "effective" }),
        item({ id: "b", type: "rating", value: "5", label: "superseded" }),
        item({ id: "c", type: "note", value: "old thought", detail: "revision 1", label: "retracted", reason: "wrong edition", observed_at: null }),
      ]),
    );
    renderPage();
    expect(await screen.findByRole("list")).not.toBeNull();
    // Nothing is hidden or deleted: all three rows are visible.
    expect(rowTexts().length).toBe(3);
    expect(screen.getAllByText("superseded").length).toBeGreaterThan(0);
    expect(screen.getAllByText("retracted").length).toBeGreaterThan(0);
    expect(screen.getByText("wrong edition")).not.toBeNull();
    // The note revision row has no observed time; only recorded is shown.
    const noteRow = screen.getAllByRole("listitem")[2];
    expect(within(noteRow).getByText(/recorded/)).not.toBeNull();
    expect(within(noteRow).queryByText(/observed/)).toBeNull();
  });

  it("links a work-scoped row to the work and falls back to the target label", async () => {
    mockListActivity.mockResolvedValueOnce(
      payload([
        item({ id: "a", type: "tag", value: "manga-translation-candidate", edition_id: null, title: null, label: "active" }),
      ]),
    );
    renderPage();
    expect(await screen.findByRole("list")).not.toBeNull();
    const link = screen.getByRole("link", { name: /work/ });
    expect(link.getAttribute("href")).toBe("/works/w-1");
    expect(screen.getByText("manga-translation-candidate")).not.toBeNull();
  });

  it("appends the next page on load more and keeps the prior rows", async () => {
    mockListActivity
      .mockResolvedValueOnce(payload([item({ id: "p1", type: "progress", value: "chapter", detail: "42" })], "cursor-2", 2))
      .mockResolvedValueOnce(
        payload([item({ id: "p2", type: "rating", value: "7" })], null, 2),
      );
    renderPage();
    expect(await screen.findByRole("list")).not.toBeNull();
    fireEvent.click(await screen.findByRole("button", { name: "Load more" }));
    await waitFor(() => expect(rowTexts().length).toBe(2));
    expect(mockListActivity).toHaveBeenLastCalledWith({ limit: 30, cursor: "cursor-2" });
    expect(screen.queryByRole("button", { name: /load more/i })).toBeNull();
    // The count is the payload total before pagination.
    expect(screen.getByText("2 changes")).not.toBeNull();
  });
});

describe("imported-release variant (contract fixture only)", () => {
  it("renders imported-release rows with a distinct label, only from payload fields", async () => {
    mockListActivity.mockResolvedValueOnce(
      payload([
        item({
          id: "rel-1",
          type: "imported_release",
          value: "chapter 43",
          detail: "Manga Title — vol. 5",
          label: "effective",
        }),
        item({ id: "per-1", type: "consumption", value: "in_progress" }),
      ]),
    );
    renderPage();
    expect(await screen.findByRole("list")).not.toBeNull();
    const rows = rowTexts();
    // The imported row is visually distinct from the personal change.
    expect(rows[0]).toContain("imported release");
    expect(rows[0]).toContain("chapter 43");
    expect(rows[1]).toContain("status");
    expect(rows[1]).toContain("in progress");
    expect(screen.getAllByText("imported release").length).toBe(1);
  });

  it("never fabricates release data when the payload carries no fields", async () => {
    mockListActivity.mockResolvedValueOnce(
      payload([
        item({ id: "rel-empty", type: "imported_release", value: null, detail: null }),
      ]),
    );
    renderPage();
    expect(await screen.findByRole("list")).not.toBeNull();
    const row = screen.getAllByRole("listitem")[0];
    // Only the distinct label and the target are shown; no invented
    // release number, count or source.
    expect(within(row).getByText("imported release")).not.toBeNull();
    expect(within(row).queryByText(/chapter|episode|volume|release no/i)).toBeNull();
  });
});

describe("activity states", () => {
  it("shows the empty state for an empty feed", async () => {
    renderPage();
    expect(await screen.findByText("No recent activity.")).not.toBeNull();
    expect(screen.getByText("0 changes")).not.toBeNull();
  });

  it("shows a failed first load as a distinguishable error and recovers on retry", async () => {
    mockListActivity
      .mockRejectedValueOnce(new ApiError("locked", 503, "the storage is locked", true))
      .mockResolvedValueOnce(
        payload([item({ id: "a", type: "progress", value: "chapter", detail: "1" })]),
      );
    renderPage();
    const alert = await screen.findByRole("alert");
    expect(alert.textContent).toContain("the storage is locked");
    // The failure is the whole state: no empty feed beside it.
    expect(screen.queryByText("No recent activity.")).toBeNull();
    fireEvent.click(screen.getByRole("button", { name: "Retry" }));
    expect(await screen.findByRole("list")).not.toBeNull();
    expect(mockListActivity).toHaveBeenCalledTimes(2);
  });

  it("keeps the prior feed beside a failed page load and retries the failed page", async () => {
    mockListActivity
      .mockResolvedValueOnce(payload([item({ id: "p1", type: "progress", value: "chapter", detail: "42" })], "cursor-2", 2))
      .mockRejectedValueOnce(new ApiError("locked", 503, "the storage is locked", true))
      .mockResolvedValueOnce(
        payload([item({ id: "p2", type: "rating", value: "7" })], null, 2),
      );
    renderPage();
    expect(await screen.findByRole("list")).not.toBeNull();
    fireEvent.click(await screen.findByRole("button", { name: "Load more" }));
    const alert = await screen.findByRole("alert");
    expect(alert.textContent).toContain("the storage is locked");
    // The prior row stays visible beside the error.
    expect(screen.getAllByRole("listitem").length).toBe(1);
    fireEvent.click(screen.getByRole("button", { name: "Retry" }));
    await waitFor(() => expect(rowTexts().length).toBe(2));
    // The retry re-runs the exact failed page.
    expect(mockListActivity).toHaveBeenLastCalledWith({ limit: 30, cursor: "cursor-2" });
  });
});
