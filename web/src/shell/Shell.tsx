// The responsive app shell: a persistent sidebar on desktop (compact
// horizontal navigation on narrow screens) and a compact header with a
// library search entry and a Quick Add action. Routes and the auth guard live
// in App.tsx and are untouched; a failed sign-out keeps the session and
// shows the server's message.
import { useState } from "react";
import type { FormEvent, JSX } from "react";
import { NavLink, Outlet, useNavigate } from "react-router-dom";
import { ApiError } from "../api/client";
import { useAuth } from "../auth/AuthContext";
import { Button, Input } from "../ui";

export function Shell(): JSX.Element {
  const { logout } = useAuth();
  const navigate = useNavigate();
  const [logoutError, setLogoutError] = useState<string | null>(null);
  const [find, setFind] = useState("");

  async function signOut(): Promise<void> {
    setLogoutError(null);
    try {
      await logout();
    } catch (err) {
      setLogoutError(err instanceof ApiError ? err.message : "sign out failed; try again");
    }
  }

  function submitFind(event: FormEvent): void {
    event.preventDefault();
    const query = find.trim();
    navigate(query === "" ? "/" : `/?query=${encodeURIComponent(query)}`);
  }

  return (
    <div className="shell">
      <aside className="shell-sidebar">
        <span className="shell-title">Media Core</span>
        <nav aria-label="Main">
          <NavLink to="/" end>
            Library
          </NavLink>
          <NavLink to="/add">Quick add</NavLink>
          <NavLink to="/activity">Activity</NavLink>
        </nav>
        <div className="shell-sidebar-footer">
          {logoutError !== null && (
            <p className="form-error" role="alert">
              {logoutError}
            </p>
          )}
          <Button className="sign-out" onClick={() => void signOut()}>
            Sign out
          </Button>
        </div>
      </aside>
      <div className="shell-main">
        <header className="shell-header">
          <form className="shell-header-form" role="search" onSubmit={submitFind}>
            <Input
              type="search"
              aria-label="Find in library"
              placeholder="Find in library"
              value={find}
              onChange={(event) => setFind(event.target.value)}
            />
            <Button size="sm" type="submit">
              Find
            </Button>
          </form>
          <Button
            className="shell-header-action"
            variant="primary"
            onClick={() => navigate("/add")}
          >
            Add work
          </Button>
        </header>
        <main className="shell-content">
          <Outlet />
        </main>
      </div>
    </div>
  );
}
