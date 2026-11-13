// The compact tag panel: the edition's active tags with explicit detach
// actions and a plain attach input. Tag names are normalized and bounded by
// the server (1-64 characters); attach and detach are idempotent, audited
// and never alter consumption, so a translation-candidate tag is searchable
// without any workflow engine. A failed action keeps the typed tag beside a
// distinguishable error and the prior tags intact.
import { useState } from "react";
import type { JSX } from "react";
import { ApiError } from "../api/client";
import { attachTag, detachTag, type AnnotatedEditionView } from "./annotationsApi";

interface TagsPanelProps {
  edition: AnnotatedEditionView;
  // Applies the server's updated edition view (and refreshes history).
  onUpdated: (view: AnnotatedEditionView) => void;
}

function errorMessage(e: unknown): string {
  if (e instanceof ApiError) return e.message;
  if (e instanceof Error) return e.message;
  return "Something went wrong";
}

export function TagsPanel({ edition, onUpdated }: TagsPanelProps): JSX.Element {
  const [tag, setTag] = useState("");
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<string | null>(null);

  const run = async (action: () => Promise<void>) => {
    setBusy(true);
    setError(null);
    try {
      await action();
    } catch (e) {
      setError(errorMessage(e));
    } finally {
      setBusy(false);
    }
  };

  const attach = () => {
    const name = tag.trim();
    if (name === "") {
      setError("Enter a tag name.");
      return;
    }
    return run(async () => {
      const view = await attachTag({
        target: "edition",
        targetId: edition.id,
        tag: name,
        expectedRevision: edition.revision,
      });
      setTag("");
      onUpdated(view);
    });
  };

  const detach = (name: string) =>
    run(async () => {
      const view = await detachTag({
        target: "edition",
        targetId: edition.id,
        tag: name,
        expectedRevision: edition.revision,
      });
      onUpdated(view);
    });

  return (
    <div>
      {edition.tags.length > 0 ? (
        <ul className="library-list">
          {edition.tags.map((t) => (
            <li key={t} className="work-edition">
              <span>{t}</span>
              <button
                type="button"
                className="library-more"
                disabled={busy}
                onClick={() => void detach(t)}
              >
                Detach {t}
              </button>
            </li>
          ))}
        </ul>
      ) : (
        <p className="login-hint">No tags.</p>
      )}
      {error !== null && (
        <div className="library-error">
          <p role="alert">{error}</p>
        </div>
      )}
      <form
        className="library-toolbar"
        onSubmit={(e) => {
          e.preventDefault();
          void attach();
        }}
      >
        <label className="library-field">
          <span>Tag</span>
          <input
            type="text"
            value={tag}
            placeholder="manga-translation-candidate"
            onChange={(e) => setTag(e.target.value)}
            disabled={busy}
          />
        </label>
        <button type="submit" disabled={busy}>
          Attach tag
        </button>
      </form>
    </div>
  );
}
