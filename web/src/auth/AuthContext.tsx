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
    inspectSession()
      .then((live) => {
        if (!cancelled) {
          setStatus(live ? "authenticated" : "anonymous");
        }
      })
      .catch(() => {
        if (!cancelled) {
          setCSRFToken(null);
          setStatus("anonymous");
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

  const logout = useCallback(async () => {
    try {
      await apiLogout();
    } finally {
      setStatus("anonymous");
    }
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
