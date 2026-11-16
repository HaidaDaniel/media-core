// The single-owner session state of the app: one in-memory inspection on
// start, then login/logout through the API client. The owner secret and
// the CSRF token never touch browser storage.
import { createContext, useCallback, useContext, useEffect, useMemo, useRef, useState } from "react";
import type { JSX, ReactNode } from "react";
import { inspectSession, login as apiLogin, logout as apiLogout } from "../api/client";

export type AuthStatus = "inspecting" | "anonymous" | "authenticated";

interface AuthContextValue {
  status: AuthStatus;
  login: (ownerSecret: string) => Promise<void>;
  logout: () => Promise<void>;
}

const AuthContext = createContext<AuthContextValue | null>(null);

export function AuthProvider({ children }: { children: ReactNode }): JSX.Element {
  const [status, setStatus] = useState<AuthStatus>("inspecting");
  // Every successful login or logout advances the generation. The initial
  // inspection applies its token and status only while its own generation
  // is still current, so a late response cannot undo a completed action.
  const generation = useRef(0);

  useEffect(() => {
    let cancelled = false;
    const mine = generation.current;
    const current = () => !cancelled && generation.current === mine;
    inspectSession(current)
      .then((live) => {
        if (current()) {
          setStatus((state) =>
            state === "inspecting" ? (live ? "authenticated" : "anonymous") : state,
          );
        }
      })
      .catch(() => {
        if (current()) {
          setStatus((state) => (state === "inspecting" ? "anonymous" : state));
        }
      });
    return () => {
      cancelled = true;
    };
  }, []);

  const login = useCallback(async (ownerSecret: string) => {
    const previous = generation.current;
    generation.current += 1;
    try {
      await apiLogin(ownerSecret);
      setStatus("authenticated");
    } catch (err) {
      if (generation.current === previous + 1) {
        generation.current = previous;
      }
      throw err;
    }
  }, []);

  // A failed logout throws to the caller and keeps the live session
  // state: the server session is invalidated only on success.
  const logout = useCallback(async () => {
    const previous = generation.current;
    generation.current += 1;
    try {
      await apiLogout();
      setStatus("anonymous");
    } catch (err) {
      if (generation.current === previous + 1) {
        generation.current = previous;
      }
      throw err;
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
