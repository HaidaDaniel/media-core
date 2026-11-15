// The release-history panel: the bounded projection of an edition's imported
// releases, each row labeled by the exact publication it came from, the
// producer's fields only (no fabrication, counts never summed), the empty
// state, and graceful degradation when the read is unavailable.
import { cleanup, render, screen } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { ApiError } from "../../api/client";
import { ReleasesPanel } from "./ReleasesPanel";
import type { ReleaseProjectionWire } from "../../api/releases";

const { mockListReleases } = vi.hoisted(() => ({
  mockListReleases: vi.fn(),
}));

vi.mock("../../api/releases", () => ({
  listReleases: (...args: unknown[]) => mockListReleases(...args),
}));

function projection(
  overrides: Partial<ReleaseProjectionWire> & { event_id: string },
): ReleaseProjectionWire {
  return {
    event_type: "release.added",
    namespace: "release-watcher",
    publication_id: `pub-${overrides.event_id}`,
    edition_id: "e-1",
    work_id: "w-1",
    release_id: null,
    release_number: "1",
    release_title: "Chapter 1",
    release_url: null,
    current_count: 1,
    count_delta: 1,
    source_time: "2026-01-02T00:00:00Z",
    detection_time: null,
    created_at: "2026-01-02T00:00:01Z",
    ...overrides,
  };
}

beforeEach(() => {
  vi.clearAllMocks();
  mockListReleases.mockResolvedValue({ items: [], next_cursor: null, total: 0 });
});

afterEach(() => {
  cleanup();
});

describe("releases panel", () => {
  it("lists the edition's releases labeled by their exact publication", async () => {
    mockListReleases.mockResolvedValueOnce({
      items: [
        projection({
          event_id: "evt-1",
          release_number: "7",
          release_title: "Chapter 7",
          publication_id: "pub-alpha",
          current_count: null,
          count_delta: null,
        }),
        projection({
          event_id: "evt-2",
          release_number: "8",
          release_title: "Chapter 8",
          publication_id: "pub-beta",
          current_count: null,
          count_delta: null,
        }),
      ],
      next_cursor: null,
      total: 2,
    });
    render(<ReleasesPanel editionId="e-1" />);
    expect(await screen.findByText("2 releases")).not.toBeNull();
    // Each row is labeled by the publication it came from.
    expect(screen.getByText("from pub-alpha")).not.toBeNull();
    expect(screen.getByText("from pub-beta")).not.toBeNull();
    // The producer's fields are rendered, not fabricated.
    expect(screen.getByText("#7 · Chapter 7")).not.toBeNull();
    expect(screen.getByText("#8 · Chapter 8")).not.toBeNull();
    // The read is bounded.
    expect(mockListReleases).toHaveBeenCalledWith("e-1", 50);
  });

  it("never sums counts across publications; each label stays separate", async () => {
    mockListReleases.mockResolvedValueOnce({
      items: [
        projection({ event_id: "evt-1", publication_id: "pub-a", current_count: 3, count_delta: 1 }),
        projection({ event_id: "evt-2", publication_id: "pub-b", current_count: 5, count_delta: 1 }),
      ],
      next_cursor: null,
      total: 2,
    });
    render(<ReleasesPanel editionId="e-1" />);
    expect(await screen.findByText("2 releases")).not.toBeNull();
    // The counts are shown per row, exactly as sent; no combined total.
    expect(screen.getByText(/count 3/)).not.toBeNull();
    expect(screen.getByText(/count 5/)).not.toBeNull();
    expect(screen.queryByText(/count 8/)).toBeNull();
  });

  it("shows the empty state when nothing is bound", async () => {
    render(<ReleasesPanel editionId="e-1" />);
    expect(
      await screen.findByText("No imported releases are bound to this edition."),
    ).not.toBeNull();
  });

  it("degrades to a note when the read is unavailable, without an error role", async () => {
    mockListReleases.mockRejectedValueOnce(
      new ApiError("internal_error", 0, "the API is unreachable", true),
    );
    render(<ReleasesPanel editionId="e-1" />);
    expect(await screen.findByText(/Release history is unavailable/)).not.toBeNull();
    expect(screen.queryByRole("alert")).toBeNull();
    expect(screen.queryAllByRole("listitem").length).toBe(0);
  });
});
