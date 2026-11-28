// Shared text input. It forwards every native prop and label, so existing
// accessible names and test ids are preserved by callers.
import { forwardRef } from "react";
import type { InputHTMLAttributes, JSX } from "react";

export interface InputProps extends InputHTMLAttributes<HTMLInputElement> {
  invalid?: boolean;
}

export const Input = forwardRef<HTMLInputElement, InputProps>(function Input(
  { className = "", invalid = false, ...rest },
  ref,
): JSX.Element {
  const classes = ["ui-input", invalid ? "ui-input-invalid" : "", className]
    .filter((part) => part !== "")
    .join(" ");
  return <input ref={ref} className={classes} {...rest} />;
});
