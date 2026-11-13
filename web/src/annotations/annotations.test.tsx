// The edition personal annotation panels: the deferred-reading note (a
// deferral explanation that never marks the edition dropped) with the
// work-scope note selection, the negative rating and its explicit clear,
// the saved intention, the translation-candidate tag, source reference
// validation with safe links, the stale-revision conflict, and input
// retention on failure. The panels compose the frozen annotation API; no
// domain rule is re-implemented in the tests or the components.
import { cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { MemoryRouter, Route, Routes } from "react-router-dom";
import { ApiError } from "../api/client";
import { EditionPage } from "../edition/EditionPage";
import type { HistoryItemWire } from "../edition/editionApi";
import type {
  AnnotatedEditionView,
  NoteWire,
} from "./annotationsApi";

const {
  mockGetAnnotatedEdition,
  mockGetEditionHistory,
  mockGetWork,
  mockRecordProgress,
  mockSetConsumption,
  mockRetractProgress,
  mockUpdateEdition,
  mockSetIntention,
  mockSetRating,
  mockAddReference,
  mockRetireReference,
  mockAddNote,
  mockReviseNote,
  mockRetractNote,
  mockAttachTag,
  mockDetachTag,
} = vi.hoisted(() => ({
  mockGetAnnotatedEdition: vi.fn(),
  mockGetEditionHistory: vi.fn(),
  mockGetWork: vi.fn(),
  mockRecordProgress: vi.fn(),
  mockSetConsumption: vi.fn(),
  mockRetractProgress: vi.fn(),
  mockUpdateEdition: vi.fn(),
  mockSetIntention: vi.fn(),
  mockSetRating: vi.fn(),
  mockAddReference: vi.fn(),
  mockRetireReference: vi.fn(),
  mockAddNote: vi.fn(),
  mockReviseNote: vi.fn(),
  mockRetractNote: vi.fn(),
  mockAttachTag: vi.fn(),
  mockDetachTag: vi.fn(),
}));

vi.mock("../edition/editionApi", () => ({
  PROGRESS_UNITS: ["chapter", "volume", "episode", "page", "percent"],
  CONSUMPTION_VALUES: [
    "not_started",
    "in_progress",
    "paused",
    "completed",
    "dropped",
  ],
  CONSUMPTION_STATUS_OPTIONS: [
    "not_started",
    "in_progress",
    "paused",
    "completed",
    "dropped",
    "unknown",
  ],
  EDITION_KINDS: [
    "manga",
    "manhwa",
    "manhua",
    "web_novel",
    "light_novel",
    "book",
    "anime",
    "movie",
    "tv",
    "game",
    "visual_novel",
    "other",
    "unknown",
  ],
  getEdition: (...args: unknown[]) => mockGetAnnotatedEdition(...args),
  getEditionHistory: (...args: unknown[]) => mockGetEditionHistory(...args),
  recordProgress: (...args: unknown[]) => mockRecordProgress(...args),
  setConsumption: (...args: unknown[]) => mockSetConsumption(...args),
  retractProgress: (...args: unknown[]) => mockRetractProgress(...args),
  updateEdition: (...args: unknown[]) => mockUpdateEdition(...args),
}));

vi.mock("../work/workApi", () => ({
  getWork: (...args: unknown[]) => mockGetWork(...args),
}));

vi.mock("./annotationsApi", () => ({
  INTENTION_OPTIONS: ["none", "interested", "saved", "planned", "not_interested"],
  REFERENCE_ROLES: [
    "publication",
    "metadata",
    "translation",
    "release_watcher",
    "translation_project",
    "content",
  ],
  getAnnotatedEdition: (...args: unknown[]) => mockGetAnnotatedEdition(...args),
  setIntention: (...args: unknown[]) => mockSetIntention(...args),
  setRating: (...args: unknown[]) => mockSetRating(...args),
  addReference: (...args: unknown[]) => mockAddReference(...args),
  retireReference: (...args: unknown[]) => mockRetireReference(...args),
  addNote: (...args: unknown[]) => mockAddNote(...args),
  reviseNote: (...args: unknown[]) => mockReviseNote(...args),
  retractNote: (...args: unknown[]) => mockRetractNote(...args),
  attachTag: (...args: unknown[]) => mockAttachTag(...args),
  detachTag: (...args: unknown[]) => mockDetachTag(...args),
}));

function note(overrides: Partial<NoteWire> = {}): NoteWire {
  return {
    id: "n-1",
    target_id: "e-1",
    text: "Deferred for later.",
    spoiler: false,
    quote: null,
    location: null,
    revision: 1,
    recorded_at: "2026-01-02T00:00:00Z",
    actor: { principal: "owner", origin: "web" },
    ...overrides,
  };
}

function edition(overrides: Partial<AnnotatedEditionView> = {}): AnnotatedEditionView {
  return {
    id: "e-1",
    work_id: "w-1",
    kind: "manga",
    language: "ja",
    other_label: null,
    revision: 3,
    titles: [
      {
        id: "t-1",
        text: "Manga Title",
        language: "ja",
        kind: "original",
        preferred: true,
        provenance: "manual",
      },
    ],
    publication: { value: "ongoing", source: "manual" },
    consumption: null,
    progress: {},
    intention: null,
    rating: null,
    tags: [],
    references: [],
    notes: [],
    ...overrides,
  };
}

function historyItem(
  overrides: Partial<HistoryItemWire> & { id: string; type: string },
): HistoryItemWire {
  return {
    work_id: "w-1",
    edition_id: "e-1",
    title: null,
    value: null,
    detail: null,
    label: "effective",
    reason: null,
    supersedes_id: null,
    observed_at: "2026-01-02T00:00:00Z",
    recorded_at: "2026-01-02T00:00:01Z",
    ...overrides,
  };
}

function renderPage(): void {
  render(
    <MemoryRouter initialEntries={["/editions/e-1"]}>
      <Routes>
        <Route path="/editions/:id" element={<EditionPage />} />
      </Routes>
    </MemoryRouter>,
  );
}

beforeEach(() => {
  vi.clearAllMocks();
  mockGetAnnotatedEdition.mockImplementation(async (id: string) =>
    id === "e-1"
      ? edition()
      : Promise.reject(new ApiError("not_found", 404, "no such edition", false)),
  );
  mockGetEditionHistory.mockResolvedValue({ items: [], next_cursor: null, total: 0 });
  mockGetWork.mockResolvedValue({
    id: "w-1",
    revision: 1,
    created_at: "2026-01-01T00:00:00Z",
    updated_at: "2026-01-01T00:00:00Z",
    archived_at: null,
    titles: [
      {
        id: "t-9",
        text: "Work Title",
        language: null,
        kind: "original",
        preferred: true,
        provenance: "manual",
      },
    ],
    editions: [],
  });
  mockRecordProgress.mockResolvedValue(edition({ revision: 4 }));
  mockSetConsumption.mockResolvedValue(edition({ revision: 4 }));
  mockRetractProgress.mockResolvedValue(edition({ revision: 4 }));
  mockUpdateEdition.mockResolvedValue(edition({ revision: 4 }));
  mockSetIntention.mockImplementation(
    async (input: { value: string; reason?: string; expectedRevision: number }) =>
      edition({
        revision: input.expectedRevision + 1,
        intention: {
          value: input.value,
          reason: input.reason ?? null,
          observed_at: "2026-01-03T00:00:00Z",
          recorded_at: "2026-01-03T00:00:01Z",
          actor: { principal: "owner", origin: "web" },
          supersedes_id: null,
        },
      }),
  );
  mockSetRating.mockImplementation(
    async (input: { value: number | null; comment?: string; expectedRevision: number }) =>
      edition({
        revision: input.expectedRevision + 1,
        rating:
          input.value === null
            ? null
            : {
                value: input.value,
                comment: input.comment ?? null,
                observed_at: "2026-01-03T00:00:00Z",
                recorded_at: "2026-01-03T00:00:01Z",
                actor: { principal: "owner", origin: "web" },
                supersedes_id: null,
              },
      }),
  );
  mockAddNote.mockImplementation(
    async (input: {
      target: "work" | "edition";
      targetId: string;
      text: string;
      spoiler?: boolean;
      quote?: string;
      location?: string;
    }) =>
      input.target === "edition"
        ? edition({
            revision: 3,
            notes: [
              note({
                text: input.text,
                spoiler: input.spoiler ?? false,
                quote: input.quote ?? null,
                location: input.location ?? null,
              }),
            ],
          })
        : {
            id: input.targetId,
            revision: 1,
            created_at: "2026-01-01T00:00:00Z",
            updated_at: "2026-01-01T00:00:00Z",
            archived_at: null,
            titles: [],
            editions: [],
          },
  );
  mockReviseNote.mockImplementation(
    async (input: { noteId: string; text: string; expectedRevision: number }) =>
      edition({
        notes: [note({ text: input.text, revision: input.expectedRevision + 1 })],
      }),
  );
  mockRetractNote.mockResolvedValue(edition({ notes: [] }));
  mockAttachTag.mockImplementation(
    async (input: { tag: string; expectedRevision: number }) =>
      edition({ revision: input.expectedRevision + 1, tags: [input.tag] }),
  );
  mockDetachTag.mockImplementation(async () => edition({ revision: 4, tags: [] }));
  mockAddReference.mockImplementation(async () =>
    edition({
      revision: 4,
      references: [
        {
          id: "r-1",
          provider: "myanimelist",
          external_id: "1234",
          url: null,
          role: "publication",
          authoritative: false,
          language: null,
          provenance: "manual",
          retired_at: null,
        },
      ],
    }),
  );
  mockRetireReference.mockResolvedValue(edition({ revision: 4, references: [] }));
});

afterEach(() => {
  cleanup();
});

describe("intention panel", () => {
  it("sets the saved intention with a reason for the deferred-reading flow", async () => {
    renderPage();
    expect(await screen.findByText("Intention: none recorded")).not.toBeNull();
    fireEvent.change(screen.getByLabelText("Intention"), { target: { value: "saved" } });
    fireEvent.change(screen.getByLabelText("Intention reason"), {
      target: { value: "reading manga on my phone is inconvenient" },
    });
    fireEvent.click(screen.getByRole("button", { name: "Set intention" }));
    expect(
      await screen.findByText(
        "Intention: saved — reading manga on my phone is inconvenient",
      ),
    ).not.toBeNull();
    expect(mockSetIntention).toHaveBeenCalledWith({
      editionId: "e-1",
      value: "saved",
      reason: "reading manga on my phone is inconvenient",
      expectedRevision: 3,
    });
  });

  it("offers the explicit none retraction and renders the current intention", async () => {
    mockGetAnnotatedEdition.mockResolvedValueOnce(
      edition({
        intention: {
          value: "planned",
          reason: null,
          observed_at: "2026-01-01T00:00:00Z",
          recorded_at: "2026-01-01T00:00:01Z",
          actor: { principal: "owner", origin: "web" },
          supersedes_id: null,
        },
      }),
    );
    mockSetIntention.mockResolvedValueOnce(edition({ revision: 4, intention: null }));
    renderPage();
    expect(await screen.findByText("Intention: planned")).not.toBeNull();
    fireEvent.change(screen.getByLabelText("Intention"), { target: { value: "none" } });
    fireEvent.click(screen.getByRole("button", { name: "Set intention" }));
    expect(await screen.findByText("Intention: none recorded")).not.toBeNull();
    expect(mockSetIntention).toHaveBeenCalledWith(
      expect.objectContaining({ value: "none", expectedRevision: 3 }),
    );
  });

  it("keeps the typed reason on a stale-revision conflict", async () => {
    mockSetIntention.mockRejectedValueOnce(
      new ApiError("conflict", 409, "the request changed since the edition was loaded", false),
    );
    renderPage();
    expect(await screen.findByText("Manga Title")).not.toBeNull();
    fireEvent.change(screen.getByLabelText("Intention reason"), {
      target: { value: "still thinking" },
    });
    fireEvent.click(screen.getByRole("button", { name: "Set intention" }));
    const alert = await screen.findByRole("alert");
    expect(alert.textContent).toContain("the request changed since the edition was loaded");
    expect((screen.getByLabelText("Intention reason") as HTMLInputElement).value).toBe(
      "still thinking",
    );
    expect(screen.getByText("Intention: none recorded")).not.toBeNull();
  });
});

describe("rating panel", () => {
  it("records a negative rating with a comment", async () => {
    renderPage();
    expect(await screen.findByText("Rating: no rating")).not.toBeNull();
    fireEvent.change(screen.getByLabelText("Rating"), { target: { value: "2" } });
    fireEvent.change(screen.getByLabelText("Rating comment"), {
      target: { value: "slow start, disappointing" },
    });
    fireEvent.click(screen.getByRole("button", { name: "Set rating" }));
    expect(
      await screen.findByText("Rating: 2/10 — slow start, disappointing"),
    ).not.toBeNull();
    expect(mockSetRating).toHaveBeenCalledWith({
      editionId: "e-1",
      value: 2,
      comment: "slow start, disappointing",
      expectedRevision: 3,
    });
  });

  it("clears the rating with an explicit null observation", async () => {
    mockGetAnnotatedEdition.mockResolvedValueOnce(
      edition({
        rating: {
          value: 8,
          comment: null,
          observed_at: "2026-01-01T00:00:00Z",
          recorded_at: "2026-01-01T00:00:01Z",
          actor: { principal: "owner", origin: "web" },
          supersedes_id: null,
        },
      }),
    );
    mockSetRating.mockResolvedValueOnce(edition({ revision: 4, rating: null }));
    renderPage();
    expect(await screen.findByText("Rating: 8/10")).not.toBeNull();
    fireEvent.click(screen.getByRole("button", { name: "Set rating" }));
    expect(await screen.findByText("Rating: no rating")).not.toBeNull();
    expect(mockSetRating).toHaveBeenCalledWith(
      expect.objectContaining({ value: null, expectedRevision: 3 }),
    );
  });

  it("corrects a mistaken rating by naming the superseded observation", async () => {
    mockGetAnnotatedEdition.mockResolvedValueOnce(
      edition({
        rating: {
          value: 8,
          comment: null,
          observed_at: "2026-01-01T00:00:00Z",
          recorded_at: "2026-01-01T00:00:01Z",
          actor: { principal: "owner", origin: "web" },
          supersedes_id: null,
        },
      }),
    );
    mockGetEditionHistory.mockResolvedValueOnce({
      items: [
        historyItem({ id: "r-8", type: "rating", value: "8", detail: null }),
        historyItem({
          id: "r-5",
          type: "rating",
          value: "5",
          detail: null,
          label: "superseded",
        }),
      ],
      next_cursor: null,
      total: 2,
    });
    renderPage();
    expect(await screen.findByText("Rating: 8/10")).not.toBeNull();
    fireEvent.change(screen.getByLabelText("Rating action"), { target: { value: "correct" } });
    const select = screen.getByLabelText("Rating to correct") as HTMLSelectElement;
    // Only the effective rating row is offered; the superseded one is not.
    expect([...select.options].map((o) => o.value)).toEqual(["", "r-8"]);
    fireEvent.change(select, { target: { value: "r-8" } });
    fireEvent.change(screen.getByLabelText("Rating"), { target: { value: "6" } });
    fireEvent.click(screen.getByRole("button", { name: "Correct rating" }));
    await waitFor(() => expect(mockSetRating).toHaveBeenCalledTimes(1));
    const input = mockSetRating.mock.calls[0][0];
    expect(input).toMatchObject({ value: 6, supersedesId: "r-8", expectedRevision: 3 });
  });

  it("keeps the typed comment on a failed rating", async () => {
    mockSetRating.mockRejectedValueOnce(
      new ApiError("conflict", 409, "the request changed since the edition was loaded", false),
    );
    renderPage();
    expect(await screen.findByText("Rating: no rating")).not.toBeNull();
    fireEvent.change(screen.getByLabelText("Rating"), { target: { value: "4" } });
    fireEvent.change(screen.getByLabelText("Rating comment"), {
      target: { value: "not there yet" },
    });
    fireEvent.click(screen.getByRole("button", { name: "Set rating" }));
    await screen.findByRole("alert");
    expect((screen.getByLabelText("Rating comment") as HTMLInputElement).value).toBe(
      "not there yet",
    );
  });
});

describe("tag panel", () => {
  it("attaches the translation-candidate tag and detaches it again", async () => {
    renderPage();
    expect(await screen.findByText("No tags.")).not.toBeNull();
    fireEvent.change(screen.getByLabelText("Tag"), {
      target: { value: "manga-translation-candidate" },
    });
    fireEvent.click(screen.getByRole("button", { name: "Attach tag" }));
    expect(await screen.findByText("manga-translation-candidate")).not.toBeNull();
    expect(mockAttachTag).toHaveBeenCalledWith({
      target: "edition",
      targetId: "e-1",
      tag: "manga-translation-candidate",
      expectedRevision: 3,
    });
    fireEvent.click(screen.getByRole("button", { name: "Detach manga-translation-candidate" }));
    expect(await screen.findByText("No tags.")).not.toBeNull();
    expect(mockDetachTag).toHaveBeenCalledWith({
      target: "edition",
      targetId: "e-1",
      tag: "manga-translation-candidate",
      expectedRevision: 4,
    });
  });

  it("keeps the typed tag on an identity conflict", async () => {
    mockAttachTag.mockRejectedValueOnce(
      new ApiError("conflict", 409, "the tag assignment changed since the edition was loaded", false),
    );
    renderPage();
    expect(await screen.findByText("No tags.")).not.toBeNull();
    fireEvent.change(screen.getByLabelText("Tag"), { target: { value: "favorite" } });
    fireEvent.click(screen.getByRole("button", { name: "Attach tag" }));
    const alert = await screen.findByRole("alert");
    expect(alert.textContent).toContain("the tag assignment changed");
    expect((screen.getByLabelText("Tag") as HTMLInputElement).value).toBe("favorite");
  });
});

describe("notes panel", () => {
  it("adds the deferred-reading note to the edition and renders it as plain text", async () => {
    renderPage();
    expect(await screen.findByText("No notes on this edition.")).not.toBeNull();
    fireEvent.change(screen.getByLabelText("Note text"), {
      target: {
        value:
          "Interesting anime. Original is manga. Saved for later because reading manga on my phone is inconvenient.",
      },
    });
    fireEvent.click(screen.getByRole("button", { name: "Add note" }));
    expect(
      await screen.findByText(
        "Interesting anime. Original is manga. Saved for later because reading manga on my phone is inconvenient.",
      ),
    ).not.toBeNull();
    expect(mockAddNote).toHaveBeenCalledWith({
      target: "edition",
      targetId: "e-1",
      text:
        "Interesting anime. Original is manga. Saved for later because reading manga on my phone is inconvenient.",
      spoiler: false,
    });
  });

  it("selects the work scope for a note and reports success without a listing", async () => {
    renderPage();
    expect(await screen.findByText("No notes on this edition.")).not.toBeNull();
    fireEvent.change(screen.getByLabelText("Note target"), { target: { value: "work" } });
    fireEvent.change(screen.getByLabelText("Note text"), {
      target: { value: "A thought about the whole work." },
    });
    fireEvent.click(screen.getByRole("button", { name: "Add note" }));
    expect(
      await screen.findByText(
        "Note saved to the work; work-scoped notes are not listed on this screen.",
      ),
    ).not.toBeNull();
    expect(mockAddNote).toHaveBeenCalledWith({
      target: "work",
      targetId: "w-1",
      text: "A thought about the whole work.",
      spoiler: false,
    });
  });

  it("renders note text escaped, never as raw HTML", async () => {
    mockGetAnnotatedEdition.mockResolvedValueOnce(
      edition({ notes: [note({ text: "<img src=x onerror=alert(1)>", quote: null })] }),
    );
    renderPage();
    expect(
      await screen.findByText("<img src=x onerror=alert(1)>"),
    ).not.toBeNull();
    expect(document.querySelector("img")).toBeNull();
  });

  it("edits a note through a guarded revision and keeps the text on a stale conflict", async () => {
    mockGetAnnotatedEdition.mockResolvedValueOnce(edition({ notes: [note()] }));
    mockReviseNote.mockRejectedValueOnce(
      new ApiError("conflict", 409, "the note changed since it was loaded", false),
    );
    renderPage();
    expect(await screen.findByText("Deferred for later.")).not.toBeNull();
    fireEvent.click(screen.getByRole("button", { name: "Edit note 1" }));
    fireEvent.change(screen.getByLabelText("Note text"), {
      target: { value: "Deferred, and why." },
    });
    fireEvent.click(screen.getByRole("button", { name: "Save revision" }));
    const alert = await screen.findByRole("alert");
    expect(alert.textContent).toContain("the note changed since it was loaded");
    // The edit input is retained for the retry; the prior note is intact
    // (the text also appears as the note select's option label).
    expect((screen.getByLabelText("Note text") as HTMLTextAreaElement).value).toBe(
      "Deferred, and why.",
    );
    expect(screen.getAllByText("Deferred for later.").length).toBeGreaterThan(0);
  });

  it("saves a note revision that appends an immutable revision", async () => {
    mockGetAnnotatedEdition.mockResolvedValueOnce(edition({ notes: [note()] }));
    renderPage();
    expect(await screen.findByText("Deferred for later.")).not.toBeNull();
    fireEvent.click(screen.getByRole("button", { name: "Edit note 1" }));
    fireEvent.change(screen.getByLabelText("Note text"), {
      target: { value: "Deferred, and why." },
    });
    fireEvent.change(screen.getByLabelText("Note location"), {
      target: { value: "chapter 12" },
    });
    fireEvent.click(screen.getByRole("button", { name: "Save revision" }));
    expect(await screen.findByText("Deferred, and why.")).not.toBeNull();
    expect(mockReviseNote).toHaveBeenCalledWith({
      noteId: "n-1",
      expectedRevision: 1,
      text: "Deferred, and why.",
      spoiler: false,
      location: "chapter 12",
    });
    expect(screen.getByText(/revision 2/)).not.toBeNull();
  });

  it("retracts a note with the note's revision guard and leaves no note behind", async () => {
    mockGetAnnotatedEdition.mockResolvedValueOnce(edition({ notes: [note()] }));
    renderPage();
    expect(await screen.findByText("Deferred for later.")).not.toBeNull();
    fireEvent.click(screen.getByRole("button", { name: "Retract note 1" }));
    fireEvent.change(screen.getByLabelText("Note retraction reason"), {
      target: { value: "mistaken entry" },
    });
    fireEvent.click(screen.getByRole("button", { name: "Retract note" }));
    expect(await screen.findByText("No notes on this edition.")).not.toBeNull();
    expect(mockRetractNote).toHaveBeenCalledWith({
      noteId: "n-1",
      expectedRevision: 1,
      reason: "mistaken entry",
    });
  });

  it("requires note text and keeps the panel usable when it is missing", async () => {
    renderPage();
    expect(await screen.findByText("No notes on this edition.")).not.toBeNull();
    fireEvent.click(screen.getByRole("button", { name: "Add note" }));
    const alert = await screen.findByRole("alert");
    expect(alert.textContent).toContain("Enter the note text.");
    expect(mockAddNote).not.toHaveBeenCalled();
  });
});

describe("references panel", () => {
  it("adds a sourced reference and renders it with its provenance", async () => {
    renderPage();
    expect(await screen.findByText("No references.")).not.toBeNull();
    fireEvent.change(screen.getByLabelText("Provider"), { target: { value: "myanimelist" } });
    fireEvent.change(screen.getByLabelText("External ID"), { target: { value: "1234" } });
    fireEvent.change(screen.getByLabelText("Reference provenance"), {
      target: { value: "manual" },
    });
    fireEvent.click(screen.getByRole("button", { name: "Add reference" }));
    expect(await screen.findByText("myanimelist")).not.toBeNull();
    expect(mockAddReference).toHaveBeenCalledWith({
      editionId: "e-1",
      provider: "myanimelist",
      externalId: "1234",
      role: "publication",
      authoritative: false,
      provenance: "manual",
      expectedRevision: 3,
    });
  });

  it("refuses a reference with both or neither identity before calling the API", async () => {
    renderPage();
    expect(await screen.findByText("No references.")).not.toBeNull();
    fireEvent.change(screen.getByLabelText("Provider"), { target: { value: "myanimelist" } });
    fireEvent.change(screen.getByLabelText("External ID"), { target: { value: "1234" } });
    fireEvent.change(screen.getByLabelText("URL"), {
      target: { value: "https://myanimelist.net/manga/1234" },
    });
    fireEvent.change(screen.getByLabelText("Reference provenance"), {
      target: { value: "manual" },
    });
    fireEvent.click(screen.getByRole("button", { name: "Add reference" }));
    const alert = await screen.findByRole("alert");
    expect(alert.textContent).toContain("Provide exactly one of external ID or URL.");
    expect(mockAddReference).not.toHaveBeenCalled();
    // The typed input is retained for the correction.
    expect((screen.getByLabelText("External ID") as HTMLInputElement).value).toBe("1234");
  });

  it("shows the server's source validation error and keeps the input", async () => {
    mockAddReference.mockRejectedValueOnce(
      new ApiError("invalid_argument", 400, 'url "ftp://example.com" is not http(s)', false),
    );
    renderPage();
    expect(await screen.findByText("No references.")).not.toBeNull();
    fireEvent.change(screen.getByLabelText("Provider"), { target: { value: "example" } });
    fireEvent.change(screen.getByLabelText("URL"), {
      target: { value: "ftp://example.com" },
    });
    fireEvent.change(screen.getByLabelText("Reference provenance"), {
      target: { value: "manual" },
    });
    fireEvent.click(screen.getByRole("button", { name: "Add reference" }));
    const alert = await screen.findByRole("alert");
    expect(alert.textContent).toContain('url "ftp://example.com" is not http(s)');
    expect((screen.getByLabelText("URL") as HTMLInputElement).value).toBe("ftp://example.com");
  });

  it("links only safe http(s) URLs with noopener and without credentials", async () => {
    mockGetAnnotatedEdition.mockResolvedValueOnce(
      edition({
        references: [
          {
            id: "r-1",
            provider: "myanimelist",
            external_id: null,
            url: "https://myanimelist.net/manga/1234",
            role: "publication",
            authoritative: true,
            language: "ja",
            provenance: "manual",
            retired_at: null,
          },
        ],
      }),
    );
    renderPage();
    const link = await screen.findByRole("link", {
      name: "https://myanimelist.net/manga/1234",
    });
    expect(link.getAttribute("href")).toBe("https://myanimelist.net/manga/1234");
    expect(link.getAttribute("rel")).toBe("noopener noreferrer");
    expect(link.getAttribute("target")).toBe("_blank");
    expect(screen.getByText("authoritative")).not.toBeNull();
  });

  it("retires a reference and keeps the prior list on a failed retirement", async () => {
    mockGetAnnotatedEdition.mockResolvedValueOnce(
      edition({
        references: [
          {
            id: "r-1",
            provider: "myanimelist",
            external_id: "1234",
            url: null,
            role: "publication",
            authoritative: false,
            language: null,
            provenance: "manual",
            retired_at: null,
          },
        ],
      }),
    );
    mockRetireReference.mockRejectedValueOnce(
      new ApiError("conflict", 409, "the request changed since the edition was loaded", false),
    );
    renderPage();
    expect(await screen.findByText("myanimelist")).not.toBeNull();
    fireEvent.change(screen.getByLabelText("Reference"), { target: { value: "r-1" } });
    fireEvent.click(screen.getByRole("button", { name: "Retire reference" }));
    const alert = await screen.findByRole("alert");
    expect(alert.textContent).toContain("the request changed since the edition was loaded");
    // The failed retirement leaves the prior reference list intact.
    expect(screen.getByText("myanimelist")).not.toBeNull();
    expect(mockRetireReference).toHaveBeenCalledWith(
      expect.objectContaining({ editionId: "e-1", referenceId: "r-1", expectedRevision: 3 }),
    );
  });
});

describe("panel composition", () => {
  it("keeps the failed panel beside the prior edition state", async () => {
    mockGetAnnotatedEdition.mockResolvedValueOnce(
      edition({
        intention: {
          value: "saved",
          reason: null,
          observed_at: "2026-01-01T00:00:00Z",
          recorded_at: "2026-01-01T00:00:01Z",
          actor: { principal: "owner", origin: "web" },
          supersedes_id: null,
        },
      }),
    );
    mockSetIntention.mockRejectedValueOnce(
      new ApiError("locked", 503, "the storage is locked", true),
    );
    renderPage();
    expect(await screen.findByText("Intention: saved")).not.toBeNull();
    fireEvent.change(screen.getByLabelText("Intention"), { target: { value: "planned" } });
    fireEvent.click(screen.getByRole("button", { name: "Set intention" }));
    const alert = await screen.findByRole("alert");
    expect(alert.textContent).toContain("the storage is locked");
    // The prior intention stays visible beside the retryable error.
    expect(screen.getByText("Intention: saved")).not.toBeNull();
  });
});
