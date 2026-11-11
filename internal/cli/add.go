package cli

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"

	"github.com/HaidaDaniel/media-core/internal/app"
	"github.com/HaidaDaniel/media-core/internal/config"
	"github.com/HaidaDaniel/media-core/internal/domain"
	"github.com/HaidaDaniel/media-core/internal/runtime"
	"github.com/HaidaDaniel/media-core/internal/store"
)

func init() {
	runtime.RegisterCommand("add", runAdd)
}

type addArgs struct {
	stateDir      string
	titles        []string
	titleKind     domain.TitleKind
	titleLanguage *string
	kind          *domain.EditionKind
	language      *string
	label         *string
	workID        *string
	references    []store.ReferenceInput
	pendingProv   *string
	pendingID     *string
	choice        *app.AdditionChoice
	token         *string
	requestID     string
}

func parseAddArgs(args []string) (*addArgs, error) {
	a := &addArgs{titleKind: domain.TitleOriginal}
	for i := 0; i < len(args); i++ {
		needValue := func() (string, error) {
			if i+1 >= len(args) {
				return "", fmt.Errorf("media-core add: %s requires a value", args[i])
			}
			i++
			return args[i], nil
		}
		strPtr := func(v string) *string { return &v }
		switch args[i] {
		case "--title":
			v, err := needValue()
			if err != nil {
				return nil, err
			}
			a.titles = append(a.titles, v)
		case "--title-kind":
			v, err := needValue()
			if err != nil {
				return nil, err
			}
			k := domain.TitleKind(v)
			if !k.Valid() {
				return nil, fmt.Errorf("media-core add: unknown title kind %q", v)
			}
			a.titleKind = k
		case "--title-language":
			v, err := needValue()
			if err != nil {
				return nil, err
			}
			a.titleLanguage = strPtr(v)
		case "--kind":
			v, err := needValue()
			if err != nil {
				return nil, err
			}
			k := domain.EditionKind(v)
			if !k.Valid() {
				return nil, fmt.Errorf("media-core add: unknown edition kind %q", v)
			}
			a.kind = &k
		case "--language":
			v, err := needValue()
			if err != nil {
				return nil, err
			}
			a.language = strPtr(v)
		case "--label":
			v, err := needValue()
			if err != nil {
				return nil, err
			}
			a.label = strPtr(v)
		case "--work-id":
			v, err := needValue()
			if err != nil {
				return nil, err
			}
			a.workID = strPtr(v)
		case "--provider":
			v, err := needValue()
			if err != nil {
				return nil, err
			}
			if a.pendingProv != nil {
				return nil, fmt.Errorf("media-core add: --provider starts a new reference; finish the previous one with --external-id or --url")
			}
			a.pendingProv = strPtr(v)
		case "--external-id":
			v, err := needValue()
			if err != nil {
				return nil, err
			}
			if a.pendingID != nil {
				return nil, fmt.Errorf("media-core add: a reference already has an external id")
			}
			a.pendingID = strPtr(v)
		case "--url":
			v, err := needValue()
			if err != nil {
				return nil, err
			}
			if a.pendingID != nil {
				return nil, fmt.Errorf("media-core add: a reference has both an external id and a url")
			}
			ref := store.ReferenceInput{Provider: a.pendingProv, URL: strPtr(v)}
			a.references = append(a.references, ref)
			a.pendingProv = nil
		case "--choice":
			v, err := needValue()
			if err != nil {
				return nil, err
			}
			c, ok := map[string]app.AdditionChoice{
				"use_existing":    app.ChoiceUseExisting,
				"use-existing":    app.ChoiceUseExisting,
				"add_edition":     app.ChoiceAddEdition,
				"add-edition":     app.ChoiceAddEdition,
				"create_distinct": app.ChoiceCreateDistinct,
				"create-distinct": app.ChoiceCreateDistinct,
			}[v]
			if !ok {
				return nil, fmt.Errorf("media-core add: unknown choice %q (use_existing, add_edition, create_distinct)", v)
			}
			a.choice = &c
		case "--candidate-token":
			v, err := needValue()
			if err != nil {
				return nil, err
			}
			a.token = strPtr(v)
		case "--request-id":
			v, err := needValue()
			if err != nil {
				return nil, err
			}
			a.requestID = v
		case "--state-dir":
			v, err := needValue()
			if err != nil {
				return nil, err
			}
			a.stateDir = v
		default:
			return nil, fmt.Errorf("media-core add: unknown argument %q", args[i])
		}
	}
	if a.pendingID != nil {
		if a.pendingProv == nil {
			return nil, fmt.Errorf("media-core add: --external-id requires --provider")
		}
		a.references = append(a.references, store.ReferenceInput{Provider: a.pendingProv, ExternalID: a.pendingID})
	} else if a.pendingProv != nil {
		return nil, fmt.Errorf("media-core add: --provider requires --external-id or --url")
	}
	return a, nil
}

