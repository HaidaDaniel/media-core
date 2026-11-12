package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/HaidaDaniel/media-core/internal/domain"
)

// Note failures. Application services map these to the stable wire error
// codes; the store stays transport-agnostic.
var (
	ErrInvalidNote  = errors.New("store: invalid note")
	ErrNoteNotFound = errors.New("store: note not found")
)

// MaxNoteTextBytes bounds note text: 16 KiB of UTF-8 bytes per the
// architecture limit. Note text is plain text only; it is stored verbatim
// and rendered escaped by the adapters, never executed as HTML.
const MaxNoteTextBytes = 16 * 1024

// NoteActor is the acting identity recorded on a note revision. It is
// stored as canonical JSON in the actor column.
type NoteActor struct {
	Principal string `json:"principal"`
	Origin    string `json:"origin,omitempty"`
	Identity  string `json:"identity,omitempty"`
}

// NoteInput is one requested note revision: bounded plain text, a spoiler
// flag and an optional quote/location.
type NoteInput struct {
	Text     string
	Spoiler  bool
	Quote    *string
	Location *string
}

// NoteInfo is one active note on a target: the note's latest non-retracted
// revision.
type NoteInfo struct {
	ID         string
	TargetID   string
	Text       string
	Spoiler    bool
	Quote      *string
	Location   *string
	Revision   int64
	RecordedAt time.Time
	Actor      NoteActor
}

// NoteTarget is the target entity of one note: exactly one of Work or
// Edition is set.
type NoteTarget struct {
	IsWork  bool
	Work    Work
	Edition Edition
}

// validateNoteInput checks the note text and optional location facts before
// any journal key is reserved: blank or oversized text, and a blank quote
// or location, are distinct invalid-note failures.
func validateNoteInput(in NoteInput) error {
	if strings.TrimSpace(in.Text) == "" {
		return fmt.Errorf("%w: text is empty", ErrInvalidNote)
	}
	if len(in.Text) > MaxNoteTextBytes {
		return fmt.Errorf("%w: text exceeds %d bytes", ErrInvalidNote, MaxNoteTextBytes)
	}
	if in.Quote != nil && strings.TrimSpace(*in.Quote) == "" {
		return fmt.Errorf("%w: quote is empty", ErrInvalidNote)
	}
	if in.Location != nil && strings.TrimSpace(*in.Location) == "" {
		return fmt.Errorf("%w: location is empty", ErrInvalidNote)
	}
	return nil
}

// noteTargetResultJSON is the journal summary of a note mutation: the
// replay re-derives the current view of the note's target.
func noteTargetResultJSON(workID, editionID sql.NullString, revision int64) string {
	if workID.Valid {
		return entityResultJSON("work_id", workID.String, revision)
	}
	return entityResultJSON("edition_id", editionID.String, revision)
}

// readNoteTx reads the note row inside the caller's transaction.
func readNoteTx(ctx context.Context, tx *sql.Tx, noteID string) (workID, editionID sql.NullString, revision int64, err error) {
	err = tx.QueryRowContext(ctx,
		`SELECT work_id, edition_id, revision FROM notes WHERE id = ?`, noteID).
		Scan(&workID, &editionID, &revision)
	if errors.Is(err, sql.ErrNoRows) {
		return sql.NullString{}, sql.NullString{}, 0, fmt.Errorf("%w: %s", ErrNoteNotFound, noteID)
	}
	return workID, editionID, revision, err
}

// lastNoteRevisionTx reads the note's latest revision row, whose revision
// number equals the note's current revision.
func lastNoteRevisionTx(ctx context.Context, tx *sql.Tx, noteID string, revision int64) (id string, retracted, spoiler bool, text string, quote, location sql.NullString, err error) {
	var retractedInt, spoilerInt int
	err = tx.QueryRowContext(ctx,
		`SELECT id, retracted, text, spoiler, quote, location FROM note_revisions
		 WHERE note_id = ? AND revision = ?`, noteID, revision).
		Scan(&id, &retractedInt, &text, &spoilerInt, &quote, &location)
	if err != nil {
		return "", false, false, "", sql.NullString{}, sql.NullString{},
			fmt.Errorf("store: read note revision: %w", err)
	}
	return id, retractedInt == 1, spoilerInt == 1, text, quote, location, nil
}

