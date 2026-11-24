// Shared card surface for works, editions and panels.
import type { HTMLAttributes, JSX } from "react";

export function Card({ className = "", ...rest }: HTMLAttributes<HTMLElement>): JSX.Element {
  return <article className={["ui-card", className].filter((p) => p !== "").join(" ")} {...rest} />;
}
