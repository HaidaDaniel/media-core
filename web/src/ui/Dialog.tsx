// Accessible modal dialog: labelled, modal to assistive tech, Escape to
// dismiss, focus moved in on open and restored on close, and Tab trapped
// inside the panel. No animation library is involved.
import { useEffect, useId, useRef } from "react";
import type { ReactNode } from "react";
import type { JSX } from "react";
import { createPortal } from "react-dom";
import { Button } from "./Button";

export interface DialogProps {
  open: boolean;
  onClose: () => void;
  title: string;
  children: ReactNode;
  footer?: ReactNode;
}

const FOCUSABLE =
  'a[href], button:not([disabled]), input:not([disabled]), select:not([disabled]), textarea:not([disabled]), summary, [tabindex]:not([tabindex="-1"])';

export function Dialog({ open, onClose, title, children, footer }: DialogProps): JSX.Element | null {
  const panel = useRef<HTMLDivElement | null>(null);
  const close = useRef(onClose);
  close.current = onClose;
  const titleId = useId();

  useEffect(() => {
    if (!open) return;
    const previous = document.activeElement instanceof HTMLElement ? document.activeElement : null;
    const focusables = () =>
      Array.from(panel.current?.querySelectorAll<HTMLElement>(FOCUSABLE) ?? []);
    focusables()[0]?.focus();

    function onKeyDown(event: KeyboardEvent): void {
      if (event.key === "Escape") {
        event.preventDefault();
        close.current();
        return;
      }
      if (event.key !== "Tab") return;
      const nodes = focusables();
      if (nodes.length === 0) return;
      const first = nodes[0];
      const last = nodes[nodes.length - 1];
      if (event.shiftKey && document.activeElement === first) {
        event.preventDefault();
        last.focus();
      } else if (!event.shiftKey && document.activeElement === last) {
        event.preventDefault();
        first.focus();
      }
    }

    document.addEventListener("keydown", onKeyDown);
    return () => {
      document.removeEventListener("keydown", onKeyDown);
      previous?.focus();
    };
  }, [open]);

  if (!open) return null;

  return createPortal(
    <div
      className="ui-dialog-backdrop"
      onMouseDown={(event) => {
        if (event.target === event.currentTarget) close.current();
      }}
    >
      <div
        className="ui-dialog"
        role="dialog"
        aria-modal="true"
        aria-labelledby={titleId}
        ref={panel}
      >
        <div className="ui-dialog-header">
          <h2 className="ui-dialog-title" id={titleId}>
            {title}
          </h2>
          <Button variant="ghost" size="sm" aria-label="Close dialog" onClick={() => close.current()}>
            Close
          </Button>
        </div>
        <div className="ui-dialog-body">{children}</div>
        {footer !== undefined && <div className="ui-dialog-footer">{footer}</div>}
      </div>
    </div>,
    document.body,
  );
}
