import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it } from "vitest";
import { CoverImage } from "./CoverImage";

afterEach(() => cleanup());

describe("CoverImage", () => {
  it("renders a neutral placeholder when local bytes fail to load", () => {
    render(<CoverImage src="/api/v1/artwork/a1/image" label="Static Bloom" />);
    const image = screen.getByRole("presentation", { hidden: true }) as HTMLImageElement | null;
    const img = document.querySelector("img");
    expect(img).not.toBeNull();
    fireEvent.error(img!);
    expect(screen.getByRole("img", { name: "No cover available for Static Bloom" })).not.toBeNull();
    expect(document.querySelector("img")).toBeNull();
    void image;
  });

  it("renders the placeholder when no URL is supplied", () => {
    render(<CoverImage src={null} label="Static Bloom" />);
    expect(screen.getByRole("img", { name: "No cover available for Static Bloom" })).not.toBeNull();
  });
});
