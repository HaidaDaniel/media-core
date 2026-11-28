// Explicit cover picker. Search happens only when the user presses the
// search button; the payload is bounded to one title, one kind and a small
// number of short alternatives/references. A confident single result is a
// dismissible suggestion, several results are user choices, none leaves a
// neutral fallback. Nothing is auto-selected and a failed search only shows
// an error inside this dialog — it never blocks record creation.
import { useEffect, useRef, useState } from "react";
import type { JSX } from "react";
import { ApiError } from "../api/client";
import { Badge, Button, Dialog, Input } from "../ui";
import {
  previewTokenFromImageUrl,
  searchArtwork,
  selectArtwork,
  type ArtworkCandidate,
  type ArtworkView,
} from "./artworkApi";
import { CoverImage } from "./CoverImage";

const MAX_TEXT = 200;
const MAX_LIST = 5;

export interface CoverPickerProps {
  open: boolean;
  onClose: () => void;
  title: string;
  kind: string;
  alternatives?: string[];
  references?: string[];
  // When a saved record exists, a selection writes through selectArtwork.
  workId?: string | null;
  editionId?: string | null;
  // Pre-create mode: the caller keeps the chosen candidate until its work
  // exists; no artwork record is written from this dialog.
  onSelected?: (artwork: ArtworkView) => void;
  onPick?: (candidate: ArtworkCandidate) => void;
}

type Status = "idle" | "searching" | "ready" | "error";

function errorMessage(err: unknown): string {
  return err instanceof ApiError ? err.message : "cover search failed; try again";
}

function bounded(values: string[] | undefined, maxLength: number): string[] {
  return (values ?? [])
    .filter((value) => value.trim() !== "")
    .slice(0, MAX_LIST)
    .map((value) => value.trim().slice(0, maxLength));
}

function safeSource(url: string): string | null {
  return /^https?:\/\//i.test(url) ? url : null;
}

