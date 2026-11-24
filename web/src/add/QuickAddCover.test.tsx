// A failed or slow provider search is confined to the cover picker: the
// title-first creation flow must still complete with no cover chosen.
import { cleanup, fireEvent, render, screen, within } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import type { JSX } from "react";
import { MemoryRouter, Route, Routes, useParams } from "react-router-dom";
import { ApiError } from "../api/client";
import type { AdditionResultWire, ResolveWire } from "./addApi";
import { QuickAddPage } from "./QuickAddPage";

const { mockResolve, mockCreate, mockSearch, mockSelect } = vi.hoisted(() => ({
  mockResolve: vi.fn(),
  mockCreate: vi.fn(),
  mockSearch: vi.fn(),
  mockSelect: vi.fn(),
}));

vi.mock("./addApi", () => ({
  resolveAddition: (...args: unknown[]) => mockResolve(...args),
  createWork: (...args: unknown[]) => mockCreate(...args),
  addEdition: vi.fn(),
  targetPathFor: (r: { edition_id: string | null; work_id: string }) =>
    r.edition_id ? `/editions/${r.edition_id}` : `/works/${r.work_id}`,
  existingWorkPath: (id: string) => `/works/${id}`,
}));

vi.mock("../artwork/artworkApi", async (importOriginal) => {
  const actual = await importOriginal<typeof import("../artwork/artworkApi")>();
  return {
    ...actual,
    searchArtwork: (...args: unknown[]) => mockSearch(...args),
    selectArtwork: (...args: unknown[]) => mockSelect(...args),
  };
});

function Probe(): JSX.Element {
  const { id } = useParams();
  return <p data-testid="probe">{id}</p>;
}

function renderAdd(): void {
  render(
    <MemoryRouter initialEntries={["/add"]}>
      <Routes>
        <Route path="/add" element={<QuickAddPage />} />
        <Route path="/editions/:id" element={<Probe />} />
        <Route path="/works/:id" element={<Probe />} />
      </Routes>
    </MemoryRouter>,
  );
}

const ready: ResolveWire = {
  outcome: "ready",
  work_id: null,
  candidates: null,
  candidate_token: null,
  request_fingerprint: null,
};

beforeEach(() => {
  vi.clearAllMocks();
  mockResolve.mockResolvedValue(ready);
  mockCreate.mockResolvedValue({
    created: true,
    work_id: "w-7",
    work_revision: 1,
    edition_id: "e-7",
  } satisfies AdditionResultWire);
});

afterEach(() => {
  cleanup();
});

describe("quick add with cover discovery", () => {
  it("keeps creating the work when the cover search fails", async () => {
    mockSearch.mockRejectedValue(new ApiError("rate_limited", 429, "provider is busy", true));
    renderAdd();
    fireEvent.change(screen.getByLabelText("Title"), { target: { value: "Neon Garden" } });
    fireEvent.click(screen.getByRole("button", { name: "Find a cover" }));
    const dialog = await screen.findByRole("dialog");
    fireEvent.click(within(dialog).getByRole("button", { name: "Search for covers" }));
    expect((await within(dialog).findByRole("alert")).textContent).toContain("provider is busy");
    fireEvent.click(within(dialog).getByRole("button", { name: "Cancel" }));

    fireEvent.click(screen.getByRole("button", { name: "Add" }));
    expect((await screen.findByTestId("probe")).textContent).toBe("e-7");
    expect(mockCreate).toHaveBeenCalledTimes(1);
    expect(mockSelect).not.toHaveBeenCalled();
  });
});
