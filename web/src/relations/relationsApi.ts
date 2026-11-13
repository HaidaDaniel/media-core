// The wire types and helpers for the relation/origin surface (the 0029
// ownership row). They mirror the frozen contract shapes; no domain rule is
// re-implemented here — scope/type validity, direction, cycle checks,
// revision guards and the claim lifecycle are enforced by the shared service.
import { get, newRequestID, post } from "../api/client";

// The lifecycle state of a relation or origin claim. The relation traversal
// returns only the non-revoked edges, so a traversed edge is proposed or
// confirmed; revoked edges stay in the mutation view's history instead.
export type ClaimState = "proposed" | "confirmed" | "revoked";

// A claim is created proposed or confirmed, never revoked.
export type ClaimStateInput = "proposed" | "confirmed";

// One traversed relation with its hop depth from the start entity. The
// direction is never flipped: from_id/to_id are exactly as stored.
export interface RelatedEdgeWire {
  relation_id: string;
  type: string;
  from_id: string;
  to_id: string;
  state: ClaimState;
  depth: number;
}

// The bounded both-directions traversal of the non-revoked relations of one
// scoped entity.
export interface RelatedViewWire {
  entity_id: string;
  scope: string;
  edges: RelatedEdgeWire[];
}

// One relation with its full lifecycle, as returned by the mutation views.
export interface RelationWire {
  id: string;
  scope: string;
  type: string;
  from_id: string;
  to_id: string;
  state: ClaimState;
  provenance: string;
  confidence: number | null;
  replaces_id: string | null;
  confirmed_by: string | null;
  confirmed_at: string | null;
  created_at: string;
  revoked_by: string | null;
  revoked_at: string | null;
}

// The relation view of one scoped entity, separated by lifecycle state:
// proposals are never rendered as confirmed facts and revoked edges keep
// their history.
export interface RelationViewWire {
  proposed: RelationWire[];
  confirmed: RelationWire[];
  revoked: RelationWire[];
}

// One explicit origin assertion on an edition; unknown is the default
// absence of a confirmed origin, never an inference.
export interface OriginClaimWire {
  id: string;
  kind: string;
  state: ClaimState;
  provenance: string;
  replaces_id: string | null;
  confirmed_by: string | null;
  confirmed_at: string | null;
  created_at: string;
  revoked_by: string | null;
  revoked_at: string | null;
}

// Every origin claim of one edition in creation order.
export interface OriginViewWire {
  edition_id: string;
  claims: OriginClaimWire[];
}

// workRelations traverses the non-revoked work-scope relations of one work
// in both directions. An absent depth is the service default (1).
export function workRelations(workId: string, depth = 1): Promise<RelatedViewWire> {
  return get<RelatedViewWire>(
    `/works/${encodeURIComponent(workId)}/relations?depth=${depth}`,
  );
}

// editionRelations traverses the non-revoked edition-scope relations of one
// edition in both directions.
export function editionRelations(editionId: string, depth = 1): Promise<RelatedViewWire> {
  return get<RelatedViewWire>(
    `/editions/${encodeURIComponent(editionId)}/relations?depth=${depth}`,
  );
}

export interface ProposeRelationInput {
  scope: "work" | "edition";
  type: string;
  fromId: string;
  toId: string;
  state: ClaimStateInput;
  provenance: string;
  // The revision of the from entity; it guards the mutation against lost
  // updates. The transport sets the actor; the body never carries one.
  expectedRevision: number;
}

// proposeRelation asserts one typed, scoped, provenance-bearing relation. A
// duplicate active edge returns the existing edge; a replacement or a
// confirming cycle is a distinguishable conflict.
export function proposeRelation(input: ProposeRelationInput): Promise<RelationViewWire> {
  return post<RelationViewWire>("/relations", {
    request_id: newRequestID(),
    expected_revision: input.expectedRevision,
    scope: input.scope,
    type: input.type,
    from_id: input.fromId,
    to_id: input.toId,
    state: input.state,
    provenance: input.provenance,
  });
}

// confirmRelation explicitly confirms one proposed relation. The revision is
// the canonical from entity's; a confirming cycle is a cycle conflict with
// no partial mutation.
export function confirmRelation(
  relationId: string,
  expectedRevision: number,
): Promise<RelationViewWire> {
  return post<RelationViewWire>(`/relations/${encodeURIComponent(relationId)}/confirm`, {
    request_id: newRequestID(),
    expected_revision: expectedRevision,
  });
}

// revokeRelation revokes one active relation; the row keeps its history and
// a replacement is a new edge.
export function revokeRelation(
  relationId: string,
  expectedRevision: number,
): Promise<RelationViewWire> {
  return post<RelationViewWire>(`/relations/${encodeURIComponent(relationId)}/revoke`, {
    request_id: newRequestID(),
    expected_revision: expectedRevision,
  });
}

export interface RecordOriginInput {
  editionId: string;
  kind: string;
  state: ClaimStateInput;
  provenance: string;
  // The edition's revision; it guards the mutation against lost updates.
  expectedRevision: number;
}

// recordOrigin asserts one explicit origin claim on an edition; a same-kind
// correction replaces the named claim and keeps its audit.
export function recordOrigin(input: RecordOriginInput): Promise<OriginViewWire> {
  return post<OriginViewWire>("/origins", {
    request_id: newRequestID(),
    expected_revision: input.expectedRevision,
    edition_id: input.editionId,
    kind: input.kind,
    state: input.state,
    provenance: input.provenance,
  });
}
