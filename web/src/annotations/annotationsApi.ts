// The wire types and fetch helpers for the personal annotation surface (the
// 0028 ownership row): intention, rating, notes, tags and sourced external
// references over one edition, plus the note and tag work-scope dispatch.
// They mirror the frozen contract shapes; no domain rule is re-implemented
// here — value bounds, normalization, the correction and retraction
// lifecycles, and the revision guards are enforced by the shared service.
import { del, get, newRequestID, patch, post } from "../api/client";
import type { EditionViewWire } from "../edition/editionApi";
import type { WorkViewWire } from "../work/workApi";

// The five bounded intention values; "none" is the explicit retraction to
// no recorded intention, not dislike.
export const INTENTION_OPTIONS = [
  "none",
  "interested",
  "saved",
  "planned",
  "not_interested",
] as const;

// The six bounded reference roles.
export const REFERENCE_ROLES = [
  "publication",
  "metadata",
  "translation",
  "release_watcher",
  "translation_project",
  "content",
] as const;

export interface ActorWire {
  principal: string;
  origin: string;
  identity?: string;
}

// The current personal intention observation; null when no live
// observation exists.
export interface IntentionStateWire {
  value: string;
  reason: string | null;
  observed_at: string;
  recorded_at: string;
  actor: ActorWire;
  supersedes_id: string | null;
}

// The current rating observation; a null value is an explicit clear, never
// zero.
export interface RatingObservationWire {
  value: number | null;
  comment: string | null;
  observed_at: string;
  recorded_at: string;
  actor: ActorWire;
  supersedes_id: string | null;
}

// One active note at its latest non-retracted revision; plain text, never
// rendered as HTML.
export interface NoteWire {
  id: string;
  target_id: string;
  text: string;
  spoiler: boolean;
  quote: string | null;
  location: string | null;
  revision: number;
  recorded_at: string;
  actor: ActorWire;
}

// One active sourced external reference; the stored URL is normalized
// http(s) without userinfo, so it is safe to link.
export interface ExternalReferenceWire {
  id: string;
  provider: string;
  external_id: string | null;
  url: string | null;
  role: string;
  authoritative: boolean;
  language: string | null;
  provenance: string;
  retired_at: string | null;
}

// The complete edition detail view: the 0034 base identity and reading
// state plus the personal annotation fields the panels compose.
export interface AnnotatedEditionView extends EditionViewWire {
  intention: IntentionStateWire | null;
  rating: RatingObservationWire | null;
  tags: string[];
  references: ExternalReferenceWire[];
  notes: NoteWire[];
}

// getAnnotatedEdition returns the full edition detail view including the
// personal annotation state.
export function getAnnotatedEdition(editionId: string): Promise<AnnotatedEditionView> {
  return get<AnnotatedEditionView>(`/editions/${encodeURIComponent(editionId)}`);
}

export interface SetIntentionInput {
  editionId: string;
  value: string;
  reason?: string;
  // The edition's revision; it guards the mutation against lost updates.
  expectedRevision: number;
}

// setIntention records one intention observation; "none" is the explicit
// retraction. A same-current-value request is a no-op without a revision
// bump.
export function setIntention(input: SetIntentionInput): Promise<AnnotatedEditionView> {
  return post<AnnotatedEditionView>(
    `/editions/${encodeURIComponent(input.editionId)}/intention`,
    {
      request_id: newRequestID(),
      expected_revision: input.expectedRevision,
      value: input.value,
      ...(input.reason !== undefined ? { reason: input.reason } : {}),
    },
  );
}

export interface SetRatingInput {
  editionId: string;
  // A 1-10 value, or null for the explicit clear.
  value: number | null;
  comment?: string;
  // Present only for a correction: the named current observation is
  // superseded and stays in history.
  supersedesId?: string;
  // The edition's revision; it guards the mutation against lost updates.
  expectedRevision: number;
}

// setRating records one rating observation, an explicit clear or a
// correction; a same-current-value request is a no-op without a revision
// bump.
export function setRating(input: SetRatingInput): Promise<AnnotatedEditionView> {
  return post<AnnotatedEditionView>(
    `/editions/${encodeURIComponent(input.editionId)}/ratings`,
    {
      request_id: newRequestID(),
      expected_revision: input.expectedRevision,
      value: input.value,
      ...(input.comment !== undefined ? { comment: input.comment } : {}),
      ...(input.supersedesId !== undefined ? { supersedes_id: input.supersedesId } : {}),
    },
  );
}

export interface AddReferenceInput {
  editionId: string;
  provider: string;
  // Exactly one of externalId or url, per the service.
  externalId?: string;
  url?: string;
  role: string;
  authoritative?: boolean;
  language?: string;
  // The explicit provenance claim; required.
  provenance: string;
  // The edition's revision; it guards the mutation against lost updates.
  expectedRevision: number;
}

// addReference attaches one sourced external reference; links are never
// fetched and an active identity on another edition conflicts.
export function addReference(input: AddReferenceInput): Promise<AnnotatedEditionView> {
  const body: Record<string, unknown> = {
    request_id: newRequestID(),
    expected_revision: input.expectedRevision,
    provider: input.provider,
    role: input.role,
    authoritative: input.authoritative === true,
    provenance: input.provenance,
  };
  if (input.externalId !== undefined) body.external_id = input.externalId;
  if (input.url !== undefined) body.url = input.url;
  if (input.language !== undefined) body.language = input.language;
  return post<AnnotatedEditionView>(
    `/editions/${encodeURIComponent(input.editionId)}/references`,
    body,
  );
}

