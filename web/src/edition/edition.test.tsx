// The edition detail screen: the fetch on (re)start, a new update and a
// mistake correction as separate actions (42→48 keeps the prior record),
// the unknown total, stale revision and invalid field failures that keep
// the input and the prior state intact, the status/publication separation,
// the retraction with a required reason, the narrow metadata corrections,
// labeled keyboard controls, and the no-bundling invariant (no notes,
// ratings or relation forms on this screen).
import { cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { MemoryRouter, Route, Routes } from "react-router-dom";
import { ApiError } from "../api/client";
import { EditionPage } from "./EditionPage";
import type {
  EditionViewWire,
  HistoryItemWire,
  ProgressRecordWire,
} from "./editionApi";
import type { WorkViewWire } from "../work/workApi";

const {
  mockGetEdition,
  mockGetEditionHistory,
  mockGetWork,
  mockRecordProgress,
  mockSetConsumption,
  mockRetractProgress,
  mockUpdateEdition,
} = vi.hoisted(() => ({
  mockGetEdition: vi.fn(),
  mockGetEditionHistory: vi.fn(),
  mockGetWork: vi.fn(),
  mockRecordProgress: vi.fn(),
  mockSetConsumption: vi.fn(),
  mockRetractProgress: vi.fn(),
  mockUpdateEdition: vi.fn(),
}));

vi.mock("./editionApi", () => ({
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
  getEdition: (...args: unknown[]) => mockGetEdition(...args),
  getEditionHistory: (...args: unknown[]) => mockGetEditionHistory(...args),
  recordProgress: (...args: unknown[]) => mockRecordProgress(...args),
  setConsumption: (...args: unknown[]) => mockSetConsumption(...args),
  retractProgress: (...args: unknown[]) => mockRetractProgress(...args),
  updateEdition: (...args: unknown[]) => mockUpdateEdition(...args),
}));

vi.mock("../work/workApi", () => ({
  getWork: (...args: unknown[]) => mockGetWork(...args),
}));

function record(
  id: string,
  unit: string,
  position: string,
  total: string | null = "100",
): ProgressRecordWire {
  return { id, unit, position, total, label: null, observed_at: "2026-01-02T00:00:00Z" };
}

function edition(overrides: Partial<EditionViewWire> = {}): EditionViewWire {
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
    ...overrides,
  };
}

function work(overrides: Partial<WorkViewWire> = {}): WorkViewWire {
  return {
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
  mockGetEdition.mockImplementation(async (id: string) =>
    id === "e-1"
      ? edition()
      : Promise.reject(new ApiError("not_found", 404, "no such edition", false)),
  );
  mockGetEditionHistory.mockResolvedValue({ items: [], next_cursor: null, total: 0 });
  mockGetWork.mockImplementation(async (id: string) => work({ id }));
  mockRecordProgress.mockImplementation(
    async (input: { unit: string; position: string; total?: string; expectedRevision: number }) =>
      edition({
        revision: input.expectedRevision + 1,
        progress: { [input.unit]: record("p-new", input.unit, input.position, input.total ?? null) },
      }),
  );
  mockSetConsumption.mockImplementation(
    async (input: { value: string; reason?: string; expectedRevision: number }) =>
      edition({
        revision: input.expectedRevision + 1,
        consumption: { value: input.value, reason: input.reason ?? null },
      }),
  );
  mockRetractProgress.mockImplementation(
    async (input: { expectedRevision: number }) =>
      edition({ revision: input.expectedRevision + 1, progress: {} }),
  );
  mockUpdateEdition.mockImplementation(
    async (input: {
      kind?: string;
      language?: string;
      otherLabel?: string;
      expectedRevision: number;
    }) =>
      edition({
        revision: input.expectedRevision + 1,
        kind: input.kind ?? "manga",
        language: input.language ?? "ja",
        other_label: input.otherLabel ?? null,
      }),
  );
});

afterEach(() => {
  cleanup();
});

