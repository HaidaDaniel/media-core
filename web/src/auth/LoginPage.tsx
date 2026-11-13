// The owner login screen: one password field for the owner secret. The
// secret is sent once over the same-origin API, never stored; a failed
// attempt keeps the input and shows the server's message.
import { useState } from "react";
import type { FormEvent, JSX } from "react";
import { ApiError } from "../api/client";
import { useAuth } from "./AuthContext";

export function LoginPage(): JSX.Element {
  const { login } = useAuth();
  const [secret, setSecret] = useState("");
  const [pending, setPending] = useState(false);
  const [error, setError] = useState<string | null>(null);

  async function submit(event: FormEvent): Promise<void> {
    event.preventDefault();
    if (pending || secret === "") {
      return;
    }
    setPending(true);
    setError(null);
    try {
      await login(secret);
      // On success the auth status flips and the app routes to the shell;
      // the input is left as-is so a failed follow-up keeps its text.
    } catch (err) {
      if (err instanceof ApiError) {
        setError(err.message);
      } else {
        setError("login failed; try again");
      }
    } finally {
      setPending(false);
    }
  }

  return (
    <main className="login-page">
      <h1>Media Core</h1>
      <p className="login-hint">Enter the owner secret to sign in.</p>
      <form onSubmit={(event) => void submit(event)}>
        <label htmlFor="owner-secret">Owner secret</label>
        <input
          id="owner-secret"
          type="password"
          autoComplete="current-password"
          value={secret}
          onChange={(event) => setSecret(event.target.value)}
          disabled={pending}
        />
        <button type="submit" disabled={pending || secret === ""}>
          {pending ? "Signing in…" : "Sign in"}
        </button>
      </form>
      {error !== null && (
        <p className="form-error" role="alert">
          {error}
        </p>
      )}
    </main>
  );
}
