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
