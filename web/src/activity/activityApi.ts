// The wire types and fetch helper for the recent-activity read (the 0029
// ownership row): the bounded feed over every personal history source with
// correction labels and target enrichment. They mirror the frozen contract
// shapes; no domain rule is re-implemented here — the ordering, the seek
// cursor and the type fingerprint are enforced by the shared projection.
import { get } from "../api/client";
import type { ExternalReferenceWire, NoteWire } from "../annotations/annotationsApi";

// One activity or history row: the source row, its correction label (the
// dimension's own state: effective/superseded/retracted for observations,
// current/superseded/retracted for note revisions, active/retired for
// references and tags, proposed/confirmed/revoked for claims) and the
// target's current title, annotation and reference enrichment.
export interface ActivityItemWire {
  id: string;
  type: string;
  work_id: string;
  edition_id: string | null;
  title: string | null;
  value: string | null;
  detail: string | null;
  label: string;
  reason: string | null;
  supersedes_id: string | null;
  replaces_id: string | null;
  observed_at: string | null;
  recorded_at: string;
  actor: {
    principal: string;
    origin: string;
    identity?: string;
  } | null;
  notes: NoteWire[];
  references: ExternalReferenceWire[];
}

// The canonical paginated feed payload: the server orders (recorded_at,
// id) descending, the opaque seek cursor is bound to the limit and the
// type fingerprint, and the total counts before pagination.
export interface ActivityListWire {
  items: ActivityItemWire[];
  next_cursor: string | null;
  total: number;
}

export interface ListActivityParams {
  // The optional type filter; absent selects every type.
  types?: string[];
  limit: number;
  cursor?: string;
}

// listActivity performs one bounded recent-activity read.
export function listActivity(params: ListActivityParams): Promise<ActivityListWire> {
  const qs = new URLSearchParams();
  for (const t of params.types ?? []) {
    qs.append("types", t);
  }
  qs.set("limit", String(params.limit));
  if (params.cursor) {
    qs.set("cursor", params.cursor);
  }
  return get<ActivityListWire>(`/activity?${qs.toString()}`);
}
