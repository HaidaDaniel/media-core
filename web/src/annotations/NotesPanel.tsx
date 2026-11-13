// The compact notes panel: the edition's plain-text notes (rendered escaped,
// never as HTML) with their spoiler flag, quote, location and revision, one
// shared form with the explicit add, edit and retract actions, and the
// work-scope note selection on add. An edit appends an immutable revision
// guarded by the note's own revision; a retraction keeps the retracted text
// in history. A later thought is always a distinct new note. Work-scoped
// notes are stored but not listed on this screen (the work view carries no
// notes field), so their add reports success without fabricating a listing.
import { useState } from "react";
import type { JSX } from "react";
import { ApiError } from "../api/client";
import {
  addNote,
  reviseNote,
  retractNote,
  type AnnotatedEditionView,
  type NoteWire,
} from "./annotationsApi";

type NoteAction = "add" | "edit" | "retract";

interface NotesPanelProps {
  edition: AnnotatedEditionView;
  // Applies the server's updated edition view (and refreshes history).
  onUpdated: (view: AnnotatedEditionView) => void;
}

function errorMessage(e: unknown): string {
  if (e instanceof ApiError) return e.message;
  if (e instanceof Error) return e.message;
  return "Something went wrong";
}

function formatInstant(iso: string): string {
  const date = new Date(iso);
  return Number.isNaN(date.getTime()) ? iso : date.toLocaleString();
}