describe("edition detail", () => {
  it("fetches the edition, history and parent work on load and shows the identity", async () => {
    mockGetEdition.mockResolvedValueOnce(
      edition({
        consumption: { value: "in_progress", reason: null },
        progress: { chapter: record("p-42", "chapter", "42") },
      }),
    );
    mockGetEditionHistory.mockResolvedValueOnce({
      items: [historyItem({ id: "h-1", type: "progress", value: "chapter", detail: "42/100" })],
      next_cursor: null,
      total: 1,
    });
    renderPage();
    expect(await screen.findByText("Manga Title")).not.toBeNull();
    expect(mockGetEdition).toHaveBeenCalledWith("e-1");
    expect(mockGetEditionHistory).toHaveBeenCalledWith("e-1");
    expect(mockGetWork).toHaveBeenCalledWith("w-1");
    expect(
      screen.getAllByText("manga").some((el) => el.className === "work-badge"),
    ).toBe(true);
    expect(screen.getByText("Publication: ongoing (source: manual)")).not.toBeNull();
    expect(screen.getByText("chapter: 42/100")).not.toBeNull();
    expect(screen.getByText("Status: in progress")).not.toBeNull();
    const link = screen.getByRole("link", { name: "Work Title" });
    expect(link.getAttribute("href")).toBe("/works/w-1");
  });

  it("shows the unknown publication and status defaults without inventing facts", async () => {
    mockGetEdition.mockResolvedValueOnce(
      edition({ publication: null, consumption: null }),
    );
    renderPage();
    expect(await screen.findByText("Manga Title")).not.toBeNull();
    expect(screen.getByText("Publication: unknown")).not.toBeNull();
    expect(screen.getByText("Status: unknown")).not.toBeNull();
  });

  it("records a new update 42→48 and keeps the prior record in history", async () => {
    mockGetEdition.mockResolvedValueOnce(
      edition({ progress: { chapter: record("p-42", "chapter", "42") } }),
    );
    mockGetEditionHistory
      .mockResolvedValueOnce({
        items: [historyItem({ id: "h-old", type: "progress", value: "chapter", detail: "42/100" })],
        next_cursor: null,
        total: 1,
      })
      .mockResolvedValueOnce({
        items: [
          historyItem({ id: "h-new", type: "progress", value: "chapter", detail: "48" }),
          historyItem({ id: "h-old", type: "progress", value: "chapter", detail: "42/100" }),
        ],
        next_cursor: null,
        total: 2,
      });
    mockRecordProgress.mockResolvedValueOnce(
      edition({ revision: 4, progress: { chapter: record("p-48", "chapter", "48", null) } }),
    );
    renderPage();
    expect(await screen.findByText("chapter: 42/100")).not.toBeNull();
    fireEvent.change(screen.getByLabelText("Position"), { target: { value: "48" } });
    fireEvent.click(screen.getByRole("button", { name: "Record update" }));
    expect(await screen.findByText("chapter: 48")).not.toBeNull();
    expect(mockRecordProgress).toHaveBeenCalledTimes(1);
    const input = mockRecordProgress.mock.calls[0][0];
    expect(input).toMatchObject({
      editionId: "e-1",
      unit: "chapter",
      position: "48",
      expectedRevision: 3,
    });
    expect(input.supersedesId).toBeUndefined();
    expect(input.total).toBeUndefined();
    // The prior record stays in the refreshed history; nothing is deleted.
    // (Each history row also feeds the retraction select, so match by set.)
    expect(screen.getAllByText("chapter 48").length).toBeGreaterThan(0);
    expect(screen.getAllByText("chapter 42/100").length).toBeGreaterThan(0);
  });

  it("sends a mistake correction as a separate action naming the superseded record", async () => {
    mockGetEdition.mockResolvedValueOnce(
      edition({ progress: { chapter: record("p-42", "chapter", "42") } }),
    );
    mockRecordProgress.mockResolvedValueOnce(
      edition({ revision: 4, progress: { chapter: record("p-47", "chapter", "47", null) } }),
    );
    renderPage();
    expect(await screen.findByText("chapter: 42/100")).not.toBeNull();
    fireEvent.change(screen.getByLabelText("Update kind"), { target: { value: "correct" } });
    fireEvent.change(screen.getByLabelText("Record to correct"), { target: { value: "p-42" } });
    fireEvent.change(screen.getByLabelText("Position"), { target: { value: "47" } });
    fireEvent.click(screen.getByRole("button", { name: "Correct mistake" }));
    expect(await screen.findByText("chapter: 47")).not.toBeNull();
    const input = mockRecordProgress.mock.calls[0][0];
    expect(input).toMatchObject({
      unit: "chapter",
      position: "47",
      supersedesId: "p-42",
      expectedRevision: 3,
    });
  });

  it("keeps the input and the prior state on a stale revision conflict", async () => {
    mockGetEdition.mockResolvedValueOnce(
      edition({ progress: { chapter: record("p-42", "chapter", "42") } }),
    );
    mockRecordProgress.mockRejectedValueOnce(
      new ApiError("conflict", 409, "the request changed since the edition was loaded", false),
    );
    renderPage();
    expect(await screen.findByText("chapter: 42/100")).not.toBeNull();
    fireEvent.change(screen.getByLabelText("Position"), { target: { value: "48" } });
    fireEvent.click(screen.getByRole("button", { name: "Record update" }));
    const alert = await screen.findByRole("alert");
    expect(alert.textContent).toContain("the request changed since the edition was loaded");
    // The input is retained for the retry and the prior state is intact.
    expect((screen.getByLabelText("Position") as HTMLInputElement).value).toBe("48");
    expect(screen.getByText("chapter: 42/100")).not.toBeNull();
  });

  it("keeps the input on an invalid field error", async () => {
    mockRecordProgress.mockRejectedValueOnce(
      new ApiError("invalid_argument", 400, 'invalid position "abc"', false),
    );
    renderPage();
    expect(await screen.findByText("Manga Title")).not.toBeNull();
    fireEvent.change(screen.getByLabelText("Position"), { target: { value: "abc" } });
    fireEvent.click(screen.getByRole("button", { name: "Record update" }));
    const alert = await screen.findByRole("alert");
    expect(alert.textContent).toContain('invalid position "abc"');
    expect((screen.getByLabelText("Position") as HTMLInputElement).value).toBe("abc");
  });

  it("does not offer or send a total for percent progress", async () => {
    renderPage();
    expect(await screen.findByText("Manga Title")).not.toBeNull();
    fireEvent.change(screen.getByLabelText("Unit"), { target: { value: "percent" } });
    expect(screen.queryByLabelText("Total")).toBeNull();
    fireEvent.change(screen.getByLabelText("Position"), { target: { value: "60" } });
    fireEvent.click(screen.getByRole("button", { name: "Record update" }));
    await waitFor(() => expect(mockRecordProgress).toHaveBeenCalledTimes(1));
    const input = mockRecordProgress.mock.calls[0][0];
    expect(input.unit).toBe("percent");
    expect(input.total).toBeUndefined();
  });

  it("changing the status does not touch the publication", async () => {
    mockGetEdition.mockResolvedValueOnce(
      edition({ consumption: { value: "not_started", reason: null } }),
    );
    mockSetConsumption.mockResolvedValueOnce(
      edition({
        revision: 4,
        consumption: { value: "completed", reason: null },
        publication: { value: "ongoing", source: "manual" },
      }),
    );
    renderPage();
    expect(await screen.findByText("Status: not started")).not.toBeNull();
    fireEvent.change(screen.getByLabelText("Status"), { target: { value: "completed" } });
    fireEvent.click(screen.getByRole("button", { name: "Set status" }));
    expect(await screen.findByText("Status: completed")).not.toBeNull();
    expect(mockSetConsumption).toHaveBeenCalledWith(
      expect.objectContaining({ value: "completed", expectedRevision: 3 }),
    );
    expect(screen.getByText("Publication: ongoing (source: manual)")).not.toBeNull();
  });

  it("retracts a progress record with a required reason and falls back to absence", async () => {
    mockGetEdition.mockResolvedValueOnce(
      edition({ progress: { chapter: record("p-42", "chapter", "42") } }),
    );
    mockGetEditionHistory
      .mockResolvedValueOnce({
        items: [
          historyItem({ id: "h-1", type: "progress", value: "chapter", detail: "42/100" }),
        ],
        next_cursor: null,
        total: 1,
      })
      .mockResolvedValueOnce({
        items: [
          historyItem({
            id: "h-1",
            type: "progress",
            value: "chapter",
            detail: null,
            label: "retracted",
            reason: "wrong unit",
          }),
        ],
        next_cursor: null,
        total: 1,
      });
    renderPage();
    expect(await screen.findByText("chapter: 42/100")).not.toBeNull();
    fireEvent.change(screen.getByLabelText("Record"), { target: { value: "h-1" } });
    fireEvent.click(screen.getByRole("button", { name: "Retract" }));
    expect((await screen.findByRole("alert")).textContent).toContain(
      "A retraction requires a reason.",
    );
    fireEvent.change(screen.getByLabelText("Retraction reason"), {
      target: { value: "wrong unit" },
    });
    fireEvent.click(screen.getByRole("button", { name: "Retract" }));
    expect(await screen.findByText("No progress recorded.")).not.toBeNull();
    expect(mockRetractProgress).toHaveBeenCalledWith(
      expect.objectContaining({ recordId: "h-1", reason: "wrong unit", expectedRevision: 3 }),
    );
    // The retracted row stays in history as retracted, not deleted.
    expect(screen.getByText("retracted")).not.toBeNull();
    // The retracted row is no longer offered for retraction.
    expect((screen.getByLabelText("Record") as HTMLSelectElement).disabled).toBe(true);
  });

  it("offers only effective progress rows for retraction", async () => {
    mockGetEdition.mockResolvedValueOnce(
      edition({ progress: { chapter: record("p-48", "chapter", "48") } }),
    );
    mockGetEditionHistory.mockResolvedValueOnce({
      items: [
        // The live record: same id as the edition's current chapter record.
        historyItem({ id: "p-48", type: "progress", value: "chapter", detail: "48/100" }),
        historyItem({
          id: "h-2",
          type: "progress",
          value: "chapter",
          detail: "42/100",
          label: "superseded",
        }),
        historyItem({
          id: "h-1",
          type: "progress",
          value: "chapter",
          detail: null,
          label: "retracted",
        }),
      ],
      next_cursor: null,
      total: 3,
    });
    renderPage();
    expect(await screen.findByText("chapter: 48/100")).not.toBeNull();
    const select = screen.getByLabelText("Record") as HTMLSelectElement;
    // Superseded and retracted rows are always rejected by the service, so
    // only the live effective row is offered.
    expect([...select.options].map((o) => o.value)).toEqual(["", "p-48"]);
  });

  it("offers the explicit unknown retraction for a mistaken status", async () => {
    mockGetEdition.mockResolvedValueOnce(
      edition({ consumption: { value: "completed", reason: null } }),
    );
    // The view renders an unknown state as the absence of a consumption.
    mockSetConsumption.mockResolvedValueOnce(
      edition({
        revision: 4,
        consumption: null,
        publication: { value: "ongoing", source: "manual" },
      }),
    );
    renderPage();
    expect(await screen.findByText("Status: completed")).not.toBeNull();
    fireEvent.change(screen.getByLabelText("Status"), { target: { value: "unknown" } });
    fireEvent.click(screen.getByRole("button", { name: "Set status" }));
    expect(await screen.findByText("Status: unknown")).not.toBeNull();
    expect(mockSetConsumption).toHaveBeenCalledWith(
      expect.objectContaining({ value: "unknown", expectedRevision: 3 }),
    );
  });

  it("sends only the changed metadata fields", async () => {
    renderPage();
    expect(await screen.findByText("Manga Title")).not.toBeNull();
    fireEvent.change(screen.getByLabelText("Language"), { target: { value: "ru" } });
    fireEvent.click(screen.getByRole("button", { name: "Save metadata" }));
    expect(
      (await screen.findByLabelText("Language")) as HTMLInputElement,
    ).toHaveProperty("value", "ru");
    const input = mockUpdateEdition.mock.calls[0][0];
    expect(input).toMatchObject({ language: "ru", expectedRevision: 3 });
    expect(input.kind).toBeUndefined();
    expect(input.otherLabel).toBeUndefined();
  });

  it("labels every control for keyboard use", async () => {
    renderPage();
    expect(await screen.findByText("Manga Title")).not.toBeNull();
    for (const label of [
      "Update kind",
      "Unit",
      "Position",
      "Total",
      "Status",
      "Reason",
      "Record",
      "Retraction reason",
      "Kind",
      "Language",
      "Other label",
    ]) {
      expect(screen.getByLabelText(label)).not.toBeNull();
    }
  });

  it("does not bundle notes, ratings or relation controls", async () => {
    renderPage();
    expect(await screen.findByText("Manga Title")).not.toBeNull();
    expect(screen.queryByRole("button", { name: /propose/i })).toBeNull();
    expect(screen.queryByRole("button", { name: /rate/i })).toBeNull();
    expect(screen.queryByLabelText(/note/i)).toBeNull();
    expect(screen.queryByLabelText(/relation/i)).toBeNull();
  });
});

