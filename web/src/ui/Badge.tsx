// Shared status badge. The tone classes are semantic (success, warning,
// danger, info); callers can still append the legacy work-badge-* classes.
import type { HTMLAttributes, JSX } from "react";

export type BadgeTone = "neutral" | "success" | "warning" | "danger" | "info";

export interface BadgeProps extends HTMLAttributes<HTMLSpanElement> {
  tone?: BadgeTone;
}

export function Badge({ tone = "neutral", className = "", ...rest }: BadgeProps): JSX.Element {
  const classes = [
    "ui-badge",
    tone === "neutral" ? "" : `ui-badge-${tone}`,
    className,
  ]
    .filter((part) => part !== "")
    .join(" ");
  return <span className={classes} {...rest} />;
}
