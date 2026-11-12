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
	runtime.RegisterCommand("tag", runTag)
}

// tagArgs is one tag assignment change on a work or an edition.
type tagArgs struct {
	libCommand
	verb      string
	target    string
	targetID  string
	tag       string
	revision  int64
	requestID string
}

func parseTagArgs(args []string) (*tagArgs, error) {
	a := &tagArgs{}
	rest, err := a.parseCommon(args)
	if err != nil {
		return nil, err
	}
	if len(rest) < 4 {
		return nil, fmt.Errorf("media-core tag: usage: media-core tag attach|detach work|edition <id> --tag <name> <flags>")
	}
	verb, target, targetID := rest[0], rest[1], rest[2]
	switch verb {
	case "attach", "detach":
	default:
		return nil, fmt.Errorf("media-core tag: unknown action %q (attach, detach)", verb)
	}
	switch target {
	case "work", "edition":
	default:
		return nil, fmt.Errorf("media-core tag: unknown target %q (work, edition)", target)
	}
	a.verb, a.target, a.targetID = verb, target, targetID
	for i := 3; i < len(rest); i++ {
		needValue := func() (string, error) {
			if i+1 >= len(rest) {
				return "", fmt.Errorf("media-core tag: %s requires a value", rest[i])
			}
			i++
			return rest[i], nil
		}
		switch rest[i] {
		case "--tag":
			v, err := needValue()
			if err != nil {
				return nil, err
			}
			a.tag = v
		case "--revision":
			v, err := needValue()
			if err != nil {
				return nil, err
			}
			n, err := strconv.Atoi(v)
			if err != nil {
				return nil, fmt.Errorf("media-core tag: revision is not a number: %v", err)
			}
			a.revision = int64(n)
		case "--request-id":
			v, err := needValue()
			if err != nil {
				return nil, err
			}
			a.requestID = v
		default:
			return nil, fmt.Errorf("media-core tag: unknown argument %q", rest[i])
		}
	}
	if a.tag == "" {
		return nil, fmt.Errorf("media-core tag: attach and detach require --tag")
	}
	return a, nil
}

// runTag attaches or detaches one tag on a work or an edition and prints
// the updated view.
func runTag(args []string, out io.Writer) error {
	a, err := parseTagArgs(args)
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
	req := app.TagRequest{
		MutationBase: app.MutationBase{
			RequestID:        requestID,
			Actor:            app.Actor{Principal: "local", Origin: domain.OriginUser},
			ExpectedRevision: &a.revision,
		},
		TargetID: a.targetID,
		Tag:      a.tag,
		Detach:   a.verb == "detach",
	}
	ctx := context.Background()
	if a.target == "work" {
		view, err := svc.SetWorkTag(ctx, req)
		if err != nil {
			return writeEnvelope(out, nil, libTyped(err))
		}
		return writeEnvelope(out, view, nil)
	}
	view, err := svc.SetEditionTag(ctx, req)
	if err != nil {
		return writeEnvelope(out, nil, libTyped(err))
	}
	return writeEnvelope(out, view, nil)
}
