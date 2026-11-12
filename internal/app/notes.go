package app

import (
	"context"
	"strings"

	"github.com/HaidaDaniel/media-core/internal/store"
)

// AddNoteRequest adds one new note on a work or an edition: bounded plain
// text, a spoiler flag and an optional quote/location. Adding a note never
// changes consumption, intention, publication or progress, and a later
// thought is always a distinct new note.
type AddNoteRequest struct {
	MutationBase
	TargetID string
	Text     string
	Spoiler  bool
	Quote    *string
	Location *string
}

// ReviseNoteRequest edits one existing note: a new immutable revision
// points to the immediately preceding one and keeps its text and audit.
// ExpectedRevision is the note's current revision.
type ReviseNoteRequest struct {
	MutationBase
	NoteID   string
	Text     string
	Spoiler  bool
	Quote    *string
	Location *string
}

// RetractNoteRequest withdraws one note: the retraction revision keeps the
// retracted text, its audit and an optional reason, and the note becomes
// absent from the target's view. ExpectedRevision is the note's current
// revision.
type RetractNoteRequest struct {
	MutationBase
	NoteID string
	Reason *string
}

func noteActor(req MutationBase) store.NoteActor {
	return store.NoteActor{
		Principal: req.Actor.Principal,
		Origin:    string(req.Actor.Origin),
		Identity:  req.Actor.Identity,
	}
}

// AddWorkNote adds one note to a work and returns the updated work view.
// The frozen work view carries no notes field, so the note is stored and
// queryable rather than rendered here.
func (s *Service) AddWorkNote(ctx context.Context, req AddNoteRequest) (WorkView, error) {
	if err := requireMutationBase(req.MutationBase); err != nil {
		return WorkView{}, invalidArgument(err.Error(), req.MutationBase)
	}
	if strings.TrimSpace(req.TargetID) == "" {
		return WorkView{}, invalidArgument("the target id is required", req.MutationBase)
	}
	hash := payloadHash(req)
	if entry, err := s.db.LookupJournal(ctx, req.RequestID); err != nil {
		return WorkView{}, mapStoreError(err, req.MutationBase)
	} else if entry != nil {
		return s.replayWorkView(ctx, entry, req.MutationBase, hash)
	}
	in := store.NoteInput{Text: req.Text, Spoiler: req.Spoiler, Quote: req.Quote, Location: req.Location}
	w, err := s.db.AddWorkNote(ctx, s.now(), req.TargetID, in, noteActor(req.MutationBase), &store.JournalInput{
		RequestID:   req.RequestID,
		Operation:   "add_note",
		RequestHash: hash,
		Actor:       req.Actor.Principal,
	})
	if err != nil {
		return s.settleWorkView(ctx, req.MutationBase, hash, err)
	}
	editions, err := s.db.ListEditions(ctx, w.ID)
	if err != nil {
		return WorkView{}, mapStoreError(err, req.MutationBase)
	}
	return workViewFrom(w, editions), nil
}

// AddEditionNote adds one note to an edition and returns the updated
// edition view with the note rendered at its first revision.
func (s *Service) AddEditionNote(ctx context.Context, req AddNoteRequest) (EditionView, error) {
	if err := requireMutationBase(req.MutationBase); err != nil {
		return EditionView{}, invalidArgument(err.Error(), req.MutationBase)
	}
	if strings.TrimSpace(req.TargetID) == "" {
		return EditionView{}, invalidArgument("the target id is required", req.MutationBase)
	}
	hash := payloadHash(req)
	if entry, err := s.db.LookupJournal(ctx, req.RequestID); err != nil {
		return EditionView{}, mapStoreError(err, req.MutationBase)
	} else if entry != nil {
		return s.replayEditionView(ctx, entry, req.MutationBase, hash)
	}
	in := store.NoteInput{Text: req.Text, Spoiler: req.Spoiler, Quote: req.Quote, Location: req.Location}
	e, err := s.db.AddEditionNote(ctx, s.now(), req.TargetID, in, noteActor(req.MutationBase), &store.JournalInput{
		RequestID:   req.RequestID,
		Operation:   "add_note",
		RequestHash: hash,
		Actor:       req.Actor.Principal,
	})
	if err != nil {
		return s.settleEditionView(ctx, req.MutationBase, hash, err)
	}
	return editionViewFrom(e), nil
}

