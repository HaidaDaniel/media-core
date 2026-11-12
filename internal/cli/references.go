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
	runtime.RegisterCommand("reference", runReference)
}

const referenceUsage = "usage: media-core reference add <edition-id> --provider <p> (--external-id <id> | --url <u>) --role <role> [--authoritative] [--language <l>] --provenance <text> --revision <n> [--request-id] | media-core reference retire <edition-id> --reference-id <id> --revision <n> [--reason <text>] [--request-id]"

// referenceArgs is one reference add or retire.
type referenceArgs struct {
	libCommand
	verb        string
	edition     string
	referenceID string
	provider    string
	externalID  *string
	url         *string
	role        domain.ReferenceRole
	auth        bool
	language    *string
	provenance  string
	reason      *string
	revision    int64
	requestID   string
}

func parseReferenceArgs(args []string) (*referenceArgs, error) {
	a := &referenceArgs{}
	rest, err := a.parseCommon(args)
	if err != nil {
		return nil, err
	}
	if len(rest) < 3 {
		return nil, fmt.Errorf("media-core reference: %s", referenceUsage)
	}
	verb, target := rest[0], rest[1]
	var start int
	switch verb {
	case "add":
		a.edition = target
		start = 2
	case "retire":
		a.edition = target
		start = 2
	default:
		return nil, fmt.Errorf("media-core reference: unknown action %q (add, retire)", verb)
	}
	a.verb = verb
	for i := start; i < len(rest); i++ {
		needValue := func() (string, error) {
			if i+1 >= len(rest) {
				return "", fmt.Errorf("media-core reference: %s requires a value", rest[i])
			}
			i++
			return rest[i], nil
		}
		switch rest[i] {
		case "--provider":
			v, err := needValue()
			if err != nil {
				return nil, err
			}
			a.provider = v
		case "--external-id":
			v, err := needValue()
			if err != nil {
				return nil, err
			}
			a.externalID = &v
		case "--url":
			v, err := needValue()
			if err != nil {
				return nil, err
			}
			a.url = &v
		case "--role":
			v, err := needValue()
			if err != nil {
				return nil, err
			}
			r := domain.ReferenceRole(v)
			if !r.Valid() {
				return nil, fmt.Errorf("media-core reference: unknown role %q (publication, metadata, translation, release_watcher, translation_project, content)", v)
			}
			a.role = r
		case "--authoritative":
			a.auth = true
		case "--language":
			v, err := needValue()
			if err != nil {
				return nil, err
			}
			a.language = &v
		case "--provenance":
			v, err := needValue()
			if err != nil {
				return nil, err
			}
			a.provenance = v
		case "--reference-id":
			v, err := needValue()
			if err != nil {
				return nil, err
			}
			a.referenceID = v
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
				return nil, fmt.Errorf("media-core reference: revision is not a number: %v", err)
			}
			a.revision = int64(n)
		case "--request-id":
			v, err := needValue()
			if err != nil {
				return nil, err
			}
			a.requestID = v
		default:
			return nil, fmt.Errorf("media-core reference: unknown argument %q", rest[i])
		}
	}
	switch verb {
	case "add":
		if a.provider == "" {
			return nil, fmt.Errorf("media-core reference: add requires --provider")
		}
		if (a.externalID == nil) == (a.url == nil) {
			return nil, fmt.Errorf("media-core reference: add requires exactly one of --external-id or --url")
		}
		if !a.role.Valid() {
			return nil, fmt.Errorf("media-core reference: add requires --role")
		}
		if a.provenance == "" {
			return nil, fmt.Errorf("media-core reference: add requires --provenance")
		}
	case "retire":
		if a.referenceID == "" {
			return nil, fmt.Errorf("media-core reference: retire requires --reference-id")
		}
	}
	return a, nil
}

// runReference attaches or retires one sourced external reference on an
// edition and prints the updated edition view.
func runReference(args []string, out io.Writer) error {
	a, err := parseReferenceArgs(args)
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
	ctx := context.Background()
	switch a.verb {
	case "add":
		view, err := svc.AddReference(ctx, app.AddReferenceRequest{
			MutationBase:  base,
			EditionID:     a.edition,
			Provider:      a.provider,
			ExternalID:    a.externalID,
			URL:           a.url,
			Role:          a.role,
			Authoritative: a.auth,
			Language:      a.language,
			Provenance:    a.provenance,
		})
		if err != nil {
			return writeEnvelope(out, nil, libTyped(err))
		}
		return writeEnvelope(out, view, nil)
	case "retire":
		view, err := svc.RetireReference(ctx, app.RetireReferenceRequest{
			MutationBase: base,
			EditionID:    a.edition,
			ReferenceID:  a.referenceID,
			Reason:       a.reason,
		})
		if err != nil {
			return writeEnvelope(out, nil, libTyped(err))
		}
		return writeEnvelope(out, view, nil)
	}
	return &runtime.ExitCodeError{Exit: ExitUsage, Err: fmt.Errorf("media-core reference: %s", referenceUsage)}
}
