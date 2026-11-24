// The artwork client contract: read path, bounded mutation payloads, same-
// origin cookies and the in-memory CSRF header on every mutating call.
import { afterEach, describe, expect, it, vi } from "vitest";
import { setCSRFToken } from "../api/client";
import {
  clearArtworkCache,
  findMissingCovers,
  getProviders,
  previewTokenFromImageUrl,
  searchArtwork,
  selectArtwork,
} from "./artworkApi";

function jsonResponse(body: unknown): Response {
  return { ok: true, status: 200, json: async () => ({ ok: true, data: body }) } as Response;
}

afterEach(() => {
  vi.unstubAllGlobals();
  setCSRFToken(null);
});

describe("artwork API client", () => {
  it("reads providers from the same-origin v1 path", async () => {
    const fetchMock = vi.fn().mockResolvedValue(jsonResponse({ providers: [] }));
    vi.stubGlobal("fetch", fetchMock);
    await expect(getProviders()).resolves.toEqual({ providers: [] });
    expect(fetchMock.mock.calls[0][0]).toBe("/api/v1/artwork/providers");
    expect(fetchMock.mock.calls[0][1].method).toBe("GET");
  });

  it("posts a bounded search payload with CSRF", async () => {
    const fetchMock = vi.fn().mockResolvedValue(jsonResponse({ candidates: [] }));
    vi.stubGlobal("fetch", fetchMock);
    setCSRFToken("csrf-1");
    await searchArtwork({
      title: "Static Bloom",
      kind: "manga",
      alternatives: ["Static Bloom (JP)"],
      references: ["https://example.org/work"],
    });
    expect(fetchMock.mock.calls[0][0]).toBe("/api/v1/artwork/search");
    expect(fetchMock.mock.calls[0][1].headers["X-CSRF-Token"]).toBe("csrf-1");
    expect(JSON.parse(fetchMock.mock.calls[0][1].body)).toEqual({
      title: "Static Bloom",
      kind: "manga",
      alternatives: ["Static Bloom (JP)"],
      references: ["https://example.org/work"],
    });
  });

  it("sends the picked preview token and omits absent optional fields", async () => {
    const fetchMock = vi.fn().mockResolvedValue(jsonResponse({ artwork: { id: "a-1" } }));
    vi.stubGlobal("fetch", fetchMock);
    setCSRFToken("csrf-1");
    await selectArtwork({ work_id: "w-1", preview_token: "tok-1" });
    expect(fetchMock.mock.calls[0][0]).toBe("/api/v1/artwork/select");
    expect(JSON.parse(fetchMock.mock.calls[0][1].body)).toEqual({
      work_id: "w-1",
      preview_token: "tok-1",
    });
  });

  it("sends replace and edition id only when explicitly supplied", async () => {
    const fetchMock = vi.fn().mockResolvedValue(jsonResponse({ artwork: { id: "a-1" } }));
    vi.stubGlobal("fetch", fetchMock);
    await selectArtwork({
      work_id: "w-1",
      edition_id: "e-1",
      preview_token: "tok-1",
      replace: true,
    });
    expect(JSON.parse(fetchMock.mock.calls[0][1].body)).toEqual({
      work_id: "w-1",
      edition_id: "e-1",
      preview_token: "tok-1",
      replace: true,
    });
  });

  it("requests small, explicit find-missing batches", async () => {
    const fetchMock = vi.fn().mockResolvedValue(
      jsonResponse({ items: [], next_offset: 5, total_missing: 0 }),
    );
    vi.stubGlobal("fetch", fetchMock);
    await findMissingCovers({ limit: 5, offset: 5 });
    expect(fetchMock.mock.calls[0][0]).toBe("/api/v1/artwork/find-missing");
    expect(JSON.parse(fetchMock.mock.calls[0][1].body)).toEqual({ limit: 5, offset: 5 });
  });

  it("clears the cache with an authenticated DELETE", async () => {
    const fetchMock = vi.fn().mockResolvedValue(jsonResponse({ removed: 2, bytes: 4096 }));
    vi.stubGlobal("fetch", fetchMock);
    setCSRFToken("csrf-1");
    await expect(clearArtworkCache()).resolves.toEqual({ removed: 2, bytes: 4096 });
    expect(fetchMock.mock.calls[0][0]).toBe("/api/v1/artwork/cache/clear");
    expect(fetchMock.mock.calls[0][1].method).toBe("DELETE");
    expect(fetchMock.mock.calls[0][1].headers["X-CSRF-Token"]).toBe("csrf-1");
  });

  it("extracts only local preview tokens", () => {
    expect(
      previewTokenFromImageUrl("/api/v1/artwork/preview/tok-42/image"),
    ).toBe("tok-42");
    expect(previewTokenFromImageUrl("https://provider.example/cover.jpg")).toBe("");
    expect(previewTokenFromImageUrl(null)).toBe("");
  });
});
