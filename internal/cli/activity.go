package cli

import (
	"context"
	"fmt"
	"io"
	"strconv"

	"github.com/HaidaDaniel/media-core/internal/app"
	"github.com/HaidaDaniel/media-core/internal/runtime"
)

func init() {
	runtime.RegisterCommand("activity", runActivity)
}

const activityUsage = "usage: media-core activity recent [--type <t>] [--limit <n>] [--cursor <c>] | media-core activity history work|edition <id> [--limit <n>] [--cursor <c>]"

// activityArgs is one recent-activity or entity-history request.
type activityArgs struct {
	libCommand
	verb    string
	entity  string
	target  string
	typ     string
	limit   int
	cursor  *string
}

func parseActivityArgs(args []string) (*activityArgs, error) {
	a := &activityArgs{}
	rest, err := a.parseCommon(args)
	if err != nil {
		return nil, err
	}
	if len(rest) < 1 {
		return nil, fmt.Errorf("media-core activity: %s", activityUsage)
	}
	verb := rest[0]
	var start int
	switch verb {
	case "recent":
		start = 1
	case "history":
		if len(rest) < 3 {
			return nil, fmt.Errorf("media-core activity: %s", activityUsage)
		}
		if rest[1] != "work" && rest[1] != "edition" {
			return nil, fmt.Errorf("media-core activity: unknown history target %q (work, edition)", rest[1])
		}
		a.entity, a.target = rest[1], rest[2]
		start = 3
	default:
		return nil, fmt.Errorf("media-core activity: unknown action %q (recent, history)", verb)
	}
	a.verb = verb
	for i := start; i < len(rest); i++ {
		needValue := func() (string, error) {
			if i+1 >= len(rest) {
				return "", fmt.Errorf("media-core activity: %s requires a value", rest[i])
			}
			i++
			return rest[i], nil
		}
		switch rest[i] {
		case "--type":
			v, err := needValue()
			if err != nil {
				return nil, err
			}
			a.typ = v
		case "--limit":
			v, err := needValue()
			if err != nil {
				return nil, err
			}
			n, err := strconv.Atoi(v)
			if err != nil {
				return nil, fmt.Errorf("media-core activity: limit is not a number: %v", err)
			}
			a.limit = n
		case "--cursor":
			v, err := needValue()
			if err != nil {
				return nil, err
			}
			a.cursor = &v
		default:
			return nil, fmt.Errorf("media-core activity: unknown argument %q", rest[i])
		}
	}
	return a, nil
}

// runActivity lists the bounded recent personal activity or the complete
// history of one work or edition and prints the paginated list envelope.
func runActivity(args []string, out io.Writer) error {
	a, err := parseActivityArgs(args)
	if err != nil {
		return &runtime.ExitCodeError{Exit: ExitUsage, Err: err}
	}
	svc, db, err := a.service()
	if err != nil {
		return libExitError(err)
	}
	defer db.Close()
	switch a.verb {
	case "recent":
		var typ []app.ActivityType
		if a.typ != "" {
			typ = []app.ActivityType{app.ActivityType(a.typ)}
		}
		var cursor *string
		if a.cursor != nil {
			cursor = a.cursor
		}
		payload, err := svc.ListActivity(context.Background(), app.ActivityQuery{
			Types:  typ,
			Limit:  a.limit,
			Cursor: cursor,
		})
		if err != nil {
			return writeEnvelope(out, nil, libTyped(err))
		}
		return writeEnvelope(out, payload, nil)
	case "history":
		var workID, editionID *string
		if a.entity == "work" {
			workID = &a.target
		} else {
			editionID = &a.target
		}
		var cursor *string
		if a.cursor != nil {
			cursor = a.cursor
		}
		payload, err := svc.GetHistory(context.Background(), app.HistoryQuery{
			WorkID:    workID,
			EditionID: editionID,
			Limit:     a.limit,
			Cursor:    cursor,
		})
		if err != nil {
			return writeEnvelope(out, nil, libTyped(err))
		}
		return writeEnvelope(out, payload, nil)
	}
	return &runtime.ExitCodeError{Exit: ExitUsage, Err: fmt.Errorf("media-core activity: %s", activityUsage)}
}