export interface RetireReferenceInput {
  editionId: string;
  referenceId: string;
  reason?: string;
  // The edition's revision; it guards the mutation against lost updates.
  expectedRevision: number;
}

// retireReference retires one reference; the rows stay in history, never
// erased.
export function retireReference(input: RetireReferenceInput): Promise<AnnotatedEditionView> {
  const body: Record<string, unknown> = {
    request_id: newRequestID(),
    expected_revision: input.expectedRevision,
  };
  if (input.reason !== undefined) body.reason = input.reason;
  return post<AnnotatedEditionView>(
    `/editions/${encodeURIComponent(input.editionId)}/references/${encodeURIComponent(
      input.referenceId,
    )}/retire`,
    body,
  );
}

// The note and tag target scopes; the body dispatches work versus edition.
export type AnnotationTarget = "work" | "edition";

// A note or tag mutation returns the updated view of its target: the
// edition view for an edition target and the work view for a work target.
export type TargetView = AnnotatedEditionView | WorkViewWire;

export interface AddNoteInput {
  target: AnnotationTarget;
  targetId: string;
  text: string;
  spoiler?: boolean;
  // Optional but must be non-blank when sent; blank values are omitted.
  quote?: string;
  location?: string;
}

// addNote adds one plain-text note to the work or edition named by the
// target scope; a later thought is always a distinct new note. The overloads
// keep the returned target view precise at the call site.
export function addNote(input: AddNoteInput & { target: "edition" }): Promise<AnnotatedEditionView>;
export function addNote(input: AddNoteInput & { target: "work" }): Promise<WorkViewWire>;
export function addNote(input: AddNoteInput): Promise<TargetView> {
  const body: Record<string, unknown> = {
    request_id: newRequestID(),
    target: input.target,
    target_id: input.targetId,
    text: input.text,
    spoiler: input.spoiler === true,
  };
  if (input.quote !== undefined) body.quote = input.quote;
  if (input.location !== undefined) body.location = input.location;
  return post<TargetView>("/notes", body);
}

export interface ReviseNoteInput {
  noteId: string;
  text: string;
  spoiler?: boolean;
  quote?: string;
  location?: string;
  // The note's current revision; it guards the edit against concurrent
  // overwrites.
  expectedRevision: number;
}

// reviseNote edits one existing note; a new immutable revision points to
// the immediately preceding one.
export function reviseNote(input: ReviseNoteInput): Promise<TargetView> {
  const body: Record<string, unknown> = {
    request_id: newRequestID(),
    expected_revision: input.expectedRevision,
    text: input.text,
    spoiler: input.spoiler === true,
  };
  if (input.quote !== undefined) body.quote = input.quote;
  if (input.location !== undefined) body.location = input.location;
  return patch<TargetView>(`/notes/${encodeURIComponent(input.noteId)}`, body);
}

export interface RetractNoteInput {
  noteId: string;
  reason?: string;
  // The note's current revision; it guards the retraction against
  // concurrent overwrites.
  expectedRevision: number;
}

// retractNote withdraws one note; the retraction revision keeps the
// retracted text and its audit, and the note becomes absent from the
// target's view.
export function retractNote(input: RetractNoteInput): Promise<TargetView> {
  const body: Record<string, unknown> = {
    request_id: newRequestID(),
    expected_revision: input.expectedRevision,
  };
  if (input.reason !== undefined) body.reason = input.reason;
  return post<TargetView>(
    `/notes/${encodeURIComponent(input.noteId)}/retract`,
    body,
  );
}

export interface TagAssignmentInput {
  target: AnnotationTarget;
  targetId: string;
  tag: string;
  // The target's revision; it guards the mutation against lost updates.
  expectedRevision: number;
}

// attachTag attaches one normalized tag to the work or edition; attach is
// idempotent and never alters consumption. The overloads keep the returned
// target view precise at the call site.
export function attachTag(input: TagAssignmentInput & { target: "edition" }): Promise<AnnotatedEditionView>;
export function attachTag(input: TagAssignmentInput & { target: "work" }): Promise<WorkViewWire>;
export function attachTag(input: TagAssignmentInput): Promise<TargetView> {
  return post<TargetView>("/tags/assignments", {
    request_id: newRequestID(),
    expected_revision: input.expectedRevision,
    target: input.target,
    target_id: input.targetId,
    tag: input.tag,
  });
}

// detachTag detaches one tag; the assignment row keeps its detach audit and
// the detach is idempotent. The overloads keep the returned target view
// precise at the call site.
export function detachTag(input: TagAssignmentInput & { target: "edition" }): Promise<AnnotatedEditionView>;
export function detachTag(input: TagAssignmentInput & { target: "work" }): Promise<WorkViewWire>;
export function detachTag(input: TagAssignmentInput): Promise<TargetView> {
  return del<TargetView>("/tags/assignments", {
    request_id: newRequestID(),
    expected_revision: input.expectedRevision,
    target: input.target,
    target_id: input.targetId,
    tag: input.tag,
  });
}
