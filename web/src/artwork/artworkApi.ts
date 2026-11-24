// Typed client for the explicit, same-origin artwork discovery endpoints.
// The response envelope is unwrapped by the shared request helpers in
// api/client.ts; every mutating call goes through post/del so the in-memory
// CSRF token is attached and credentials stay same-origin. Preview images are
// local URLs served by the API; the browser never fetches a provider host.
import { del, get, post } from "../api/client";

export interface ArtworkProviderWire {
  id: string;
  label: string;
  kinds: string[];
  attribution_url: string;
  required: boolean;
}

export interface ProvidersWire {
  providers: ArtworkProviderWire[];
}

export interface ArtworkCandidate {
  provider: string;
  external_id: string;
  title: string;
  alternative_titles: string[];
  kind: string;
  image_url: string | null;
  preview_token: string;
  attribution_url: string;
  score: number;
  reason: string;
}

export interface SearchArtworkInput {
  title: string;
  kind: string;
  alternatives: string[];
  references: string[];
}

export interface SearchArtworkWire {
  candidates: ArtworkCandidate[];
}

// A saved artwork view. The image is always a local cached URL; provider and
// attribution are the provenance shown to the user.
export interface ArtworkView {
  id: string;
  work_id: string;
  edition_id: string | null;
  provider: string;
  external_id: string;
  title: string;
  source_url: string;
  attribution_url: string;
  image_url: string;
  selected_by_user: boolean;
  kind: string;
}

export interface SelectArtworkInput {
  work_id: string;
  edition_id?: string | null;
  preview_token: string;
  replace?: boolean;
}

export interface SelectArtworkWire {
  artwork: ArtworkView;
}

export interface MissingWorkWire {
  id: string;
  title: string;
  kind: string;
}

export interface MissingCoverItem {
  work: MissingWorkWire;
  status: "suggested" | "ambiguous" | "not_found" | "error" | "timeout";
  candidates: ArtworkCandidate[];
}

export interface FindMissingCoversInput {
  limit?: number;
  cursor?: string;
}

export interface FindMissingCoversWire {
  items: MissingCoverItem[];
  next_cursor: string;
  total_missing: number;
}

export interface ClearArtworkCacheWire {
  removed: number;
  bytes: number;
}

// previewTokenFromImageUrl reads the token carried by a local preview URL so
// selectArtwork can name the candidate the user actually picked. No network
// request is made here; an unrecognised URL yields an empty token and the
// caller reports that the candidate cannot be selected.
export function previewTokenFromImageUrl(imageUrl: string | null): string {
  if (imageUrl === null) return "";
  const match = /\/artwork\/preview\/([^/?#]+)\/image/.exec(imageUrl);
  return match?.[1] ?? "";
}

export function getProviders(): Promise<ProvidersWire> {
  return get<ProvidersWire>("/artwork/providers");
}

export function searchArtwork(
  input: SearchArtworkInput,
  signal?: AbortSignal,
): Promise<SearchArtworkWire> {
  return post<SearchArtworkWire>("/artwork/search", {
    title: input.title,
    kind: input.kind,
    alternatives: input.alternatives,
    references: input.references,
  }, signal);
}

export function selectArtwork(input: SelectArtworkInput): Promise<SelectArtworkWire> {
  const body: Record<string, unknown> = {
    work_id: input.work_id,
    preview_token: input.preview_token,
  };
  if (input.edition_id != null && input.edition_id !== "") {
    body.edition_id = input.edition_id;
  }
  if (input.replace === true) {
    body.replace = true;
  }
  return post<SelectArtworkWire>("/artwork/select", body);
}

export function findMissingCovers(
  input: FindMissingCoversInput = {},
  signal?: AbortSignal,
): Promise<FindMissingCoversWire> {
  return post<FindMissingCoversWire>("/artwork/find-missing", {
    limit: input.limit ?? 5,
    cursor: input.cursor ?? "",
  }, signal);
}

export function clearArtworkCache(): Promise<ClearArtworkCacheWire> {
  return del<ClearArtworkCacheWire>("/artwork/cache/clear");
}
