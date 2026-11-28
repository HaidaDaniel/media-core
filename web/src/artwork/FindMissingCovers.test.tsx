import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { FindMissingCovers } from "./FindMissingCovers";

const { mockFindMissing } = vi.hoisted(() => ({ mockFindMissing: vi.fn() }));

vi.mock("./artworkApi", async (importOriginal) => {
  const actual = await importOriginal<typeof import("./artworkApi")>();
  return { ...actual, findMissingCovers: (...args: unknown[]) => mockFindMissing(...args) };
});

beforeEach(() => vi.clearAllMocks());
afterEach(() => cleanup());

describe("FindMissingCovers timeout retry", () => {
  it("retries the timed-out batch instead of advancing past it", async () => {
    mockFindMissing
      .mockResolvedValueOnce({
        items: [
          { work: { id: "w-1", title: "Slow", kind: "book" }, status: "timeout", candidates: [] },
        ],
        next_cursor: "after-slow",
        total_missing: 3,
      })
      .mockResolvedValueOnce({
        items: [
          { work: { id: "w-1", title: "Slow", kind: "book" }, status: "not_found", candidates: [] },
        ],
        next_cursor: "after-slow",
        total_missing: 3,
      });
    render(<FindMissingCovers />);
    fireEvent.click(screen.getByRole("button", { name: "Find missing covers" }));
    expect(await screen.findByRole("button", { name: "Retry this batch" })).not.toBeNull();
    fireEvent.click(screen.getByRole("button", { name: "Retry this batch" }));
    await vi.waitFor(() => expect(mockFindMissing).toHaveBeenCalledTimes(2));
    expect(mockFindMissing.mock.calls[1][0]).toEqual({ limit: 5, cursor: "" });
    expect(await screen.findByText("not found")).not.toBeNull();
    expect(screen.queryByRole("button", { name: "Retry this batch" })).toBeNull();
  });
});

describe("FindMissingCovers cancellation", () => {
  it("aborts the in-flight request instead of only hiding the spinner", async () => {
    const captured: { signal: AbortSignal | null } = { signal: null };
    mockFindMissing.mockImplementation((_input: unknown, signal: AbortSignal) => {
      captured.signal = signal;
      return new Promise((_resolve, reject) => {
        signal.addEventListener("abort", () => reject(new DOMException("aborted", "AbortError")));
      });
    });
    render(<FindMissingCovers />);
    fireEvent.click(screen.getByRole("button", { name: "Find missing covers" }));
    await screen.findByRole("button", { name: "Cancel" });
    fireEvent.click(screen.getByRole("button", { name: "Cancel" }));
    expect(captured.signal?.aborted).toBe(true);
    expect(screen.queryByRole("alert")).toBeNull();
  });
});
