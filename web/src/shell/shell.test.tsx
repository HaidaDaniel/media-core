// App routing: anonymous visitors are sent to the login screen, a live
// session renders the shell navigation with the routed placeholder, and an
// unknown address shows the not-found state instead of a blank page.
import { cleanup, render, screen } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { App } from "../App";

const { mockInspectSession } = vi.hoisted(() => ({
  mockInspectSession: vi.fn(),
}));

vi.mock("../api/client", () => ({
  inspectSession: (...args: unknown[]) => mockInspectSession(...args),
  login: vi.fn(),
  logout: vi.fn(),
  setCSRFToken: vi.fn(),
}));

function navigateTo(path: string): void {
  window.history.pushState({}, "", path);
}

beforeEach(() => {
  window.history.pushState({}, "", "/");
});

afterEach(() => {
  cleanup();
  vi.clearAllMocks();
});

describe("App routing", () => {
  it("sends an anonymous visitor from a library route to the login screen", async () => {
    mockInspectSession.mockResolvedValueOnce(false);
    navigateTo("/works/w-1");
    render(<App />);
    expect(await screen.findByLabelText("Owner secret")).not.toBeNull();
  });

  it("sends an authenticated visitor from the login route to the shell", async () => {
    mockInspectSession.mockResolvedValueOnce(true);
    navigateTo("/login");
    render(<App />);
    expect(await screen.findByRole("link", { name: /library/i })).not.toBeNull();
  });

  it("renders the shell navigation with the work placeholder for a live session", async () => {
    mockInspectSession.mockResolvedValueOnce(true);
    navigateTo("/works/w-1");
    render(<App />);
    expect(await screen.findByRole("heading", { name: "Work details" })).not.toBeNull();
    expect(screen.getByRole("link", { name: /quick add/i })).not.toBeNull();
    expect(screen.getByRole("link", { name: /activity/i })).not.toBeNull();
    expect(screen.getByRole("button", { name: /sign out/i })).not.toBeNull();
  });

  it("shows the not-found state for an unknown address", async () => {
    mockInspectSession.mockResolvedValueOnce(true);
    navigateTo("/no-such-screen");
    render(<App />);
    expect(await screen.findByRole("heading", { name: "Not found" })).not.toBeNull();
  });
});