export function NotesPanel({ edition, onUpdated }: NotesPanelProps): JSX.Element {
  const [action, setAction] = useState<NoteAction>("add");
  const [target, setTarget] = useState<"edition" | "work">("edition");
  const [noteId, setNoteId] = useState("");
  const [text, setText] = useState("");
  const [spoiler, setSpoiler] = useState(false);
  const [quote, setQuote] = useState("");
  const [location, setLocation] = useState("");
  const [reason, setReason] = useState("");
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [savedToWork, setSavedToWork] = useState(false);

  const editing = edition.notes.find((n) => n.id === noteId) ?? null;

  // startEdit seeds the shared form from the note; the input is the note's
  // current text, so a failed save keeps it for the retry.
  const startEdit = (note: NoteWire) => {
    setAction("edit");
    setNoteId(note.id);
    setText(note.text);
    setSpoiler(note.spoiler);
    setQuote(note.quote ?? "");
    setLocation(note.location ?? "");
    setError(null);
    setSavedToWork(false);
  };

  const startRetract = (note: NoteWire) => {
    setAction("retract");
    setNoteId(note.id);
    setReason("");
    setError(null);
    setSavedToWork(false);
  };

  const startAdd = () => {
    setAction("add");
    setNoteId("");
    setText("");
    setSpoiler(false);
    setQuote("");
    setLocation("");
    setError(null);
    setSavedToWork(false);
  };

  const submit = async () => {
    const trimmedText = text.trim();
    const trimmedQuote = quote.trim();
    const trimmedLocation = location.trim();
    if ((action === "add" || action === "edit") && trimmedText === "") {
      setError("Enter the note text.");
      return;
    }
    if (action !== "add" && editing === null) {
      setError("Choose the note.");
      return;
    }
    setBusy(true);
    setError(null);
    setSavedToWork(false);
    try {
      if (action === "add") {
        const content = {
          text: trimmedText,
          spoiler,
          quote: trimmedQuote === "" ? undefined : trimmedQuote,
          location: trimmedLocation === "" ? undefined : trimmedLocation,
        };
        if (target === "edition") {
          const view = await addNote({ target: "edition", targetId: edition.id, ...content });
          onUpdated(view);
        } else {
          await addNote({ target: "work", targetId: edition.work_id, ...content });
          setSavedToWork(true);
        }
        setText("");
        setSpoiler(false);
        setQuote("");
        setLocation("");
      } else if (action === "edit" && editing !== null) {
        const view = await reviseNote({
          noteId: editing.id,
          expectedRevision: editing.revision,
          text: trimmedText,
          spoiler,
          quote: trimmedQuote === "" ? undefined : trimmedQuote,
          location: trimmedLocation === "" ? undefined : trimmedLocation,
        });
        setAction("add");
        setNoteId("");
        onUpdated(view as AnnotatedEditionView);
      } else if (action === "retract" && editing !== null) {
        const view = await retractNote({
          noteId: editing.id,
          expectedRevision: editing.revision,
          reason: reason.trim() === "" ? undefined : reason.trim(),
        });
        setAction("add");
        setNoteId("");
        setReason("");
        onUpdated(view as AnnotatedEditionView);
      }
    } catch (e) {
      // The typed input is retained for the retry; the prior notes stay
      // intact beside the distinguishable error.
      setError(errorMessage(e));
    } finally {
      setBusy(false);
    }
  };

  return (
    <div>
      {edition.notes.length > 0 ? (
        <ul className="library-list">
          {edition.notes.map((note) => (
            <li key={note.id} className="work-edition">
              {note.spoiler && <span className="work-badge">spoiler</span>}
              <span>{note.text}</span>
              {note.quote !== null && <span className="login-hint">“{note.quote}”</span>}
              {note.location !== null && <span className="login-hint">({note.location})</span>}
              <span className="login-hint">
                revision {note.revision} · {formatInstant(note.recorded_at)}
              </span>
              <button
                type="button"
                className="library-more"
                disabled={busy}
                onClick={() => startEdit(note)}
              >
                Edit note {note.revision}
              </button>
              <button
                type="button"
                className="library-more"
                disabled={busy}
                onClick={() => startRetract(note)}
              >
                Retract note {note.revision}
              </button>
            </li>
          ))}
        </ul>
      ) : (
        <p className="login-hint">No notes on this edition.</p>
      )}
      {error !== null && (
        <div className="library-error">
          <p role="alert">{error}</p>
        </div>
      )}
      {savedToWork && (
        <p className="login-hint" role="status">
          Note saved to the work; work-scoped notes are not listed on this screen.
        </p>
      )}
      <form
        className="library-toolbar"
        onSubmit={(e) => {
          e.preventDefault();
          void submit();
        }}
      >
        <label className="library-field">
          <span>Note action</span>
          <select
            value={action}
            onChange={(e) => {
              const next = e.target.value as NoteAction;
              if (next === "add") {
                startAdd();
              } else {
                setAction(next);
              }
            }}
            disabled={busy}
          >
            <option value="add">add note</option>
            <option value="edit" disabled={edition.notes.length === 0}>
              edit note
            </option>
            <option value="retract" disabled={edition.notes.length === 0}>
              retract note
            </option>
          </select>
        </label>
        {action === "add" ? (
          <label className="library-field">
            <span>Note target</span>
            <select
              value={target}
              onChange={(e) => setTarget(e.target.value === "work" ? "work" : "edition")}
              disabled={busy}
            >
              <option value="edition">this edition</option>
              <option value="work">the work</option>
            </select>
          </label>
        ) : (
          <label className="library-field">
            <span>Note</span>
            <select
              value={noteId}
              onChange={(e) => setNoteId(e.target.value)}
              disabled={busy}
            >
              <option value="">Choose a note…</option>
              {edition.notes.map((note) => (
                <option key={note.id} value={note.id}>
                  {note.text.length > 40 ? `${note.text.slice(0, 40)}…` : note.text}
                </option>
              ))}
            </select>
          </label>
        )}
        {action !== "retract" && (
          <>
            <label className="library-field">
              <span>Note text</span>
              <textarea
                value={text}
                onChange={(e) => setText(e.target.value)}
                disabled={busy}
                rows={2}
              />
            </label>
            <label className="library-field">
              <span>Spoiler</span>
              <input
                type="checkbox"
                checked={spoiler}
                onChange={(e) => setSpoiler(e.target.checked)}
                disabled={busy}
              />
            </label>
            <label className="library-field">
              <span>Note quote</span>
              <input
                type="text"
                value={quote}
                onChange={(e) => setQuote(e.target.value)}
                disabled={busy}
              />
            </label>
            <label className="library-field">
              <span>Note location</span>
              <input
                type="text"
                value={location}
                onChange={(e) => setLocation(e.target.value)}
                disabled={busy}
              />
            </label>
          </>
        )}
        {action === "retract" && (
          <label className="library-field">
            <span>Note retraction reason</span>
            <input
              type="text"
              value={reason}
              onChange={(e) => setReason(e.target.value)}
              disabled={busy}
            />
          </label>
        )}
        <button type="submit" disabled={busy}>
          {action === "add" ? "Add note" : action === "edit" ? "Save revision" : "Retract note"}
        </button>
      </form>
    </div>
  );
}
