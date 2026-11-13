// Auth state on start: a live session inspects to authenticated, a missing
// session to anonymous, and the sign-out action flips back to anonymous.
import { act, cleanup, fireEvent, render, screen } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import type { JSX } from "react";
import { MemoryRouter } from "react-router-dom";
import { AuthProvider, useAuth } from "./AuthContext";
import { Shell } from "../shell/Shell";
import { LoginPage } from "./LoginPage";

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

  it("keeps the session state when logout fails", async () => {
    mockInspectSession.mockResolvedValueOnce(true);
    mockLogout.mockRejectedValueOnce(new Error("forbidden: CSRF mismatch"));

    function LogoutButton(): JSX.Element {
      const { logout } = useAuth();
      return (
        <button type="button" onClick={() => void logout().catch(() => undefined)}>
          Sign out
        </button>
      );
    }

    render(
      <AuthProvider>
        <StatusProbe />
        <LogoutButton />
      </AuthProvider>,
    );
    await screen.findByTestId("status");
    await vi.waitFor(() =>
      expect((screen.getByTestId("status")).textContent).toBe("authenticated"),
    );
    fireEvent.click(screen.getByRole("button", { name: /sign out/i }));
    await vi.waitFor(() => expect(mockLogout).toHaveBeenCalledTimes(1));
    expect((screen.getByTestId("status")).textContent).toBe("authenticated");
  });

  it("does not let a slow initial inspection overwrite a completed login", async () => {
    let resolveInspect: (live: boolean) => void = () => {
      // replaced by the deferred promise below
    };
    mockInspectSession.mockImplementationOnce(
      () =>
        new Promise<boolean>((resolve) => {
          resolveInspect = resolve;
        }),
    );

    render(
      <AuthProvider>
        <LoginPage />
        <StatusProbe />
      </AuthProvider>,
    );

    const input = (await screen.findByLabelText("Owner secret")) as HTMLInputElement;
    fireEvent.change(input, { target: { value: "secret" } });
    fireEvent.click(screen.getByRole("button", { name: /sign in/i }));
    await vi.waitFor(() =>
      expect((screen.getByTestId("status")).textContent).toBe("authenticated"),
    );

    resolveInspect(false);
    await act(async () => {
      // let the late inspection settle
    });
    expect((screen.getByTestId("status")).textContent).toBe("authenticated");
  });
});
