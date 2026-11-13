// The responsive app shell: the navigation header and the routed content.
// The nav wraps on narrow screens (360px) and every link is keyboard
// reachable; the session state shows the sign-out action when live.
import { NavLink, Outlet } from "react-router-dom";
import type { JSX } from "react";
import { useAuth } from "../auth/AuthContext";

export function Shell(): JSX.Element {
  const { logout } = useAuth();

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
        <button type="button" className="sign-out" onClick={() => void logout()}>
          Sign out
        </button>
      </header>
      <main className="shell-content">
        <Outlet />
      </main>
    </div>
  );
}
