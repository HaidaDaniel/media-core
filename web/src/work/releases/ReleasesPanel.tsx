// The release-history panel mounted per edition in the work screen (task
// 0043). It reads the bounded projection of the edition's imported releases
// and renders each row labeled by the exact publication it came from — the
// producer's fields only, never fabricated, and counts never summed. It is a
// pure read: personal progress and intention are never touched, and when the
// read is unavailable the panel degrades to a note rather than an error, so
// the library still works without Release Watcher.
import { useEffect, useRef, useState } from "react";
import type { JSX } from "react";
import { ApiError } from "../../api/client";
import {
  listReleases,
  type ReleaseProjectionWire,
} from "../../api/releases";

const LIMIT = 50;

function errorMessage(err: unknown): string {
  return err instanceof ApiError ? err.message : "the release history could not be loaded";
}

// A one-line summary of the producer's release fields; absent fields are
// omitted, never invented, and counts are shown exactly as sent (never
// summed).
function releaseSummary(item: ReleaseProjectionWire): string {
  const parts: string[] = [];
  if (item.release_number) {
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

function formatInstant(iso: string): string {
  const date = new Date(iso);
  return Number.isNaN(date.getTime()) ? iso : date.toLocaleString();
}

// ReleasesPanel renders the labeled release history of one edition.
export function ReleasesPanel({ editionId }: { editionId: string }): JSX.Element {
  const [items, setItems] = useState<ReleaseProjectionWire[] | null>(null);
  const [total, setTotal] = useState<number | null>(null);
  const [error, setError] = useState<string | null>(null);
  const [retryTick, setRetryTick] = useState(0);
  const generation = useRef(0);

  useEffect(() => {
    const gen = ++generation.current;
    setError(null);
    listReleases(editionId, LIMIT)
      .then((payload) => {
        if (gen !== generation.current) return;
        setItems(payload.items);
        setTotal(payload.total);
      })
      .catch((err: unknown) => {
        if (gen === generation.current) setError(errorMessage(err));
      });
  }, [editionId, retryTick]);

  return (
    <div className="work-releases">
      <h4>Releases</h4>
      {error !== null ? (
        <p className="login-hint" role="status">
          Release history is unavailable: {error}
          {" "}
          <button type="button" onClick={() => setRetryTick((t) => t + 1)}>
            Reload
          </button>
        </p>
      ) : items === null ? (
        <p className="library-loading">Loading releases…</p>
      ) : items.length === 0 ? (
        <p className="login-hint">No imported releases are bound to this edition.</p>
      ) : (
        <>
          {total !== null && (
            <p className="library-count">
              {total} {total === 1 ? "release" : "releases"}
            </p>
          )}
          <ul className="library-list">
            {items.map((item) => (
              <li key={item.event_id} className="work-edition">
                <span className="work-badge work-badge-imported">
                  {item.event_type.replace(/_/g, " ")}
                </span>
                <span>{releaseSummary(item)}</span>
                <span className="login-hint">
                  from {item.publication_id ?? item.namespace}
                </span>
                {item.source_time !== null && item.source_time !== undefined && (
                  <span className="login-hint">
                    observed {formatInstant(item.source_time)} ·{" "}
                  </span>
                )}
                <span className="login-hint">
                  imported {formatInstant(item.created_at)}
                </span>
              </li>
            ))}
          </ul>
        </>
      )}
    </div>
  );
}
