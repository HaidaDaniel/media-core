// The work detail screen: one logical work with its active titles/aliases,
// each edition keeping its own consumption/intention/rating/progress and
// notes, the work's relations, each edition's relations (where anime→manga
// source navigation lives), and the deliberate propose/confirm/revoke/origin
// forms. The work view is the frozen read; per-edition state and notes come
// from each edition detail. A proposed claim is never rendered as a
// confirmed fact, and originality is never inferred from grouping.
import { useCallback, useEffect, useRef, useState } from "react";
import type { JSX } from "react";
import { Link, useParams } from "react-router-dom";
import { ApiError } from "../api/client";
import {
  ORIGIN_KINDS,
  RelationRow,
  WORK_RELATION_TYPES,
  type RelationEdgeLike,
} from "../relations/RelationRow";
import {
  confirmRelation,
  editionRelations,
  proposeRelation,
  recordOrigin,
  revokeRelation,
  type ClaimStateInput,
  type OriginViewWire,
  type RelatedViewWire,
  type RelationViewWire,
  workRelations,
} from "../relations/relationsApi";
import { getEdition, getWork, type EditionViewWire, type WorkViewWire } from "./workApi";

function errorMessage(e: unknown): string {
  if (e instanceof ApiError) return e.message;
  if (e instanceof Error) return e.message;
  return "Something went wrong";
}

function preferredTitle(titles: { text: string; preferred: boolean }[]): string | null {
  const preferred = titles.find((t) => t.preferred) ?? titles[0];
  return preferred ? preferred.text : null;
}

// A readable one-line summary of an edition's individual progress units; an
// absent unit is omitted, never zero.
// The work-scope relation types stored symmetrically: their canonical from
// entity is the lower-sorting endpoint, so the revision guard is not always
// the current work's revision.
const SYMMETRIC_WORK_TYPES = new Set(["same_underlying_work", "related"]);

function progressSummary(progress: EditionViewWire["progress"]): string {
  const parts: string[] = [];
  for (const unit of ["chapter", "volume", "episode", "page", "percent"] as const) {
    const record = progress[unit];
    if (record) {
      parts.push(`${record.unit}: ${record.position}${record.total ? `/${record.total}` : ""}`);
    }
  }
  return parts.join(", ");
}

