// The edition detail screen: the edition's identity, kind, language and
// publication fact with its source, the parent work, per-unit progress with
// the explicit "record new update" versus "correct mistake" actions, the
// consumption status, the bounded observation history with retraction, and
// the narrow metadata corrections. The frozen read is the edition view; a
// failed load keeps the prior state intact with a distinguishable error, and
// notes, ratings and relation forms stay out of this screen.
import { useCallback, useEffect, useRef, useState } from "react";
import type { JSX } from "react";
import { Link, useParams } from "react-router-dom";
import { ApiError } from "../api/client";
import { getWork } from "../work/workApi";
import {
  CONSUMPTION_STATUS_OPTIONS,
  EDITION_KINDS,
  PROGRESS_UNITS,
  getEdition,
  getEditionHistory,
  recordProgress,
  retractProgress,
  setConsumption,
  updateEdition,
  type EditionViewWire,
  type HistoryItemWire,
  type HistoryListWire,
  type ProgressRecordWire,
} from "./editionApi";

function errorMessage(e: unknown): string {
  if (e instanceof ApiError) return e.message;
  if (e instanceof Error) return e.message;
  return "Something went wrong";
}

function preferredTitle(titles: { text: string; preferred: boolean }[]): string | null {
  const preferred = titles.find((t) => t.preferred) ?? titles[0];
  return preferred ? preferred.text : null;
}

function formatInstant(iso: string | null): string {
  if (iso === null) return "unknown";
  const date = new Date(iso);
  return Number.isNaN(date.getTime()) ? iso : date.toLocaleString();
}

// The live per-unit progress records of one edition; an absent unit is
// omitted, never zero.
function currentProgressRecords(edition: EditionViewWire): ProgressRecordWire[] {
  const records: ProgressRecordWire[] = [];
  for (const unit of PROGRESS_UNITS) {
    const record = edition.progress[unit];
    if (record !== null && record !== undefined) records.push(record);
  }
  return records;
}

function progressText(record: ProgressRecordWire): string {
  return `${record.unit}: ${record.position}${record.total ? `/${record.total}` : ""}`;
}

// One history row: the dimension, its value/detail and the row state. A
// retracted row has no value and shows as retracted, not deleted.
function historyText(item: HistoryItemWire): string {
  if (item.type === "progress") {
    const detail = item.detail !== null ? item.detail.replace(/\/$/, "") : null;
    return detail !== null ? `${item.value ?? "progress"} ${detail}` : `${item.value ?? "progress"}`;
  }
  if (item.detail !== null) return `${item.value ?? item.type}: ${item.detail}`;
  return item.value ?? item.type;
}

// The row state badge: the live per-unit record of a progress row is
// current; every other row keeps the server's effective/superseded/
// retracted state, so an old position is shown, never hidden or deleted.
function historyBadge(item: HistoryItemWire, edition: EditionViewWire): string {
  if (item.type === "progress" && item.value !== null) {
    const record = currentProgressRecords(edition).find((r) => r.unit === item.value);
    if (record !== undefined && record.id === item.id) return "current";
  }
  return item.label;
}

