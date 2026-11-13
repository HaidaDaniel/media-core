// The route table: the login screen for anonymous visitors and the shell
// for the live session. Route state lives in the URL; there is no global
// state framework.
import type { JSX } from "react";
import { BrowserRouter, Navigate, Route, Routes } from "react-router-dom";
import { AuthProvider, useAuth } from "./auth/AuthContext";
import { LoginPage } from "./auth/LoginPage";
import {
  ActivityPage,
  EditionPage,
  LibraryPage,
  NotFoundPage,
  QuickAddPage,
  WorkPage,
} from "./shell/Placeholders";
import { Shell } from "./shell/Shell";

function RequireAuth(): JSX.Element {
  const { status } = useAuth();
  if (status === "inspecting") {
    return <p className="shell-loading">Checking session…</p>;
  }
  if (status === "anonymous") {
    return <Navigate to="/login" replace />;
  }
  return <Shell />;
}

function PublicOnly(): JSX.Element {
  const { status } = useAuth();
  if (status === "authenticated") {
    return <Navigate to="/" replace />;
  }
  return <LoginPage />;
}

export function App(): JSX.Element {
  return (
    <AuthProvider>
      <BrowserRouter>
        <Routes>
          <Route path="/login" element={<PublicOnly />} />
          <Route path="/" element={<RequireAuth />}>
            <Route index element={<LibraryPage />} />
            <Route path="add" element={<QuickAddPage />} />
            <Route path="works/:id" element={<WorkPage />} />
            <Route path="editions/:id" element={<EditionPage />} />
            <Route path="activity" element={<ActivityPage />} />
            <Route path="*" element={<NotFoundPage />} />
          </Route>
        </Routes>
      </BrowserRouter>
    </AuthProvider>
  );
}
