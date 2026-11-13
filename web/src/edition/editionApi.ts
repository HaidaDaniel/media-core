// The wire types and helpers for the edition detail surface (the 0034
// ownership row): the edition read with its individual personal state, the
// bounded observation history, and the progress/consumption/retraction/
// metadata mutations. They mirror the frozen contract shapes; no domain
// rule is re-implemented here — unit and position validity, the correction
// and retraction lifecycle, and the revision guards are enforced by the
// shared service.
import { get, newRequestID, patch, post } from "../api/client";

// The five bounded progress units; a position is an exact decimal and the
// total is optional (an unknown total is never carried forward).
export const PROGRESS_UNITS = [
  "chapter",
  "volume",
  "episode",
  "page",
  "percent",
] as const;
export type ProgressUnit = (typeof PROGRESS_UNITS)[number];

// The defined consumption states; the explicit unknown is the absence of a
// live observation, never an invented value.
export const CONSUMPTION_VALUES = [
  "not_started",
  "in_progress",
  "paused",
  "completed",
  "dropped",
] as const;
export type ConsumptionValue = (typeof CONSUMPTION_VALUES)[number];

// The status options of the screen: the five states plus the explicit
// retraction value "unknown", which withdraws a mistaken state and stays in
// history distinct from never having an observation.
export const CONSUMPTION_STATUS_OPTIONS = [
  ...CONSUMPTION_VALUES,
  "unknown",
] as const;

// The edition kinds, in the library's display order.
export const EDITION_KINDS = [
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
] as const;

export interface TitleWire {
  id: string;
  text: string;
  language: string | null;
  kind: string;
  preferred: boolean;
  provenance: string;
}

export interface PublicationStateWire {
  value: string;
  source: string | null;
}

export interface ConsumptionStateWire {
  value: string;
  reason: string | null;
}

export interface ProgressRecordWire {
  id: string;
  unit: string;
  position: string;
  total: string | null;
  label: string | null;
  observed_at: string;
}

// The five bounded progress units as explicit fields; an absent unit is
// null or omitted (never zero).
export interface ProgressByUnitWire {
  chapter?: ProgressRecordWire | null;
  volume?: ProgressRecordWire | null;
  episode?: ProgressRecordWire | null;
  page?: ProgressRecordWire | null;
  percent?: ProgressRecordWire | null;
}

// The edition detail view with its individual personal state. The work
// scope is referenced by id only; the editions of a work keep these
// separate and are never combined.
export interface EditionViewWire {
  id: string;
  work_id: string;
  kind: string;
  language: string | null;
  other_label: string | null;
  revision: number;
  titles: TitleWire[];
  publication: PublicationStateWire | null;
  consumption: ConsumptionStateWire | null;
  progress: ProgressByUnitWire;
}

// One observation in the bounded entity history. The label carries the row
// state: effective, superseded or retracted; a retracted progress row has no
// value and detail.
export interface HistoryItemWire {
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
  observed_at: string | null;
  recorded_at: string;
}

export interface HistoryListWire {
  items: HistoryItemWire[];
  next_cursor: string | null;
  total: number;
}

// getEdition returns one edition detail view with its individual state.
export function getEdition(editionId: string): Promise<EditionViewWire> {
  return get<EditionViewWire>(`/editions/${encodeURIComponent(editionId)}`);
}

// getEditionHistory returns one page of the edition's observation history,
// newest first; the cursor comes from the previous page's next_cursor.
export function getEditionHistory(
  editionId: string,
  limit = 30,
  cursor?: string,
): Promise<HistoryListWire> {
  const qs = new URLSearchParams();
  qs.set("limit", String(limit));
  if (cursor !== undefined && cursor !== "") {
    qs.set("cursor", cursor);
  }
  return get<HistoryListWire>(
    `/editions/${encodeURIComponent(editionId)}/history?${qs.toString()}`,
  );
}

export interface RecordProgressInput {
  editionId: string;
  unit: string;
  position: string;
  // The known total; absent for an unknown total (and forbidden for
  // percent, which implies 100).
  total?: string;
  // Present only for a correction: the named current observation of the
  // same unit is superseded and stays in history.
  supersedesId?: string;
  // The edition's revision; it guards the mutation against lost updates.
  expectedRevision: number;
}

// recordProgress appends one progress observation; a same-current-value
// request is a no-op without a revision bump.
export function recordProgress(input: RecordProgressInput): Promise<EditionViewWire> {
  return post<EditionViewWire>(
    `/editions/${encodeURIComponent(input.editionId)}/progress`,
    {
      request_id: newRequestID(),
      expected_revision: input.expectedRevision,
      unit: input.unit,
      position: input.position,
      ...(input.total !== undefined ? { total: input.total } : {}),
      ...(input.supersedesId !== undefined ? { supersedes_id: input.supersedesId } : {}),
    },
  );
}

export interface SetConsumptionInput {
  editionId: string;
  value: string;
  reason?: string;
  // The edition's revision; it guards the mutation against lost updates.
  expectedRevision: number;
}

// setConsumption records one consumption observation; a same-current-value
// request is a no-op without a revision bump. Status never changes
// publication.
export function setConsumption(input: SetConsumptionInput): Promise<EditionViewWire> {
  return post<EditionViewWire>(
    `/editions/${encodeURIComponent(input.editionId)}/consumption`,
    {
      request_id: newRequestID(),
      expected_revision: input.expectedRevision,
      value: input.value,
      ...(input.reason !== undefined ? { reason: input.reason } : {}),
    },
  );
}

export interface RetractProgressInput {
  editionId: string;
  recordId: string;
  // A retraction requires a reason.
  reason: string;
  // The edition's revision; it guards the mutation against lost updates.
  expectedRevision: number;
}

// retractProgress explicitly retracts one progress observation; the row
// stays in history and the unit's current position falls back to the next
// live observation or to absence.
export function retractProgress(input: RetractProgressInput): Promise<EditionViewWire> {
  return post<EditionViewWire>(
    `/editions/${encodeURIComponent(input.editionId)}/history/${encodeURIComponent(input.recordId)}/retract`,
    {
      request_id: newRequestID(),
      expected_revision: input.expectedRevision,
      reason: input.reason,
    },
  );
}

export interface UpdateEditionInput {
  editionId: string;
  // Each field is sent only when the caller changed it; the server requires
  // at least one.
  kind?: string;
  language?: string;
  otherLabel?: string;
  // The edition's revision; it guards the mutation against lost updates.
  expectedRevision: number;
}

// updateEdition corrects the narrow edition metadata (kind, language,
// other label); titles are corrected at work scope, not here.
export function updateEdition(input: UpdateEditionInput): Promise<EditionViewWire> {
  const body: Record<string, unknown> = {
    request_id: newRequestID(),
    expected_revision: input.expectedRevision,
  };
  if (input.kind !== undefined) body.kind = input.kind;
  if (input.language !== undefined) body.language = input.language;
  if (input.otherLabel !== undefined) body.other_label = input.otherLabel;
  return patch<EditionViewWire>(`/editions/${encodeURIComponent(input.editionId)}`, body);
}
