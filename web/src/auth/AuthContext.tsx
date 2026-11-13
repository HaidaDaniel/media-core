// The single-owner session state of the app: one in-memory inspection on
// start, then login/logout through the API client. The owner secret and
// the CSRF token never touch browser storage.
import { createContext, useCallback, useContext, useEffect, useMemo, useState } from "react";
import type { JSX, ReactNode } from "react";
import { inspectSession, login as apiLogin, logout as apiLogout, setCSRFToken } from "../api/client";

export type AuthStatus = "inspecting" | "anonymous" | "authenticated";

interface AuthContextValue {
  status: AuthStatus;
  login: (ownerSecret: string) => Promise<void>;
  logout: () => Promise<void>;
}

const AuthContext = createContext<AuthContextValue | null>(null);

export function AuthProvider({ children }: { children: ReactNode }): JSX.Element {
  const [status, setStatus] = useState<AuthStatus>("inspecting");

  useEffect(() => {
    let cancelled = false;
    // The initial inspection settles only while the provider is still
    // inspecting, so a slow response can never overwrite a completed
    // login or logout.
    inspectSession()
      .then((live) => {
        if (!cancelled) {
          setStatus((current) =>
            current === "inspecting" ? (live ? "authenticated" : "anonymous") : current,
          );
        }
      })
      .catch(() => {
        if (!cancelled) {
          setCSRFToken(null);
          setStatus((current) => (current === "inspecting" ? "anonymous" : current));
        }
      });
    return () => {
      cancelled = true;
    };
  }, []);

  const login = useCallback(async (ownerSecret: string) => {
    await apiLogin(ownerSecret);
    setStatus("authenticated");
  }, []);

  // A failed logout throws to the caller and keeps the live session
  // state: the server session is invalidated only on success.
  const logout = useCallback(async () => {
    await apiLogout();
    setStatus("anonymous");
  }, []);

  const value = useMemo(() => ({ status, login, logout }), [status, login, logout]);
  return <AuthContext.Provider value={value}>{children}</AuthContext.Provider>;
}

export function useAuth(): AuthContextValue {
  const ctx = useContext(AuthContext);
  if (ctx === null) {
    throw new Error("useAuth must be used inside AuthProvider");
  }
  return ctx;
}
