// One readable relation row: the relation rendered as a sentence with its
// direction relative to the current entity, its confirmation state (a
// proposed claim is never rendered as a confirmed fact), and a deliberate
// link to the related entity. Confirm and revoke are deliberate actions,
// offered only when this screen's entity is the edge's canonical from entity.
import type { JSX } from "react";
import { Link } from "react-router-dom";
import type { RelatedEdgeWire } from "./relationsApi";

// The v1 relation types by scope. Work-scope rows bind works; edition-scope
// rows bind editions. The split is the server's, not a client rule.
export const WORK_RELATION_TYPES = [
  "same_underlying_work",
  "sequel_of",
  "side_story_of",
  "related",
] as const;

export const EDITION_RELATION_TYPES = [
  "adaptation_of",
  "translation_of",
  "alternative_version",
  "related",
] as const;

// The v1 origin kinds; unknown is the explicit default absence, never an
// inference from a media kind or provider.
export const ORIGIN_KINDS = ["original", "anime_original", "unknown"] as const;

// The human phrasing of each relation type. Directed types read from the
// stored from endpoint; symmetric types read the same from either end.
const RELATION_PHRASES: Record<string, { directed: string; symmetric: string }> = {
  adaptation_of: { directed: "is an adaptation of", symmetric: "" },
  translation_of: { directed: "is a translation of", symmetric: "" },
  alternative_version: { directed: "", symmetric: "is an alternative version of" },
  same_underlying_work: { directed: "", symmetric: "shares the underlying work with" },
  sequel_of: { directed: "is a sequel of", symmetric: "" },
  side_story_of: { directed: "is a side story of", symmetric: "" },
  related: { directed: "", symmetric: "is related to" },
};

const SYMMETRIC_TYPES = new Set([
  "alternative_version",
  "same_underlying_work",
  "related",
]);

// describeRelation renders one edge as a sentence anchored on the current
// entity; the stored direction is never flipped.
export function describeRelation(
  type: string,
  currentIsFrom: boolean,
  otherLabel: string,
  currentWord: string,
): string {
  const phrase = RELATION_PHRASES[type];
  if (!phrase) {
    return `${currentWord} has a "${type}" relation with ${otherLabel}`;
  }
  if (SYMMETRIC_TYPES.has(type)) {
    return `${currentWord} ${phrase.symmetric} ${otherLabel}`;
  }
  if (currentIsFrom) {
    return `${currentWord} ${phrase.directed} ${otherLabel}`;
  }
  return `${otherLabel} ${phrase.directed} ${currentWord}`;
}

export interface RelationRowProps {
  edge: RelatedEdgeWire;
  scope: "work" | "edition";
  // The id of the entity this screen is about; it is one endpoint of the
  // edge, and the other endpoint is the navigation target.
  currentId: string;
  // Resolves the display label of an endpoint id; falls back to the id when
  // the title is unknown — nothing is invented.
  label: (id: string) => string;
  // Whether the confirm/revoke actions are offered here (the canonical from
  // entity is the current one, so its revision guards the mutation).
  actionable: boolean;
  busy?: boolean;
  onConfirm?: (relationId: string) => void;
  onRevoke?: (relationId: string) => void;
}

export function RelationRow({
  edge,
  scope,
  currentId,
  label,
  actionable,
  busy = false,
  onConfirm,
  onRevoke,
}: RelationRowProps): JSX.Element {
  const currentIsFrom = edge.from_id === currentId;
  const otherId = currentIsFrom ? edge.to_id : edge.from_id;
  const otherLabel = label(otherId);
  const currentWord = scope === "work" ? "This work" : "This edition";
  const sentence = describeRelation(edge.type, currentIsFrom, otherLabel, currentWord);
  const proposed = edge.state === "proposed";
  const target = scope === "work" ? `/works/${encodeURIComponent(otherId)}` : `/editions/${encodeURIComponent(otherId)}`;
  return (
    <li className="work-edition">
      <span>{sentence}</span>
      <span className={proposed ? "work-badge work-badge-in_progress" : "work-badge"}>
        {proposed ? "Proposed" : "Confirmed"}
      </span>
      <Link to={target}>{otherLabel}</Link>
      {actionable && (
        <span className="library-actions">
          {proposed && onConfirm && (
            <button type="button" disabled={busy} onClick={() => onConfirm(edge.relation_id)}>
              Confirm
            </button>
          )}
          {onRevoke && (
            <button type="button" disabled={busy} onClick={() => onRevoke(edge.relation_id)}>
              Revoke
            </button>
          )}
        </span>
      )}
    </li>
  );
}
