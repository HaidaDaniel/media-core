// The rediscovered library surface: poster grid by default, a persisted
// grid/list URL toggle, media-type quick tabs, optional cover rendering with
// a neutral placeholder, and an explicit (never automatic) cover lookup.
import { cleanup, fireEvent, render, screen, within } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import type { JSX } from "react";
import { MemoryRouter, useLocation } from "react-router-dom";
import type { LibraryListWire, TitleWire, WorkCardWire } from "./libraryApi";
import { LibraryPage } from "./LibraryPage";

const { mockListWorks, mockEditionConsumption, mockFindMissing, mockSelect } = vi.hoisted(() => ({
  mockListWorks: vi.fn(),
  mockEditionConsumption: vi.fn(),
  mockFindMissing: vi.fn(),
  mockSelect: vi.fn(),
}));

vi.mock("./libraryApi", () => ({
  listWorks: (...args: unknown[]) => mockListWorks(...args),
  editionConsumption: (...args: unknown[]) => mockEditionConsumption(...args),
}));

vi.mock("../artwork/artworkApi", async (importOriginal) => {
  const actual = await importOriginal<typeof import("../artwork/artworkApi")>();
  return {
    ...actual,
    findMissingCovers: (...args: unknown[]) => mockFindMissing(...args),
    selectArtwork: (...args: unknown[]) => mockSelect(...args),
  };
});

function title(text: string): TitleWire {
  return {
    id: `t-${text}`,
    text,
    language: "en",
    kind: "original",
    preferred: true,
    provenance: "manual",
  };
}

function card(overrides: Partial<WorkCardWire> = {}): WorkCardWire {
  return {
    work_id: "w-1",
    revision: 0,
    created_at: "2026-01-01T00:00:00Z",
    updated_at: "2026-01-01T00:00:00Z",
    archived_at: null,
    titles: [title("Static Bloom")],
    editions: [{ id: "e-1", kind: "manga", language: "en", revision: 0 }],
    matched_edition_ids: [],
    ...overrides,
  };
}

function payload(items: WorkCardWire[]): LibraryListWire {
  return { items, next_cursor: null, total: items.length };
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
  mockFindMissing.mockResolvedValue({ items: [], next_cursor: "", total_missing: 0 });
});

afterEach(() => {
  cleanup();
});

describe("library grid and discovery", () => {
  it("defaults to the poster grid and persists the list alternative in the URL", async () => {
    renderPage("/");
    expect(await screen.findByText("Static Bloom")).not.toBeNull();
    expect(document.querySelector("ul.library-cards")).not.toBeNull();
    expect(screen.getByTestId("url").textContent).toBe("/");

    fireEvent.click(screen.getByRole("button", { name: "List view" }));
    await vi.waitFor(() => expect(screen.getByTestId("url").textContent).toBe("/?view=list"));
    expect(document.querySelector("ul.library-cards")).toBeNull();

    fireEvent.click(screen.getByRole("button", { name: "Grid view" }));
    await vi.waitFor(() => expect(screen.getByTestId("url").textContent).toBe("/"));
    expect(document.querySelector("ul.library-cards")).not.toBeNull();
  });

  it("restores a list URL as the list alternative", async () => {
    renderPage("/?view=list");
    expect(await screen.findByText("Static Bloom")).not.toBeNull();
    expect(document.querySelector("ul.library-cards")).toBeNull();
    expect(screen.getByRole("button", { name: "Grid view" })).not.toBeNull();
  });

  it("uses the media-type quick tabs as the same kind filter", async () => {
    renderPage("/");
    expect(await screen.findByText("Static Bloom")).not.toBeNull();
    fireEvent.click(screen.getByRole("button", { name: "anime" }));
    await vi.waitFor(() =>
      expect(mockListWorks).toHaveBeenLastCalledWith(expect.objectContaining({ kind: ["anime"] })),
    );
    expect(screen.getByTestId("url").textContent).toBe("/?kind=anime");
    fireEvent.click(screen.getByRole("button", { name: "anime" }));
    await vi.waitFor(() => expect(screen.getByTestId("url").textContent).toBe("/"));
  });

  it("renders supplied cover images and a neutral placeholder otherwise", async () => {
    mockListWorks.mockResolvedValueOnce(
      payload([
        card({
          work_id: "w-covered",
          titles: [title("Covered")],
          artwork: {
            id: "a-1",
            image_url: "/api/v1/artwork/preview/tok-9/image",
            provider: "kitsu",
            attribution_url: "https://kitsu.app",
          },
        }),
        card({ work_id: "w-bare", titles: [title("Bare")] }),
      ]),
    );
    renderPage("/");
    expect(await screen.findByText("Covered")).not.toBeNull();
    expect(
      document.querySelector('img[src="/api/v1/artwork/preview/tok-9/image"]'),
    ).not.toBeNull();
    expect(screen.getAllByRole("img", { name: /No cover available/ }).length).toBeGreaterThan(0);
    expect(document.querySelectorAll("article.work-card").length).toBe(2);
  });

  it("never requests missing covers until the user asks and never selects automatically", async () => {
    renderPage("/");
    expect(await screen.findByText("Static Bloom")).not.toBeNull();
    expect(mockFindMissing).not.toHaveBeenCalled();
    expect(mockSelect).not.toHaveBeenCalled();
    const panel = screen.getByRole("region", { name: "Cover discovery" });
    fireEvent.click(within(panel).getByRole("button", { name: "Find missing covers" }));
    await vi.waitFor(() => expect(mockFindMissing).toHaveBeenCalledWith({ limit: 5, cursor: "" }, expect.any(AbortSignal)));
    expect(mockSelect).not.toHaveBeenCalled();
  });

  it("shows a bounded progress line for an explicit second batch", async () => {
    mockFindMissing
      .mockResolvedValueOnce({
        items: [
          {
            work: { id: "w-2", title: "No Cover", kind: "book" },
            status: "not_found",
            candidates: [],
          },
        ],
        next_cursor: "cursor-1",
        total_missing: 7,
      })
      .mockResolvedValueOnce({ items: [], next_cursor: "", total_missing: 7 });
    renderPage("/");
    expect(await screen.findByText("Static Bloom")).not.toBeNull();
    const panel = screen.getByRole("region", { name: "Cover discovery" });
    fireEvent.click(within(panel).getByRole("button", { name: "Find missing covers" }));
    expect(await within(panel).findByText(/1 checked · 7 missing now/)).not.toBeNull();
    fireEvent.click(within(panel).getByRole("button", { name: "Check next 5" }));
    await vi.waitFor(() =>
      expect(mockFindMissing).toHaveBeenLastCalledWith({ limit: 5, cursor: "cursor-1" }, expect.any(AbortSignal)),
    );
  });
});
