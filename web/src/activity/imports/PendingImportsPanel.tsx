// The pending-imports control mounted in the recent-activity screen (task
// 0043). It lists the bounded queue of imported events with no active binding
// and offers the explicit bind: a publication is mapped to an edition by id,
// never inferred from a title. A bind that collides with a different active
// edition is a distinguishable conflict (use rebind to correct it); the prior
// queue stays visible beside the error. When the integration read is
// unavailable the control degrades to a note rather than an error state, so
// the library still works without Release Watcher.
import { useCallback, useEffect, useRef, useState } from "react";
import type { JSX } from "react";
import { ApiError } from "../../api/client";
import {
  bindRelease,
  listPendingImports,
  type PendingImportWire,
} from "../../api/releases";

const LIMIT = 50;

function errorMessage(err: unknown): string {
  return err instanceof ApiError ? err.message : "the pending imports could not be loaded";
}

// A one-line summary of the producer's release fields; absent fields are
// omitted, never invented, and counts are shown exactly as the producer sent
// them (never summed).
function releaseSummary(item: PendingImportWire): string {
  const parts: string[] = [];
  if (item.release_number !== null && item.release_number !== undefined) {
    parts.push(`#${item.release_number}`);
  }
  if (item.release_title) {
    parts.push(item.release_title);
  }
  if (item.count_delta !== null && item.count_delta !== undefined) {
    parts.push(`Δ${item.count_delta}`);
  }
  if (item.current_count !== null && item.current_count !== undefined) {
    parts.push(`count ${item.current_count}`);
  }
  return parts.join(" · ");
}

export function PendingImportsPanel(): JSX.Element | null {
  const [items, setItems] = useState<PendingImportWire[] | null>(null);
  const [total, setTotal] = useState<number | null>(null);
  const [error, setError] = useState<string | null>(null);
  const [retryTick, setRetryTick] = useState(0);
  const [busy, setBusy] = useState(false);
  const [bindErrors, setBindErrors] = useState<Record<string, string>>({});
  // Per-row edition id drafts; the input is retained on a failed bind so the
  // user can correct and retry without re-typing.
  const [editionDrafts, setEditionDrafts] = useState<Record<string, string>>({});
  const generation = useRef(0);

  // Load the pending queue; a newer request invalidates the prior one so a
  // stale response never overwrites a fresh result.
  useEffect(() => {
    const gen = ++generation.current;
    setError(null);
    listPendingImports(LIMIT)
      .then((payload) => {
        if (gen !== generation.current) return;
        setItems(payload.items);
        setTotal(payload.total);
      })
      .catch((err: unknown) => {
        if (gen === generation.current) setError(errorMessage(err));
      });
  }, [retryTick]);

  const reload = useCallback(() => {
    setRetryTick((t) => t + 1);
  }, []);

  // bind maps one pending event's publication to an edition by id. On success
  // the event leaves the queue, so the queue is reloaded.
  const bind = useCallback(
    async (item: PendingImportWire, editionId: string) => {
      if (!item.publication_id) return;
      const edition = editionId.trim();
      if (edition === "") {
        setBindErrors((prev) => ({ ...prev, [item.id]: "Enter the edition id." }));
        return;
      }
      setBusy(true);
      setBindErrors((prev) => {
        const next = { ...prev };
        delete next[item.id];
        return next;
      });
      try {
        await bindRelease({ publicationId: item.publication_id, editionId: edition });
        reload();
      } catch (err) {
        setBindErrors((prev) => ({ ...prev, [item.id]: errorMessage(err) }));
      } finally {
        setBusy(false);
      }
    },
    [reload],
  );

  if (items === null && total === null && error === null) {
    return null;
  }

  return (
    <div className="activity-imports">
      <h3>Pending imports</h3>
      {error !== null ? (
        <p className="login-hint" role="status">
          Import history is unavailable: {error}
          {" "}
          <button type="button" onClick={reload}>
            Reload
          </button>
        </p>
      ) : (
        <>
          {total !== null && (
            <p className="library-count">
              {total} {total === 1 ? "pending import" : "pending imports"}
            </p>
          )}
          {items && items.length === 0 ? (
            <p className="library-loading">Nothing is waiting to be bound.</p>
          ) : (
            <ul className="library-list">
              {items?.map((item) => {
                const bindable = item.publication_id !== null;
                return (
                  <li key={item.id} className="work-card">
                    <span className="work-badge work-badge-imported">
                      {item.event_type.replace(/_/g, " ")}
                    </span>
                    <span>{item.publication_id ?? item.event_id}</span>
                    {releaseSummary(item) !== "" && (
                      <span>{releaseSummary(item)}</span>
                    )}
                    {item.source_time !== null && item.source_time !== undefined && (
                      <span className="login-hint">source {item.source_time}</span>
                    )}
                    {bindable && (
                      <form
                        className="library-toolbar"
                        onSubmit={(e) => {
                          e.preventDefault();
                          void bind(item, editionDrafts[item.id] ?? "");
                        }}
                      >
                        <label className="library-field">
                          <span>Edition id</span>
                          <input
                            type="text"
                            value={editionDrafts[item.id] ?? ""}
                            onChange={(e) =>
                              setEditionDrafts((prev) => ({
                                ...prev,
                                [item.id]: e.target.value,
                              }))
                            }
                            disabled={busy}
                          />
                        </label>
                        <button type="submit" disabled={busy}>
                          Bind
                        </button>
                      </form>
                    )}
                    {!bindable && (
                      <span className="login-hint">no publication id; cannot be bound</span>
                    )}
                    {bindErrors[item.id] !== undefined && (
                      <div className="library-error" role="alert">
                        <p>{bindErrors[item.id]}</p>
                      </div>
                    )}
                  </li>
                );
              })}
            </ul>
          )}
        </>
      )}
    </div>
  );
}
