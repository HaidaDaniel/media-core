// Auth state on start: a live session inspects to authenticated, a missing
// session to anonymous, and the sign-out action flips back to anonymous.
import { act, cleanup, fireEvent, render, screen } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import type { JSX } from "react";
import { MemoryRouter } from "react-router-dom";
import { AuthProvider, useAuth } from "./AuthContext";
import { Shell } from "../shell/Shell";
import { LoginPage } from "./LoginPage";
import { ApiError } from "../api/client";

const { mockInspectSession, mockLogin, mockLogout, mockSetCSRFToken } = vi.hoisted(() => ({
  mockInspectSession: vi.fn(),
  mockLogin: vi.fn(),
  mockLogout: vi.fn(),
  mockSetCSRFToken: vi.fn(),
}));

vi.mock("../api/client", async (importOriginal) => {
  const actual = await importOriginal<typeof import("../api/client")>();
  return {
    ...actual,
    inspectSession: (...args: unknown[]) => mockInspectSession(...args),
    login: (...args: unknown[]) => mockLogin(...args),
    logout: (...args: unknown[]) => mockLogout(...args),
    setCSRFToken: (...args: unknown[]) => mockSetCSRFToken(...args),
  };
});

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

  it("a late failed inspection cannot clear state after a completed login", async () => {
    let rejectInspect: (error: Error) => void = () => undefined;
    mockInspectSession.mockImplementationOnce(
      () =>
        new Promise<boolean>((_resolve, reject) => {
          rejectInspect = reject;
        }),
    );
    mockLogin.mockResolvedValueOnce(undefined);
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
    const callsAfterLogin = mockSetCSRFToken.mock.calls.length;
    rejectInspect(new Error("late failure"));
    await act(async () => undefined);
    expect(mockSetCSRFToken.mock.calls.slice(callsAfterLogin)).toEqual([]);
    expect((screen.getByTestId("status")).textContent).toBe("authenticated");
  });

  it("expires the inspection generation after a completed login", async () => {
    let resolveInspect: (live: boolean) => void = () => undefined;
    let guard: (() => boolean) | undefined;
    mockInspectSession.mockImplementationOnce((shouldApply: () => boolean) => {
      guard = shouldApply;
      return new Promise<boolean>((resolve) => {
        resolveInspect = resolve;
      });
    });
    mockLogin.mockResolvedValueOnce(undefined);
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
    expect(guard).toBeDefined();
    expect(guard?.()).toBe(false);
    resolveInspect(true);
    await act(async () => undefined);
    expect((screen.getByTestId("status")).textContent).toBe("authenticated");
  });
  it("expires the inspection generation before the login promise resolves", async () => {
    let resolveInspect: (live: boolean) => void = () => undefined;
    let resolveLogin: () => void = () => undefined;
    let guard: (() => boolean) | undefined;
    mockInspectSession.mockImplementationOnce((shouldApply: () => boolean) => {
      guard = shouldApply;
      return new Promise<boolean>((resolve) => {
        resolveInspect = resolve;
      });
    });
    mockLogin.mockImplementationOnce(
      () =>
        new Promise<void>((resolve) => {
          resolveLogin = resolve;
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
    expect(guard?.()).toBe(false);
    await act(async () => {
      resolveInspect(true);
    });
    expect((screen.getByTestId("status")).textContent).toBe("inspecting");
    await act(async () => {
      resolveLogin();
    });
    expect((screen.getByTestId("status")).textContent).toBe("authenticated");
  });

  it("lets the initial inspection settle after a failed login", async () => {
    let resolveInspect: (live: boolean) => void = () => undefined;
    mockInspectSession.mockImplementationOnce(
      () =>
        new Promise<boolean>((resolve) => {
          resolveInspect = resolve;
        }),
    );
    mockLogin.mockRejectedValueOnce(new ApiError("unauthorized", 401, "bad secret", false));
    render(
      <AuthProvider>
        <LoginPage />
        <StatusProbe />
      </AuthProvider>,
    );
    const input = (await screen.findByLabelText("Owner secret")) as HTMLInputElement;
    fireEvent.change(input, { target: { value: "wrong" } });
    fireEvent.click(screen.getByRole("button", { name: /sign in/i }));
    await vi.waitFor(() => expect(screen.queryByRole("alert")).not.toBeNull());
    await act(async () => {
      resolveInspect(false);
    });
    expect((screen.getByTestId("status")).textContent).toBe("anonymous");
  });

});
