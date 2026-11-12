package cli

import (
	"context"
	"fmt"
	"io"
	"strconv"

	"github.com/HaidaDaniel/media-core/internal/app"
	"github.com/HaidaDaniel/media-core/internal/domain"
	"github.com/HaidaDaniel/media-core/internal/runtime"
)

func init() {
	runtime.RegisterCommand("note", runNote)
}

const noteUsage = "usage: media-core note add work|edition <id> --text <text> [--spoiler] [--quote <text>] [--location <text>] [--request-id] | media-core note revise <note-id> --text <text> [--spoiler] [--quote <text>] [--location <text>] --revision <n> [--request-id] | media-core note retract <note-id> --revision <n> [--reason <text>] [--request-id]"

// noteArgs is one note add, revise or retract.
type noteArgs struct {
	libCommand
	verb      string
	entity    string
	target    string
	text      string
	spoiler   bool
	quote     *string
	location  *string
	reason    *string
	revision  int64
	haveRev   bool
	requestID string
}

func parseNoteArgs(args []string) (*noteArgs, error) {
	a := &noteArgs{}
	rest, err := a.parseCommon(args)
	if err != nil {
		return nil, err
	}
	if len(rest) < 2 {
		return nil, fmt.Errorf("media-core note: %s", noteUsage)
	}
	verb, target := rest[0], rest[1]
	var start int
	switch verb {
	case "add":
		if target != "work" && target != "edition" {
			return nil, fmt.Errorf("media-core note: unknown note target %q (work, edition)", target)
		}
		if len(rest) < 3 {
			return nil, fmt.Errorf("media-core note: %s", noteUsage)
		}
		a.entity, a.target = target, rest[2]
		start = 3
	case "revise", "retract":
		a.target = target
		start = 2
	default:
		return nil, fmt.Errorf("media-core note: unknown action %q (add, revise, retract)", verb)
	}
	a.verb = verb
	for i := start; i < len(rest); i++ {
		needValue := func() (string, error) {
			if i+1 >= len(rest) {
				return "", fmt.Errorf("media-core note: %s requires a value", rest[i])
			}
			i++
			return rest[i], nil
		}
		switch rest[i] {
		case "--text":
			v, err := needValue()
			if err != nil {
				return nil, err
			}
			a.text = v
		case "--spoiler":
			a.spoiler = true
		case "--quote":
			v, err := needValue()
			if err != nil {
				return nil, err
			}
			a.quote = &v
		case "--location":
			v, err := needValue()
			if err != nil {
				return nil, err
			}
			a.location = &v
		case "--reason":
			v, err := needValue()
			if err != nil {
				return nil, err
			}
			a.reason = &v
		case "--revision":
			v, err := needValue()
			if err != nil {
				return nil, err
			}
			n, err := strconv.Atoi(v)
			if err != nil {
				return nil, fmt.Errorf("media-core note: revision is not a number: %v", err)
			}
			a.revision = int64(n)
			a.haveRev = true
		case "--request-id":
			v, err := needValue()
			if err != nil {
				return nil, err
			}
			a.requestID = v
		default:
			return nil, fmt.Errorf("media-core note: unknown argument %q", rest[i])
		}
	}
	if (a.verb == "add" || a.verb == "revise") && a.text == "" {
		return nil, fmt.Errorf("media-core note: %s requires --text", a.verb)
	}
	if (a.verb == "revise" || a.verb == "retract") && !a.haveRev {
		return nil, fmt.Errorf("media-core note: %s requires --revision (the note's current revision)", a.verb)
	}
	return a, nil
}

// runNote adds, revises or retracts one plain-text note on a work or an
// edition and prints the updated view of the note's target.
func runNote(args []string, out io.Writer) error {
	a, err := parseNoteArgs(args)
	if err != nil {
		return &runtime.ExitCodeError{Exit: ExitUsage, Err: err}
	}
	svc, db, err := a.service()
	if err != nil {
		return libExitError(err)
	}
	defer db.Close()
	requestID := a.requestID
	if requestID == "" {
		id, err := domain.NewUUID()
		if err != nil {
			return &runtime.ExitCodeError{Exit: ExitError, Err: err}
		}
		requestID = id
	}
	switch a.verb {
	case "add":
		req := app.AddNoteRequest{
			MutationBase: app.MutationBase{
				RequestID: requestID,
				Actor:     app.Actor{Principal: "local", Origin: domain.OriginUser},
			},
			TargetID: a.target,
			Text:     a.text,
			Spoiler:  a.spoiler,
			Quote:    a.quote,
			Location: a.location,
		}
		var view any
		if a.entity == "work" {
			view, err = svc.AddWorkNote(context.Background(), req)
		} else {
			view, err = svc.AddEditionNote(context.Background(), req)
		}
		if err != nil {
			return writeEnvelope(out, nil, libTyped(err))
		}
		return writeEnvelope(out, view, nil)
	case "revise":
		req := app.ReviseNoteRequest{
			MutationBase: app.MutationBase{
				RequestID:        requestID,
				Actor:            app.Actor{Principal: "local", Origin: domain.OriginUser},
				ExpectedRevision: &a.revision,
			},
			NoteID:   a.target,
			Text:     a.text,
			Spoiler:  a.spoiler,
			Quote:    a.quote,
			Location: a.location,
		}
		view, err := svc.ReviseNote(context.Background(), req)
		if err != nil {
			return writeEnvelope(out, nil, libTyped(err))
		}
		return writeEnvelope(out, view, nil)
	case "retract":
		req := app.RetractNoteRequest{
			MutationBase: app.MutationBase{
				RequestID:        requestID,
				Actor:            app.Actor{Principal: "local", Origin: domain.OriginUser},
				ExpectedRevision: &a.revision,
			},
			NoteID: a.target,
			Reason: a.reason,
		}
		view, err := svc.RetractNote(context.Background(), req)
		if err != nil {
			return writeEnvelope(out, nil, libTyped(err))
		}
		return writeEnvelope(out, view, nil)
	}
	return &runtime.ExitCodeError{Exit: ExitUsage, Err: fmt.Errorf("media-core note: %s", noteUsage)}
}
