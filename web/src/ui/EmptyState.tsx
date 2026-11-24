// Shared empty state: one short explanation and at most one recovery action.
import type { ReactNode } from "react";
import type { JSX } from "react";
import { Button } from "./Button";

export interface EmptyStateProps {
  title: string;
  children?: ReactNode;
  actionLabel?: string;
  onAction?: () => void;
  className?: string;
}

export function EmptyState({
  title,
  children,
  actionLabel,
  onAction,
  className = "",
}: EmptyStateProps): JSX.Element {
  return (
    <div className={["ui-empty-state", className].filter((p) => p !== "").join(" ")}>
      <h3>{title}</h3>
      {children}
      {actionLabel !== undefined && onAction !== undefined && (
        <div className="ui-empty-state-actions">
          <Button onClick={onAction}>{actionLabel}</Button>
        </div>
      )}
    </div>
  );
}
