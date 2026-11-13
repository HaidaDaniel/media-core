// The library screen: URL-backed filter state, debounced Unicode search,
// stale-fetch cancellation, the filtered count, per-edition consumption
// badges, missing metadata as "Unknown", the empty and error states with a
// retry, and labeled keyboard controls.
import { act, cleanup, fireEvent, render, screen, within } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import type { JSX } from "react";
import { MemoryRouter, useLocation } from "react-router-dom";
import { ApiError } from "../api/client";
import { LibraryPage } from "./LibraryPage";
import type {
  EditionSummaryWire,
  LibraryListWire,
  TitleWire,
  WorkCardWire,
} from "./libraryApi";

const { mockListWorks, mockEditionConsumption } = vi.hoisted(() => ({
  mockListWorks: vi.fn(),
  mockEditionConsumption: vi.fn(),
}));

vi.mock("./libraryApi", () => ({
  listWorks: (...args: unknown[]) => mockListWorks(...args),
  editionConsumption: (...args: unknown[]) => mockEditionConsumption(...args),
}));

function title(text: string, preferred = true, language: string | null = "en"): TitleWire {
  return {
    id: `t-${text}`,
    text,
    language,
    kind: preferred ? "original" : "alias",
    preferred,
    provenance: "manual",
  };
}

function edition(id: string, kind: string, language: string | null = "en"): EditionSummaryWire {
  return { id, kind, language, revision: 0 };
}

function card(overrides: Partial<WorkCardWire> = {}): WorkCardWire {
  return {
    work_id: "w-1",
    revision: 0,
    created_at: "2026-01-01T00:00:00Z",
    updated_at: "2026-01-01T00:00:00Z",
    archived_at: null,
    titles: [title("Static Bloom")],
    editions: [edition("e-1", "manga")],
    matched_edition_ids: ["e-1"],
    ...overrides,
  };
}

function payload(
  items: WorkCardWire[],
  nextCursor: string | null = null,
  total?: number,
): LibraryListWire {
  return { items, next_cursor: nextCursor, total: total ?? items.length };
}

function LocationProbe(): JSX.Element {
  const location = useLocation();
  return <span data-testid="url">{location.pathname + location.search}</span>;
}

function renderPage(path = "/"): void {
  render(
    <MemoryRouter initialEntries={[path]}>
      <LibraryPage />
      <LocationProbe />
    </MemoryRouter>,
  );
}

beforeEach(() => {
  vi.clearAllMocks();
  mockListWorks.mockResolvedValue(payload([card()]));
  mockEditionConsumption.mockResolvedValue("completed");
});

afterEach(() => {
  cleanup();
});

describe("library listing", () => {
  it("lists the filtered works with the filtered count, badges and edition links", async () => {
    renderPage("/?query=bloom&kind=manga&consumption=completed&sort=rating&limit=10");
    expect(await screen.findByText("Static Bloom")).not.toBeNull();
    expect(mockListWorks).toHaveBeenCalledWith({
      query: "bloom",
      kind: ["manga"],
      consumption: ["completed"],
      sort: "rating",
      limit: 10,
    });
    expect(screen.getByText("1 work")).not.toBeNull();
    const badge = await screen.findByText("Completed");
    expect(badge.className).toContain("work-badge-completed");
    expect(screen.getByRole("link", { name: /manga/i }).getAttribute("href")).toBe(
      "/editions/e-1",
    );
  });

  it("keeps a typed Unicode search in the URL and renders the server match", async () => {
    renderPage("/");
    const input = screen.getByLabelText("Search") as HTMLInputElement;
    fireEvent.change(input, { target: { value: "STÄNDE" } });
    mockListWorks.mockResolvedValue(
      payload([card({ titles: [title("Stände", true, "de")] })]),
    );
    expect(await screen.findByText("Stände")).not.toBeNull();
    expect(mockListWorks).toHaveBeenLastCalledWith(
      expect.objectContaining({ query: "STÄNDE" }),
    );
    // The URL carries the search, so a refresh restores it.
    expect(screen.getByTestId("url").textContent).toBe("/?query=ST%C3%84NDE");
    // The server-matched edition is labeled while the filter is active.
    expect(screen.getAllByText("match").length).toBeGreaterThan(0);
  });

  it("resets the filters back to the whole library", async () => {
    renderPage("/?kind=manga&consumption=completed&sort=rating");
    expect(await screen.findByText("Static Bloom")).not.toBeNull();
    fireEvent.click(screen.getByRole("button", { name: "Reset" }));
    await vi.waitFor(() =>
      expect(mockListWorks).toHaveBeenLastCalledWith({
        query: undefined,
        kind: undefined,
        consumption: undefined,
        sort: "title",
        limit: 30,
      }),
    );
    expect(screen.getByTestId("url").textContent).toBe("/");
  });

  it("discards a stale response that resolves after a newer request", async () => {
    renderPage("/");
    expect(await screen.findByText("Static Bloom")).not.toBeNull();
    const input = screen.getByLabelText("Search") as HTMLInputElement;
    let resolveA: (value: LibraryListWire) => void = () => {
      // replaced by the deferred promise below
    };
    mockListWorks.mockImplementationOnce(
      () =>
        new Promise<LibraryListWire>((resolve) => {
          resolveA = resolve;
        }),
    );
    mockListWorks.mockResolvedValueOnce(
      payload([card({ work_id: "w-2", titles: [title("Second Work")] })]),
    );

    fireEvent.change(input, { target: { value: "aaa" } });
    await vi.waitFor(() => expect(mockListWorks).toHaveBeenCalledTimes(2));
    fireEvent.change(input, { target: { value: "bbb" } });
    await vi.waitFor(() => expect(mockListWorks).toHaveBeenCalledTimes(3));
    expect(await screen.findByText("Second Work")).not.toBeNull();

    resolveA(payload([card({ work_id: "w-3", titles: [title("Stale Work")] })]));
    await act(async () => {
      // let the stale response settle
    });
    expect(screen.queryByText("Stale Work")).toBeNull();
    expect(screen.getByText("Second Work")).not.toBeNull();
  });

  it("renders missing metadata as Unknown without inventing values", async () => {
    mockListWorks.mockResolvedValueOnce(
      payload([
        card({
          work_id: "w-x",
          titles: [title("Mystery", true, null)],
          editions: [edition("e-x", "book", null)],
          matched_edition_ids: [],
        }),
      ]),
    );
    mockEditionConsumption.mockRejectedValueOnce(new Error("boom"));
    renderPage("/");
    expect(await screen.findByText("Mystery")).not.toBeNull();
    // Scoped to the card: the toolbar's status filter also offers "Unknown".
    const article = screen.getByRole("article");
    const unknowns = within(article).getAllByText("Unknown");
    expect(unknowns.length).toBe(2);
  });
});

