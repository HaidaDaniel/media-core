// One work in the library: the preferred title, the alias line and one row
// per edition with its kind, language and current consumption badge. The
// badges distinguish the states of unrelated editions without combining
// them; missing metadata renders as "Unknown" — nothing is invented. A cover
// is rendered only from the API-supplied local artwork field; without it a
// neutral CSS placeholder is shown.
import type { JSX } from "react";
import { Link } from "react-router-dom";
import { CoverImage } from "../artwork/CoverImage";
import { Badge, Card } from "../ui";
import type { WorkCardWire } from "./libraryApi";

const CONSUMPTION_LABELS: Record<string, string> = {
  completed: "Completed",
  in_progress: "In progress",
  paused: "Paused",
  dropped: "Dropped",
  not_started: "Not started",
  unknown: "Unknown",
};

const CONSUMPTION_TONES: Record<string, "neutral" | "success" | "info" | "warning" | "danger"> = {
  completed: "success",
  in_progress: "info",
  paused: "warning",
  dropped: "danger",
};

function consumptionLabel(value: string | undefined): string {
  return CONSUMPTION_LABELS[value ?? "unknown"] ?? "Unknown";
}

function safeSource(url: string): string | null {
  return /^https?:\/\//i.test(url) ? url : null;
}

interface WorkCardViewProps {
  card: WorkCardWire;
  // The per-edition consumption values resolved from the edition details;
  // an absent entry is the explicit unknown state.
  states: Record<string, string>;
  view: "grid" | "list";
  // Marks the server-matched editions when a search or filter is active.
  activeFilter: boolean;
}

export function WorkCardView({ card, states, view, activeFilter }: WorkCardViewProps): JSX.Element {
  const preferred = card.titles.find((t) => t.preferred) ?? card.titles[0];
  const aliases = card.titles.filter((t) => t.id !== preferred?.id);
  const matched = new Set(card.matched_edition_ids);
  const showMatch = activeFilter && matched.size > 0;
  const artwork = card.artwork ?? null;
  const source = artwork !== null ? safeSource(artwork.attribution_url) : null;
  return (
    <Card
      className={
        view === "grid" ? "work-card work-card-cards" : "work-card work-card-list"
      }
    >
      <div className="work-card-media">
        <CoverImage src={artwork?.image_url ?? null} label={preferred?.text} />
        {artwork !== null && <span className="work-card-provider">{artwork.provider}</span>}
        {source !== null && (
          <a
            className="work-card-provider"
            href={source}
            target="_blank"
            rel="noopener noreferrer"
            aria-label="Cover source"
          >
            Source
          </a>
        )}
      </div>
      <div className="work-card-body">
        <h3 className="work-card-title">
          <Link to={`/works/${encodeURIComponent(card.work_id)}`}>
            {preferred ? preferred.text : "Unknown"}
          </Link>
          {preferred?.language ? <span className="work-card-lang">{preferred.language}</span> : null}
        </h3>
        {aliases.length > 0 && (
          <p className="work-card-aliases">
            {aliases.map((t) => (
              <span key={t.id} className="work-card-alias">
                {t.text}
                {t.language ? ` (${t.language})` : ""}
              </span>
            ))}
          </p>
        )}
        {card.progress != null && card.progress !== "" && (
          <p className="work-card-progress">{card.progress}</p>
        )}
        <ul className="work-editions">
          {card.editions.map((e) => (
            <li
              key={e.id}
              className={
                showMatch && matched.has(e.id) ? "work-edition work-edition-matched" : "work-edition"
              }
            >
              <Link className="work-edition-link" to={`/editions/${encodeURIComponent(e.id)}`}>
                <span className="work-edition-kind">{e.kind}</span>
                <span className="work-edition-lang">{e.language ?? "Unknown"}</span>
              </Link>
              {showMatch && matched.has(e.id) && <span className="work-edition-match">match</span>}
              <Badge
                tone={CONSUMPTION_TONES[states[e.id] ?? "unknown"] ?? "neutral"}
                className={`work-badge work-badge-${states[e.id] ?? "unknown"}`}
              >
                {consumptionLabel(states[e.id])}
              </Badge>
            </li>
          ))}
          {card.editions.length === 0 && <li className="work-edition">No editions yet</li>}
        </ul>
      </div>
    </Card>
  );
}