// bumpNoteRevisionTx advances the note revision; a concurrent writer is a
// distinct revision mismatch.
func bumpNoteRevisionTx(ctx context.Context, tx *sql.Tx, noteID string, revision int64) error {
	res, err := tx.ExecContext(ctx,
		`UPDATE notes SET revision = revision + 1 WHERE id = ? AND revision = ?`, noteID, revision)
	if err != nil {
		return fmt.Errorf("store: bump note revision: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("store: bump note revision: %w", err)
	}
	if n != 1 {
		return fmt.Errorf("%w: expected %d, but the note was updated concurrently", ErrRevisionMismatch, revision)
	}
	return nil
}

func (d *Database) noteTarget(ctx context.Context, workID, editionID sql.NullString) (NoteTarget, error) {
	if workID.Valid {
		w, err := d.GetWork(ctx, workID.String)
		return NoteTarget{IsWork: true, Work: w}, err
	}
	e, err := d.GetEdition(ctx, editionID.String)
	return NoteTarget{Edition: e}, err
}

// addNoteTx creates one note and its first revision inside the caller's
// transaction. The target must exist; the target revision is not guarded
// or bumped: a later thought is always a distinct new note.
func (d *Database) addNoteTx(ctx context.Context, tx *sql.Tx, now time.Time, column, targetID string, in NoteInput, actor NoteActor) (noteID string, err error) {
	noteID, err = domain.NewUUID()
	if err != nil {
		return "", err
	}
	revID, err := domain.NewUUID()
	if err != nil {
		return "", err
	}
	actorJSON, err := json.Marshal(actor)
	if err != nil {
		return "", fmt.Errorf("store: encode note actor: %w", err)
	}
	nowS := instantUTC(now)
	var exists string
	if err := tx.QueryRowContext(ctx,
		`SELECT id FROM `+noteTargetTable(column)+` WHERE id = ?`, targetID).Scan(&exists); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return "", noteTargetNotFound(column, targetID)
		}
		return "", fmt.Errorf("store: check note target: %w", err)
	}
	// The non-target column stays SQL NULL so the exactly-one-target
	// check holds.
	var workID, editionID any
	if column == "work_id" {
		workID = targetID
	} else {
		editionID = targetID
	}
	if _, err := tx.ExecContext(ctx,
		`INSERT INTO notes (id, work_id, edition_id, revision, created_at)
		 VALUES (?, ?, ?, 1, ?)`,
		noteID, workID, editionID, nowS); err != nil {
		return "", fmt.Errorf("store: insert note: %w", err)
	}
	if _, err := tx.ExecContext(ctx,
		`INSERT INTO note_revisions (id, note_id, revision, prev_id, retracted, text, spoiler, quote, location, reason, recorded_at, actor)
		 VALUES (?, ?, 1, NULL, 0, ?, ?, ?, ?, NULL, ?, ?)`,
		revID, noteID, in.Text, boolInt(in.Spoiler), in.Quote, in.Location, nowS, string(actorJSON)); err != nil {
		return "", fmt.Errorf("store: insert note revision: %w", err)
	}
	return noteID, nil
}

func noteTargetTable(column string) string {
	if column == "work_id" {
		return "works"
	}
	return "editions"
}

func noteTargetNotFound(column, targetID string) error {
	if column == "work_id" {
		return fmt.Errorf("%w: %s", ErrWorkNotFound, targetID)
	}
	return fmt.Errorf("%w: %s", ErrEditionNotFound, targetID)
}

// AddWorkNote adds one new note to a work: a note row plus its first
// immutable revision, with the optional journal row in the same
// transaction. The work revision is not bumped.
func (d *Database) AddWorkNote(ctx context.Context, now time.Time, workID string, in NoteInput, actor NoteActor, journal *JournalInput) (Work, error) {
	if err := validateNoteInput(in); err != nil {
		return Work{}, err
	}
	tx, err := d.beginImmediate(ctx)
	if err != nil {
		return Work{}, err
	}
	defer tx.Rollback()
	if _, err := d.addNoteTx(ctx, tx, now, "work_id", workID, in, actor); err != nil {
		return Work{}, err
	}
	if err := recordJournalOr(tx, ctx, journal, now, entityResultJSON("work_id", workID, 1)); err != nil {
		return Work{}, err
	}
	if err := tx.Commit(); err != nil {
		return Work{}, fmt.Errorf("store: commit work note: %w", err)
	}
	return d.GetWork(ctx, workID)
}