export function WorkPage(): JSX.Element {
  const { id: workId = "" } = useParams<{ id: string }>();

  const [work, setWork] = useState<WorkViewWire | null>(null);
  const [workError, setWorkError] = useState<string | null>(null);
  const [editions, setEditions] = useState<Record<string, EditionViewWire>>({});
  const [editionErrors, setEditionErrors] = useState<Record<string, string>>({});
  const [workRel, setWorkRel] = useState<RelatedViewWire | null>(null);
  const [workRelError, setWorkRelError] = useState<string | null>(null);
  // The full relation view returned by a propose/confirm/revoke mutation:
  // unlike the traversal it carries per-edge provenance, so after an action
  // the section renders from it. It is cleared by the next full load.
  const [workRelFull, setWorkRelFull] = useState<RelationViewWire | null>(null);
  const [editionRels, setEditionRels] = useState<Record<string, RelatedViewWire>>({});
  const [editionRelErrors, setEditionRelErrors] = useState<Record<string, string>>({});
  const [labels, setLabels] = useState<Record<string, string>>({});
  const [origins, setOrigins] = useState<Record<string, OriginViewWire>>({});
  const [busy, setBusy] = useState(false);
  const [actionError, setActionError] = useState<string | null>(null);

  // Propose form (work scope): the current work is always the from endpoint.
  const [proposeType, setProposeType] = useState<string>(WORK_RELATION_TYPES[0]);
  const [proposeState, setProposeState] = useState<ClaimStateInput>("proposed");
  const [proposeOther, setProposeOther] = useState("");
  const [proposeProvenance, setProposeProvenance] = useState("");
  const [fieldError, setFieldError] = useState<string | null>(null);

  // Origin form: targets one of this work's editions.
  const [originEdition, setOriginEdition] = useState("");
  const [originKind, setOriginKind] = useState<string>(ORIGIN_KINDS[0]);
  const [originState, setOriginState] = useState<ClaimStateInput>("proposed");
  const [originProvenance, setOriginProvenance] = useState("");

  const generation = useRef(0);

  const resolveLabels = useCallback(
    async (gen: number, w: WorkViewWire, wRel: RelatedViewWire | null, eRels: Record<string, RelatedViewWire>) => {
      const wanted = new Map<string, "work" | "edition">();
      const collect = (view: RelatedViewWire | null, scope: "work" | "edition", currentId: string) => {
        if (!view) return;
        for (const edge of view.edges) {
          const other = edge.from_id === currentId ? edge.to_id : edge.from_id;
          if (other !== currentId) wanted.set(other, scope);
        }
      };
      collect(wRel, "work", w.id);
      for (const e of w.editions) collect(eRels[e.id], "edition", e.id);
      const resolved: Record<string, string> = {};
      await Promise.all(
        [...wanted.entries()].map(async ([id, scope]) => {
          try {
            const view = scope === "work" ? await getWork(id) : await getEdition(id);
            resolved[id] = preferredTitle(view.titles) ?? id;
          } catch {
            resolved[id] = id;
          }
        }),
      );
      if (generation.current === gen) {
        setLabels((prev) => ({ ...prev, ...resolved }));
      }
    },
    [],
  );

  // load returns whether it applied its state as the latest generation; a
  // stale load (the screen moved on) reports false so callers do not write
  // results that belong to a previous work.
  const load = useCallback(async (): Promise<boolean> => {
    if (!workId) return false;
    const gen = ++generation.current;
    setWorkError(null);
    try {
      const w = await getWork(workId);
      if (generation.current !== gen) return false;
      setWork(w);
      const editionIds = w.editions.map((e) => e.id);
      const [editionResults, workRelResult, editionRelResults] = await Promise.all([
        Promise.allSettled(editionIds.map((id) => getEdition(id))),
        workRelations(workId).then(
          (value) => ({ ok: true as const, value }),
          (error) => ({ ok: false as const, error }),
        ),
        Promise.allSettled(editionIds.map((id) => editionRelations(id))),
      ]);
      if (generation.current !== gen) return false;
      const eds: Record<string, EditionViewWire> = {};
      const edErrs: Record<string, string> = {};
      editionResults.forEach((r, i) => {
        const eid = editionIds[i];
        if (r.status === "fulfilled") eds[eid] = r.value;
        else edErrs[eid] = errorMessage(r.reason);
      });
      setEditions(eds);
      setEditionErrors(edErrs);
      const wRel = workRelResult.ok ? workRelResult.value : null;
      setWorkRel(wRel);
      setWorkRelError(workRelResult.ok ? null : errorMessage(workRelResult.error));
      // A fresh traversal is the source again; the mutation view is stale.
      setWorkRelFull(null);
      const eRels: Record<string, RelatedViewWire> = {};
      const eRelErrs: Record<string, string> = {};
      editionRelResults.forEach((r, i) => {
        const eid = editionIds[i];
        if (r.status === "fulfilled") eRels[eid] = r.value;
        else eRelErrs[eid] = errorMessage(r.reason);
      });
      setEditionRels(eRels);
      setEditionRelErrors(eRelErrs);
      await resolveLabels(gen, w, wRel, eRels);
      return generation.current === gen;
    } catch (e) {
      if (generation.current !== gen) return false;
      setWorkError(errorMessage(e));
      return false;
    }
  }, [workId, resolveLabels]);

  useEffect(() => {
    void load();
  }, [load]);

  const label = useCallback((id: string): string => labels[id] ?? id, [labels]);

  const submitPropose = async () => {
    if (!work) return;
    const other = proposeOther.trim();
    const provenance = proposeProvenance.trim();
    if (other === "") {
      setFieldError("Enter the other work's id.");
      return;
    }
    if (provenance === "") {
      setFieldError("Provenance is required.");
      return;
    }
    setFieldError(null);
    setActionError(null);
    setBusy(true);
    try {
      // The server guards the canonical from entity's revision; for a
      // symmetric type whose lower-sorting endpoint is the other work, that
      // is the other work, so its revision must be fetched first.
      let expectedRevision = work.revision;
      if (SYMMETRIC_WORK_TYPES.has(proposeType) && other < work.id) {
        const otherWork = await getWork(other);
        expectedRevision = otherWork.revision;
      }
      const view = await proposeRelation({
        scope: "work",
        type: proposeType,
        fromId: work.id,
        toId: other,
        state: proposeState,
        provenance,
        expectedRevision,
      });
      setProposeOther("");
      setProposeProvenance("");
      // Reload for the fresh traversal and revision; only if this load was
      // still current (the screen did not move on) do we let the
      // provenance-carrying mutation view render the section.
      if (await load()) {
        setWorkRelFull(view);
      }
    } catch (e) {
      setActionError(errorMessage(e));
    } finally {
      setBusy(false);
    }
  };

  const runRelationAction = async (action: "confirm" | "revoke", relationId: string) => {
    if (!work) return;
    setActionError(null);
    setBusy(true);
    try {
      const view =
        action === "confirm"
          ? await confirmRelation(relationId, work.revision)
          : await revokeRelation(relationId, work.revision);
      if (await load()) {
        setWorkRelFull(view);
      }
    } catch (e) {
      setActionError(errorMessage(e));
    } finally {
      setBusy(false);
    }
  };

  const selectedOriginEdition =
    originEdition || (work?.editions[0]?.id ?? "");

  const submitOrigin = async () => {
    if (!work || selectedOriginEdition === "") return;
    const provenance = originProvenance.trim();
    if (provenance === "") {
      setFieldError("Provenance is required.");
      return;
    }
    const summary = work.editions.find((e) => e.id === selectedOriginEdition);
    if (!summary) return;
    setFieldError(null);
    setActionError(null);
    setBusy(true);
    try {
      const view = await recordOrigin({
        editionId: selectedOriginEdition,
        kind: originKind,
        state: originState,
        provenance,
        expectedRevision: summary.revision,
      });
      setOrigins((prev) => ({ ...prev, [selectedOriginEdition]: view }));
      setOriginProvenance("");
      // The record bumps the edition revision, so reload to keep the
      // next record's revision guard fresh.
      await load();
    } catch (e) {
      setActionError(errorMessage(e));
    } finally {
      setBusy(false);
    }
  };

  if (work === null) {
    if (workError !== null) {
      return (
        <section className="placeholder">
          <h2>Work</h2>
          <div className="library-error">
            <p role="alert">{workError}</p>
            <button type="button" onClick={() => void load()}>
              Retry
            </button>
          </div>
        </section>
      );
    }
    return (
      <section className="placeholder">
        <h2>Work</h2>
        <p className="library-loading">Loading…</p>
      </section>
    );
  }

  const preferred = work.titles.find((t) => t.preferred) ?? work.titles[0];
  const aliases = work.titles.filter((t) => t.id !== preferred?.id);

  // The rows of the "Related works" section: after a mutation the full view
  // (which carries provenance) is preferred; otherwise the traversal.
  const workRelationRows: (RelationEdgeLike & { provenance?: string })[] =
    workRelFull !== null
      ? [...workRelFull.proposed, ...workRelFull.confirmed].map((r) => ({
          relation_id: r.id,
          type: r.type,
          from_id: r.from_id,
          to_id: r.to_id,
          state: r.state,
          provenance: r.provenance,
        }))
      : (workRel?.edges ?? []).map((e) => ({
          relation_id: e.relation_id,
          type: e.type,
          from_id: e.from_id,
          to_id: e.to_id,
          state: e.state,
        }));

  return (
    <section className="placeholder">
      <h2>
        {preferred ? preferred.text : "Unknown"}
        {preferred?.language ? <span className="work-card-lang"> {preferred.language}</span> : null}
      </h2>
      {aliases.length > 0 && (
        <p className="work-card-aliases">
          {aliases.map((t) => (
            <span key={t.id} className="work-card-alias">
              {t.text}
              {t.language ? ` (${t.language})` : ""}
            </span>
          ))}
        </p>
      )}
      {work.archived_at !== null && <p className="login-hint">Archived</p>}

      {workError !== null && (
        <div className="library-error">
          <p role="alert">The work could not be refreshed: {workError}</p>
          <button type="button" onClick={() => void load()}>
            Retry
          </button>
        </div>
      )}
      {actionError !== null && (
        <div className="library-error">
          <p role="alert">{actionError}</p>
        </div>
      )}

      <h3>Editions</h3>
      <ul className="library-list">
        {work.editions.map((summary) => {
          const detail = editions[summary.id];
          const rel = editionRels[summary.id];
          return (
            <li key={summary.id} className="work-card">
              <h3>
                <Link to={`/editions/${encodeURIComponent(summary.id)}`}>{summary.kind}</Link>{" "}
                <span className="work-card-lang">{summary.language ?? "Unknown"}</span>
              </h3>
              {detail ? (
                <EditionState edition={detail} />
              ) : (
                <p className="form-error" role="alert">
                  {editionErrors[summary.id] ?? "Loading…"}
                </p>
              )}
              {detail && detail.notes.length > 0 && (
                <ul className="work-editions">
                  {detail.notes.map((note) => (
                    <li key={note.id} className="work-edition">
                      <span>{note.text}</span>
                      {note.spoiler && <span className="work-badge">Spoiler</span>}
                    </li>
                  ))}
                </ul>
              )}
              {rel && rel.edges.length > 0 && (
                <>
                  <p className="login-hint">Related editions</p>
                  <ul className="library-list">
                    {rel.edges.map((edge) => (
                      <RelationRow
                        key={edge.relation_id}
                        edge={edge}
                        scope="edition"
                        currentId={summary.id}
                        label={label}
                        actionable={false}
                      />
                    ))}
                  </ul>
                </>
              )}
              {rel === undefined && editionRelErrors[summary.id] !== undefined && (
                <p className="form-error" role="alert">
                  {editionRelErrors[summary.id]}
                </p>
              )}
            </li>
          );
        })}
        {work.editions.length === 0 && <li className="work-edition">No editions yet</li>}
      </ul>

      <h3>Related works</h3>
      {workRelationRows.length > 0 && (
        <ul className="library-list">
          {workRelationRows.map((row) => (
            <RelationRow
              key={row.relation_id}
              edge={row}
              scope="work"
              currentId={work.id}
              label={label}
              provenance={row.provenance}
              actionable={row.from_id === work.id}
              busy={busy}
              onConfirm={(id) => void runRelationAction("confirm", id)}
              onRevoke={(id) => void runRelationAction("revoke", id)}
            />
          ))}
        </ul>
      )}
      {workRelationRows.length === 0 && (
        <p className="login-hint">No related works.</p>
      )}
      {workRel === null && workRelError !== null && (
        <div className="library-error">
          <p role="alert">{workRelError}</p>
          <button type="button" onClick={() => void load()}>
            Retry
          </button>
        </div>
      )}

      <h3>Propose a relation</h3>
      {fieldError !== null && (
        <p className="form-error" role="alert">
          {fieldError}
        </p>
      )}
      <form
        className="library-toolbar"
        onSubmit={(e) => {
          e.preventDefault();
          void submitPropose();
        }}
      >
        <label className="library-field">
          <span>Relation</span>
          <select
            value={proposeType}
            onChange={(e) => setProposeType(e.target.value)}
            disabled={busy}
          >
            {WORK_RELATION_TYPES.map((t) => (
              <option key={t} value={t}>
                {t.replace(/_/g, " ")}
              </option>
            ))}
          </select>
        </label>
        <label className="library-field">
          <span>State</span>
          <select
            value={proposeState}
            onChange={(e) => setProposeState(e.target.value as ClaimStateInput)}
            disabled={busy}
          >
            <option value="proposed">proposed</option>
            <option value="confirmed">confirmed</option>
          </select>
        </label>
        <label className="library-field">
          <span>Other work id</span>
          <input
            type="text"
            value={proposeOther}
            onChange={(e) => setProposeOther(e.target.value)}
            disabled={busy}
          />
        </label>
        <label className="library-field">
          <span>Provenance</span>
          <input
            type="text"
            value={proposeProvenance}
            onChange={(e) => setProposeProvenance(e.target.value)}
            disabled={busy}
          />
        </label>
        <button type="submit" disabled={busy}>
          Propose
        </button>
      </form>

      <h3>Origin</h3>
      <form
        className="library-toolbar"
        onSubmit={(e) => {
          e.preventDefault();
          void submitOrigin();
        }}
      >
        <label className="library-field">
          <span>Edition</span>
          <select
            value={selectedOriginEdition}
            onChange={(e) => setOriginEdition(e.target.value)}
            disabled={busy || work.editions.length === 0}
          >
            {work.editions.map((e) => (
              <option key={e.id} value={e.id}>
                {e.kind}
                {e.language ? ` (${e.language})` : ""}
              </option>
            ))}
          </select>
        </label>
        <label className="library-field">
          <span>Kind</span>
          <select
            value={originKind}
            onChange={(e) => setOriginKind(e.target.value)}
            disabled={busy}
          >
            {ORIGIN_KINDS.map((k) => (
              <option key={k} value={k}>
                {k.replace(/_/g, " ")}
              </option>
            ))}
          </select>
        </label>
        <label className="library-field">
          <span>State</span>
          <select
            value={originState}
            onChange={(e) => setOriginState(e.target.value as ClaimStateInput)}
            disabled={busy}
          >
            <option value="proposed">proposed</option>
            <option value="confirmed">confirmed</option>
          </select>
        </label>
        <label className="library-field">
          <span>Provenance</span>
          <input
            type="text"
            value={originProvenance}
            onChange={(e) => setOriginProvenance(e.target.value)}
            disabled={busy}
          />
        </label>
        <button type="submit" disabled={busy || work.editions.length === 0}>
          Record origin
        </button>
      </form>
      {selectedOriginEdition !== "" && (
        <OriginClaims
          view={origins[selectedOriginEdition]}
          editionKind={
            work.editions.find((e) => e.id === selectedOriginEdition)?.kind ?? ""
          }
        />
      )}
    </section>
  );
}

