// Shared button primitive. It defaults to type="button" so a bare <Button>
// inside a form never submits by accident; callers pass type="submit".
import { forwardRef } from "react";
import type { ButtonHTMLAttributes, JSX } from "react";

export type ButtonVariant = "primary" | "secondary" | "ghost" | "danger";
export type ButtonSize = "sm" | "md";

export interface ButtonProps extends ButtonHTMLAttributes<HTMLButtonElement> {
  variant?: ButtonVariant;
  size?: ButtonSize;
}

export const Button = forwardRef<HTMLButtonElement, ButtonProps>(function Button(
  { variant = "secondary", size = "md", className = "", type = "button", ...rest },
  ref,
): JSX.Element {
  const classes = [`ui-button ui-button-${variant} ui-button-${size}`, className]
    .filter((part) => part !== "")
    .join(" ");
  return <button ref={ref} type={type} className={classes} {...rest} />;
});
