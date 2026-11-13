// The wire types and fetch helpers for the manual quick-add surface (the
// 0026 ownership row). Resolve never writes and reports ready, an existing
// identity, or candidates with a short-lived decision token; create commits
// one explicit decision with exact idempotent replay. All candidate
// decisions come from the server; the client never fuzzy-matches.
import { newRequestID, post } from "../api/client";

// One requested title: text, optional language, kind, preferred flag and
// provenance. The manual quick-add form always asserts the user provenance.
export interface TitleInputWire {
  text: string;
  language: string | null;
  kind: string;
  preferred: boolean;
  provenance: string;
}

// One external reference: exactly one of external_id (with a provider) or
// url. The shared service validates the shape.
export interface ReferenceInputWire {
  provider?: string;
  external_id?: string;
  url?: string;
}

// The shared addition payload of resolve and create: the requested title,
// the optional edition attributes and the optional references.
export interface AdditionPayload {
  titles: TitleInputWire[];
  kind?: string;
  language?: string;
  other_label?: string;
  references: ReferenceInputWire[];
}

// One possible identity match with its server-provided evidence; a match is
// never identity until the caller chooses.
export interface CandidateWire {
  work_id: string;
  edition_id: string | null;
  evidence: string[];
}

// The resolve outcome: ready (create now), existing (an identity matched)
// or candidates (choose before anything is written).
export interface ResolveWire {
  outcome: "ready" | "existing" | "candidates";
  work_id: string | null;
  candidates: CandidateWire[] | null;
  candidate_token: string | null;
  request_fingerprint: string | null;
}

// The create outcome. Created is false with reason already_exists when the
// decision confirmed an existing entry; no personal state is implied.
export interface AdditionResultWire {
  created: boolean;
  reason?: string;
  work_id: string;
  work_revision: number;
  edition_id: string | null;
}

export interface CreateOptions {
  // The idempotency key of this create attempt; an ambiguous failure is
  // retried with the same key and payload, an exact replay then returns the
  // stored result.
  requestId: string;
  choice?: "use_existing" | "add_edition" | "create_distinct";
  candidateToken?: string;
  workId?: string;
}

// resolveAddition runs the read-only identity resolution for one manual
// addition; it never writes and never treats a title match as confirmed
// identity.
export function resolveAddition(payload: AdditionPayload): Promise<ResolveWire> {
  return post<ResolveWire>("/additions/resolve", {
    request_id: newRequestID(),
    ...payload,
  });
}

// createWork commits one explicit decision through POST /works: a ready
// result creates directly, and a candidate decision sends its choice with
// the resolve token.
export function createWork(
  payload: AdditionPayload,
  options: CreateOptions,
): Promise<AdditionResultWire> {
  return post<AdditionResultWire>("/works", {
    request_id: options.requestId,
    work_id: options.workId,
    ...payload,
    choice: options.choice,
    candidate_token: options.candidateToken,
  });
}

// addEdition adds a distinct edition under an existing work; the route fixes
// the choice to add_edition and takes the work id from the path.
export function addEdition(
  workId: string,
  payload: AdditionPayload,
  options: CreateOptions,
): Promise<AdditionResultWire> {
  return post<AdditionResultWire>(`/works/${encodeURIComponent(workId)}/editions`, {
    request_id: options.requestId,
    ...payload,
    candidate_token: options.candidateToken,
  });
}

// openTarget resolves where a create outcome leads: the new or existing
// edition when the result names one, otherwise the work.
export function targetPathFor(result: AdditionResultWire): string {
  return result.edition_id
    ? `/editions/${encodeURIComponent(result.edition_id)}`
    : `/works/${encodeURIComponent(result.work_id)}`;
}

// existingWorkPath is where the existing-identity outcome navigates.
export function existingWorkPath(workId: string): string {
  return `/works/${encodeURIComponent(workId)}`;
}