// EditionState renders one edition's individual personal dimensions; a null
// dimension is the explicit unknown state, and the editions of a work are
// never combined.
function EditionState({ edition }: { edition: EditionViewWire }): JSX.Element {
  const consumption = edition.consumption?.value ?? "unknown";
  const intention = edition.intention?.value ?? "none";
  const rating = edition.rating?.value ?? null;
  const progress = progressSummary(edition.progress);
  const badgeClass =
    consumption === "completed"
      ? "work-badge work-badge-completed"
      : consumption === "in_progress"
        ? "work-badge work-badge-in_progress"
        : "work-badge";
  return (
    <p>
      <span className={badgeClass}>{consumption.replace(/_/g, " ")}</span>{" "}
      <span className="login-hint">
        intention: {intention.replace(/_/g, " ")} · rating: {rating ?? "none"}
        {progress !== "" ? ` · ${progress}` : ""}
      </span>
    </p>
  );
}

// OriginClaims renders the recorded origin claims of one edition; with none
// recorded the source is the explicit unknown default, never an inference.
function OriginClaims({
  view,
  editionKind,
}: {
  view: OriginViewWire | undefined;
  editionKind: string;
}): JSX.Element {
  if (!view || view.claims.length === 0) {
    return (
      <p className="login-hint">
        Source: unknown (no origin claim recorded for this {editionKind} on this
        screen yet).
      </p>
    );
  }
  return (
    <ul className="library-list">
      {view.claims.map((claim) => (
        <li key={claim.id} className="work-edition">
          <span>
            Source: {claim.kind.replace(/_/g, " ")}
          </span>
          <span className={claim.state === "proposed" ? "work-badge work-badge-in_progress" : "work-badge"}>
            {claim.state === "proposed" ? "Proposed" : "Confirmed"}
          </span>
          <span className="login-hint">{claim.provenance}</span>
        </li>
      ))}
    </ul>
  );
}
