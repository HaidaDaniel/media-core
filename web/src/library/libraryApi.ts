// The wire types and fetch helpers for the catalog read surface (the 0025
// ownership row). They mirror the frozen contract shapes; no domain rule is
// re-implemented here — filtering, sorting and cursor bounds are enforced by
// the shared service behind the API.
import { get } from "../api/client";

export interface TitleWire {
  id: string;
  text: string;
  language: string | null;
  kind: string;
  preferred: boolean;
  provenance: string;
}

export interface EditionSummaryWire {
  id: string;
  kind: string;
  language: string | null;
  revision: number;
}

// The optional cover reference carried by a work card. The image URL is a
// same-origin local preview; a null artwork means "no cover", never a made-up
// image or a third-party URL fetched by the browser.
export interface WorkCardArtworkWire {
  id: string;
  image_url: string;
  provider: string;
  attribution_url: string;
}

// One search result: the work with its active titles, its edition summaries
// and the editions that satisfied the whole query plus filter predicate set.
export interface WorkCardWire {
  work_id: string;
  revision: number;
  created_at: string;
  updated_at: string;
  archived_at: string | null;
  titles: TitleWire[];
  editions: EditionSummaryWire[];
  matched_edition_ids: string[];
  artwork?: WorkCardArtworkWire | null;
}

// The canonical paginated list payload: opaque seek cursor, stable ordering
// and the total count of filtered works before pagination.
export interface LibraryListWire {
  items: WorkCardWire[];
  next_cursor: string | null;
  total: number;
}

// The current personal consumption of one edition; null when no live
// observation exists (the explicit unknown state).
interface ConsumptionStateWire {
  value: string;
}

interface EditionStateWire {
  consumption: ConsumptionStateWire | null;
}

export interface ListWorksParams {
  query?: string;
  kind?: string[];
  consumption?: string[];
  // The intention dimension; "none" matches editions without a recorded
  // intention.
  intention?: string[];
  // The tag dimension, satisfied by a work tag or the qualifying edition's
  // own tag.
  tags?: string[];
  // The minimum current rating of the qualifying edition; an absent or
  // cleared rating never matches.
  minRating?: number;
  sort: string;
  limit: number;
  cursor?: string;
}

// listWorks performs one bounded library search. The repeated parameters
// carry the kind, consumption, intention and tag selections; an absent
// selection omits the parameter entirely rather than sending an empty
// value.
export function listWorks(params: ListWorksParams): Promise<LibraryListWire> {
  const qs = new URLSearchParams();
  if (params.query) {
    qs.set("query", params.query);
  }
  for (const k of params.kind ?? []) {
    qs.append("kind", k);
  }
  for (const c of params.consumption ?? []) {
    qs.append("consumption", c);
  }
  for (const i of params.intention ?? []) {
    qs.append("intention", i);
  }
  for (const t of params.tags ?? []) {
    qs.append("tag", t);
  }
  if (params.minRating !== undefined) {
    qs.set("min-rating", String(params.minRating));
  }
  if (params.sort) {
    qs.set("sort", params.sort);
  }
  qs.set("limit", String(params.limit));
  if (params.cursor) {
    qs.set("cursor", params.cursor);
  }
  const encoded = qs.toString();
  return get<LibraryListWire>(encoded === "" ? "/works" : `/works?${encoded}`);
}

// editionConsumption reports one edition's current consumption value. The
// list payload carries no per-edition state, so the badges resolve it from
// the edition detail; an absent state is "unknown", never a guess.
export function editionConsumption(editionId: string): Promise<string> {
  return get<EditionStateWire>(`/editions/${encodeURIComponent(editionId)}`).then(
    (view) => view.consumption?.value ?? "unknown",
  );
}