// AddEditionNote adds one new note to an edition with the same rules as
// AddWorkNote; the edition revision is not bumped.
func (d *Database) AddEditionNote(ctx context.Context, now time.Time, editionID string, in NoteInput, actor NoteActor, journal *JournalInput) (Edition, error) {
	if err := validateNoteInput(in); err != nil {
		return Edition{}, err
	}
	tx, err := d.beginImmediate(ctx)
	if err != nil {
		return Edition{}, err
	}
	defer tx.Rollback()
	if _, err := d.addNoteTx(ctx, tx, now, "edition_id", editionID, in, actor); err != nil {
		return Edition{}, err
	}
	if err := recordJournalOr(tx, ctx, journal, now, entityResultJSON("edition_id", editionID, 1)); err != nil {
		return Edition{}, err
	}
	if err := tx.Commit(); err != nil {
		return Edition{}, fmt.Errorf("store: commit edition note: %w", err)
	}
	return d.GetEdition(ctx, editionID)
}

// ReviseNote appends one new revision to an existing note: the expected
// revision must match the note's current revision, and the new revision
// points to the immediately preceding one, which keeps its text and audit.
// A revised note is active again even when it was retracted. The note
// revision guards against concurrent overwrites; the target entity's
// revision is not bumped.
func (d *Database) ReviseNote(ctx context.Context, now time.Time, noteID string, expectedRevision int64, in NoteInput, actor NoteActor, journal *JournalInput) (NoteTarget, error) {
	if err := validateNoteInput(in); err != nil {
		return NoteTarget{}, err
	}
	revID, err := domain.NewUUID()
	if err != nil {
		return NoteTarget{}, err
	}
	actorJSON, err := json.Marshal(actor)
	if err != nil {
		return NoteTarget{}, fmt.Errorf("store: encode note actor: %w", err)
	}
	tx, err := d.beginImmediate(ctx)
	if err != nil {
		return NoteTarget{}, err
	}
	defer tx.Rollback()
	workID, editionID, revision, err := readNoteTx(ctx, tx, noteID)
	if err != nil {
		return NoteTarget{}, err
	}
	if revision != expectedRevision {
		return NoteTarget{}, fmt.Errorf("%w: expected %d, current %d", ErrRevisionMismatch, expectedRevision, revision)
	}
	lastID, _, _, _, _, _, err := lastNoteRevisionTx(ctx, tx, noteID, revision)
	if err != nil {
		return NoteTarget{}, err
	}
	if _, err := tx.ExecContext(ctx,
		`INSERT INTO note_revisions (id, note_id, revision, prev_id, retracted, text, spoiler, quote, location, reason, recorded_at, actor)
		 VALUES (?, ?, ?, ?, 0, ?, ?, ?, ?, NULL, ?, ?)`,
		revID, noteID, revision+1, lastID, in.Text, boolInt(in.Spoiler), in.Quote, in.Location, instantUTC(now), string(actorJSON)); err != nil {
		return NoteTarget{}, fmt.Errorf("store: insert note revision: %w", err)
	}
	if err := bumpNoteRevisionTx(ctx, tx, noteID, revision); err != nil {
		return NoteTarget{}, err
	}
	if err := recordJournalOr(tx, ctx, journal, now, noteTargetResultJSON(workID, editionID, revision+1)); err != nil {
		return NoteTarget{}, err
	}
	if err := tx.Commit(); err != nil {
		return NoteTarget{}, fmt.Errorf("store: commit note revision: %w", err)
	}
	return d.noteTarget(ctx, workID, editionID)
}

