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
  | { kind: "create"; options: CreateOptions }
  | { kind: "use_existing"; workId: string; options: CreateOptions }
  | { kind: "add_edition"; workId: string; options: CreateOptions }
  | { kind: "create_distinct"; options: CreateOptions };

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
    const attempt = (): Promise<{ path: string }> => {
      const { options } = action;
      const payload = buildPayload();
      switch (action.kind) {
        case "create":
          return createWork(payload, options).then((r) => ({ path: targetPathFor(r) }));
        case "use_existing":
          return createWork(payload, options).then((r) => ({
            path: existingWorkPath(r.work_id),
          }));
        case "add_edition":
          return addEdition(action.workId, payload, options).then((r) => ({
            path: targetPathFor(r),
          }));
        case "create_distinct":
          return createWork(payload, options).then((r) => ({ path: targetPathFor(r) }));
      }
    };
    attempt()
      .then(({ path }) => {
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
    resolveAddition(buildPayload())
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
    if (choice === "use_existing") {
      runCreate({ kind: "use_existing", workId: workId as string, options });
    } else if (choice === "add_edition") {
      runCreate({ kind: "add_edition", workId: workId as string, options });
    } else {
      runCreate({ kind: "create_distinct", options });
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
          <select value={kind} onChange={(e) => setKind(e.target.value)}>
            <option value="">Not set</option>
            {KINDS.map((k) => (
              <option key={k} value={k}>
                {k.replace("_", " ")}
              </option>
            ))}
          </select>
        </label>
        {kind === "other" && (
          <label className="library-field">
            <span>Other label (required for "other")</span>
            <input type="text" value={otherLabel} onChange={(e) => setOtherLabel(e.target.value)} />
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
          <button
            type="button"
            disabled={busy || reResolvePending}
            onClick={() => choose("use_existing", c.work_id, token)}
          >
            Use existing
          </button>
          <button
            type="button"
            disabled={busy || reResolvePending || kind === ""}
            title={kind === "" ? "Choose a medium to add an edition" : undefined}
            onClick={() => choose("add_edition", c.work_id, token)}
          >
            Add edition under this work
          </button>
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
            <button type="button" onClick={() => navigate(existingWorkPath(phase.workId))}>
              Open existing record
            </button>
            <button type="button" onClick={backToForm}>
              Back to form
            </button>
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
            <button
              type="button"
              disabled={busy || reResolvePending || phase.result.candidate_token === null}
              onClick={() =>
                phase.result.candidate_token !== null &&
                choose("create_distinct", null, phase.result.candidate_token)
              }
            >
              Create as a distinct work
            </button>
            <button type="button" onClick={backToForm}>
              Start over
            </button>
          </div>
        </div>
      )}

      {phase.name === "create_error" && (
        <div className="quick-add-result">
          <p className="form-error" role="alert">
            {phase.message}
          </p>
          <div className="quick-add-actions">
            {phase.stale && (
              <button type="button" onClick={runResolve}>
                Resolve again
              </button>
            )}
            <button type="button" onClick={() => runCreate(phase.action)}>
              Retry
            </button>
            <button type="button" onClick={backToForm}>
              Back to form
            </button>
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
            <input
              type="text"
              value={title}
              onChange={(e) => setTitle(e.target.value)}
              placeholder="The title you want to add"
            />
          </label>
          {kindControls("Medium (optional)")}
          <label className="library-field">
            <span>Language (optional)</span>
            <input
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
              <input
                type="url"
                value={url}
                onChange={(e) => setUrl(e.target.value)}
                placeholder="https://…"
              />
            </label>
            <label className="library-field">
              <span>Provider (optional, with external ID)</span>
              <input
                type="text"
                value={provider}
                onChange={(e) => setProvider(e.target.value)}
              />
            </label>
            <label className="library-field">
              <span>External ID (optional, with provider)</span>
              <input
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

          <button type="button" disabled={busy} onClick={submit}>
            {phase.name === "resolving" ? "Checking…" : phase.name === "creating" ? "Saving…" : "Add"}
          </button>
        </form>
      )}
    </section>
  );
}
