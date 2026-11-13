// The responsive app shell: the navigation header and the routed content.
// The nav wraps on narrow screens (360px) and every link is keyboard
// reachable; the session state shows the sign-out action when live. A
// failed sign-out keeps the session and shows the server's message.
import { useState } from "react";
import type { JSX } from "react";
import { NavLink, Outlet } from "react-router-dom";
import { ApiError } from "../api/client";
import { useAuth } from "../auth/AuthContext";

export function Shell(): JSX.Element {
  const { logout } = useAuth();
  const [logoutError, setLogoutError] = useState<string | null>(null);

  async function signOut(): Promise<void> {
    setLogoutError(null);
    try {
      await logout();
    } catch (err) {
      setLogoutError(err instanceof ApiError ? err.message : "sign out failed; try again");
    }
  }

  return (
    <div className="shell">
      <header className="shell-header">
        <span className="shell-title">Media Core</span>
        <nav aria-label="Main">
          <NavLink to="/" end>
            Library
          </NavLink>
          <NavLink to="/add">Quick add</NavLink>
          <NavLink to="/activity">Activity</NavLink>
        </nav>
        <button type="button" className="sign-out" onClick={() => void signOut()}>
          Sign out
        </button>
        {logoutError !== null && (
          <p className="form-error" role="alert">
            {logoutError}
          </p>
        )}
      </header>
      <main className="shell-content">
        <Outlet />
      </main>
    </div>
  );
}
