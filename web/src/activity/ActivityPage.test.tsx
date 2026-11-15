// The reachable parent-panel test for the pending-imports control: the
// control is mounted inside the recent-activity screen (no independent UI
// screen), so this renders the parent page and proves the pending queue and
// the explicit bind are reachable from it.
import { cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { MemoryRouter } from "react-router-dom";
import { ActivityPage } from "./ActivityPage";
import type { ActivityItemWire, ActivityListWire } from "./activityApi";
import type { PendingImportWire } from "../api/releases";

const { mockListActivity, mockListPendingImports, mockBindRelease } = vi.hoisted(() => ({
  mockListActivity: vi.fn(),
  mockListPendingImports: vi.fn(),
  mockBindRelease: vi.fn(),
}));

vi.mock("./activityApi", () => ({
  listActivity: (...args: unknown[]) => mockListActivity(...args),
}));

vi.mock("../api/releases", () => ({
  listPendingImports: (...args: unknown[]) => mockListPendingImports(...args),
  bindRelease: (...args: unknown[]) => mockBindRelease(...args),
}));

function activityItem(id: string): ActivityItemWire {
  return {
    id,
    type: "progress",
    work_id: "w-1",
    edition_id: "e-1",
    title: "Manga Title",
    value: "chapter",
    detail: "12",
    label: "effective",
    reason: null,
    supersedes_id: null,
    replaces_id: null,
    observed_at: null,
    recorded_at: "2026-01-02T00:00:00Z",
    actor: null,
    notes: [],
    references: [],
  };
}

function feed(items: ActivityItemWire[]): ActivityListWire {
  return { items, next_cursor: null, total: items.length };
}

function pendingItem(id: string): PendingImportWire {
  return {
    id,
    event_id: `evt-${id}`,
    event_type: "release.added",
    namespace: "release-watcher",
    publication_id: `pub-${id}`,
    external_work_ref: null,
    release_id: null,
    release_number: "3",
    release_title: "Chapter 3",
    release_url: null,
    current_count: 3,
    count_delta: 1,
    source_time: "2026-01-02T00:00:00Z",
    detection_time: null,
    created_at: "2026-01-02T00:00:01Z",
  };
}

function renderPage(): void {
  render(
    <MemoryRouter>
      <ActivityPage />
    </MemoryRouter>,
  );
}

beforeEach(() => {
  vi.clearAllMocks();
  mockListActivity.mockResolvedValue(feed([activityItem("a-1")]));
  mockListPendingImports.mockResolvedValue({
    items: [pendingItem("1")],
    next_cursor: null,
    total: 1,
  });
  mockBindRelease.mockResolvedValue({
    id: "b-1",
    namespace: "release-watcher",
    publication_id: "pub-1",
    edition_id: "e-1",
    work_id: "w-1",
    state: "active",
    created_by: "hermes",
    created_at: "2026-01-02T00:00:01Z",
    replaces_id: null,
  });
});

afterEach(() => {
  cleanup();
});

describe("activity screen with the pending-imports control", () => {
  it("reaches the pending queue and the explicit bind from the activity screen", async () => {
    renderPage();
    // The personal feed and the mounted control are both present.
    expect(await screen.findByText("Manga Title")).not.toBeNull();
    expect(screen.getByRole("heading", { name: "Pending imports" })).not.toBeNull();
    // The pending event is visible with its publication id.
    expect(screen.getByText("1 pending import")).not.toBeNull();
    expect(screen.getByText("pub-1")).not.toBeNull();
    // The bind control is reachable from the parent screen.
    const input = screen.getByLabelText("Edition id");
    fireEvent.change(input, { target: { value: "e-1" } });
    fireEvent.click(screen.getByRole("button", { name: "Bind" }));
    await waitFor(() =>
      expect(mockBindRelease).toHaveBeenCalledWith({
        publicationId: "pub-1",
        editionId: "e-1",
      }),
    );
  });

  it("keeps the activity feed intact when the imports read is unavailable", async () => {
    mockListPendingImports.mockRejectedValueOnce(
      Object.assign(new Error("the API is unreachable"), {
        code: "internal_error",
      }),
    );
    renderPage();
    // The feed is still there; the control degrades to a note.
    expect(await screen.findByText("Manga Title")).not.toBeNull();
    expect(await screen.findByText(/Import history is unavailable/)).not.toBeNull();
    // No error alert is raised for the unavailable integration.
    expect(screen.queryByRole("alert")).toBeNull();
  });
});