describe("edition failure contract", () => {
  it("shows a failed first load as a distinguishable error and recovers on retry", async () => {
    mockGetEdition.mockRejectedValueOnce(
      new ApiError("not_found", 404, "no such edition", false),
    );
    renderPage();
    const alert = await screen.findByRole("alert");
    expect(alert.textContent).toContain("no such edition");
    mockGetEdition.mockResolvedValue(edition());
    fireEvent.click(screen.getByRole("button", { name: "Retry" }));
    expect(await screen.findByText("Manga Title")).not.toBeNull();
  });

  it("keeps the edition usable when the history fails", async () => {
    mockGetEdition.mockResolvedValueOnce(
      edition({ progress: { chapter: record("p-42", "chapter", "42") } }),
    );
    mockGetEditionHistory.mockRejectedValueOnce(
      new ApiError("locked", 503, "the storage is locked", true),
    );
    renderPage();
    expect(await screen.findByText("chapter: 42/100")).not.toBeNull();
    expect((await screen.findByRole("alert")).textContent).toContain("the storage is locked");
    fireEvent.change(screen.getByLabelText("Position"), { target: { value: "48" } });
    fireEvent.click(screen.getByRole("button", { name: "Record update" }));
    expect(await screen.findByText("chapter: 48")).not.toBeNull();
  });

  it("keeps the prior edition beside a distinguishable error when a refresh fails", async () => {
    mockGetEdition.mockResolvedValueOnce(
      edition({ progress: { chapter: record("p-42", "chapter", "42") } }),
    );
    mockGetEditionHistory
      .mockResolvedValueOnce({ items: [], next_cursor: null, total: 0 })
      .mockRejectedValueOnce(new ApiError("locked", 503, "the storage is locked", true));
    renderPage();
    expect(await screen.findByText("chapter: 42/100")).not.toBeNull();
    fireEvent.change(screen.getByLabelText("Position"), { target: { value: "48" } });
    fireEvent.click(screen.getByRole("button", { name: "Record update" }));
    // The mutation succeeded and its view applied; the follow-up history
    // refresh failed, so the prior edition stays beside the error.
    expect(await screen.findByText("chapter: 48")).not.toBeNull();
    expect((await screen.findAllByRole("alert")).some((el) => el.textContent?.includes("the storage is locked"))).toBe(true);
  });
});
