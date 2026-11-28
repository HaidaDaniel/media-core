// The cover picker is explicit: no search on open, one confident result is a
// dismissible suggestion, several results are user choices, none is a neutral
// fallback, and a failed search never auto-selects a cover.
import { cleanup, fireEvent, render, screen, within } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { ApiError } from "../api/client";
import { CoverPicker } from "./CoverPicker";
import type { ArtworkCandidate } from "./artworkApi";

const { mockSearch, mockSelect } = vi.hoisted(() => ({
  mockSearch: vi.fn(),
  mockSelect: vi.fn(),
}));

vi.mock("./artworkApi", async (importOriginal) => {
  const actual = await importOriginal<typeof import("./artworkApi")>();
  return {
    ...actual,
    searchArtwork: (...args: unknown[]) => mockSearch(...args),
    selectArtwork: (...args: unknown[]) => mockSelect(...args),
  };
});

function candidate(overrides: Partial<ArtworkCandidate> = {}): ArtworkCandidate {
  return {
    provider: "kitsu",
    external_id: "ext-1",
    title: "Static Bloom",
    alternative_titles: [],
    kind: "manga",
    image_url: "/api/v1/artwork/preview/tok-1/image",
    preview_token: "tok-1",
    attribution_url: "https://kitsu.app",
    score: 0.92,
    reason: "title and kind match",
    ...overrides,
  };
}

function renderPicker(props: Partial<Parameters<typeof CoverPicker>[0]> = {}) {
  return render(
    <CoverPicker
      open
      onClose={() => undefined}
      title="Static Bloom"
      kind="manga"
      alternatives={["Static Bloom (JP)"]}
      references={["https://example.org/work"]}
      {...props}
    />,
  );
}

beforeEach(() => {
  vi.clearAllMocks();
});

afterEach(() => {
  cleanup();
});

describe("cover picker", () => {
  it("searches only on explicit user action with a bounded payload", async () => {
    mockSearch.mockResolvedValue({ candidates: [] });
    renderPicker();
    expect(mockSearch).not.toHaveBeenCalled();
    fireEvent.click(screen.getByRole("button", { name: "Search for covers" }));
    await vi.waitFor(() =>
      expect(mockSearch).toHaveBeenCalledWith(
        {
          title: "Static Bloom",
          kind: "manga",
          alternatives: ["Static Bloom (JP)"],
          references: ["https://example.org/work"],
        },
        expect.any(AbortSignal),
      ),
    );
  });

  it("offers one confident result as a dismissible suggestion", async () => {
    mockSearch.mockResolvedValue({ candidates: [candidate()] });
    const onPick = vi.fn();
    renderPicker({ onPick });
    fireEvent.click(screen.getByRole("button", { name: "Search for covers" }));
    expect(await screen.findByText("One confident match. Use it only if it is right.")).not.toBeNull();
    expect(screen.getByRole("button", { name: "Use this cover" })).not.toBeNull();
    expect(screen.getByRole("button", { name: "Choose another" })).not.toBeNull();
    expect(screen.getAllByRole("button", { name: "Cancel" }).length).toBeGreaterThan(0);
    expect(onPick).not.toHaveBeenCalled();
    fireEvent.click(screen.getByRole("button", { name: "Choose another" }));
    expect(screen.queryByText("One confident match. Use it only if it is right.")).toBeNull();
    fireEvent.click(screen.getByRole("button", { name: "Use this cover" }));
    expect(onPick).toHaveBeenCalledTimes(1);
    expect(mockSelect).not.toHaveBeenCalled();
  });

  it("shows several candidates with cover, title, kind and source for user choice", async () => {
    mockSearch.mockResolvedValue({
      candidates: [
        candidate(),
        candidate({ external_id: "ext-2", title: "Bloom", provider: "openlibrary", kind: "book" }),
      ],
    });
    renderPicker();
    fireEvent.click(screen.getByRole("button", { name: "Search for covers" }));
    expect(await screen.findByText(/2 candidates/)).not.toBeNull();
    const dialog = screen.getByRole("dialog");
    expect(within(dialog).getAllByRole("button", { name: "Use this cover" }).length).toBe(2);
    expect(within(dialog).getByText("Static Bloom")).not.toBeNull();
    expect(within(dialog).getByText("Bloom")).not.toBeNull();
    expect(within(dialog).getAllByText("kitsu").length).toBeGreaterThan(0);
  });

  it("leaves a neutral fallback when nothing matches", async () => {
    mockSearch.mockResolvedValue({ candidates: [] });
    renderPicker();
    fireEvent.click(screen.getByRole("button", { name: "Search for covers" }));
    expect(await screen.findByText(/No cover candidates found/)).not.toBeNull();
    expect(screen.queryByRole("button", { name: "Use this cover" })).toBeNull();
  });

  it("shows a failed search inside the dialog without selecting anything", async () => {
    mockSearch.mockRejectedValue(new ApiError("rate_limited", 429, "provider is busy", true));
    const onPick = vi.fn();
    renderPicker({ onPick });
    fireEvent.click(screen.getByRole("button", { name: "Search for covers" }));
    expect((await screen.findByRole("alert")).textContent).toContain("provider is busy");
    expect(screen.getByRole("button", { name: "Try again" })).not.toBeNull();
    expect(onPick).not.toHaveBeenCalled();
    expect(mockSelect).not.toHaveBeenCalled();
  });

  it("only writes a saved cover with an explicit pick and replace choice", async () => {
    mockSearch.mockResolvedValue({ candidates: [candidate()] });
    mockSelect.mockResolvedValue({ artwork: { id: "a-1", title: "Static Bloom" } });
    const onSelected = vi.fn();
    const onClose = vi.fn();
    renderPicker({ workId: "w-1", editionId: "e-1", onSelected, onClose });
    fireEvent.click(screen.getByRole("button", { name: "Search for covers" }));
    await screen.findByText("One confident match. Use it only if it is right.");
    fireEvent.click(screen.getByLabelText("Replace an existing cover"));
    fireEvent.click(screen.getByRole("button", { name: "Use this cover" }));
    await vi.waitFor(() =>
      expect(mockSelect).toHaveBeenCalledWith({
        work_id: "w-1",
        edition_id: "e-1",
        preview_token: "tok-1",
        replace: true,
      }),
    );
    await vi.waitFor(() => expect(onSelected).toHaveBeenCalledTimes(1));
    expect(onClose).toHaveBeenCalledTimes(1);
  });
});

describe("cover picker safety labels", () => {
  it("does not call a weak single result confident", async () => {
    mockSearch.mockResolvedValue({ candidates: [candidate({ score: 0.5 })] });
    renderPicker();
    fireEvent.click(screen.getByRole("button", { name: "Search for covers" }));
    expect(await screen.findByText(/One possible match/)).not.toBeNull();
    expect(screen.queryByText(/One confident match/)).toBeNull();
  });

  it("drops candidates that cannot be selected", async () => {
    mockSearch.mockResolvedValue({ candidates: [candidate({ preview_token: "", image_url: null })] });
    renderPicker();
    fireEvent.click(screen.getByRole("button", { name: "Search for covers" }));
    expect(await screen.findByText(/No cover candidates found/)).not.toBeNull();
    expect(screen.queryByRole("button", { name: "Use this cover" })).toBeNull();
  });
});
