// Explicit "find missing covers" panel. Nothing runs on page load and there
// is no polling: each batch is a user action with a bounded limit of five,
// visible offset/total progress, a Cancel action and per-item user choice.
// A cover is only written when the user picks a candidate, and replacing an
// existing user-selected cover needs the explicit replace checkbox.
import { useRef, useState } from "react";
import type { JSX } from "react";
import { ApiError } from "../api/client";
import { Badge, Button, Input } from "../ui";
import {
  findMissingCovers,
  previewTokenFromImageUrl,
  selectArtwork,
  type ArtworkCandidate,
  type ArtworkView,
  type MissingCoverItem,
} from "./artworkApi";
import { CoverImage } from "./CoverImage";

const PAGE_SIZE = 5;

export interface FindMissingCoversProps {
  onCoverSelected?: (artwork: ArtworkView) => void;
}

function errorMessage(err: unknown): string {
  return err instanceof ApiError ? err.message : "cover lookup failed; try again";
}

function statusTone(status: MissingCoverItem["status"]): "success" | "warning" | "danger" | "info" {
  if (status === "suggested") return "success";
  if (status === "ambiguous") return "warning";
  if (status === "error") return "danger";
  return "info";
}

function statusText(status: MissingCoverItem["status"]): string {
  if (status === "suggested") return "one suggestion";
  if (status === "ambiguous") return "several candidates";
  if (status === "error") return "lookup error";
  return "not found";
}

export function FindMissingCovers({ onCoverSelected }: FindMissingCoversProps): JSX.Element {
  const [items, setItems] = useState<MissingCoverItem[]>([]);
  const [offset, setOffset] = useState(0);
  const [total, setTotal] = useState<number | null>(null);
  const [busy, setBusy] = useState(false);
  const [started, setStarted] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [replaceIds, setReplaceIds] = useState<Set<string>>(new Set());
  const [saved, setSaved] = useState<Record<string, string>>({});
  const cancelled = useRef(false);

  async function run(nextOffset: number): Promise<void> {
    if (busy) return;
    cancelled.current = false;
    setBusy(true);
    setError(null);
    try {
      const page = await findMissingCovers({ limit: PAGE_SIZE, offset: nextOffset });
      if (cancelled.current) return;
      setItems((prev) => (nextOffset === 0 ? page.items : [...prev, ...page.items]));
      setOffset(page.next_offset);
      setTotal(page.total_missing);
      setStarted(true);
    } catch (err) {
      if (!cancelled.current) setError(errorMessage(err));
    } finally {
      if (!cancelled.current) setBusy(false);
    }
  }

  function cancel(): void {
    cancelled.current = true;
    setBusy(false);
    setError(null);
  }

  function toggleReplace(workId: string): void {
    setReplaceIds((prev) => {
      const next = new Set(prev);
      if (next.has(workId)) next.delete(workId);
      else next.add(workId);
      return next;
    });
  }

  async function choose(work: MissingCoverItem["work"], candidate: ArtworkCandidate): Promise<void> {
    const token = candidate.preview_token || previewTokenFromImageUrl(candidate.image_url);
    if (token === "") {
      setError("This candidate has no local preview token and cannot be selected.");
      return;
    }
    setError(null);
    try {
      const result = await selectArtwork({
        work_id: work.id,
        preview_token: token,
        replace: replaceIds.has(work.id),
      });
      setSaved((prev) => ({ ...prev, [work.id]: result.artwork.title }));
      onCoverSelected?.(result.artwork);
    } catch (err) {
      setError(errorMessage(err));
    }
  }

  const hasMore = total !== null && offset < total;

  return (
    <section className="artwork-find-panel" aria-label="Cover discovery">
      <div className="artwork-find-heading">
        <h3>Find missing covers</h3>
        <Button onClick={() => void run(0)} disabled={busy}>
          {started ? "Start over" : "Find missing covers"}
        </Button>
      </div>
      <p className="login-hint">
        Searches a small batch at a time. Covers are never selected automatically.
      </p>

      {started && (
        <p className="artwork-progress" aria-live="polite">
          {items.length} of {total ?? "?"} checked · next offset {offset}
        </p>
      )}

      {error !== null && (
        <div className="library-error" role="alert">
          <p>{error}</p>
          <Button size="sm" onClick={() => void run(offset)}>
            Retry
          </Button>
        </div>
      )}

      {busy && (
        <div className="quick-add-actions">
          <span className="shell-loading">Checking up to {PAGE_SIZE} works…</span>
          <Button variant="danger" size="sm" onClick={cancel}>
            Cancel
          </Button>
        </div>
      )}

      {!busy && started && hasMore && (
        <div className="quick-add-actions">
          <Button onClick={() => void run(offset)}>Check next {PAGE_SIZE}</Button>
        </div>
      )}

      {items.length > 0 && (
        <ul className="artwork-results">
          {items.map((item) => (
            <li className="artwork-result" key={item.work.id}>
              <CoverImage
                src={item.candidates[0]?.image_url ?? null}
                label={item.work.title}
                className="artwork-result-cover"
              />
              <div className="artwork-result-body">
                <p>
                  <strong>{item.work.title}</strong> <Badge>{item.work.kind}</Badge>
                </p>
                <p>
                  <Badge tone={statusTone(item.status)}>{statusText(item.status)}</Badge>
                </p>
                {saved[item.work.id] !== undefined ? (
                  <p className="cover-pick-note">Cover saved: {saved[item.work.id]}</p>
                ) : item.status === "not_found" || item.status === "error" ? (
                  <p className="artwork-notice">
                    {item.status === "error"
                      ? "This lookup failed; no cover was changed."
                      : "No candidate met the match rules; nothing was changed."}
                  </p>
                ) : (
                  <>
                    <label className="artwork-replace">
                      <Input
                        type="checkbox"
                        checked={replaceIds.has(item.work.id)}
                        onChange={() => toggleReplace(item.work.id)}
                      />
                      Replace an existing cover (only with your explicit go-ahead)
                    </label>
                    <ul className="artwork-candidates">
                      {item.candidates.map((candidate, index) => (
                        <li
                          className="artwork-candidate"
                          key={`${candidate.provider}/${candidate.external_id}/${index}`}
                        >
                          <CoverImage
                            src={candidate.image_url}
                            label={candidate.title}
                            className="artwork-candidate-cover"
                          />
                          <div className="artwork-candidate-meta">
                            <span>{candidate.title}</span>
                            <Badge>{candidate.kind}</Badge>
                            <span>{candidate.provider}</span>
                          </div>
                          <Button size="sm" onClick={() => void choose(item.work, candidate)}>
                            Use this cover
                          </Button>
                        </li>
                      ))}
                    </ul>
                  </>
                )}
              </div>
            </li>
          ))}
        </ul>
      )}
    </section>
  );
}
