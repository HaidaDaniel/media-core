// The work detail screen: per-edition individual state and notes, the
// work's relations, edition-scope source navigation (anime→manga), proposed
// claims labeled as not-confirmed, the unknown-source default, cycle and
// conflict handling, labeled keyboard controls, and the failure contract
// (a failed load keeps prior state intact with a distinguishable error).
import { cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import type { JSX } from "react";
import { MemoryRouter, Route, Routes, useLocation } from "react-router-dom";
import { ApiError } from "../api/client";
import { WorkPage } from "./WorkPage";
import type {
  EditionSummaryWire,
  EditionViewWire,
  WorkViewWire,
} from "./workApi";
import type {
  OriginClaimWire,
  OriginViewWire,
  RelatedEdgeWire,
  RelatedViewWire,
} from "../relations/relationsApi";

const {
  mockGetWork,
  mockGetEdition,
  mockWorkRelations,
  mockEditionRelations,
  mockProposeRelation,
  mockConfirmRelation,
  mockRevokeRelation,
  mockRecordOrigin,
} = vi.hoisted(() => ({
  mockGetWork: vi.fn(),
  mockGetEdition: vi.fn(),
  mockWorkRelations: vi.fn(),
  mockEditionRelations: vi.fn(),
  mockProposeRelation: vi.fn(),
  mockConfirmRelation: vi.fn(),
  mockRevokeRelation: vi.fn(),
  mockRecordOrigin: vi.fn(),
}));

vi.mock("./workApi", () => ({
  getWork: (...args: unknown[]) => mockGetWork(...args),
  getEdition: (...args: unknown[]) => mockGetEdition(...args),
}));

vi.mock("../relations/relationsApi", () => ({
  workRelations: (...args: unknown[]) => mockWorkRelations(...args),
  editionRelations: (...args: unknown[]) => mockEditionRelations(...args),
  proposeRelation: (...args: unknown[]) => mockProposeRelation(...args),
  confirmRelation: (...args: unknown[]) => mockConfirmRelation(...args),
  revokeRelation: (...args: unknown[]) => mockRevokeRelation(...args),
  recordOrigin: (...args: unknown[]) => mockRecordOrigin(...args),
}));

function title(text: string, preferred = true, language: string | null = "en") {
  return {
    id: `t-${text}`,
    text,
    language,
    kind: preferred ? "original" : "alias",
    preferred,
    provenance: "manual",
  };
}

function summary(id: string, kind: string, language: string | null = "en", revision = 1): EditionSummaryWire {
  return { id, kind, language, revision };
}

function work(overrides: Partial<WorkViewWire> = {}): WorkViewWire {
  return {
    id: "w-1",
    revision: 7,
    created_at: "2026-01-01T00:00:00Z",
    updated_at: "2026-01-01T00:00:00Z",
    archived_at: null,
    titles: [title("Anime Work")],
    editions: [summary("e-1", "anime")],
    ...overrides,
  };
}

function edition(
  id: string,
  kind: string,
  language: string | null = "en",
  overrides: Partial<EditionViewWire> = {},
): EditionViewWire {
  return {
    id,
    work_id: "w-1",
    kind,
    language,
    other_label: null,
    revision: 3,
    titles: [title(`${kind} title`)],
    consumption: null,
    intention: null,
    rating: null,
    progress: {},
    notes: [],
    ...overrides,
  };
}

function relatedView(edges: RelatedEdgeWire[] = []): RelatedViewWire {
  return { entity_id: "w-1", scope: "work", edges };
}

function edge(
  relationId: string,
  type: string,
  fromId: string,
  toId: string,
  state: "proposed" | "confirmed",
): RelatedEdgeWire {
  return { relation_id: relationId, type, from_id: fromId, to_id: toId, state, depth: 1 };
}

function claim(
  id: string,
  kind: string,
  state: "proposed" | "confirmed",
  provenance: string,
): OriginClaimWire {
  return {
    id,
    kind,
    state,
    provenance,
    replaces_id: null,
    confirmed_by: null,
    confirmed_at: null,
    created_at: "2026-01-01T00:00:00Z",
    revoked_by: null,
    revoked_at: null,
  };
}

function originView(editionId: string, claims: OriginClaimWire[]): OriginViewWire {
  return { edition_id: editionId, claims };
}

function LocationProbe(): JSX.Element {
  const location = useLocation();
  return <span data-testid="url">{location.pathname}</span>;
}

function renderPage(): void {
  render(
    <MemoryRouter initialEntries={["/works/w-1"]}>
      <Routes>
        <Route path="/works/:id" element={<WorkPage />} />
      </Routes>
      <LocationProbe />
    </MemoryRouter>,
  );
}

beforeEach(() => {
  vi.clearAllMocks();
  mockGetWork.mockImplementation(async (id: string) =>
    id === "w-1"
      ? work()
      : work({ id, titles: [title(`Other Work ${id}`)], editions: [] }),
  );
  mockGetEdition.mockImplementation(async (id: string) =>
    id === "e-1"
      ? edition("e-1", "anime")
      : edition(id, "manga", "ja", { work_id: "w-2", titles: [title("Manga Title", true, "ja")] }),
  );
  mockWorkRelations.mockResolvedValue(relatedView([]));
  mockEditionRelations.mockResolvedValue(relatedView([]));
  mockProposeRelation.mockResolvedValue({ proposed: [], confirmed: [], revoked: [] });
  mockConfirmRelation.mockResolvedValue({ proposed: [], confirmed: [], revoked: [] });
  mockRevokeRelation.mockResolvedValue({ proposed: [], confirmed: [], revoked: [] });
  mockRecordOrigin.mockResolvedValue(originView("e-1", []));
});

afterEach(() => {
  cleanup();
});

describe("work detail", () => {
  it("shows the titles, each edition's individual state, and notes without combining editions", async () => {
    mockGetEdition.mockImplementationOnce(async (id: string) =>
      id === "e-1"
        ? edition("e-1", "anime", "ja", {
            consumption: { value: "in_progress", reason: null },
            intention: { value: "planned", reason: null },
            rating: { value: 8, comment: null },
            notes: [{ id: "n-1", target_id: "e-1", text: "A thought", spoiler: true, quote: null, location: null, revision: 1, recorded_at: "2026-01-01T00:00:00Z" }],
          })
        : edition(id, "manga"),
    );
    renderPage();
    expect(await screen.findByText("Anime Work")).not.toBeNull();
    expect(screen.getByText("in progress")).not.toBeNull();
    expect(screen.getByText(/intention: planned/)).not.toBeNull();
    expect(screen.getByText(/rating: 8/)).not.toBeNull();
    expect(screen.getByText("A thought")).not.toBeNull();
    expect(screen.getByText("Spoiler")).not.toBeNull();
  });

  it("follows an anime→manga adaptation in either direction and navigates to the source", async () => {
    mockEditionRelations.mockImplementation(async (id: string) =>
      id === "e-1"
        ? { entity_id: id, scope: "edition", edges: [edge("r-1", "adaptation_of", "e-1", "e-2", "confirmed")] }
        : relatedView([]),
    );
    renderPage();
    expect(await screen.findByText("This edition is an adaptation of Manga Title")).not.toBeNull();
    const link = screen.getByRole("link", { name: "Manga Title" });
    expect(link.getAttribute("href")).toBe("/editions/e-2");
    fireEvent.click(link);
    await waitFor(() => expect(screen.getByTestId("url").textContent).toBe("/editions/e-2"));
  });

  it("labels a proposed relation as not confirmed and offers a deliberate confirm", async () => {
    mockWorkRelations.mockResolvedValue(
      relatedView([edge("r-9", "related", "w-1", "w-2", "proposed")]),
    );
    renderPage();
    expect(await screen.findByText("This work is related to Other Work w-2")).not.toBeNull();
    const badge = screen.getByText("Proposed");
    expect(badge.className).toContain("work-badge-in_progress");
    expect(screen.getByRole("button", { name: "Confirm" })).not.toBeNull();
    expect(screen.queryByRole("button", { name: "Revoke" })).not.toBeNull();
  });

  it("shows the unknown-source default when no origin is recorded and never infers one", async () => {
    renderPage();
    expect(
      await screen.findByText(/Source: unknown \(no origin claim recorded for this anime on this screen yet\)\./),
    ).not.toBeNull();
  });

  it("renders the provenance of a relation created from the form", async () => {
    let workRelCalls = 0;
    mockWorkRelations.mockImplementation(async () => {
      workRelCalls += 1;
      // The reload after the propose includes the newly asserted relation.
      if (workRelCalls >= 2) {
        return relatedView([edge("r-1", "related", "w-1", "w-2", "proposed")]);
      }
      return relatedView([]);
    });
    mockProposeRelation.mockResolvedValueOnce({
      proposed: [
        {
          id: "r-1",
          scope: "work",
          type: "related",
          from_id: "w-1",
          to_id: "w-2",
          state: "proposed",
          provenance: "manual",
          confidence: null,
          replaces_id: null,
          confirmed_by: null,
          confirmed_at: null,
          created_at: "2026-01-01T00:00:00Z",
          revoked_by: null,
          revoked_at: null,
        },
      ],
      confirmed: [],
      revoked: [],
    });
    renderPage();
    expect(await screen.findByText("Anime Work")).not.toBeNull();
    fireEvent.change(screen.getByLabelText("Other work id"), { target: { value: "w-2" } });
    fireEvent.change(screen.getAllByLabelText("Provenance")[0], { target: { value: "manual" } });
    fireEvent.click(screen.getByRole("button", { name: "Propose" }));
    // After the mutation the full view renders, carrying the provenance.
    expect(await screen.findByText("(manual)")).not.toBeNull();
    expect(screen.getByText("This work is related to Other Work w-2")).not.toBeNull();
  });

  it("surfaces a cycle conflict on confirm and leaves the prior state intact", async () => {
    mockWorkRelations.mockResolvedValue(
      relatedView([edge("r-9", "sequel_of", "w-1", "w-2", "proposed")]),
    );
    mockConfirmRelation.mockRejectedValueOnce(
      new ApiError("cycle_detected", 409, "sequel_of w-1 -> w-2 loops the confirmed sequel graph", false),
    );
    renderPage();
    expect(await screen.findByText("This work is a sequel of Other Work w-2")).not.toBeNull();
    fireEvent.click(screen.getByRole("button", { name: "Confirm" }));
    const alert = await screen.findByRole("alert");
    expect(alert.textContent).toContain("loops the confirmed sequel graph");
    expect(mockConfirmRelation).toHaveBeenCalledWith("r-9", 7);
    // The proposed row stays visible; nothing was silently confirmed.
    expect(screen.getByText("This work is a sequel of Other Work w-2")).not.toBeNull();
    expect(screen.getByText("Proposed")).not.toBeNull();
  });

  it("keeps the propose form input on a server failure and records the origin on success", async () => {
    mockProposeRelation.mockRejectedValueOnce(
      new ApiError("conflict", 409, "the request changed since the candidates were resolved", false),
    );
    renderPage();
    expect(await screen.findByText("Anime Work")).not.toBeNull();
    fireEvent.change(screen.getByLabelText("Other work id"), { target: { value: "w-2" } });
    fireEvent.change(screen.getAllByLabelText("Provenance")[0], { target: { value: "manual" } });
    fireEvent.click(screen.getByRole("button", { name: "Propose" }));
    const alert = await screen.findByRole("alert");
    expect(alert.textContent).toContain("the request changed since the candidates were resolved");
    // Input retained for the retry.
    expect((screen.getByLabelText("Other work id") as HTMLInputElement).value).toBe("w-2");
    expect((screen.getAllByLabelText("Provenance")[0] as HTMLInputElement).value).toBe("manual");

    mockRecordOrigin.mockResolvedValueOnce(
      originView("e-1", [claim("o-1", "original", "confirmed", "manual")]),
    );
    fireEvent.change(screen.getAllByLabelText("Provenance")[1], { target: { value: "manual" } });
    fireEvent.click(screen.getByRole("button", { name: "Record origin" }));
    expect(await screen.findByText("Source: original")).not.toBeNull();
    expect(mockRecordOrigin).toHaveBeenCalledWith(
      expect.objectContaining({ editionId: "e-1", kind: "original", state: "proposed" }),
    );
  });

  it("guards a symmetric relation with the lower-sorting work's revision", async () => {
    // "a-1" sorts below the current work "w-1", so the canonical from entity
    // is "a-1" and its revision must guard the mutation.
    mockGetWork.mockImplementation(async (id: string) =>
      id === "w-1"
        ? work()
        : work({ id: "a-1", revision: 5, titles: [title("Alpha Work")], editions: [] }),
    );
    mockProposeRelation.mockResolvedValueOnce({ proposed: [], confirmed: [], revoked: [] });
    renderPage();
    expect(await screen.findByText("Anime Work")).not.toBeNull();
    fireEvent.change(screen.getByLabelText("Relation"), { target: { value: "related" } });
    fireEvent.change(screen.getByLabelText("Other work id"), { target: { value: "a-1" } });
    fireEvent.change(screen.getAllByLabelText("Provenance")[0], { target: { value: "manual" } });
    fireEvent.click(screen.getByRole("button", { name: "Propose" }));
    await waitFor(() =>
      expect(mockProposeRelation).toHaveBeenCalledWith(
        expect.objectContaining({ expectedRevision: 5, toId: "a-1" }),
      ),
    );
  });

  it("labels every control for keyboard use", async () => {
    renderPage();
    expect(await screen.findByText("Anime Work")).not.toBeNull();
    expect(screen.getByLabelText("Relation")).not.toBeNull();
    expect(screen.getByLabelText("Other work id")).not.toBeNull();
    expect(screen.getByLabelText("Edition")).not.toBeNull();
    expect(screen.getByLabelText("Kind")).not.toBeNull();
    expect(screen.getAllByLabelText("State").length).toBe(2);
    expect(screen.getAllByLabelText("Provenance").length).toBe(2);
  });
});

describe("work failure contract", () => {
  it("shows a failed first load as a distinguishable error and recovers on retry", async () => {
    mockGetWork.mockRejectedValueOnce(new ApiError("not_found", 404, "no such work", false));
    renderPage();
    const alert = await screen.findByRole("alert");
    expect(alert.textContent).toContain("no such work");
    mockGetWork.mockImplementation(async (id: string) =>
      id === "w-1" ? work() : work({ id, titles: [title(`Other Work ${id}`)], editions: [] }),
    );
    fireEvent.click(screen.getByRole("button", { name: "Retry" }));
    expect(await screen.findByText("Anime Work")).not.toBeNull();
  });

  it("keeps the work visible when an edition detail fails, without inventing state", async () => {
    mockGetEdition.mockImplementationOnce(async (id: string) => {
      if (id === "e-1") throw new ApiError("storage_error", 500, "the storage failed", false);
      return edition(id, "manga");
    });
    renderPage();
    expect(await screen.findByText("Anime Work")).not.toBeNull();
    const alert = await screen.findByRole("alert");
    expect(alert.textContent).toContain("the storage failed");
    // No consumption/intention state is fabricated for the failed edition.
    expect(screen.queryByText(/intention:/)).toBeNull();
  });

  it("keeps the prior work beside a distinguishable error when a refresh fails", async () => {
    mockGetWork
      .mockResolvedValueOnce(work())
      .mockRejectedValueOnce(new ApiError("locked", 503, "the storage is locked", true));
    mockProposeRelation.mockResolvedValueOnce({ proposed: [], confirmed: [], revoked: [] });
    renderPage();
    expect(await screen.findByText("Anime Work")).not.toBeNull();
    fireEvent.change(screen.getByLabelText("Other work id"), { target: { value: "w-2" } });
    fireEvent.change(screen.getAllByLabelText("Provenance")[0], { target: { value: "manual" } });
    fireEvent.click(screen.getByRole("button", { name: "Propose" }));
    // The reload after the successful propose fails, but the prior work stays
    // visible beside the distinguishable refresh banner.
    const alert = await screen.findByRole("alert");
    expect(alert.textContent).toContain("could not be refreshed");
    expect(screen.getByText("Anime Work")).not.toBeNull();
  });
});
