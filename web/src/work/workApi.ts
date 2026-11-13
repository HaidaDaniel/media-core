// The wire types and fetch helpers for the work detail surface (the 0025
// ownership row reads). They mirror the frozen contract shapes and carry only
// the fields the work screen renders; no domain rule is re-implemented here.
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

// The work detail view: identity, active titles and edition summaries. The
// frozen work view carries no notes field, so notes arrive with each edition
// detail rather than here.
export interface WorkViewWire {
  id: string;
  revision: number;
  created_at: string;
  updated_at: string;
  archived_at: string | null;
  titles: TitleWire[];
  editions: EditionSummaryWire[];
}

export interface NoteWire {
  id: string;
  target_id: string;
  text: string;
  spoiler: boolean;
  quote: string | null;
  location: string | null;
  revision: number;
  recorded_at: string;
}

// One current personal dimension of an edition; null means no live
// observation (the explicit unknown state), never an invented value.
export interface ConsumptionStateWire {
  value: string;
  reason: string | null;
}

export interface IntentionStateWire {
  value: string;
  reason: string | null;
}

export interface RatingObservationWire {
  value: number | null;
  comment: string | null;
}

export interface ProgressRecordWire {
  unit: string;
  position: string;
  total: string | null;
  label: string | null;
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

// The edition detail view with its individual personal state and notes.
export interface EditionViewWire {
  id: string;
  work_id: string;
  kind: string;
  language: string | null;
  other_label: string | null;
  revision: number;
  titles: TitleWire[];
  consumption: ConsumptionStateWire | null;
  intention: IntentionStateWire | null;
  rating: RatingObservationWire | null;
  progress: ProgressByUnitWire;
  notes: NoteWire[];
}

// getWork returns one work detail view.
export function getWork(workId: string): Promise<WorkViewWire> {
  return get<WorkViewWire>(`/works/${encodeURIComponent(workId)}`);
}

// getEdition returns one edition detail view with its individual state and
// notes; the editions of one work keep these separate and are never
// combined.
export function getEdition(editionId: string): Promise<EditionViewWire> {
  return get<EditionViewWire>(`/editions/${encodeURIComponent(editionId)}`);
}
