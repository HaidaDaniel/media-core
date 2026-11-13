// The compact reference panel: the edition's active sourced references with
// their role, provider, external ID or URL, the explicit authoritative
// assertion and provenance, an explicit retire action, and the add form.
// Links are rendered only for http(s) URLs (the stored URL is normalized
// without userinfo, so it carries no credentials) with noopener/noreferrer;
// links are never fetched by the app. Identity conflicts and validation
// failures come from the server and keep the typed input beside a
// distinguishable error.
import { useState } from "react";
import type { JSX } from "react";
import { ApiError } from "../api/client";
import {
  addReference,
  retireReference,
  REFERENCE_ROLES,
  type AnnotatedEditionView,
  type ExternalReferenceWire,
} from "./annotationsApi";

interface ReferencesPanelProps {
  edition: AnnotatedEditionView;
  // Applies the server's updated edition view (and refreshes history).
  onUpdated: (view: AnnotatedEditionView) => void;
}

function errorMessage(e: unknown): string {
  if (e instanceof ApiError) return e.message;
  if (e instanceof Error) return e.message;
  return "Something went wrong";
}

// One safe external link: only an absolute http(s) URL is linked, and the
// stored URL never carries userinfo, so no credential ever reaches an href.
function ReferenceLink({ reference }: { reference: ExternalReferenceWire }): JSX.Element {
  const url = reference.url;
  if (url !== null && (url.startsWith("https://") || url.startsWith("http://"))) {
    return (
      <a href={url} target="_blank" rel="noopener noreferrer">
        {url}
      </a>
    );
  }
  return <span>{url ?? (reference.external_id ?? "")}</span>;
}

export function ReferencesPanel({ edition, onUpdated }: ReferencesPanelProps): JSX.Element {
  const [provider, setProvider] = useState("");
  const [externalId, setExternalId] = useState("");
  const [url, setUrl] = useState("");
  const [role, setRole] = useState<string>(REFERENCE_ROLES[0]);
  const [authoritative, setAuthoritative] = useState(false);
  const [language, setLanguage] = useState("");
  const [provenance, setProvenance] = useState("");
  const [retireId, setRetireId] = useState("");
  const [retireReason, setRetireReason] = useState("");
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<string | null>(null);

  const add = async () => {
    const trimmedId = externalId.trim();
    const trimmedUrl = url.trim();
    if (trimmedId === "" && trimmedUrl === "") {
      setError("Provide an external ID or a URL.");
      return;
    }
    if (trimmedId !== "" && trimmedUrl !== "") {
      setError("Provide exactly one of external ID or URL.");
      return;
    }
    setBusy(true);
    setError(null);
    try {
      const view = await addReference({
        editionId: edition.id,
        provider: provider.trim(),
        externalId: trimmedId === "" ? undefined : trimmedId,
        url: trimmedUrl === "" ? undefined : trimmedUrl,
        role,
        authoritative,
        language: language.trim() === "" ? undefined : language.trim(),
        provenance: provenance.trim(),
        expectedRevision: edition.revision,
      });
      setProvider("");
      setExternalId("");
      setUrl("");
      setRole(REFERENCE_ROLES[0]);
      setAuthoritative(false);
      setLanguage("");
      setProvenance("");
      onUpdated(view);
    } catch (e) {
      setError(errorMessage(e));
    } finally {
      setBusy(false);
    }
  };

  const retire = async () => {
    if (retireId === "") {
      setError("Choose the reference to retire.");
      return;
    }
    setBusy(true);
    setError(null);
    try {
      const view = await retireReference({
        editionId: edition.id,
        referenceId: retireId,
        reason: retireReason.trim() === "" ? undefined : retireReason.trim(),
        expectedRevision: edition.revision,
      });
      setRetireId("");
      setRetireReason("");
      onUpdated(view);
    } catch (e) {
      setError(errorMessage(e));
    } finally {
      setBusy(false);
    }
  };

  return (
    <div>
      {edition.references.length > 0 ? (
        <ul className="library-list">
          {edition.references.map((reference) => (
            <li key={reference.id} className="work-edition">
              <span className="work-badge">{reference.role.replace(/_/g, " ")}</span>
              <span>{reference.provider}</span>
              <ReferenceLink reference={reference} />
              {reference.authoritative && <span className="work-badge">authoritative</span>}
              {reference.language !== null && <span>{reference.language}</span>}
              <span className="login-hint">{reference.provenance}</span>
            </li>
          ))}
        </ul>
      ) : (
        <p className="login-hint">No references.</p>
      )}
      {error !== null && (
        <div className="library-error">
          <p role="alert">{error}</p>
        </div>
      )}
      <form
        className="library-toolbar"
        onSubmit={(e) => {
          e.preventDefault();
          void add();
        }}
      >
        <label className="library-field">
          <span>Provider</span>
          <input
            type="text"
            value={provider}
            onChange={(e) => setProvider(e.target.value)}
            disabled={busy}
          />
        </label>
        <label className="library-field">
          <span>External ID</span>
          <input
            type="text"
            value={externalId}
            onChange={(e) => setExternalId(e.target.value)}
            disabled={busy}
          />
        </label>
        <label className="library-field">
          <span>URL</span>
          <input
            type="url"
            value={url}
            onChange={(e) => setUrl(e.target.value)}
            disabled={busy}
          />
        </label>
        <label className="library-field">
          <span>Reference role</span>
          <select value={role} onChange={(e) => setRole(e.target.value)} disabled={busy}>
            {REFERENCE_ROLES.map((r) => (
              <option key={r} value={r}>
                {r.replace(/_/g, " ")}
              </option>
            ))}
          </select>
        </label>
        <label className="library-field">
          <span>Reference language</span>
          <input
            type="text"
            value={language}
            onChange={(e) => setLanguage(e.target.value)}
            disabled={busy}
          />
        </label>
        <label className="library-field">
          <span>Reference provenance</span>
          <input
            type="text"
            value={provenance}
            onChange={(e) => setProvenance(e.target.value)}
            disabled={busy}
          />
        </label>
        <label className="library-field">
          <span>Authoritative</span>
          <input
            type="checkbox"
            checked={authoritative}
            onChange={(e) => setAuthoritative(e.target.checked)}
            disabled={busy}
          />
        </label>
        <button type="submit" disabled={busy}>
          Add reference
        </button>
      </form>
      {edition.references.length > 0 && (
        <form
          className="library-toolbar"
          onSubmit={(e) => {
            e.preventDefault();
            void retire();
          }}
        >
          <label className="library-field">
            <span>Reference</span>
            <select
              value={retireId}
              onChange={(e) => setRetireId(e.target.value)}
              disabled={busy}
            >
              <option value="">Choose a reference…</option>
              {edition.references.map((reference) => (
                <option key={reference.id} value={reference.id}>
                  {reference.provider} · {reference.role}
                </option>
              ))}
            </select>
          </label>
          <label className="library-field">
            <span>Retirement reason</span>
            <input
              type="text"
              value={retireReason}
              onChange={(e) => setRetireReason(e.target.value)}
              disabled={busy}
            />
          </label>
          <button type="submit" disabled={busy}>
            Retire reference
          </button>
        </form>
      )}
    </div>
  );
}
