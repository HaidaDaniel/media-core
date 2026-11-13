// Login screen behavior: a failed attempt (wrong secret, rate limit)
// keeps the typed input and shows the server's message; a successful
// attempt exchanges the secret through the client and never stores it.
import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { ApiError } from "../api/client";
import { AuthProvider } from "./AuthContext";
import { LoginPage } from "./LoginPage";

const { mockInspectSession, mockLogin, mockLogout } = vi.hoisted(() => ({
  mockInspectSession: vi.fn(),
  mockLogin: vi.fn(),
  mockLogout: vi.fn(),
}));

vi.mock("../api/client", () => {
  class MockApiError extends Error {
    code: string;
    status: number;
    retryable: boolean;

    constructor(code: string, status: number, message: string, retryable: boolean) {
      super(message);
      this.name = "ApiError";
      this.code = code;
      this.status = status;
      this.retryable = retryable;
    }
  }
  return {
    ApiError: MockApiError,
    inspectSession: (...args: unknown[]) => mockInspectSession(...args),
    login: (...args: unknown[]) => mockLogin(...args),
    logout: (...args: unknown[]) => mockLogout(...args),
    setCSRFToken: vi.fn(),
  };
});

function renderLogin(): void {
  render(
    <AuthProvider>
      <LoginPage />
    </AuthProvider>,
  );
}

beforeEach(() => {
  mockInspectSession.mockResolvedValue(false);
  mockLogin.mockReset();
  mockLogout.mockReset();
});

afterEach(() => {
  cleanup();
  vi.clearAllMocks();
});

describe("LoginPage", () => {
  it("keeps the typed secret and shows the server message on a wrong secret", async () => {
    mockLogin.mockRejectedValueOnce(new ApiError("unauthorized", 401, "owner secret does not match", false));
    renderLogin();
    const input = (await screen.findByLabelText("Owner secret")) as HTMLInputElement;
    fireEvent.change(input, { target: { value: "wrong-secret" } });
    fireEvent.click(screen.getByRole("button", { name: /sign in/i }));
    expect((await screen.findByRole("alert")).textContent).toContain("owner secret does not match");
    expect(input.value).toBe("wrong-secret");
  });

  it("keeps the typed secret and shows the rate limit message", async () => {
    mockLogin.mockRejectedValueOnce(new ApiError("rate_limited", 429, "too many login attempts; slow down", true));
    renderLogin();
    const input = (await screen.findByLabelText("Owner secret")) as HTMLInputElement;
    fireEvent.change(input, { target: { value: "secret-1" } });
    fireEvent.click(screen.getByRole("button", { name: /sign in/i }));
    expect((await screen.findByRole("alert")).textContent).toContain("too many login attempts; slow down");
    expect(input.value).toBe("secret-1");
  });

  it("does not submit an empty secret", async () => {
    renderLogin();
    await screen.findByLabelText("Owner secret");
    fireEvent.click(screen.getByRole("button", { name: /sign in/i }));
    expect(mockLogin).not.toHaveBeenCalled();
  });

  it("exchanges the secret through the client on success", async () => {
    mockLogin.mockResolvedValueOnce(undefined);
    renderLogin();
    const input = (await screen.findByLabelText("Owner secret")) as HTMLInputElement;
    fireEvent.change(input, { target: { value: "owner-secret" } });
    fireEvent.click(screen.getByRole("button", { name: /sign in/i }));
    expect(await vi.waitFor(() => mockLogin)).toHaveBeenCalledWith("owner-secret");
  });
});
