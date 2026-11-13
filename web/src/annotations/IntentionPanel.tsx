// The compact intention panel: the current intention with its reason and
// the explicit set action over the five bounded values, where "none" is the
// deliberate retraction to no recorded intention. The server decides value
// validity and no-op semantics; a failed set keeps the typed input beside a
// distinguishable error and the prior state intact.
import { useState } from "react";
import type { JSX } from "react";
import { ApiError } from "../api/client";
import {
  INTENTION_OPTIONS,
  setIntention,
  type AnnotatedEditionView,
} from "./annotationsApi";

interface IntentionPanelProps {
  edition: AnnotatedEditionView;
  // Applies the server's updated edition view (and refreshes history).
  onUpdated: (view: AnnotatedEditionView) => void;
}

function errorMessage(e: unknown): string {
  if (e instanceof ApiError) return e.message;
  if (e instanceof Error) return e.message;
  return "Something went wrong";
}

export function IntentionPanel({ edition, onUpdated }: IntentionPanelProps): JSX.Element {
  const [value, setValue] = useState<string>(INTENTION_OPTIONS[0]);
  const [reason, setReason] = useState("");
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<string | null>(null);

  const submit = async () => {
    setBusy(true);
    setError(null);
    try {
      const view = await setIntention({
        editionId: edition.id,
        value,
        reason: reason.trim() === "" ? undefined : reason.trim(),
        expectedRevision: edition.revision,
      });
      setReason("");
      onUpdated(view);
    } catch (e) {
      setError(errorMessage(e));
    } finally {
      setBusy(false);
    }
  };

  const current = edition.intention;

  return (
    <div>
      <p className="login-hint">
        {current !== null
          ? `Intention: ${current.value.replace(/_/g, " ")}` +
            (current.reason !== null ? ` — ${current.reason}` : "")
          : "Intention: none recorded"}
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
          <span>Intention</span>
          <select
            value={value}
            onChange={(e) => setValue(e.target.value)}
            disabled={busy}
          >
            {INTENTION_OPTIONS.map((v) => (
              <option key={v} value={v}>
                {v === "none" ? "no intention (retract)" : v.replace(/_/g, " ")}
              </option>
            ))}
          </select>
        </label>
        <label className="library-field">
          <span>Intention reason</span>
          <input
            type="text"
            value={reason}
            onChange={(e) => setReason(e.target.value)}
            disabled={busy}
          />
        </label>
        <button type="submit" disabled={busy}>
          Set intention
        </button>
      </form>
    </div>
  );
}
