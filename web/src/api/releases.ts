// The wire types and fetch helpers for the release import, binding and
// history surface (task 0043). They mirror the frozen contract shapes and
// carry only the fields the screens render; no domain rule is re-implemented
// here — the idempotency, the explicit map, the conflict, the projection and
// the pending queue are enforced by the shared service.
import { del, get, newRequestID, post } from "../api/client";

// One imported release made visible for an edition through an active binding.
// The release fields are the producer's, labeled by the exact publication they
// came from; counts are never summed.
export interface ReleaseProjectionWire {
  event_id: string;
  event_type: string;
  namespace: string;
  publication_id: string | null;
  edition_id: string;
  work_id: string;
  release_id: string | null;
  release_number: string | null;
  release_title: string | null;
  release_url: string | null;
  current_count: number | null;
  count_delta: number | null;
  source_time: string | null;
  detection_time: string | null;
  created_at: string;
}

// The canonical paginated release-history payload for one edition.
export interface ReleaseListWire {
  items: ReleaseProjectionWire[];
  next_cursor: string | null;
  total: number;
}

// One pending import record: an imported event with no active binding.
export interface PendingImportWire {
  id: string;
  event_id: string;
  event_type: string;
  namespace: string;
  publication_id: string | null;
  external_work_ref: string | null;
  release_id: string | null;
  release_number: string | null;
  release_title: string | null;
  release_url: string | null;
  current_count: number | null;
  count_delta: number | null;
  source_time: string | null;
  detection_time: string | null;
  created_at: string;
}

// The canonical paginated pending-queue payload.
export interface PendingImportListWire {
  items: PendingImportWire[];
  next_cursor: string | null;
  total: number;
}

// One explicit, audited publication→edition binding.
export interface ReleaseBindingWire {
  id: string;
  namespace: string;
  publication_id: string;
  edition_id: string;
  work_id: string;
  state: string;
  created_by: string;
  created_at: string;
  replaces_id: string | null;
}

// listReleases reads the bounded release history of one edition, oldest
// first. An absent limit is the server default; the opaque cursor is bound
// to the limit.
export function listReleases(
  editionId: string,
  limit = 50,
  cursor?: string,
): Promise<ReleaseListWire> {
  const qs = new URLSearchParams();
  qs.set("limit", String(limit));
  if (cursor) {
    qs.set("cursor", cursor);
  }
  return get<ReleaseListWire>(
    `/editions/${encodeURIComponent(editionId)}/releases?${qs.toString()}`,
  );
}

// listPendingImports reads the bounded pending import queue, oldest first.
export function listPendingImports(
  limit = 50,
  cursor?: string,
): Promise<PendingImportListWire> {
  const qs = new URLSearchParams();
  qs.set("limit", String(limit));
  if (cursor) {
    qs.set("cursor", cursor);
  }
  return get<PendingImportListWire>(`/pending-imports?${qs.toString()}`);
}

export interface BindReleaseInput {
  publicationId: string;
  editionId: string;
  // The producer namespace; absent uses the stable default.
  namespace?: string;
}

// bindRelease explicitly maps one confirmed publication to an edition. It is
// idempotent; a publication already bound to a different edition is a
// conflict, never a silent re-point.
export function bindRelease(input: BindReleaseInput): Promise<ReleaseBindingWire> {
  return post<ReleaseBindingWire>("/releases/bindings", {
    request_id: newRequestID(),
    ...(input.namespace !== undefined ? { namespace: input.namespace } : {}),
    publication_id: input.publicationId,
    edition_id: input.editionId,
  });
}

// rebindRelease explicitly corrects a publication's binding in one step: it
// revokes the current active binding and creates its replacement, so the old
// binding and the raw event history stay intact.
export function rebindRelease(
  input: BindReleaseInput,
): Promise<ReleaseBindingWire> {
  return post<ReleaseBindingWire>("/releases/bindings/rebind", {
    request_id: newRequestID(),
    ...(input.namespace !== undefined ? { namespace: input.namespace } : {}),
    publication_id: input.publicationId,
    edition_id: input.editionId,
  });
}

export interface RevokeReleaseInput {
  publicationId: string;
  // The producer namespace; absent uses the stable default.
  namespace?: string;
}

// revokeRelease revokes one publication's active binding, un-projecting its
// imported events while keeping the binding row and the raw event history.
export function revokeRelease(
  input: RevokeReleaseInput,
): Promise<{ status: string; namespace: string; publication_id: string }> {
  return del("/releases/bindings", {
    request_id: newRequestID(),
    ...(input.namespace !== undefined ? { namespace: input.namespace } : {}),
    publication_id: input.publicationId,
  });
}
