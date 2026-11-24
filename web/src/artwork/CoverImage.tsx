// A cover is shown only when the API supplied a local image URL. A missing
// URL renders a neutral CSS placeholder; nothing is invented or fetched from
// a third-party host by the browser.
import type { JSX } from "react";

export interface CoverImageProps {
  src: string | null | undefined;
  label?: string;
  className?: string;
}

export function CoverImage({ src, label, className = "" }: CoverImageProps): JSX.Element {
  const classes = ["cover-image", className].filter((part) => part !== "").join(" ");
  if (src === null || src === undefined || src === "") {
    return (
      <div
        className={`${classes} cover-image-placeholder`}
        role="img"
        aria-label={label !== undefined ? `No cover available for ${label}` : "No cover available"}
      />
    );
  }
  return <img className={classes} src={src} alt="" loading="lazy" />;
}
