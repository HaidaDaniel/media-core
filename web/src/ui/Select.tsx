// Shared select. Options are ordinary children, so value/label behavior and
// keyboard use stay identical to the native control.
import { forwardRef } from "react";
import type { SelectHTMLAttributes, JSX } from "react";

export const Select = forwardRef<HTMLSelectElement, SelectHTMLAttributes<HTMLSelectElement>>(
  function Select({ className = "", ...rest }, ref): JSX.Element {
    return <select ref={ref} className={["ui-select", className].filter((p) => p !== "").join(" ")} {...rest} />;
  },
);
