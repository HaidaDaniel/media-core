// Loading placeholder. Decorative by default so screen readers hear the
// surrounding status text once, not one announcement per skeleton block.
import type { HTMLAttributes, JSX } from "react";

export function Skeleton({ className = "", ...rest }: HTMLAttributes<HTMLSpanElement>): JSX.Element {
  return (
    <span
      aria-hidden="true"
      className={["ui-skeleton", className].filter((p) => p !== "").join(" ")}
      {...rest}
    />
  );
}