export function CoverPicker({
  open,
  onClose,
  title,
  kind,
  alternatives,
  references,
  workId = null,
  editionId = null,
  onSelected,
  onPick,
}: CoverPickerProps): JSX.Element {
  const [status, setStatus] = useState<Status>("idle");
  const [candidates, setCandidates] = useState<ArtworkCandidate[]>([]);
  const [error, setError] = useState<string | null>(null);
  const [showList, setShowList] = useState(false);
  const [replace, setReplace] = useState(false);
  const [saving, setSaving] = useState(false);
  const abortRef = useRef<AbortController | null>(null);

  useEffect(() => {
    abortRef.current?.abort();
    abortRef.current = null;
    if (!open) return;
    setStatus("idle");
    setCandidates([]);
    setError(null);
    setShowList(false);
    setReplace(false);
    setSaving(false);
    return () => {
      abortRef.current?.abort();
    };
  }, [open]);

  function search(): void {
    abortRef.current?.abort();
    const controller = new AbortController();
    abortRef.current = controller;
    setStatus("searching");
    setError(null);
    setCandidates([]);
    setShowList(false);
    void searchArtwork({
      title: title.trim().slice(0, MAX_TEXT),
      kind: kind.trim().slice(0, 64),
      alternatives: bounded(alternatives, MAX_TEXT),
      references: bounded(references, MAX_TEXT * 2),
    }, controller.signal)
      .then((result) => {
        const selectable = (result.candidates ?? []).filter(
          (candidate) => (candidate.preview_token || previewTokenFromImageUrl(candidate.image_url)) !== "",
        );
        setCandidates(selectable);
        setStatus("ready");
      })
      .catch((err: unknown) => {
        if ((err as { name?: string } | null)?.name === "AbortError") return;
        setError(errorMessage(err));
        setStatus("error");
      })
      .finally(() => {
        if (abortRef.current === controller) {
          abortRef.current = null;
        }
      });
  }

  async function choose(candidate: ArtworkCandidate): Promise<void> {
    setError(null);
    if (workId === null) {
      onPick?.(candidate);
      onClose();
      return;
    }
    const token = candidate.preview_token || previewTokenFromImageUrl(candidate.image_url);
    if (token === "") {
      setError("This candidate has no local preview token and cannot be selected.");
      return;
    }
    setSaving(true);
    try {
      const result = await selectArtwork({
        work_id: workId,
        edition_id: editionId,
        preview_token: token,
        replace,
      });
      onSelected?.(result.artwork);
      onClose();
    } catch (err) {
      setError(errorMessage(err));
    } finally {
      setSaving(false);
    }
  }

  const single = status === "ready" && candidates.length === 1 && !showList;
  const confident = single && (candidates[0]?.score ?? 0) >= 0.9;

  function candidateCard(candidate: ArtworkCandidate, key: string): JSX.Element {
    const source = safeSource(candidate.attribution_url);
    return (
      <li className="artwork-candidate" key={key}>
        <CoverImage src={candidate.image_url} label={candidate.title} className="artwork-candidate-cover" />
        <div className="artwork-candidate-meta">
          <span>{candidate.title}</span>
          <Badge>{candidate.kind}</Badge>
          <span>{candidate.provider}</span>
          {candidate.reason !== "" && <span className="login-hint">{candidate.reason}</span>}
          {source !== null && (
            <a href={source} target="_blank" rel="noopener noreferrer">
              Source
            </a>
          )}
        </div>
        <Button
          size="sm"
          variant="primary"
          disabled={saving}
          onClick={() => void choose(candidate)}
        >
          Use this cover
        </Button>
      </li>
    );
  }

  return (
    <Dialog
      open={open}
      onClose={onClose}
      title="Find a cover"
      footer={
        <>
          {workId !== null && (
            <label className="artwork-replace">
              <Input
                type="checkbox"
                checked={replace}
                onChange={(event) => setReplace(event.target.checked)}
              />
              Replace an existing cover
            </label>
          )}
          <Button variant="ghost" onClick={onClose}>
            Cancel
          </Button>
        </>
      }
    >
      <p className="login-hint">
        Searching {title.trim() === "" ? "this work" : `“${title.trim()}”`}
        {kind === "" ? "" : ` (${kind})`}. Covers are suggested only; you choose
        whether to use one.
      </p>

      <Button variant="primary" onClick={search} disabled={status === "searching" || saving}>
        {status === "searching" ? "Searching…" : "Search for covers"}
      </Button>

      {error !== null && (
        <div className="library-error artwork-picker-error" role="alert">
          <p>{error}</p>
          <Button size="sm" onClick={search}>
            Try again
          </Button>
        </div>
      )}

      {status === "ready" && candidates.length === 0 && (
        <p className="artwork-notice artwork-picker-empty">
          No cover candidates found. The record can still be saved without a cover.
        </p>
      )}

      {status === "ready" && candidates.length > 0 && (
        <div className="artwork-picker-results">
          {single ? (
            <div className="artwork-suggestion">
              <ul className="artwork-candidates">
                {candidateCard(candidates[0], candidates[0].external_id || "candidate-0")}
              </ul>
              <div>
                <p>
                  {confident
                    ? "One confident match. Use it only if it is right."
                    : "One possible match. Use it only if it is right."}
                </p>
                <div className="quick-add-actions">
                  <Button variant="ghost" onClick={() => setShowList(true)}>
                    Choose another
                  </Button>
                  <Button variant="ghost" onClick={onClose}>
                    Cancel
                  </Button>
                </div>
              </div>
            </div>
          ) : (
            <>
              <p className="artwork-notice">
                {candidates.length} {candidates.length === 1 ? "candidate" : "candidates"}. Pick one or
                keep the record without a cover.
              </p>
              <ul className="artwork-candidates">
                {candidates.map((candidate, index) =>
                  candidateCard(candidate, `${candidate.provider}/${candidate.external_id}/${index}`),
                )}
              </ul>
            </>
          )}
        </div>
      )}
    </Dialog>
  );
}
