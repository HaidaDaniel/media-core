// Auth state on start: a live session inspects to authenticated, a missing
// session to anonymous, and the sign-out action flips back to anonymous.
import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import type { JSX } from "react";
import { MemoryRouter } from "react-router-dom";
import { AuthProvider, useAuth } from "./AuthContext";
import { Shell } from "../shell/Shell";

const { mockInspectSession, mockLogout } = vi.hoisted(() => ({
  mockInspectSession: vi.fn(),
  mockLogout: vi.fn(),
}));

vi.mock("../api/client", () => ({
  inspectSession: (...args: unknown[]) => mockInspectSession(...args),
  login: vi.fn(),
  logout: (...args: unknown[]) => mockLogout(...args),
  setCSRFToken: vi.fn(),
}));

function StatusProbe(): JSX.Element {
  const { status } = useAuth();
  return <span data-testid="status">{status}</span>;
}

beforeEach(() => {
  mockLogout.mockResolvedValue(undefined);
});

afterEach(() => {
  cleanup();
  vi.clearAllMocks();
});

describe("AuthProvider", () => {
  it("inspects to authenticated when a live session exists", async () => {
    mockInspectSession.mockResolvedValueOnce(true);
    render(
      <AuthProvider>
        <StatusProbe />
      </AuthProvider>,
    );
    expect((await screen.findByTestId("status")).textContent).toBe("authenticated");
  });

  it("inspects to anonymous when no session exists", async () => {
    mockInspectSession.mockResolvedValueOnce(false);
    render(
      <AuthProvider>
        <StatusProbe />
      </AuthProvider>,
    );
    expect((await screen.findByTestId("status")).textContent).toBe("anonymous");
  });

  it("treats an inspection failure as anonymous", async () => {
    mockInspectSession.mockRejectedValueOnce(new Error("boom"));
    render(
      <AuthProvider>
        <StatusProbe />
      </AuthProvider>,
    );
    expect((await screen.findByTestId("status")).textContent).toBe("anonymous");
  });

  it("signing out calls the client logout", async () => {
    mockInspectSession.mockResolvedValueOnce(true);
    render(
      <AuthProvider>
        <MemoryRouter>
          <Shell />
        </MemoryRouter>
      </AuthProvider>,
    );
    const signOut = await screen.findByRole("button", { name: /sign out/i });
    fireEvent.click(signOut);
    await vi.waitFor(() => expect(mockLogout).toHaveBeenCalled());
  });
});
