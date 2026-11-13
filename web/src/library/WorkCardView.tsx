// One work in the library: the preferred title, the alias line and one row
// per edition with its kind, language and current consumption badge. The
// badges distinguish the states of unrelated editions without combining
// them; missing metadata renders as "Unknown" — nothing is invented.
import type { JSX } from "react";
import { Link } from "react-router-dom";
import type { WorkCardWire } from "./libraryApi";

const CONSUMPTION_LABELS: Record<string, string> = {
  completed: "Completed",
  in_progress: "In progress",
  paused: "Paused",
  dropped: "Dropped",
  not_started: "Not started",
  unknown: "Unknown",
};

function consumptionLabel(value: string | undefined): string {
  return CONSUMPTION_LABELS[value ?? "unknown"] ?? "Unknown";
}

interface WorkCardViewProps {
  card: WorkCardWire;
  // The per-edition consumption values resolved from the edition details;
  // an absent entry is the explicit unknown state.
  states: Record<string, string>;
  view: "list" | "cards";
  // Marks the server-matched editions when a search or filter is active.
  activeFilter: boolean;
}

export function WorkCardView({ card, states, view, activeFilter }: WorkCardViewProps): JSX.Element {
  const preferred = card.titles.find((t) => t.preferred) ?? card.titles[0];
  const aliases = card.titles.filter((t) => t.id !== preferred?.id);
  const matched = new Set(card.matched_edition_ids);
  const showMatch = activeFilter && matched.size > 0;
  return (
    <article className={view === "cards" ? "work-card work-card-cards" : "work-card"}>
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
      <ul className="work-editions">
        {card.editions.map((e) => (
          <li
            key={e.id}
            className={showMatch && matched.has(e.id) ? "work-edition work-edition-matched" : "work-edition"}
          >
            <Link className="work-edition-link" to={`/editions/${encodeURIComponent(e.id)}`}>
              <span className="work-edition-kind">{e.kind}</span>
              <span className="work-edition-lang">{e.language ?? "Unknown"}</span>
            </Link>
            {showMatch && matched.has(e.id) && <span className="work-edition-match">match</span>}
            <span className={`work-badge work-badge-${states[e.id] ?? "unknown"}`}>
              {consumptionLabel(states[e.id])}
            </span>
          </li>
        ))}
        {card.editions.length === 0 && <li className="work-edition">No editions yet</li>}
      </ul>
    </article>
  );
}