// ReviseNote appends one revision to a note and returns the updated view of
// the note's target. A stale expected revision is a conflict; an exact
// retry replays.
func (s *Service) ReviseNote(ctx context.Context, req ReviseNoteRequest) (any, error) {
	if err := requireMutationBase(req.MutationBase); err != nil {
		return nil, invalidArgument(err.Error(), req.MutationBase)
	}
	if req.ExpectedRevision == nil {
		return nil, invalidArgument("expected_revision is required for existing-entity mutations", req.MutationBase)
	}
	if strings.TrimSpace(req.NoteID) == "" {
		return nil, invalidArgument("note id is required", req.MutationBase)
	}
	hash := payloadHash(req)
	if entry, err := s.db.LookupJournal(ctx, req.RequestID); err != nil {
		return nil, mapStoreError(err, req.MutationBase)
	} else if entry != nil {
		return s.replayNoteView(ctx, entry, req.MutationBase, hash)
	}
	in := store.NoteInput{Text: req.Text, Spoiler: req.Spoiler, Quote: req.Quote, Location: req.Location}
	t, err := s.db.ReviseNote(ctx, s.now(), req.NoteID, *req.ExpectedRevision, in, noteActor(req.MutationBase), &store.JournalInput{
		RequestID:   req.RequestID,
		Operation:   "revise_note",
		RequestHash: hash,
		Actor:       req.Actor.Principal,
	})
	if err != nil {
		return s.settleNoteView(ctx, req.MutationBase, hash, err)
	}
	return s.noteTargetView(ctx, t)
}

// RetractNote retracts a note and returns the updated view of the note's
// target; the note disappears from the view, its revisions stay as audit.
func (s *Service) RetractNote(ctx context.Context, req RetractNoteRequest) (any, error) {
	if err := requireMutationBase(req.MutationBase); err != nil {
		return nil, invalidArgument(err.Error(), req.MutationBase)
	}
	if req.ExpectedRevision == nil {
		return nil, invalidArgument("expected_revision is required for existing-entity mutations", req.MutationBase)
	}
	if strings.TrimSpace(req.NoteID) == "" {
		return nil, invalidArgument("note id is required", req.MutationBase)
	}
	hash := payloadHash(req)
	if entry, err := s.db.LookupJournal(ctx, req.RequestID); err != nil {
		return nil, mapStoreError(err, req.MutationBase)
	} else if entry != nil {
		return s.replayNoteView(ctx, entry, req.MutationBase, hash)
	}
	t, err := s.db.RetractNote(ctx, s.now(), req.NoteID, *req.ExpectedRevision, req.Reason, noteActor(req.MutationBase), &store.JournalInput{
		RequestID:   req.RequestID,
		Operation:   "retract_note",
		RequestHash: hash,
		Actor:       req.Actor.Principal,
	})
	if err != nil {
		return s.settleNoteView(ctx, req.MutationBase, hash, err)
	}
	return s.noteTargetView(ctx, t)
}

// noteTargetView renders the note's target as its work or edition view.
func (s *Service) noteTargetView(ctx context.Context, t store.NoteTarget) (any, error) {
	if t.IsWork {
		editions, err := s.db.ListEditions(ctx, t.Work.ID)
		if err != nil {
			return nil, mapStoreError(err, MutationBase{})
		}
		return workViewFrom(t.Work, editions), nil
	}
	return editionViewFrom(t.Edition), nil
}

// replayNoteView settles a same-key note mutation against the journal: a
// matching hash re-derives the stored target view, a mismatched hash is a
// key-collision conflict, and a corrupt row is a storage error.
func (s *Service) replayNoteView(ctx context.Context, entry *store.JournalEntry, base MutationBase, hash string) (any, error) {
	if entry.RequestHash != hash {
		return nil, conflict("the idempotency key was already used with a different payload", base)
	}
	sum, aerr := decodeEntitySummary(entry, base)
	if aerr != nil {
		return nil, aerr
	}
	if sum.WorkID != "" {
		return s.GetWork(ctx, sum.WorkID)
	}
	return s.GetEdition(ctx, sum.EditionID)
}

// settleNoteView consults the journal after a failed note mutation: a
// concurrent commit of the same key replays its stored view, a different
// payload on the same key conflicts, and anything else maps the error.
func (s *Service) settleNoteView(ctx context.Context, base MutationBase, hash string, err error) (any, error) {
	if entry, lerr := s.db.LookupJournal(ctx, base.RequestID); lerr != nil {
		return nil, mapStoreError(lerr, base)
	} else if entry != nil {
		return s.replayNoteView(ctx, entry, base, hash)
	}
	return nil, mapStoreError(err, base)
}
