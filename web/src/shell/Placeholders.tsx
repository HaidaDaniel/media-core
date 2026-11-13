// The route placeholders until each screen lands in its own task. They
// render an honest empty state, never a fake screen.
import type { JSX } from "react";

interface PlaceholderProps {
  title: string;
  note: string;
}

export function PlaceholderPage({ title, note }: PlaceholderProps): JSX.Element {
  return (
    <section className="placeholder">
      <h2>{title}</h2>
      <p>{note}</p>
    </section>
  );
}

export function LibraryPage(): JSX.Element {
  return <PlaceholderPage title="Library" note="The library list and search land in task 0031." />;
}

export function QuickAddPage(): JSX.Element {
  return <PlaceholderPage title="Quick add" note="Manual quick add lands in task 0032." />;
}

export function WorkPage(): JSX.Element {
  return <PlaceholderPage title="Work details" note="Work details and related media land in task 0033." />;
}

export function EditionPage(): JSX.Element {
  return <PlaceholderPage title="Edition details" note="Edition details land in task 0034." />;
}

export function ActivityPage(): JSX.Element {
  return <PlaceholderPage title="Recent activity" note="The activity feed lands in task 0036." />;
}

export function NotFoundPage(): JSX.Element {
  return <PlaceholderPage title="Not found" note="No screen lives at this address." />;
}
