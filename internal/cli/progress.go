package cli

import (
	"context"
	"fmt"
	"io"
	"strconv"
	"time"

	"github.com/HaidaDaniel/media-core/internal/app"
	"github.com/HaidaDaniel/media-core/internal/domain"
	"github.com/HaidaDaniel/media-core/internal/runtime"
	"github.com/HaidaDaniel/media-core/internal/store"
)

func init() {
	runtime.RegisterCommand("progress", runProgress)
}

// progressArgs is one edition progress update or retraction.
type progressArgs struct {
	libCommand
	editionID    string
	unit         domain.ProgressUnit
	position     string
	total        *string
	label        *string
	reason       *string
	observedAt   *time.Time
	supersedesID *string
	recordID     string
	revision     int64
	requestID    string
}

func parseProgressArgs(args []string) (*progressArgs, error) {
	a := &progressArgs{}
	rest, err := a.parseCommon(args)
	if err != nil {
		return nil, err
	}
	if len(rest) < 3 {
		return nil, fmt.Errorf("media-core progress: usage: media-core progress set <edition-id> <flags> | media-core progress retract <edition-id> --record-id <id> --reason <text> <flags>")
	}
	verb, target := rest[0], rest[1]
	if target == "" {
		return nil, fmt.Errorf("media-core progress: an edition id is required")
	}
	a.editionID = target
	switch verb {
	case "set":
	case "retract":
	default:
		return nil, fmt.Errorf("media-core progress: unknown action %q (set, retract)", verb)
	}
	for i := 2; i < len(rest); i++ {
		needValue := func() (string, error) {
			if i+1 >= len(rest) {
				return "", fmt.Errorf("media-core progress: %s requires a value", rest[i])
			}
			i++
			return rest[i], nil
		}
		switch rest[i] {
		case "--unit":
			v, err := needValue()
			if err != nil {
				return nil, err
			}
			if !domain.ProgressUnit(v).Valid() {
				return nil, fmt.Errorf("media-core progress: unknown unit %q (chapter, volume, episode, page, percent)", v)
			}
			a.unit = domain.ProgressUnit(v)
		case "--position":
			v, err := needValue()
			if err != nil {
				return nil, err
			}
			if _, ok := store.ParseProgressDecimal(v); !ok {
				return nil, fmt.Errorf("media-core progress: invalid position %q (non-negative decimal, up to 3 fractional digits, at most 1000000000)", v)
			}
			a.position = v
		case "--total":
			v, err := needValue()
			if err != nil {
				return nil, err
			}
			if _, ok := store.ParseProgressDecimal(v); !ok {
				return nil, fmt.Errorf("media-core progress: invalid total %q (non-negative decimal, up to 3 fractional digits, at most 1000000000)", v)
			}
			a.total = &v
		case "--label":
			v, err := needValue()
			if err != nil {
				return nil, err
			}
			a.label = &v
		case "--reason":
			v, err := needValue()
			if err != nil {
				return nil, err
			}
			a.reason = &v
		case "--observed-at":
			v, err := needValue()
			if err != nil {
				return nil, err
			}
			t, err := time.Parse(time.RFC3339, v)
			if err != nil {
				return nil, fmt.Errorf("media-core progress: observed-at is not RFC3339: %v", err)
			}
			a.observedAt = &t
		case "--supersedes-id":
			v, err := needValue()
			if err != nil {
				return nil, err
			}
			a.supersedesID = &v
		case "--record-id":
			v, err := needValue()
			if err != nil {
				return nil, err
			}
			a.recordID = v
		case "--revision":
			v, err := needValue()
			if err != nil {
				return nil, err
			}
			n, err := strconv.Atoi(v)
			if err != nil {
				return nil, fmt.Errorf("media-core progress: revision is not a number: %v", err)
			}
			a.revision = int64(n)
		case "--request-id":
			v, err := needValue()
			if err != nil {
				return nil, err
			}
			a.requestID = v
		default:
			return nil, fmt.Errorf("media-core progress: unknown argument %q", rest[i])
		}
	}
	switch verb {
	case "set":
		if a.unit == "" {
			return nil, fmt.Errorf("media-core progress: set requires --unit")
		}
		if a.position == "" {
			return nil, fmt.Errorf("media-core progress: set requires --position")
		}
	case "retract":
		if a.recordID == "" {
			return nil, fmt.Errorf("media-core progress: retract requires --record-id")
		}
		if a.reason == nil || *a.reason == "" {
			return nil, fmt.Errorf("media-core progress: retract requires --reason")
		}
	}
	return a, nil
}

// runProgress records or retracts one edition progress observation and
// prints the updated edition view.
func runProgress(args []string, out io.Writer) error {
	a, err := parseProgressArgs(args)
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
	base := app.MutationBase{
		RequestID:        requestID,
		Actor:            app.Actor{Principal: "local", Origin: domain.OriginUser},
		ExpectedRevision: &a.revision,
	}
	var view app.EditionView
	switch {
	case a.unit != "":
		var observedAt time.Time
		if a.observedAt != nil {
			observedAt = *a.observedAt
		}
		view, err = svc.RecordProgress(context.Background(), app.RecordProgressRequest{
			MutationBase: base,
			EditionID:    a.editionID,
			Unit:         a.unit,
			Position:     a.position,
			Total:        a.total,
			Label:        a.label,
			Reason:       a.reason,
			ObservedAt:   observedAt,
			SupersedesID: a.supersedesID,
		})
	default:
		view, err = svc.RetractProgress(context.Background(), app.RetractProgressRequest{
			MutationBase: base,
			EditionID:    a.editionID,
			RecordID:     a.recordID,
			Reason:       *a.reason,
		})
	}
	if err != nil {
		return writeEnvelope(out, nil, libTyped(err))
	}
	return writeEnvelope(out, view, nil)
}