// runAdd resolves a manual addition and either creates it, reports an
// existing entry, or prints the candidates for an explicit follow-up choice.
// Exactly one JSON envelope goes to stdout; a human line goes to stderr on
// failure.
func runAdd(args []string, out io.Writer) error {
	a, err := parseAddArgs(args)
	if err != nil {
		return &runtime.ExitCodeError{Exit: ExitUsage, Err: err}
	}
	cfg, err := config.Resolve(config.Options{StateDir: a.stateDir})
	if err != nil {
		return &runtime.ExitCodeError{Exit: ExitError, Err: err}
	}
	db, err := runtime.OpenDatabase(cfg)
	if err != nil {
		return addExitError(err)
	}
	defer db.Close()
	svc := app.NewService(db)
	req, err := buildAdditionRequest(a)
	if err != nil {
		return &runtime.ExitCodeError{Exit: ExitUsage, Err: err}
	}
	ctx := context.Background()
	var (
		env  app.Envelope
		data any
		fail *app.AppError
	)
	if a.choice == nil {
		res, resErr := svc.ResolveAddition(ctx, req)
		if resErr != nil {
			var ae *app.AppError
			if errors.As(resErr, &ae) {
				fail = ae
			} else {
				fail = app.NewAppError(app.ErrStorageError, resErr.Error())
			}
		} else {
			switch res.Outcome {
			case app.OutcomeCandidates:
				data = res
			case app.OutcomeExisting, app.OutcomeReady:
				// Both create paths go through the mutation use case: the
				// ready path creates, the existing path returns the
				// created:false result with the matched work and edition.
				created, createErr := svc.CreateLibraryEntry(ctx, req)
				if createErr != nil {
					if !errors.As(createErr, &fail) {
						fail = mapAddFailure(createErr)
					}
				} else {
					data = created
				}
			}
		}
	} else {
		created, createErr := svc.CreateLibraryEntry(ctx, req)
		if createErr != nil {
			if !errors.As(createErr, &fail) {
				fail = mapAddFailure(createErr)
			}
		} else {
			data = created
		}
	}
	if fail != nil {
		env = app.Failure(fail)
	} else {
		env = app.Success(data)
	}
	b, err := json.Marshal(env)
	if err != nil {
		return &runtime.ExitCodeError{Exit: ExitError, Err: err}
	}
	fmt.Fprintln(out, string(b))
	if fail != nil {
		return &runtime.ExitCodeError{Exit: fail.Code.CLIExit(), Err: fail}
	}
	return nil
}

// mapAddFailure wraps a non-typed service failure as a storage error.
func mapAddFailure(err error) *app.AppError {
	var ae *app.AppError
	if errors.As(err, &ae) {
		return ae
	}
	return app.NewAppError(app.ErrStorageError, err.Error())
}

func buildAdditionRequest(a *addArgs) (app.AdditionRequest, error) {
	req := app.AdditionRequest{}
	for _, text := range a.titles {
		req.Titles = append(req.Titles, store.TitleInput{
			Text:       text,
			Language:   a.titleLanguage,
			Kind:       a.titleKind,
			Provenance: "manual",
		})
	}
	req.Kind = a.kind
	req.Language = a.language
	req.OtherLabel = a.label
	req.WorkID = a.workID
	req.References = a.references
	req.Choice = a.choice
	req.CandidateToken = a.token
	req.RequestID = a.requestID
	if req.RequestID == "" {
		id, err := domain.NewUUID()
		if err != nil {
			return req, err
		}
		req.RequestID = id
	}
	req.Actor = app.Actor{Principal: "local", Origin: domain.OriginUser}
	return req, nil
}

// addExitError maps add failures to contract exit codes: a locked state
// exits 4, typed application errors keep their code, config and storage
// failures exit 1.
func addExitError(err error) error {
	if errors.Is(err, store.ErrLocked) {
		return &runtime.ExitCodeError{Exit: ExitLocked, Err: err}
	}
	var ae *app.AppError
	if errors.As(err, &ae) {
		return &runtime.ExitCodeError{Exit: ae.Code.CLIExit(), Err: ae}
	}
	return &runtime.ExitCodeError{Exit: ExitError, Err: err}
}
