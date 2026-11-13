// The daily library screen. All filter, sort and search state lives in the
// URL, so a refresh or a back step restores the exact listing; the search
// input is debounced before it touches the URL, and an obsolete response can
// never overwrite a newer one (logical cancellation by generation counter).
// The prior list and count stay visible until a response succeeds, so a
// failed first load shows the error alone and a failed change or page load
// keeps the prior list and count intact beside the error and a retry.
import { useCallback, useEffect, useRef, useState } from "react";
import type { JSX } from "react";
import { useSearchParams } from "react-router-dom";
import { ApiError } from "../api/client";
import { editionConsumption, listWorks, type WorkCardWire } from "./libraryApi";
import { WorkCardView } from "./WorkCardView";

const SEARCH_DEBOUNCE_MS = 250;
const LIMITS = [10, 30, 50, 100];
const DEFAULT_LIMIT = 30;
const KINDS = [
  "manga",
  "manhwa",
  "manhua",
  "web_novel",
  "light_novel",
  "book",
  "anime",
  "movie",
  "tv",
  "game",
  "visual_novel",
  "other",
  "unknown",
];
const CONSUMPTIONS = [
  "not_started",
  "in_progress",
  "paused",
  "completed",
  "dropped",
  "unknown",
];
const INTENTIONS = ["none", "interested", "saved", "planned", "not_interested"];
const MIN_RATINGS = [1, 2, 3, 4, 5, 6, 7, 8, 9, 10];
const SORTS = ["title", "created_at", "updated_at", "rating"];

function errorMessage(err: unknown): string {
  return err instanceof ApiError ? err.message : "the library could not be loaded";
}

