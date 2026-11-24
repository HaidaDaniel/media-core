// The quick-add screen: title-first manual addition with optional edition
// and reference fields. The server resolves identity first — an exact
// reference opens the existing record without writing, possible title
// matches are shown with their evidence and nothing is merged or created
// until the user chooses, and a ready result creates directly (title-only
// additions complete with no metadata, cover or LLM). Input is retained on
// every failure; an ambiguous create failure is retried with the same
// idempotency key and payload so an exact replay returns the stored result.
import { useEffect, useRef, useState } from "react";
import type { JSX } from "react";
import { useNavigate } from "react-router-dom";
import { ApiError, newRequestID } from "../api/client";
import { CoverPicker } from "../artwork/CoverPicker";
import {
  previewTokenFromImageUrl,
  selectArtwork,
  type ArtworkCandidate,
} from "../artwork/artworkApi";
import { Button, Input, Select } from "../ui";
import {
  addEdition,
  createWork,
  existingWorkPath,
  resolveAddition,
  targetPathFor,
  type AdditionPayload,
  type CandidateWire,
  type CreateOptions,
  type ReferenceInputWire,
  type ResolveWire,
} from "./addApi";

const KINDS = [
  "manga",
  "manhwa",
  "manhua",
  "web_novel",
  "light_novel",
  "book",
  "anime",
  "movie",
  "tv",
  "game",
  "visual_novel",
  "other",
  "unknown",
];

// One in-flight create decision; the request id is minted once per attempt
// and reused when the attempt is retried after an ambiguous failure.
type CreateAction =
  | { kind: "create"; options: CreateOptions; payload: AdditionPayload }
  | { kind: "use_existing"; workId: string; options: CreateOptions; payload: AdditionPayload }
  | { kind: "add_edition"; workId: string; options: CreateOptions; payload: AdditionPayload }
  | { kind: "create_distinct"; options: CreateOptions; payload: AdditionPayload };

type Phase =
  | { name: "form" }
  | { name: "resolving" }
  | { name: "candidates"; result: ResolveWire }
  | { name: "existing"; workId: string }
  | { name: "creating"; action: CreateAction }
  | { name: "create_error"; action: CreateAction; message: string; stale: boolean };

function errorMessage(err: unknown): string {
  return err instanceof ApiError ? err.message : "the request failed";
}

