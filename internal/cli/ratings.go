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
)

func init() {
	runtime.RegisterCommand("rate", runRate)
}

const rateUsage = "usage: media-core rate <edition-id> --value <1-10> [--comment <text>] [--observed-at <RFC3339>] [--supersedes-id <id>] --revision <n> [--request-id] | media-core rate <edition-id> --clear --revision <n> [--request-id]"

// rateArgs is one edition rating update: a new 1-10 value or an explicit
// clear, with an optional comment, observation instant and correction
// target.
type rateArgs struct {
	libCommand
	edition      string
	value        *int
	clear        bool
	comment      *string
	observedAt   *time.Time
	supersedesID *string
	revision     int64
	requestID    string
}

func parseRateArgs(args []string) (*rateArgs, error) {
	a := &rateArgs{}
	rest, err := a.parseCommon(args)
	if err != nil {
		return nil, err
	}
	if len(rest) < 2 {
		return nil, fmt.Errorf("media-core rate: %s", rateUsage)
	}
	a.edition = rest[0]
	for i := 1; i < len(rest); i++ {
		needValue := func() (string, error) {
			if i+1 >= len(rest) {
				return "", fmt.Errorf("media-core rate: %s requires a value", rest[i])
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
			n, err := strconv.Atoi(v)
			if err != nil {
				return nil, fmt.Errorf("media-core rate: value must be an integer between 1 and 10: %v", err)
			}
			a.value = &n
		case "--clear":
			a.clear = true
		case "--comment":
			v, err := needValue()
			if err != nil {
				return nil, err
			}
			a.comment = &v
		case "--observed-at":
			v, err := needValue()
			if err != nil {
				return nil, err
			}
			t, err := time.Parse(time.RFC3339, v)
			if err != nil {
				return nil, fmt.Errorf("media-core rate: observed-at is not RFC3339: %v", err)
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
				return nil, fmt.Errorf("media-core rate: revision is not a number: %v", err)
			}
			a.revision = int64(n)
		case "--request-id":
			v, err := needValue()
			if err != nil {
				return nil, err
			}
			a.requestID = v
		default:
			return nil, fmt.Errorf("media-core rate: unknown argument %q", rest[i])
		}
	}
	if a.clear && a.value != nil {
		return nil, fmt.Errorf("media-core rate: --clear and --value are mutually exclusive")
	}
	if !a.clear && a.value == nil {
		return nil, fmt.Errorf("media-core rate: %s", rateUsage)
	}
	if a.clear && (a.comment != nil || a.observedAt != nil || a.supersedesID != nil) {
		return nil, fmt.Errorf("media-core rate: --clear accepts only --clear, --revision and --request-id")
	}
	return a, nil
}

// runRate sets, changes, clears or corrects an edition's personal rating
// and prints the updated edition view.
func runRate(args []string, out io.Writer) error {
	a, err := parseRateArgs(args)
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
	req := app.SetRatingRequest{
		MutationBase: app.MutationBase{
			RequestID:        requestID,
			Actor:            app.Actor{Principal: "local", Origin: domain.OriginUser},
			ExpectedRevision: &a.revision,
		},
		EditionID:    a.edition,
		Value:        a.value,
		Comment:      a.comment,
		ObservedAt:   a.observedAt,
		SupersedesID: a.supersedesID,
	}
	view, err := svc.SetRating(context.Background(), req)
	if err != nil {
		return writeEnvelope(out, nil, libTyped(err))
	}
	return writeEnvelope(out, view, nil)
}
