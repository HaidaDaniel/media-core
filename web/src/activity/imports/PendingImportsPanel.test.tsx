// The pending-imports control: the bounded queue of unbound imported events,
// the explicit bind by edition id (never by title), a conflicting bind that
// is a distinguishable error with the input retained, the empty state, and the
// graceful degradation when the read is unavailable.
import { cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { ApiError } from "../../api/client";
import { PendingImportsPanel } from "./PendingImportsPanel";
import type { PendingImportWire } from "../../api/releases";

const { mockListPendingImports, mockBindRelease } = vi.hoisted(() => ({
  mockListPendingImports: vi.fn(),
  mockBindRelease: vi.fn(),
}));

vi.mock("../../api/releases", () => ({
  listPendingImports: (...args: unknown[]) => mockListPendingImports(...args),
  bindRelease: (...args: unknown[]) => mockBindRelease(...args),
}));

function pendingItem(
  overrides: Partial<PendingImportWire> & { id: string },
): PendingImportWire {
  return {
    event_id: `evt-${overrides.id}`,
    event_type: "release.added",
    namespace: "release-watcher",
    publication_id: `pub-${overrides.id}`,
    external_work_ref: null,
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

function queue(items: PendingImportWire[]) {
  return { items, next_cursor: null, total: items.length };
}

beforeEach(() => {
  vi.clearAllMocks();
  mockListPendingImports.mockResolvedValue(queue([]));
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

describe("pending imports panel", () => {
  it("lists the pending queue and offers the explicit bind by edition id", async () => {
    mockListPendingImports.mockResolvedValueOnce(
      queue([
        pendingItem({
          id: "1",
          release_number: "42",
          release_title: "Chapter 42",
          current_count: null,
          count_delta: null,
        }),
        pendingItem({ id: "2", publication_id: null }),
      ]),
    );
    render(<PendingImportsPanel />);
    expect(await screen.findByText("2 pending imports")).not.toBeNull();
    // The bindable row shows its publication id and release fields.
    expect(screen.getByText("pub-1")).not.toBeNull();
    expect(screen.getByText("#42 · Chapter 42")).not.toBeNull();
    // Only the row with a publication id offers a bind control.
    expect(screen.getAllByLabelText("Edition id").length).toBe(1);
    expect(screen.getByRole("button", { name: "Bind" })).not.toBeNull();
    // The unbindable row is labeled as such, not fabricated.
    expect(screen.getByText(/no publication id; cannot be bound/)).not.toBeNull();
  });

  it("binds a publication to an edition and reloads the queue", async () => {
    mockListPendingImports
      .mockResolvedValueOnce(queue([pendingItem({ id: "1" })]))
      .mockResolvedValueOnce(queue([]));
    mockBindRelease.mockResolvedValueOnce({
      id: "b-1",
      namespace: "release-watcher",
      publication_id: "pub-1",
      edition_id: "e-9",
      work_id: "w-1",
      state: "active",
      created_by: "hermes",
      created_at: "2026-01-02T00:00:01Z",
      replaces_id: null,
    });
    render(<PendingImportsPanel />);
    const input = await screen.findByLabelText("Edition id");
    fireEvent.change(input, { target: { value: "e-9" } });
    fireEvent.click(screen.getByRole("button", { name: "Bind" }));
    await waitFor(() => expect(mockBindRelease).toHaveBeenCalledWith({
      publicationId: "pub-1",
      editionId: "e-9",
    }));
    // The bound event leaves the queue on reload.
    expect(await screen.findByText("0 pending imports")).not.toBeNull();
  });

  it("keeps the edition id on a conflicting bind and shows the conflict", async () => {
    mockListPendingImports.mockResolvedValue(queue([pendingItem({ id: "1" })]));
    mockBindRelease.mockRejectedValueOnce(
      new ApiError("conflict", 409, "publication is already bound to a different edition", false),
    );
    render(<PendingImportsPanel />);
    const input = await screen.findByLabelText("Edition id");
    fireEvent.change(input, { target: { value: "e-9" } });
    fireEvent.click(screen.getByRole("button", { name: "Bind" }));
    const alert = await screen.findByRole("alert");
    expect(alert.textContent).toContain("already bound to a different edition");
    // The input is retained for the retry.
    expect((screen.getByLabelText("Edition id") as HTMLInputElement).value).toBe("e-9");
  });

  it("shows the empty state when nothing is pending", async () => {
    render(<PendingImportsPanel />);
    expect(await screen.findByText("Nothing is waiting to be bound.")).not.toBeNull();
  });

  it("degrades to a note when the read is unavailable, without an error role", async () => {
    mockListPendingImports.mockRejectedValueOnce(
      new ApiError("internal_error", 0, "the API is unreachable", true),
    );
    render(<PendingImportsPanel />);
    expect(await screen.findByText(/Import history is unavailable/)).not.toBeNull();
    // The degradation is a status note, never an alert or a list.
    expect(screen.queryByRole("alert")).toBeNull();
    expect(screen.queryAllByRole("listitem").length).toBe(0);
  });
});
