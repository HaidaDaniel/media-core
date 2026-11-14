// The recent-activity screen: the bounded feed of personal changes with
// useful dates and links rather than analytics. The server projects every
// history source in (recorded_at, id) descending order, and the screen
// renders the payload order as-is — it never re-sorts. Each row carries its
// dimension, the correction label the projection computed (so a superseded
// or retracted row is shown, never hidden), the target title with a link to
// the edition or work, the reason when one was recorded, and the observed
// and recorded times. Imported-release rows (none exist before task 0042's
// importer lands) are rendered with a distinct label and only the fields
// the payload carries — no release data is ever fabricated. A failed first
// load shows the error alone and a failed page load keeps the prior feed
// beside a distinguishable error with a retry.
import { useCallback, useEffect, useRef, useState } from "react";
import type { JSX } from "react";
import { Link } from "react-router-dom";
import { ApiError } from "../api/client";
import { listActivity, type ActivityItemWire } from "./activityApi";

const LIMIT = 30;

// The display name of one feed row's dimension; the reserved imported
// release type is distinct from every personal dimension.
function typeLabel(type: string): string {
  switch (type) {
    case "consumption":
      return "status";
    case "progress":
      return "progress";
    case "intention":
      return "intention";
    case "rating":
      return "rating";
    case "publication":
      return "publication";
    case "note":
      return "note";
    case "reference":
      return "reference";
    case "relation":
      return "relation";
    case "origin":
      return "origin";
    case "tag":
      return "tag";
    case "imported_release":
      return "imported release";
    default:
      return type;
  }
}

// The row sentence from the payload fields only: nothing is invented when
// a field is absent, and an unknown dimension renders its raw value and
// detail rather than a guess.
function rowText(item: ActivityItemWire): string {
  const value = item.value;
  const detail = item.detail;
  switch (item.type) {
    case "progress":
      return value !== null ? `${value} ${detail ?? ""}`.trim() : "retracted";
    case "rating":
      if (value === null) return "cleared";
      return `${value}/10${detail !== null ? ` — ${detail}` : ""}`;
    case "consumption":
    case "intention":
      return value !== null ? value.replace(/_/g, " ") : "retracted";
    case "note":
      return value !== null ? `${value}${detail !== null ? ` (${detail})` : ""}` : "retracted";
    case "reference":
      return value !== null ? `${value} · ${detail ?? ""}`.trim() : "retired";
    default:
      // An unknown dimension without fields renders its badge and target
      // only; nothing is invented.
      return [value, detail].filter((part) => part !== null).join(" · ");
  }
}

function formatInstant(iso: string): string {
  const date = new Date(iso);
  return Number.isNaN(date.getTime()) ? iso : date.toLocaleString();
}

function TargetLink({ item }: { item: ActivityItemWire }): JSX.Element {
  const target =
    item.edition_id !== null
      ? { href: `/editions/${encodeURIComponent(item.edition_id)}`, label: "edition" }
      : { href: `/works/${encodeURIComponent(item.work_id)}`, label: "work" };
  const name = item.title ?? target.label;
  return (
    <Link to={target.href}>
      {name}
      {item.title === null ? ` (${target.label})` : ""}
    </Link>
  );
}

function ActivityRow({ item }: { item: ActivityItemWire }): JSX.Element {
  const text = rowText(item);
  return (
    <li className="activity-row">
      <span
        className={`work-badge ${item.type === "imported_release" ? "work-badge-imported" : ""}`}
      >
        {typeLabel(item.type)}
      </span>
      {text !== "" && <span>{text}</span>}
      <TargetLink item={item} />
      <span className={`work-badge work-badge-${item.label}`}>{item.label}</span>
      {item.reason !== null && <span className="login-hint">{item.reason}</span>}
      <span className="login-hint">
        {item.observed_at !== null ? `observed ${formatInstant(item.observed_at)} · ` : ""}
        recorded {formatInstant(item.recorded_at)}
      </span>
    </li>
  );
}

function errorMessage(err: unknown): string {
  return err instanceof ApiError ? err.message : "the activity feed could not be loaded";
}

export function ActivityPage(): JSX.Element {
  const [items, setItems] = useState<ActivityItemWire[]>([]);
  const [total, setTotal] = useState<number | null>(null);
  const [nextCursor, setNextCursor] = useState<string | null>(null);
  const [loading, setLoading] = useState(true);
  const [refreshing, setRefreshing] = useState(false);
  const [pageLoading, setPageLoading] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [retryTick, setRetryTick] = useState(0);
  const generation = useRef(0);

  // Load the first page; a newer request invalidates every in-flight one,
  // so a stale response is dropped, and the prior feed stays visible until
  // a response succeeds.
  useEffect(() => {
    const gen = ++generation.current;
    setError(null);
    if (total === null) {
      setLoading(true);
    } else {
      setRefreshing(true);
    }
    listActivity({ limit: LIMIT })
      .then((payload) => {
        if (gen !== generation.current) {
          return;
        }
        setItems(payload.items);
        setTotal(payload.total);
        setNextCursor(payload.next_cursor);
      })
      .catch((err: unknown) => {
        if (gen === generation.current) {
          setError(errorMessage(err));
          // A failed first page invalidates any cursor from the prior
          // request, so a retry starts a fresh feed.
          setNextCursor(null);
        }
      })
      .finally(() => {
        if (gen === generation.current) {
          setLoading(false);
          setRefreshing(false);
        }
      });
    // total is intentionally not a dependency: it only chooses between the
    // loading and refreshing states on the initial mount.
  }, [retryTick]);

  const loadMore = useCallback(() => {
    if (nextCursor === null) {
      return;
    }
    const gen = generation.current;
    const cursor = nextCursor;
    setPageLoading(true);
    listActivity({ limit: LIMIT, cursor })
      .then((payload) => {
        if (gen !== generation.current) {
          return;
        }
        setItems((prev) => [...prev, ...payload.items]);
        setNextCursor(payload.next_cursor);
      })
      .catch((err: unknown) => {
        if (gen === generation.current) {
          setError(errorMessage(err));
        }
      })
      .finally(() => {
        if (gen === generation.current) {
          setPageLoading(false);
        }
      });
  }, [nextCursor]);

  function retry(): void {
    setError(null);
    if (nextCursor !== null) {
      loadMore();
    } else {
      setRetryTick((t) => t + 1);
    }
  }

  return (
    <section className="activity">
      <h2>Recent activity</h2>
      {total !== null && (
        <p className="library-count" role="status">
          {total} {total === 1 ? "change" : "changes"}
        </p>
      )}
      {error !== null && (
        <div className="library-error" role="alert">
          <p>{error}</p>
          <button type="button" onClick={retry}>
            Retry
          </button>
        </div>
      )}
      {loading ? (
        <p className="library-loading">Loading activity…</p>
      ) : error !== null && items.length === 0 ? null : items.length === 0 ? (
        <p className="library-loading">No recent activity.</p>
      ) : (
        <>
          {refreshing && (
            <p className="library-refreshing" role="status">
              Updating…
            </p>
          )}
          <ul className="library-list">
            {items.map((item) => (
              <ActivityRow key={item.id} item={item} />
            ))}
          </ul>
        </>
      )}
      {nextCursor !== null && (
        <button
          type="button"
          className="library-more"
          disabled={pageLoading || loading || refreshing}
          onClick={() => void loadMore()}
        >
          {pageLoading ? "Loading more…" : "Load more"}
        </button>
      )}
    </section>
  );
}