export function LibraryPage(): JSX.Element {
  const [params, setParams] = useSearchParams();
  const query = params.get("query") ?? "";
  const kind = params.get("kind") ?? "";
  const consumption = params.get("consumption") ?? "";
  const intention = params.get("intention") ?? "";
  const tag = params.get("tag") ?? "";
  const minRating = MIN_RATINGS.includes(Number(params.get("min-rating")))
    ? Number(params.get("min-rating"))
    : undefined;
  const sort = SORTS.includes(params.get("sort") ?? "") ? (params.get("sort") as string) : "title";
  const limit = LIMITS.includes(Number(params.get("limit"))) ? Number(params.get("limit")) : DEFAULT_LIMIT;
  const view = params.get("view") === "cards" ? "cards" : "list";
  const activeFilter =
    query !== "" || kind !== "" || consumption !== "" || intention !== "" || tag !== "" ||
    minRating !== undefined;

  const [text, setText] = useState(query);
  const [tagText, setTagText] = useState(tag);
  const [items, setItems] = useState<WorkCardWire[]>([]);
  const [total, setTotal] = useState<number | null>(null);
  const [nextCursor, setNextCursor] = useState<string | null>(null);
  const [loading, setLoading] = useState(true);
  const [refreshing, setRefreshing] = useState(false);
  const [pageLoading, setPageLoading] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [states, setStates] = useState<Record<string, string>>({});
  const [retryTick, setRetryTick] = useState(0);
  const generation = useRef(0);
  const knownStates = useRef(new Set<string>());

  // Keep the inputs in sync with the URL (back/forward and resets).
  useEffect(() => {
    setText(query);
  }, [query]);

  useEffect(() => {
    setTagText(tag);
  }, [tag]);

  // Debounce the typed search before it changes the URL.
  useEffect(() => {
    if (text === query) {
      return;
    }
    const timer = setTimeout(() => {
      const next = new URLSearchParams(params);
      if (text.trim() === "") {
        next.delete("query");
      } else {
        next.set("query", text.trim());
      }
      setParams(next);
    }, SEARCH_DEBOUNCE_MS);
    return () => clearTimeout(timer);
  }, [text, query, params, setParams]);

  // Debounce the typed tag before it changes the URL; the server normalizes
  // the name, the screen only carries it through.
  useEffect(() => {
    if (tagText === tag) {
      return;
    }
    const timer = setTimeout(() => {
      const next = new URLSearchParams(params);
      if (tagText.trim() === "") {
        next.delete("tag");
      } else {
        next.set("tag", tagText.trim());
      }
      setParams(next);
    }, SEARCH_DEBOUNCE_MS);
    return () => clearTimeout(timer);
  }, [tagText, tag, params, setParams]);

  // Load the first page whenever the URL listing request changes. A newer
  // request invalidates every in-flight one, so a stale response is dropped,
  // and the prior list and count stay visible until a response succeeds.
  useEffect(() => {
    const gen = ++generation.current;
    setError(null);
    if (total === null) {
      setLoading(true);
    } else {
      setRefreshing(true);
    }
    listWorks({
      query: query || undefined,
      kind: kind ? [kind] : undefined,
      consumption: consumption ? [consumption] : undefined,
      intention: intention ? [intention] : undefined,
      tags: tag ? [tag] : undefined,
      minRating,
      sort,
      limit,
    })
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
          // request, so a retry starts a fresh listing.
          setNextCursor(null);
        }
      })
      .finally(() => {
        if (gen === generation.current) {
          setLoading(false);
          setRefreshing(false);
        }
      });
  }, [query, kind, consumption, intention, tag, minRating, sort, limit, retryTick]);

  // Resolve the per-edition consumption badges for the visible cards. The
  // list payload carries no per-edition state, so the values come from the
  // edition details; a failed detail never fabricates a state.
  useEffect(() => {
    const gen = generation.current;
    const ids = [...new Set(items.flatMap((c) => c.editions.map((e) => e.id)))];
    const missing = ids.filter((id) => !knownStates.current.has(id));
    if (missing.length === 0) {
      return;
    }
    let cancelled = false;
    void Promise.allSettled(missing.map((id) => editionConsumption(id))).then((results) => {
      if (cancelled || gen !== generation.current) {
        return;
      }
      const patch: Record<string, string> = {};
      results.forEach((result, i) => {
        if (result.status === "fulfilled") {
          patch[missing[i]] = result.value;
          knownStates.current.add(missing[i]);
        }
      });
      if (Object.keys(patch).length > 0) {
        setStates((prev) => ({ ...prev, ...patch }));
      }
    });
    return () => {
      cancelled = true;
    };
  }, [items]);

  const loadMore = useCallback(() => {
    if (nextCursor === null) {
      return;
    }
    const gen = generation.current;
    const cursor = nextCursor;
    setPageLoading(true);
    listWorks({
      query: query || undefined,
      kind: kind ? [kind] : undefined,
      consumption: consumption ? [consumption] : undefined,
      intention: intention ? [intention] : undefined,
      tags: tag ? [tag] : undefined,
      minRating,
      sort,
      limit,
      cursor,
    })
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
  }, [nextCursor, query, kind, consumption, intention, tag, minRating, sort, limit]);

  function setParam(name: string, value: string): void {
    const next = new URLSearchParams(params);
    if (value === "") {
      next.delete(name);
    } else {
      next.set(name, value);
    }
    setParams(next);
  }

  function resetFilters(): void {
    const next = new URLSearchParams();
    if (view === "cards") {
      next.set("view", "cards");
    }
    setText("");
    setTagText("");
    setParams(next);
  }

  function retry(): void {
    setError(null);
    if (nextCursor !== null) {
      loadMore();
    } else {
      setRetryTick((t) => t + 1);
    }
  }

  const countLabel = `${total} ${total === 1 ? "work" : "works"}`;

  return (
    <section className="library">
      <form className="library-toolbar" role="search" onSubmit={(e) => e.preventDefault()}>
        <label className="library-field library-field-search">
          <span>Search</span>
          <input
            type="search"
            value={text}
            placeholder="Title or alias"
            onChange={(e) => setText(e.target.value)}
          />
        </label>
        <label className="library-field">
          <span>Medium</span>
          <select value={kind} onChange={(e) => setParam("kind", e.target.value)}>
            <option value="">All</option>
            {KINDS.map((k) => (
              <option key={k} value={k}>
                {k.replace("_", " ")}
              </option>
            ))}
          </select>
        </label>
        <label className="library-field">
          <span>Status</span>
          <select
            value={consumption}
            onChange={(e) => setParam("consumption", e.target.value)}
          >
            <option value="">All</option>
            {CONSUMPTIONS.map((c) => (
              <option key={c} value={c}>
                {c === "unknown" ? "Unknown" : c.replace("_", " ")}
              </option>
            ))}
          </select>
        </label>
        <label className="library-field">
          <span>Intention</span>
          <select value={intention} onChange={(e) => setParam("intention", e.target.value)}>
            <option value="">All</option>
            {INTENTIONS.map((i) => (
              <option key={i} value={i}>
                {i === "none" ? "No intention" : i.replace("_", " ")}
              </option>
            ))}
          </select>
        </label>
        <label className="library-field">
          <span>Tag</span>
          <input
            type="search"
            value={tagText}
            placeholder="manga-translation-candidate"
            onChange={(e) => setTagText(e.target.value)}
          />
        </label>
        <label className="library-field">
          <span>Minimum rating</span>
          <select
            value={minRating === undefined ? "" : String(minRating)}
            onChange={(e) => setParam("min-rating", e.target.value)}
          >
            <option value="">Any</option>
            {MIN_RATINGS.map((n) => (
              <option key={n} value={n}>
                {n}/10 or more
              </option>
            ))}
          </select>
        </label>
        <label className="library-field">
          <span>Sort</span>
          <select value={sort} onChange={(e) => setParam("sort", e.target.value)}>
            {SORTS.map((s) => (
              <option key={s} value={s}>
                {s === "title"
                  ? "Title"
                  : s === "created_at"
                    ? "Created"
                    : s === "updated_at"
                      ? "Updated"
                      : "Rating"}
              </option>
            ))}
          </select>
        </label>
        <label className="library-field">
          <span>Page size</span>
          <select value={String(limit)} onChange={(e) => setParam("limit", e.target.value)}>
            {LIMITS.map((n) => (
              <option key={n} value={n}>
                {n}
              </option>
            ))}
          </select>
        </label>
        <div className="library-actions">
          <button type="button" onClick={resetFilters}>
            Reset
          </button>
          <button
            type="button"
            aria-pressed={view === "cards"}
            onClick={() => setParam("view", view === "cards" ? "" : "cards")}
          >
            {view === "cards" ? "List view" : "Card view"}
          </button>
        </div>
      </form>

      {total !== null && (
        <p className="library-count" role="status">
          {countLabel}
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
        <p className="library-loading">Loading the library…</p>
      ) : error !== null && items.length === 0 ? null : items.length === 0 ? (
        <div className="library-empty">
          <p>No works match the current filters.</p>
          <button type="button" onClick={resetFilters}>
            Reset filters
          </button>
        </div>
      ) : (
        <>
          {refreshing && (
            <p className="library-refreshing" role="status">
              Updating…
            </p>
          )}
          <ul className={view === "cards" ? "library-list library-cards" : "library-list"}>
            {items.map((card) => (
              <li key={card.work_id}>
                <WorkCardView
                  card={card}
                  states={states}
                  view={view}
                  activeFilter={activeFilter}
                />
              </li>
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