describe("library states", () => {
  it("shows the empty state with a reset action", async () => {
    mockListWorks.mockResolvedValueOnce(payload([]));
    renderPage("/");
    expect(await screen.findByText("No works match the current filters.")).not.toBeNull();
    expect(screen.getByRole("button", { name: "Reset filters" })).not.toBeNull();
    expect(screen.getByText("0 works")).not.toBeNull();
  });

  it("shows a failed first load as a distinguishable error and recovers on retry", async () => {
    mockListWorks.mockRejectedValueOnce(new ApiError("locked", 503, "the storage is locked", true));
    mockListWorks.mockResolvedValueOnce(payload([card()]));
    renderPage("/");
    const alert = await screen.findByRole("alert");
    expect(alert.textContent).toContain("the storage is locked");
    // The failure is the whole state: no empty result and no count are
    // asserted alongside it.
    expect(screen.queryByText("No works match the current filters.")).toBeNull();
    expect(screen.queryByText("0 works")).toBeNull();
    fireEvent.click(screen.getByRole("button", { name: "Retry" }));
    expect(await screen.findByText("Static Bloom")).not.toBeNull();
    expect(mockListWorks).toHaveBeenCalledTimes(2);
  });

  it("keeps the prior list and the failed filter when a change fails, and retries it", async () => {
    renderPage("/");
    expect(await screen.findByText("Static Bloom")).not.toBeNull();
    mockListWorks.mockRejectedValueOnce(new ApiError("locked", 503, "the storage is locked", true));
    fireEvent.change(screen.getByLabelText("Medium") as HTMLSelectElement, {
      target: { value: "anime" },
    });
    await vi.waitFor(() =>
      expect(mockListWorks).toHaveBeenLastCalledWith(
        expect.objectContaining({ kind: ["anime"] }),
      ),
    );
    const alert = await screen.findByRole("alert");
    expect(alert.textContent).toContain("the storage is locked");
    // The prior state stays intact: the old list and its count remain
    // visible, and the URL keeps the failed filter.
    expect(screen.getByText("Static Bloom")).not.toBeNull();
    expect(screen.getByText("1 work")).not.toBeNull();
    expect(screen.getByTestId("url").textContent).toBe("/?kind=anime");
    mockListWorks.mockResolvedValue(payload([card()]));
    fireEvent.click(screen.getByRole("button", { name: "Retry" }));
    expect(await screen.findByText("Static Bloom")).not.toBeNull();
  });

  it("appends the next page on load more and keeps the filtered total", async () => {
    mockListWorks.mockResolvedValueOnce(payload([card({ work_id: "w-1" })], "cursor-2", 2));
    mockListWorks.mockResolvedValueOnce(
      payload([card({ work_id: "w-2", titles: [title("Second Work")] })], null, 2),
    );
    renderPage("/");
    expect(await screen.findByText("Static Bloom")).not.toBeNull();
    expect(screen.getByText("2 works")).not.toBeNull();
    fireEvent.click(await screen.findByRole("button", { name: "Load more" }));
    expect(await screen.findByText("Second Work")).not.toBeNull();
    expect(screen.queryByRole("button", { name: /load more/i })).toBeNull();
    expect(mockListWorks).toHaveBeenLastCalledWith(
      expect.objectContaining({ cursor: "cursor-2" }),
    );
  });

  it("labels every control for keyboard use", async () => {
    renderPage("/");
    expect(await screen.findByText("Static Bloom")).not.toBeNull();
    for (const name of ["Search", "Medium", "Status", "Sort", "Page size"]) {
      expect(screen.getByLabelText(name)).not.toBeNull();
    }
  });

  it("offers every valid kind and consumption value in the filters", async () => {
    renderPage("/");
    expect(await screen.findByText("Static Bloom")).not.toBeNull();
    const kinds = within(screen.getByLabelText("Medium")).getAllByRole("option");
    expect(kinds.length).toBe(14); // "All" plus the 13 valid kinds
    expect(kinds.some((o) => (o as HTMLOptionElement).value === "unknown")).toBe(true);
    const statuses = within(screen.getByLabelText("Status")).getAllByRole("option");
    expect(statuses.length).toBe(7); // "All" plus the five states and unknown
  });
});