export function QuickAddPage(): JSX.Element {
  const navigate = useNavigate();

  const [title, setTitle] = useState("");
  const [language, setLanguage] = useState("");
  const [kind, setKind] = useState("");
  const [otherLabel, setOtherLabel] = useState("");
  const [url, setUrl] = useState("");
  const [provider, setProvider] = useState("");
  const [externalId, setExternalId] = useState("");

  const [phase, setPhase] = useState<Phase>({ name: "form" });
  const [fieldError, setFieldError] = useState<string | null>(null);
  const [serverError, setServerError] = useState<string | null>(null);
  // Cover discovery is an explicit, optional action. A search failure only
  // appears inside the picker; it never delays or blocks record creation.
  const [pickerOpen, setPickerOpen] = useState(false);
  const [pickedCover, setPickedCover] = useState<ArtworkCandidate | null>(null);

  const busy = phase.name === "resolving" || phase.name === "creating";

  function buildPayload(): AdditionPayload {
    // A URL/ID-only entry sends no title so the server can still resolve a
    // known identity; an unknown identity then reports that a title is
    // required, which is the manual-title fallback.
    const trimmedTitle = title.trim();
    const payload: AdditionPayload = {
      titles:
        trimmedTitle === ""
          ? []
          : [
              {
                text: trimmedTitle,
                language: language.trim() || null,
                kind: "original",
                preferred: true,
                provenance: "manual",
              },
            ],
      references: [],
    };
    if (kind !== "") {
      payload.kind = kind;
      payload.language = language.trim() || undefined;
      if (kind === "other" && otherLabel.trim() !== "") {
        payload.other_label = otherLabel.trim();
      }
    }
    if (url.trim() !== "") {
      payload.references.push({ url: url.trim() });
    }
    if (externalId.trim() !== "") {
      const ref: ReferenceInputWire = { external_id: externalId.trim() };
      if (provider.trim() !== "") {
        ref.provider = provider.trim();
      }
      payload.references.push(ref);
    }
    return payload;
  }

  function runCreate(action: CreateAction): void {
    setPhase({ name: "creating", action });
    const attempt = (): Promise<{ path: string; workId: string; editionId: string | null }> => {
      const { options, payload } = action;
      switch (action.kind) {
        case "create":
          return createWork(payload, options).then((r) => ({
            path: targetPathFor(r),
            workId: r.work_id,
            editionId: r.edition_id,
          }));
        case "use_existing":
          return createWork(payload, options).then((r) => ({
            path: existingWorkPath(r.work_id),
            workId: r.work_id,
            editionId: r.edition_id,
          }));
        case "add_edition":
          return addEdition(action.workId, payload, options).then((r) => ({
            path: targetPathFor(r),
            workId: r.work_id,
            editionId: r.edition_id,
          }));
        case "create_distinct":
          return createWork(payload, options).then((r) => ({
            path: targetPathFor(r),
            workId: r.work_id,
            editionId: r.edition_id,
          }));
      }
    };
    attempt()
      .then(({ path, workId, editionId }) => {
        // A picked cover is attached after the record exists. This is a
        // best-effort follow-up: a failed artwork write never undoes a
        // successful creation or delays navigation.
        const cover = pickedCover;
        const token = cover === null ? "" : previewTokenFromImageUrl(cover.image_url);
        if (cover !== null && token !== "") {
          void selectArtwork({
            work_id: workId,
            edition_id: editionId,
            preview_token: token,
          }).catch(() => undefined);
        }
        navigate(path);
      })
      .catch((err: unknown) => {
        const stale = err instanceof ApiError && err.isConflict;
        setPhase({
          name: "create_error",
          action,
          message: errorMessage(err),
          stale,
        });
      });
  }

  function runResolve(): void {
    resolvedSignature.current = signature;
    setPhase({ name: "resolving" });
    setServerError(null);
    const payload = buildPayload();
    resolveAddition(payload)
      .then((result) => {
        if (result.outcome === "existing") {
          if (result.work_id === null) {
            throw new Error("the API returned an existing outcome without a work id");
          }
          setPhase({ name: "existing", workId: result.work_id });
        } else if (result.outcome === "candidates") {
          setPhase({ name: "candidates", result });
        } else {
          runCreate({
            kind: "create",
            options: { requestId: newRequestID() },
            payload,
          });
        }
      })
      .catch((err: unknown) => {
        setPhase({ name: "form" });
        setServerError(errorMessage(err));
      });
  }

  // The candidate token is bound to the resolved payload's fingerprint, so a
  // field edit in the candidates phase (medium or other label) invalidates
  // it; re-resolve instead of letting the first choice fail with a stale
  // token.
  const resolvedSignature = useRef<string | null>(null);
  const signature = [
    title.trim(),
    language.trim(),
    kind,
    otherLabel.trim(),
    url.trim(),
    provider.trim(),
    externalId.trim(),
  ].join("|");

  useEffect(() => {
    if (phase.name !== "candidates" || signature === resolvedSignature.current) {
      return;
    }
    const timer = setTimeout(runResolve, 250);
    return () => clearTimeout(timer);
    // runResolve and resolvedSignature are stable closures of the current
    // state; the signature captures every payload field.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [signature, phase.name]);

  // While a re-resolve is pending the displayed token is stale, so the
  // choices stay disabled instead of risking a stale-token conflict.
  const reResolvePending =
    phase.name === "candidates" && signature !== resolvedSignature.current;

  function submit(): void {
    const hasReference = url.trim() !== "" || externalId.trim() !== "";
    if (title.trim() === "" && !hasReference) {
      setFieldError("A title is required (or a known reference).");
      return;
    }
    setFieldError(null);
    runResolve();
  }

  function choose(
    choice: "use_existing" | "add_edition" | "create_distinct",
    workId: string | null,
    token: string,
  ): void {
    const options: CreateOptions = {
      requestId: newRequestID(),
      choice,
      candidateToken: token,
    };
    if (workId !== null) {
      options.workId = workId;
    }
    const payload = buildPayload();
    if (choice === "use_existing") {
      runCreate({ kind: "use_existing", workId: workId as string, options, payload });
    } else if (choice === "add_edition") {
      runCreate({ kind: "add_edition", workId: workId as string, options, payload });
    } else {
      runCreate({ kind: "create_distinct", options, payload });
    }
  }

  function backToForm(): void {
    setPhase({ name: "form" });
  }

  function kindControls(hint: string): JSX.Element {
    return (
      <>
        <label className="library-field">
          <span>{hint}</span>
          <Select value={kind} onChange={(e) => setKind(e.target.value)}>
            <option value="">Not set</option>
            {KINDS.map((k) => (
              <option key={k} value={k}>
                {k.replace("_", " ")}
              </option>
            ))}
          </Select>
        </label>
        {kind === "other" && (
          <label className="library-field">
            <span>Other label (required for "other")</span>
            <Input type="text" value={otherLabel} onChange={(e) => setOtherLabel(e.target.value)} />
          </label>
        )}
      </>
    );
  }

  function candidateButtons(c: CandidateWire, token: string): JSX.Element {
    // The server's candidate identity is work plus edition, so two candidate
    // editions of one work are distinct items.
    return (
      <li key={`${c.work_id}/${c.edition_id ?? ""}`} className="quick-add-candidate">
        <p className="login-hint">Work {c.work_id}</p>
        {c.evidence.map((e) => (
          <p key={e} className="login-hint">
            {e}
          </p>
        ))}
        <div className="quick-add-actions">
          <Button
            disabled={busy || reResolvePending}
            onClick={() => choose("use_existing", c.work_id, token)}
          >
            Use existing
          </Button>
          <Button
            disabled={busy || reResolvePending || kind === ""}
            title={kind === "" ? "Choose a medium to add an edition" : undefined}
            onClick={() => choose("add_edition", c.work_id, token)}
          >
            Add edition under this work
          </Button>
        </div>
      </li>
    );
  }

  return (
    <section className="login-page quick-add">
      <h2>Quick add</h2>

      {phase.name === "existing" && (
        <div className="quick-add-result">
          <p>
            This identity already exists in the library as work{" "}
            <span className="login-hint">{phase.workId}</span>. Nothing was created.
          </p>
          <div className="quick-add-actions">
            <Button onClick={() => navigate(existingWorkPath(phase.workId))}>
              Open existing record
            </Button>
            <Button onClick={backToForm}>Back to form</Button>
          </div>
        </div>
      )}

      {phase.name === "candidates" && (
        <div className="quick-add-result">
          <p>
            Possible matches for this title. Nothing is merged or created until you
            choose.
          </p>
          {kindControls("Medium (optional; required to add an edition)")}
          <ul className="quick-add-candidates library-list">
            {(phase.result.candidates ?? []).map((c) =>
              phase.result.candidate_token
                ? candidateButtons(c, phase.result.candidate_token)
                : null,
            )}
          </ul>
          <div className="quick-add-actions">
            <Button
              disabled={busy || reResolvePending || phase.result.candidate_token === null}
              onClick={() =>
                phase.result.candidate_token !== null &&
                choose("create_distinct", null, phase.result.candidate_token)
              }
            >
              Create as a distinct work
            </Button>
            <Button onClick={backToForm}>Start over</Button>
          </div>
        </div>
      )}

      {phase.name === "create_error" && (
        <div className="quick-add-result">
          <p className="form-error" role="alert">
            {phase.message}
          </p>
          <div className="quick-add-actions">
            {phase.stale && <Button onClick={runResolve}>Resolve again</Button>}
            <Button onClick={() => runCreate(phase.action)}>Retry</Button>
            <Button onClick={backToForm}>Back to form</Button>
          </div>
        </div>
      )}

      {(phase.name === "form" ||
        phase.name === "resolving" ||
        phase.name === "creating" ||
        phase.name === "create_error") && (
        <form onSubmit={(e) => e.preventDefault()}>
          <label className="library-field">
            <span>Title</span>
            <Input
              type="text"
              value={title}
              onChange={(e) => setTitle(e.target.value)}
              placeholder="The title you want to add"
            />
          </label>
          {kindControls("Medium (optional)")}
          <label className="library-field">
            <span>Language (optional)</span>
            <Input
              type="text"
              value={language}
              onChange={(e) => setLanguage(e.target.value)}
              placeholder="e.g. ja"
            />
          </label>
          <fieldset className="quick-add-reference">
            <legend>Reference (optional)</legend>
            <label className="library-field">
              <span>URL</span>
              <Input
                type="url"
                value={url}
                onChange={(e) => setUrl(e.target.value)}
                placeholder="https://…"
              />
            </label>
            <label className="library-field">
              <span>Provider (optional, with external ID)</span>
              <Input
                type="text"
                value={provider}
                onChange={(e) => setProvider(e.target.value)}
              />
            </label>
            <label className="library-field">
              <span>External ID (optional, with provider)</span>
              <Input
                type="text"
                value={externalId}
                onChange={(e) => setExternalId(e.target.value)}
              />
            </label>
          </fieldset>

          {fieldError !== null && <p className="form-error" role="alert">{fieldError}</p>}
          {serverError !== null && <p className="form-error" role="alert">{serverError}</p>}
          {phase.name === "resolving" && (
            <p className="shell-loading">Checking for existing matches…</p>
          )}
          {phase.name === "creating" && <p className="shell-loading">Saving…</p>}
          {pickedCover !== null && (
            <p className="cover-pick-note">
              Cover picked: {pickedCover.title} from {pickedCover.provider}. It is saved
              only after the work is created, and only if you chose it.
            </p>
          )}

          <div className="quick-add-actions">
            <Button disabled={busy} onClick={submit}>
              {phase.name === "resolving"
                ? "Checking…"
                : phase.name === "creating"
                  ? "Saving…"
                  : "Add"}
            </Button>
            <Button disabled={title.trim() === ""} onClick={() => setPickerOpen(true)}>
              Find a cover
            </Button>
          </div>
        </form>
      )}

      <CoverPicker
        open={pickerOpen}
        onClose={() => setPickerOpen(false)}
        title={title}
        kind={kind}
        alternatives={[]}
        references={[url.trim(), externalId.trim()].filter((value) => value !== "")}
        onPick={(candidate) => setPickedCover(candidate)}
      />
    </section>
  );
}