export function EditionPage(): JSX.Element {
  const { id: editionId = "" } = useParams<{ id: string }>();

  const [edition, setEdition] = useState<EditionViewWire | null>(null);
  const [loadError, setLoadError] = useState<string | null>(null);
  const [workTitle, setWorkTitle] = useState<string | null>(null);
  const [history, setHistory] = useState<HistoryListWire | null>(null);
  const [historyError, setHistoryError] = useState<string | null>(null);
  const [historyLoadingMore, setHistoryLoadingMore] = useState(false);
  const [busy, setBusy] = useState(false);
  const [actionError, setActionError] = useState<string | null>(null);
  const [fieldError, setFieldError] = useState<string | null>(null);

  // Progress form: the mode makes a new update and a mistake correction two
  // separate actions; a correction names the current record it supersedes.
  const [progMode, setProgMode] = useState<"new" | "correct">("new");
  const [progUnit, setProgUnit] = useState<string>(PROGRESS_UNITS[0]);
  const [progPosition, setProgPosition] = useState("");
  const [progTotal, setProgTotal] = useState("");
  const [progSupersedes, setProgSupersedes] = useState("");

  // Status form.
  const [consValue, setConsValue] = useState<string>(CONSUMPTION_STATUS_OPTIONS[0]);
  const [consReason, setConsReason] = useState("");

  // Retraction form: only progress observations are retractable.
  const [retractRecord, setRetractRecord] = useState("");
  const [retractReason, setRetractReason] = useState("");

  // Metadata form: seeded from the edition on load and after a metadata
  // save; an empty language or other label means "unchanged" (the frozen
  // body cannot clear a field).
  const [metaKind, setMetaKind] = useState<string>("");
  const [metaLanguage, setMetaLanguage] = useState("");
  const [metaOther, setMetaOther] = useState("");

  const generation = useRef(0);

  const syncMetaFrom = (ed: EditionViewWire) => {
    setMetaKind(ed.kind);
    setMetaLanguage(ed.language ?? "");
    setMetaOther(ed.other_label ?? "");
  };

  // load returns whether it applied its state as the latest generation; a
  // stale load (the screen moved on) reports false so callers do not write
  // results that belong to a previous edition.
  const load = useCallback(async (): Promise<boolean> => {
    if (!editionId) return false;
    const gen = ++generation.current;
    setLoadError(null);
    setActionError(null);
    try {
      const ed = await getEdition(editionId);
      if (generation.current !== gen) return false;
      setEdition(ed);
      syncMetaFrom(ed);
      const [histRes, workRes] = await Promise.all([
        getEditionHistory(editionId).then(
          (value) => ({ ok: true as const, value }),
          (error: unknown) => ({ ok: false as const, error }),
        ),
        getWork(ed.work_id).then(
          (value) => ({ ok: true as const, value }),
          (error: unknown) => ({ ok: false as const, error }),
        ),
      ]);
      if (generation.current !== gen) return false;
      if (histRes.ok) {
        setHistory(histRes.value);
        setHistoryError(null);
      } else {
        setHistoryError(errorMessage(histRes.error));
      }
      if (workRes.ok) setWorkTitle(preferredTitle(workRes.value.titles) ?? ed.work_id);
      return true;
    } catch (e) {
      if (generation.current !== gen) return false;
      setLoadError(errorMessage(e));
      return false;
    }
  }, [editionId]);

  useEffect(() => {
    void load();
  }, [load]);

  // refreshHistory re-reads the first history page after a mutation; a
  // failed refresh keeps the prior history beside a distinguishable error.
  // The caller passes the generation captured before its mutation so a
  // navigation in between discards this refresh.
  const refreshHistory = useCallback(async (gen: number): Promise<boolean> => {
    if (!editionId) return false;
    try {
      const page = await getEditionHistory(editionId);
      if (generation.current !== gen) return false;
      setHistory(page);
      setHistoryError(null);
      return true;
    } catch (e) {
      if (generation.current === gen) setHistoryError(errorMessage(e));
      return false;
    }
  }, [editionId]);

  const loadMoreHistory = async () => {
    const cursor = history?.next_cursor;
    if (!editionId || cursor === null || historyLoadingMore) return;
    const gen = generation.current;
    setHistoryLoadingMore(true);
    try {
      const page = await getEditionHistory(editionId, 30, cursor);
      if (generation.current === gen) {
        setHistory((prev) =>
          prev !== null ? { ...page, items: [...prev.items, ...page.items] } : page,
        );
        setHistoryError(null);
      }
    } catch (e) {
      if (generation.current === gen) setHistoryError(errorMessage(e));
    } finally {
      if (generation.current === gen) setHistoryLoadingMore(false);
    }
  };

  const records = edition !== null ? currentProgressRecords(edition) : [];
  const correcting = records.find((r) => r.id === progSupersedes) ?? null;
  const effectiveUnit = correcting !== null ? correcting.unit : progUnit;

  const submitProgress = async () => {
    if (edition === null) return;
    const position = progPosition.trim();
    const total = progTotal.trim();
    if (position === "") {
      setFieldError("Enter the position.");
      return;
    }
    if (progMode === "correct" && correcting === null) {
      setFieldError("Choose the record to correct.");
      return;
    }
    setFieldError(null);
    setActionError(null);
    const gen = generation.current;
    setBusy(true);
    try {
      const ed = await recordProgress({
        editionId: edition.id,
        unit: effectiveUnit,
        position,
        // An unknown total is sent absent, never carried forward.
        total: total === "" ? undefined : total,
        supersedesId: progMode === "correct" ? correcting?.id : undefined,
        expectedRevision: edition.revision,
      });
      if (generation.current !== gen) return;
      setEdition(ed);
      setProgPosition("");
      setProgTotal("");
      setProgSupersedes("");
      setProgMode("new");
      await refreshHistory(gen);
    } catch (e) {
      if (generation.current === gen) setActionError(errorMessage(e));
    } finally {
      setBusy(false);
    }
  };

  const submitStatus = async () => {
    if (edition === null) return;
    setFieldError(null);
    setActionError(null);
    const gen = generation.current;
    setBusy(true);
    try {
      const ed = await setConsumption({
        editionId: edition.id,
        value: consValue,
        reason: consReason.trim() === "" ? undefined : consReason.trim(),
        expectedRevision: edition.revision,
      });
      if (generation.current !== gen) return;
      setEdition(ed);
      setConsReason("");
      await refreshHistory(gen);
    } catch (e) {
      if (generation.current === gen) setActionError(errorMessage(e));
    } finally {
      setBusy(false);
    }
  };

  const submitRetract = async () => {
    if (edition === null || retractRecord === "") return;
    const reason = retractReason.trim();
    if (reason === "") {
      setFieldError("A retraction requires a reason.");
      return;
    }
    setFieldError(null);
    setActionError(null);
    const gen = generation.current;
    setBusy(true);
    try {
      const ed = await retractProgress({
        editionId: edition.id,
        recordId: retractRecord,
        reason,
        expectedRevision: edition.revision,
      });
      if (generation.current !== gen) return;
      setEdition(ed);
      setRetractRecord("");
      setRetractReason("");
      await refreshHistory(gen);
    } catch (e) {
      if (generation.current === gen) setActionError(errorMessage(e));
    } finally {
      setBusy(false);
    }
  };

  const submitMetadata = async () => {
    if (edition === null) return;
    const language = metaLanguage.trim();
    const other = metaOther.trim();
    const changes: { kind?: string; language?: string; otherLabel?: string } = {};
    if (metaKind !== edition.kind) changes.kind = metaKind;
    if (language !== "" && language !== edition.language) changes.language = language;
    if (other !== "" && other !== edition.other_label) changes.otherLabel = other;
    if (Object.keys(changes).length === 0) {
      setFieldError("No changes to save.");
      return;
    }
    setFieldError(null);
    setActionError(null);
    const gen = generation.current;
    setBusy(true);
    try {
      const ed = await updateEdition({
        editionId: edition.id,
        ...changes,
        expectedRevision: edition.revision,
      });
      if (generation.current !== gen) return;
      setEdition(ed);
      syncMetaFrom(ed);
      await refreshHistory(gen);
    } catch (e) {
      if (generation.current === gen) setActionError(errorMessage(e));
    } finally {
      setBusy(false);
    }
  };

  if (edition === null) {
    if (loadError !== null) {
      return (
        <section className="placeholder">
          <h2>Edition</h2>
          <div className="library-error">
            <p role="alert">{loadError}</p>
            <button type="button" onClick={() => void load()}>
              Retry
            </button>
          </div>
        </section>
      );
    }
    return (
      <section className="placeholder">
        <h2>Edition</h2>
        <p className="library-loading">Loading…</p>
      </section>
    );
  }

  const preferred = edition.titles.find((t) => t.preferred) ?? edition.titles[0];
  const publicationSource = edition.publication?.source ?? "";
  const consumptionReason = edition.consumption?.reason ?? "";
  const progressRecords = currentProgressRecords(edition);
  // Only effective rows are retractable: the projection keeps value=unit on
  // every progress row, but the service rejects retraction markers and
  // already-superseded rows.
  const retractable = (history?.items ?? []).filter(
    (item) => item.type === "progress" && item.label === "effective",
  );

  return (
    <section className="placeholder">
      <h2>
        {preferred ? preferred.text : "Unknown"}
        {edition.language !== null && (
          <span className="work-card-lang"> {edition.language}</span>
        )}
      </h2>
      <p>
        <span className="work-badge">{edition.kind}</span>
        {edition.other_label !== null && <span> · {edition.other_label}</span>}
      </p>
      <p className="login-hint">
        Publication: {edition.publication?.value ?? "unknown"}
        {publicationSource !== "" ? ` (source: ${publicationSource})` : ""}
      </p>
      <p>
        Work:{" "}
        <Link to={`/works/${encodeURIComponent(edition.work_id)}`}>
          {workTitle ?? edition.work_id}
        </Link>
      </p>

      {actionError !== null && (
        <div className="library-error">
          <p role="alert">{actionError}</p>
        </div>
      )}

      <h3>Progress</h3>
      {progressRecords.length > 0 ? (
        <ul className="library-list">
          {progressRecords.map((record) => (
            <li key={record.id} className="work-edition">
              {progressText(record)}
            </li>
          ))}
        </ul>
      ) : (
        <p className="login-hint">No progress recorded.</p>
      )}
      {fieldError !== null && (
        <p className="form-error" role="alert">
          {fieldError}
        </p>
      )}
      <form
        className="library-toolbar"
        onSubmit={(e) => {
          e.preventDefault();
          void submitProgress();
        }}
      >
        <label className="library-field">
          <span>Update kind</span>
          <select
            value={progMode}
            onChange={(e) => {
              setProgMode(e.target.value === "correct" ? "correct" : "new");
              setProgSupersedes("");
            }}
            disabled={busy}
          >
            <option value="new">record new update</option>
            <option value="correct" disabled={records.length === 0}>
              correct mistake
            </option>
          </select>
        </label>
        {progMode === "correct" ? (
          <label className="library-field">
            <span>Record to correct</span>
            <select
              value={progSupersedes}
              onChange={(e) => setProgSupersedes(e.target.value)}
              disabled={busy}
            >
              <option value="">Choose a record…</option>
              {records.map((record) => (
                <option key={record.id} value={record.id}>
                  {progressText(record)}
                </option>
              ))}
            </select>
          </label>
        ) : (
          <label className="library-field">
            <span>Unit</span>
            <select
              value={progUnit}
              onChange={(e) => setProgUnit(e.target.value)}
              disabled={busy}
            >
              {PROGRESS_UNITS.map((unit) => (
                <option key={unit} value={unit}>
                  {unit}
                </option>
              ))}
            </select>
          </label>
        )}
        <label className="library-field">
          <span>Position</span>
          <input
            type="text"
            inputMode="decimal"
            value={progPosition}
            onChange={(e) => setProgPosition(e.target.value)}
            disabled={busy}
          />
        </label>
        {effectiveUnit !== "percent" && (
          <label className="library-field">
            <span>Total</span>
            <input
              type="text"
              inputMode="decimal"
              value={progTotal}
              onChange={(e) => setProgTotal(e.target.value)}
              disabled={busy}
            />
          </label>
        )}
        <button type="submit" disabled={busy}>
          {progMode === "correct" ? "Correct mistake" : "Record update"}
        </button>
      </form>

      <h3>Status</h3>
      <p className="login-hint">
        {edition.consumption !== null
          ? `Status: ${edition.consumption.value.replace(/_/g, " ")}`
          : "Status: unknown"}
        {consumptionReason !== "" ? ` — ${consumptionReason}` : ""}
      </p>
      <form
        className="library-toolbar"
        onSubmit={(e) => {
          e.preventDefault();
          void submitStatus();
        }}
      >
        <label className="library-field">
          <span>Status</span>
          <select
            value={consValue}
            onChange={(e) => setConsValue(e.target.value)}
            disabled={busy}
          >
            {CONSUMPTION_STATUS_OPTIONS.map((value) => (
              <option key={value} value={value}>
                {value === "unknown" ? "unknown (retract state)" : value.replace(/_/g, " ")}
              </option>
            ))}
          </select>
        </label>
        <label className="library-field">
          <span>Reason</span>
          <input
            type="text"
            value={consReason}
            onChange={(e) => setConsReason(e.target.value)}
            disabled={busy}
          />
        </label>
        <button type="submit" disabled={busy}>
          Set status
        </button>
      </form>

      <h3>History</h3>
      {historyError !== null && (
        <div className="library-error">
          <p role="alert">{historyError}</p>
        </div>
      )}
      <ul className="library-list">
        {(history?.items ?? []).map((item) => (
          <li key={item.id} className="work-edition">
            <span>{historyText(item)}</span>
            <span className={`work-badge work-badge-${historyBadge(item, edition)}`}>
              {historyBadge(item, edition)}
            </span>
            {item.reason !== null && <span className="login-hint">{item.reason}</span>}
            <span className="login-hint">
              observed {formatInstant(item.observed_at)} · recorded{" "}
              {formatInstant(item.recorded_at)}
            </span>
          </li>
        ))}
        {history !== null && history.items.length === 0 && (
          <li className="work-edition">No observations yet.</li>
        )}
        {history === null && historyError === null && (
          <li className="work-edition">Loading…</li>
        )}
      </ul>
      {history?.next_cursor !== null && (
        <button
          type="button"
          className="library-more"
          disabled={historyLoadingMore || busy}
          onClick={() => void loadMoreHistory()}
        >
          {historyLoadingMore ? "Loading more…" : "Load more"}
        </button>
      )}

      <h4>Retract a progress observation</h4>
      <form
        className="library-toolbar"
        onSubmit={(e) => {
          e.preventDefault();
          void submitRetract();
        }}
      >
        <label className="library-field">
          <span>Record</span>
          <select
            value={retractRecord}
            onChange={(e) => setRetractRecord(e.target.value)}
            disabled={busy || retractable.length === 0}
          >
            <option value="">Choose a record…</option>
            {retractable.map((item) => (
              <option key={item.id} value={item.id}>
                {historyText(item)}
              </option>
            ))}
          </select>
        </label>
        <label className="library-field">
          <span>Retraction reason</span>
          <input
            type="text"
            value={retractReason}
            onChange={(e) => setRetractReason(e.target.value)}
            disabled={busy}
          />
        </label>
        <button type="submit" disabled={busy || retractable.length === 0}>
          Retract
        </button>
      </form>

      <h3>Metadata</h3>
      <form
        className="library-toolbar"
        onSubmit={(e) => {
          e.preventDefault();
          void submitMetadata();
        }}
      >
        <label className="library-field">
          <span>Kind</span>
          <select
            value={metaKind}
            onChange={(e) => setMetaKind(e.target.value)}
            disabled={busy}
          >
            {EDITION_KINDS.map((kind) => (
              <option key={kind} value={kind}>
                {kind.replace(/_/g, " ")}
              </option>
            ))}
          </select>
        </label>
        <label className="library-field">
          <span>Language</span>
          <input
            type="text"
            value={metaLanguage}
            onChange={(e) => setMetaLanguage(e.target.value)}
            disabled={busy}
          />
        </label>
        <label className="library-field">
          <span>Other label</span>
          <input
            type="text"
            value={metaOther}
            onChange={(e) => setMetaOther(e.target.value)}
            disabled={busy}
          />
        </label>
        <button type="submit" disabled={busy}>
          Save metadata
        </button>
      </form>
    </section>
  );
}
