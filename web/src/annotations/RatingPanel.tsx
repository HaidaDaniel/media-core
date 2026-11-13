// The compact rating panel: the current 1-10 rating with its comment, an
// explicit set action, the explicit clear (a null observation, never zero)
// and a mistake correction that names the current observation it supersedes.
// Prior and superseded ratings stay visible in the screen's history section;
// the panel only composes the controls. A failed action keeps the typed
// input beside a distinguishable error and the prior state intact.
import { useState } from "react";
import type { JSX } from "react";
import { ApiError } from "../api/client";
import type { HistoryItemWire } from "../edition/editionApi";
import { setRating, type AnnotatedEditionView } from "./annotationsApi";

const RATING_VALUES = [1, 2, 3, 4, 5, 6, 7, 8, 9, 10] as const;

interface RatingPanelProps {
  edition: AnnotatedEditionView;
  // The screen's observation history; the effective rating rows of this
  // edition are the only records a correction may name.
  history: HistoryItemWire[];
  // Applies the server's updated edition view (and refreshes history).
  onUpdated: (view: AnnotatedEditionView) => void;
}

function errorMessage(e: unknown): string {
  if (e instanceof ApiError) return e.message;
  if (e instanceof Error) return e.message;
  return "Something went wrong";
}

export function RatingPanel({
  edition,
  history,
  onUpdated,
}: RatingPanelProps): JSX.Element {
  const [mode, setMode] = useState<"new" | "correct">("new");
  const [value, setValue] = useState<string>("");
  const [comment, setComment] = useState("");
  const [supersedes, setSupersedes] = useState("");
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<string | null>(null);

  // Only effective rating rows are correctable: a superseded or cleared
  // (null value) row is rejected by the service, so it is never offered.
  const correctable = history.filter(
    (item) => item.type === "rating" && item.label === "effective" && item.value !== null,
  );

  const submit = async () => {
    if (mode === "correct" && supersedes === "") {
      setError("Choose the rating to correct.");
      return;
    }
    setBusy(true);
    setError(null);
    try {
      const view = await setRating({
        editionId: edition.id,
        // An empty selection is the explicit clear, sent as a null value.
        value: value === "" ? null : Number(value),
        comment: comment.trim() === "" ? undefined : comment.trim(),
        supersedesId: mode === "correct" ? supersedes : undefined,
        expectedRevision: edition.revision,
      });
      setComment("");
      setSupersedes("");
      setMode("new");
      onUpdated(view);
    } catch (e) {
      setError(errorMessage(e));
    } finally {
      setBusy(false);
    }
  };

  const current = edition.rating;

  return (
    <div>
      <p className="login-hint">
        {current !== null && current.value !== null
          ? `Rating: ${current.value}/10` +
            (current.comment !== null ? ` — ${current.comment}` : "")
          : "Rating: no rating"}
      </p>
      {error !== null && (
        <div className="library-error">
          <p role="alert">{error}</p>
        </div>
      )}
      <form
        className="library-toolbar"
        onSubmit={(e) => {
          e.preventDefault();
          void submit();
        }}
      >
        <label className="library-field">
          <span>Rating action</span>
          <select
            value={mode}
            onChange={(e) => {
              setMode(e.target.value === "correct" ? "correct" : "new");
              setSupersedes("");
            }}
            disabled={busy}
          >
            <option value="new">record new rating</option>
            <option value="correct" disabled={correctable.length === 0}>
              correct mistake
            </option>
          </select>
        </label>
        <label className="library-field">
          <span>Rating</span>
          <select
            value={value}
            onChange={(e) => setValue(e.target.value)}
            disabled={busy}
          >
            <option value="">clear (no rating)</option>
            {RATING_VALUES.map((n) => (
              <option key={n} value={n}>
                {n}/10
              </option>
            ))}
          </select>
        </label>
        {mode === "correct" && (
          <label className="library-field">
            <span>Rating to correct</span>
            <select
              value={supersedes}
              onChange={(e) => setSupersedes(e.target.value)}
              disabled={busy}
            >
              <option value="">Choose a rating…</option>
              {correctable.map((item) => (
                <option key={item.id} value={item.id}>
                  {item.value}/10
                  {item.detail !== null ? ` — ${item.detail}` : ""}
                </option>
              ))}
            </select>
          </label>
        )}
        <label className="library-field">
          <span>Rating comment</span>
          <input
            type="text"
            value={comment}
            onChange={(e) => setComment(e.target.value)}
            disabled={busy}
          />
        </label>
        <button type="submit" disabled={busy}>
          {mode === "correct" ? "Correct rating" : "Set rating"}
        </button>
      </form>
    </div>
  );
}
