// App routing: anonymous visitors are sent to the login screen, a live
// session renders the shell navigation with the routed placeholder, and an
// unknown address shows the not-found state instead of a blank page.
import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { App } from "../App";

const { mockInspectSession, mockLogout, ApiErrorStub } = vi.hoisted(() => {
  class ApiErrorStub extends Error {
    code: string;

    constructor(code: string, message: string) {
      super(message);
      this.name = "ApiError";
      this.code = code;
    }
  }
  return { mockInspectSession: vi.fn(), mockLogout: vi.fn(), ApiErrorStub };
});

vi.mock("../api/client", () => ({
  inspectSession: (...args: unknown[]) => mockInspectSession(...args),
  login: vi.fn(),
  logout: (...args: unknown[]) => mockLogout(...args),
  setCSRFToken: vi.fn(),
  ApiError: ApiErrorStub,
  // Path-aware: the work route renders the real work screen, which reads the
  // work detail and the work relations; every other read is the empty list.
  get: vi.fn((path: string) => {
    if (path.includes("/relations")) {
      return Promise.resolve({ entity_id: "w-1", scope: "work", edges: [] });
    }
    if (path.startsWith("/works/")) {
      return Promise.resolve({
        id: "w-1",
        revision: 1,
        created_at: "2026-01-01T00:00:00Z",
        updated_at: "2026-01-01T00:00:00Z",
        archived_at: null,
        titles: [
          {
            id: "t-1",
            text: "Work details",
            language: null,
            kind: "original",
            preferred: true,
            provenance: "manual",
          },
        ],
        editions: [],
      });
    }
    return Promise.resolve({ items: [], next_cursor: null, total: 0 });
  }),
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

  it("renders the shell navigation with the work screen for a live session", async () => {
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

  it("shows the server message when sign out fails and keeps the navigation", async () => {
    mockInspectSession.mockResolvedValueOnce(true);
    mockLogout.mockRejectedValueOnce(new ApiErrorStub("forbidden", "CSRF mismatch"));
    navigateTo("/");
    render(<App />);
    const signOut = await screen.findByRole("button", { name: /sign out/i });
    fireEvent.click(signOut);
    expect((await screen.findByRole("alert")).textContent).toBe("CSRF mismatch");
    expect(screen.getByRole("navigation")).not.toBeNull();
  });
});
