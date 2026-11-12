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
	runtime.RegisterCommand("status", runStatus)
}

// statusArgs is one personal consumption status update.
type statusArgs struct {
	libCommand
	editionID    string
	value        string
	reason       *string
	observedAt   *time.Time
	supersedesID *string
	revision     int64
	requestID    string
}

func parseStatusArgs(args []string) (*statusArgs, error) {
	a := &statusArgs{}
	rest, err := a.parseCommon(args)
	if err != nil {
		return nil, err
	}
	if len(rest) < 3 {
		return nil, fmt.Errorf("media-core status: usage: media-core status set <edition-id> <flags>")
	}
	verb, target := rest[0], rest[1]
	if verb != "set" {
		return nil, fmt.Errorf("media-core status: unknown action %q (set)", verb)
	}
	a.editionID = target
	for i := 2; i < len(rest); i++ {
		needValue := func() (string, error) {
			if i+1 >= len(rest) {
				return "", fmt.Errorf("media-core status: %s requires a value", rest[i])
			}
			i++
			return rest[i], nil
		}
		switch rest[i] {
		case "--value":
			v, err := needValue()
			if err != nil {
				return nil, err
			}
			if !store.ValidConsumptionValue(v) {
				return nil, fmt.Errorf("media-core status: unknown consumption value %q (not_started, in_progress, paused, completed, dropped, unknown)", v)
			}
			a.value = v
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
				return nil, fmt.Errorf("media-core status: observed-at is not RFC3339: %v", err)
			}
			a.observedAt = &t
		case "--supersedes-id":
			v, err := needValue()
			if err != nil {
				return nil, err
			}
			a.supersedesID = &v
		case "--revision":
			v, err := needValue()
			if err != nil {
				return nil, err
			}
			n, err := strconv.Atoi(v)
			if err != nil {
				return nil, fmt.Errorf("media-core status: revision is not a number: %v", err)
			}
			a.revision = int64(n)
		case "--request-id":
			v, err := needValue()
			if err != nil {
				return nil, err
			}
			a.requestID = v
		default:
			return nil, fmt.Errorf("media-core status: unknown argument %q", rest[i])
		}
	}
	if a.value == "" {
		return nil, fmt.Errorf("media-core status: set requires --value")
	}
	return a, nil
}

// runStatus records one personal consumption observation for an edition and
// prints the updated edition view.
func runStatus(args []string, out io.Writer) error {
	a, err := parseStatusArgs(args)
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
	req := app.SetConsumptionRequest{
		MutationBase: app.MutationBase{
			RequestID:        requestID,
			Actor:            app.Actor{Principal: "local", Origin: domain.OriginUser},
			ExpectedRevision: &a.revision,
		},
		EditionID:    a.editionID,
		Value:        a.value,
		Reason:       a.reason,
		ObservedAt:   a.observedAt,
		SupersedesID: a.supersedesID,
	}
	view, err := svc.SetConsumption(context.Background(), req)
	if err != nil {
		return writeEnvelope(out, nil, libTyped(err))
	}
	return writeEnvelope(out, view, nil)
}
