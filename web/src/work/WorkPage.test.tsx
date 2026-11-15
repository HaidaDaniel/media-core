// The reachable parent-panel test for the release-history control: the panel
// is mounted per edition inside the work screen (no independent UI screen),
// so this renders the parent page and proves the labeled release history is
// reachable from it.
import { cleanup, render, screen } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { MemoryRouter, Route, Routes } from "react-router-dom";
import { WorkPage } from "./WorkPage";
import type { EditionViewWire, WorkViewWire } from "./workApi";
import type { ReleaseProjectionWire } from "../api/releases";

const {
  mockGetWork,
  mockGetEdition,
  mockWorkRelations,
  mockEditionRelations,
  mockListReleases,
} = vi.hoisted(() => ({
  mockGetWork: vi.fn(),
  mockGetEdition: vi.fn(),
  mockWorkRelations: vi.fn(),
  mockEditionRelations: vi.fn(),
  mockListReleases: vi.fn(),
}));

vi.mock("./workApi", () => ({
  getWork: (...args: unknown[]) => mockGetWork(...args),
  getEdition: (...args: unknown[]) => mockGetEdition(...args),
}));

vi.mock("../relations/relationsApi", () => ({
  workRelations: (...args: unknown[]) => mockWorkRelations(...args),
  editionRelations: (...args: unknown[]) => mockEditionRelations(...args),
  proposeRelation: () => Promise.resolve({ proposed: [], confirmed: [], revoked: [] }),
  confirmRelation: () => Promise.resolve({ proposed: [], confirmed: [], revoked: [] }),
  revokeRelation: () => Promise.resolve({ proposed: [], confirmed: [], revoked: [] }),
  recordOrigin: () => Promise.resolve({ edition_id: "e-1", claims: [] }),
}));

vi.mock("../api/releases", () => ({
  listReleases: (...args: unknown[]) => mockListReleases(...args),
}));

function work(): WorkViewWire {
  return {
    id: "w-1",
    revision: 7,
    created_at: "2026-01-01T00:00:00Z",
    updated_at: "2026-01-01T00:00:00Z",
    archived_at: null,
    titles: [
      { id: "t-1", text: "Anime Work", language: "en", kind: "original", preferred: true, provenance: "manual" },
    ],
    editions: [{ id: "e-1", kind: "anime", language: "en", revision: 1 }],
  };
}

function edition(): EditionViewWire {
  return {
    id: "e-1",
    work_id: "w-1",
    kind: "anime",
    language: "en",
    other_label: null,
    revision: 3,
    titles: [
      { id: "t-e1", text: "anime title", language: "en", kind: "original", preferred: true, provenance: "manual" },
    ],
    consumption: null,
    intention: null,
    rating: null,
    progress: {},
    notes: [],
  };
}

function projection(eventId: string, publicationId: string, number: string): ReleaseProjectionWire {
  return {
    event_id: eventId,
    event_type: "release.added",
    namespace: "release-watcher",
    publication_id: publicationId,
    edition_id: "e-1",
    work_id: "w-1",
    release_id: null,
    release_number: number,
    release_title: `Chapter ${number}`,
    release_url: null,
    current_count: null,
    count_delta: null,
    source_time: "2026-01-02T00:00:00Z",
    detection_time: null,
    created_at: "2026-01-02T00:00:01Z",
  };
}

function renderPage(): void {
  render(
    <MemoryRouter initialEntries={["/works/w-1"]}>
      <Routes>
        <Route path="/works/:id" element={<WorkPage />} />
      </Routes>
    </MemoryRouter>,
  );
}

beforeEach(() => {
  vi.clearAllMocks();
  mockGetWork.mockResolvedValue(work());
  mockGetEdition.mockResolvedValue(edition());
  mockWorkRelations.mockResolvedValue({ entity_id: "w-1", scope: "work", edges: [] });
  mockEditionRelations.mockResolvedValue({ entity_id: "e-1", scope: "edition", edges: [] });
  mockListReleases.mockResolvedValue({
    items: [projection("evt-1", "pub-alpha", "5")],
    next_cursor: null,
    total: 1,
  });
});

afterEach(() => {
  cleanup();
});

describe("work screen with the release-history panel", () => {
  it("reaches the labeled release history from the work screen", async () => {
    renderPage();
    // The work screen and the per-edition panel are both present.
    expect(await screen.findByText("Anime Work")).not.toBeNull();
    expect(screen.getByRole("heading", { name: "Releases" })).not.toBeNull();
    // The labeled release is visible, tagged by its exact publication.
    expect(await screen.findByText("1 release")).not.toBeNull();
    expect(await screen.findByText("from pub-alpha")).not.toBeNull();
    expect(await screen.findByText("#5 · Chapter 5")).not.toBeNull();
    // The read is scoped to this edition.
    expect(mockListReleases).toHaveBeenCalledWith("e-1", 50);
  });

  it("keeps the work visible when the release read is unavailable", async () => {
    mockListReleases.mockRejectedValueOnce(
      Object.assign(new Error("the API is unreachable"), { code: "internal_error" }),
    );
    renderPage();
    // The work is still there; the panel degrades to a note.
    expect(await screen.findByText("Anime Work")).not.toBeNull();
    expect(await screen.findByText(/Release history is unavailable/)).not.toBeNull();
    // No error alert is raised for the unavailable integration.
    expect(screen.queryByRole("alert")).toBeNull();
  });
});