// RetractNote appends one retraction revision to an existing note: the
// retraction keeps the retracted text, its audit and an optional reason,
// and the note becomes absent from the target's view. An already-retracted
// note is a no-op without a revision bump, but the journal still settles so
// exact retries replay.
func (d *Database) RetractNote(ctx context.Context, now time.Time, noteID string, expectedRevision int64, reason *string, actor NoteActor, journal *JournalInput) (NoteTarget, error) {
	tx, err := d.beginImmediate(ctx)
	if err != nil {
		return NoteTarget{}, err
	}
	defer tx.Rollback()
	workID, editionID, revision, err := readNoteTx(ctx, tx, noteID)
	if err != nil {
		return NoteTarget{}, err
	}
	if revision != expectedRevision {
		return NoteTarget{}, fmt.Errorf("%w: expected %d, current %d", ErrRevisionMismatch, expectedRevision, revision)
	}
	lastID, retracted, spoiler, text, quote, location, err := lastNoteRevisionTx(ctx, tx, noteID, revision)
	if err != nil {
		return NoteTarget{}, err
	}
	if !retracted {
		revID, err := domain.NewUUID()
		if err != nil {
			return NoteTarget{}, err
		}
		actorJSON, err := json.Marshal(actor)
		if err != nil {
			return NoteTarget{}, fmt.Errorf("store: encode note actor: %w", err)
		}
		if _, err := tx.ExecContext(ctx,
			`INSERT INTO note_revisions (id, note_id, revision, prev_id, retracted, text, spoiler, quote, location, reason, recorded_at, actor)
			 VALUES (?, ?, ?, ?, 1, ?, ?, ?, ?, ?, ?, ?)`,
			revID, noteID, revision+1, lastID, text, boolInt(spoiler), quote, location, reason, instantUTC(now), string(actorJSON)); err != nil {
			return NoteTarget{}, fmt.Errorf("store: insert note retraction: %w", err)
		}
		if err := bumpNoteRevisionTx(ctx, tx, noteID, revision); err != nil {
			return NoteTarget{}, err
		}
	}
	if err := recordJournalOr(tx, ctx, journal, now, noteTargetResultJSON(workID, editionID, revision)); err != nil {
		return NoteTarget{}, err
	}
	if err := tx.Commit(); err != nil {
		return NoteTarget{}, fmt.Errorf("store: commit note retraction: %w", err)
	}
	return d.noteTarget(ctx, workID, editionID)
}

// currentNotes lists one target's active notes: every note whose latest
// revision is not a retraction, each shown at its latest revision, in
// creation order. Retracted notes are absent from the view, never deleted.
func (d *Database) currentNotes(ctx context.Context, q queryer, column, targetID string) ([]NoteInfo, error) {
	rows, err := q.QueryContext(ctx,
		`SELECT n.id, n.`+column+`, nr.text, nr.spoiler, nr.quote, nr.location, nr.revision, nr.recorded_at, nr.actor
		 FROM notes n
		 JOIN note_revisions nr ON nr.note_id = n.id AND nr.revision = n.revision
		 WHERE n.`+column+` = ? AND nr.retracted = 0
		 ORDER BY n.created_at, n.id`, targetID)
	if err != nil {
		return nil, fmt.Errorf("store: query notes: %w", err)
	}
	defer rows.Close()
	notes := []NoteInfo{}
	for rows.Next() {
		var (
			n          NoteInfo
			target     string
			spoiler    int
			quote      sql.NullString
			location   sql.NullString
			recordedAt string
			actor      string
		)
		if err := rows.Scan(&n.ID, &target, &n.Text, &spoiler, &quote, &location, &n.Revision, &recordedAt, &actor); err != nil {
			return nil, fmt.Errorf("store: scan note: %w", err)
		}
		n.TargetID = target
		n.Spoiler = spoiler == 1
		if quote.Valid {
			n.Quote = &quote.String
		}
		if location.Valid {
			n.Location = &location.String
		}
		if err := json.Unmarshal([]byte(actor), &n.Actor); err != nil {
			return nil, fmt.Errorf("store: parse note actor: %w", err)
		}
		n.RecordedAt, err = parseInstant(recordedAt)
		if err != nil {
			return nil, fmt.Errorf("store: parse note recorded_at: %w", err)
		}
		notes = append(notes, n)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return notes, nil
}

// ListEditionNotes lists the edition's active notes.
func (d *Database) ListEditionNotes(ctx context.Context, editionID string) ([]NoteInfo, error) {
	return d.currentNotes(ctx, d.db, "edition_id", editionID)
}

// ListWorkNotes lists the work's active notes.
func (d *Database) ListWorkNotes(ctx context.Context, workID string) ([]NoteInfo, error) {
	return d.currentNotes(ctx, d.db, "work_id", workID)
}
