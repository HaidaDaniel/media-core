package cli

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strconv"

	"github.com/HaidaDaniel/media-core/internal/app"
	"github.com/HaidaDaniel/media-core/internal/config"
	"github.com/HaidaDaniel/media-core/internal/domain"
	"github.com/HaidaDaniel/media-core/internal/runtime"
	"github.com/HaidaDaniel/media-core/internal/store"
)

func init() {
	runtime.RegisterCommand("list", runList)
	runtime.RegisterCommand("show", runShow)
	runtime.RegisterCommand("title", runTitle)
	runtime.RegisterCommand("edition", runEdition)
}

type libCommand struct {
	stateDir string
}

func (c *libCommand) parseCommon(args []string) ([]string, error) {
	rest := []string{}
	for i := 0; i < len(args); i++ {
		if args[i] == "--state-dir" {
			if i+1 >= len(args) {
				return nil, fmt.Errorf("media-core: --state-dir requires a value")
			}
			i++
			c.stateDir = args[i]
		} else {
			rest = append(rest, args[i])
		}
	}
	return rest, nil
}

func (c *libCommand) service() (*app.Service, *store.Database, error) {
	cfg, err := config.Resolve(config.Options{StateDir: c.stateDir})
	if err != nil {
		return nil, nil, err
	}
	db, err := runtime.OpenDatabase(cfg)
	if err != nil {
		return nil, nil, err
	}
	return app.NewService(db), db, nil
}

func writeEnvelope(out io.Writer, data any, fail *app.AppError) error {
	var env app.Envelope
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

func libTyped(err error) *app.AppError {
	var ae *app.AppError
	if errors.As(err, &ae) {
		return ae
	}
	return app.NewAppError(app.ErrStorageError, err.Error())
}

// listArgs is one bounded library listing request.
type listArgs struct {
	libCommand
	query        *string
	kinds        []domain.EditionKind
	publications []domain.Publication
	consumptions []domain.Consumption
	unknownCons  bool
	intentions   []domain.Intention
	tags         []string
	sort         app.LibrarySort
	limit        int
	cursor       *string
}

func parseListArgs(args []string) (*listArgs, error) {
	a := &listArgs{sort: app.SortTitle}
	rest, err := a.parseCommon(args)
	if err != nil {
		return nil, err
	}
	for i := 0; i < len(rest); i++ {
		needValue := func() (string, error) {
			if i+1 >= len(rest) {
				return "", fmt.Errorf("media-core list: %s requires a value", rest[i])
			}
			i++
			return rest[i], nil
		}
		switch rest[i] {
		case "--query":
			v, err := needValue()
			if err != nil {
				return nil, err
			}
			a.query = &v
		case "--kind":
			v, err := needValue()
			if err != nil {
				return nil, err
			}
			k := domain.EditionKind(v)
			if !k.Valid() {
				return nil, fmt.Errorf("media-core list: unknown edition kind %q", v)
			}
			a.kinds = append(a.kinds, k)
		case "--publication":
			v, err := needValue()
			if err != nil {
				return nil, err
			}
			p := domain.Publication(v)
			if !p.Valid() {
				return nil, fmt.Errorf("media-core list: unknown publication %q", v)
			}
			a.publications = append(a.publications, p)
		case "--consumption":
			v, err := needValue()
			if err != nil {
				return nil, err
			}
			if v == "unknown" {
				a.unknownCons = true
			} else {
				c := domain.Consumption(v)
				if !c.Valid() {
					return nil, fmt.Errorf("media-core list: unknown consumption %q", v)
				}
				a.consumptions = append(a.consumptions, c)
			}
		case "--intention":
			v, err := needValue()
			if err != nil {
				return nil, err
			}
			i := domain.Intention(v)
			if !i.Valid() {
				return nil, fmt.Errorf("media-core list: unknown intention %q (none, interested, saved, planned, not_interested)", v)
			}
			a.intentions = append(a.intentions, i)
		case "--tag":
			v, err := needValue()
			if err != nil {
				return nil, err
			}
			a.tags = append(a.tags, v)
		case "--sort":
			v, err := needValue()
			if err != nil {
				return nil, err
			}
			s := app.LibrarySort(v)
			if !s.Valid() {
				return nil, fmt.Errorf("media-core list: unknown sort %q (title, created_at, updated_at)", v)
			}
			a.sort = s
		case "--limit":
			v, err := needValue()
			if err != nil {
				return nil, err
			}
			n, err := strconv.Atoi(v)
			if err != nil {
				return nil, fmt.Errorf("media-core list: limit is not a number: %v", err)
			}
			a.limit = n
		case "--cursor":
			v, err := needValue()
			if err != nil {
				return nil, err
			}
			a.cursor = &v
		default:
			return nil, fmt.Errorf("media-core list: unknown argument %q", rest[i])
		}
	}
	return a, nil
}

// runList lists distinct works as work cards with an opaque seek cursor.
func runList(args []string, out io.Writer) error {
	a, err := parseListArgs(args)
	if err != nil {
		return &runtime.ExitCodeError{Exit: ExitUsage, Err: err}
	}
	svc, db, err := a.service()
	if err != nil {
		return libExitError(err)
	}
	defer db.Close()
	payload, err := svc.SearchLibrary(context.Background(), app.LibraryFilter{
		Query:              a.query,
		Kinds:              a.kinds,
		Publications:       a.publications,
		Consumptions:       a.consumptions,
		UnknownConsumption: a.unknownCons,
		Intentions:         a.intentions,
		Tags:               a.tags,
		Sort:               a.sort,
		Limit:              a.limit,
		Cursor:             a.cursor,
	})
	if err != nil {
		return writeEnvelope(out, nil, libTyped(err))
	}
	return writeEnvelope(out, payload, nil)
}

func libExitError(err error) error {
	if errors.Is(err, store.ErrLocked) {
		return &runtime.ExitCodeError{Exit: ExitLocked, Err: err}
	}
	var ae *app.AppError
	if errors.As(err, &ae) {
		return &runtime.ExitCodeError{Exit: ae.Code.CLIExit(), Err: ae}
	}
	return &runtime.ExitCodeError{Exit: ExitError, Err: err}
}

// runShow prints the work or edition detail view.
func runShow(args []string, out io.Writer) error {
	c := &libCommand{}
	rest, err := c.parseCommon(args)
	if err != nil {
		return &runtime.ExitCodeError{Exit: ExitUsage, Err: err}
	}
	if len(rest) != 2 {
		return &runtime.ExitCodeError{Exit: ExitUsage, Err: fmt.Errorf("media-core show: usage: media-core show work|edition <id>")}
	}
	svc, db, err := c.service()
	if err != nil {
		return libExitError(err)
	}
	defer db.Close()
	ctx := context.Background()
	switch rest[0] {
	case "work":
		view, err := svc.GetWork(ctx, rest[1])
		if err != nil {
			return writeEnvelope(out, nil, libTyped(err))
		}
		return writeEnvelope(out, view, nil)
	case "edition":
		view, err := svc.GetEdition(ctx, rest[1])
		if err != nil {
			return writeEnvelope(out, nil, libTyped(err))
		}
		return writeEnvelope(out, view, nil)
	default:
		return &runtime.ExitCodeError{Exit: ExitUsage, Err: fmt.Errorf("media-core show: unknown target %q (work, edition)", rest[0])}
	}
}

// titleArgs is one title correction on a work or an edition.
type titleArgs struct {
	libCommand
	target     string
	entity     string
	revision   int64
	text       string
	titleKind  domain.TitleKind
	language   *string
	provenance string
	retire     []string
	preferred  *string
	requestID  string
}

func (a *titleArgs) request() (app.TitleMutationRequest, error) {
	req := app.TitleMutationRequest{
		MutationBase: app.MutationBase{
			RequestID:        a.requestID,
			Actor:            app.Actor{Principal: "local", Origin: domain.OriginUser},
			ExpectedRevision: &a.revision,
		},
		TargetID: a.target,
	}
	if a.text != "" {
		req.Add = append(req.Add, store.TitleInput{
			Text:       a.text,
			Language:   a.language,
			Kind:       a.titleKind,
			Provenance: a.provenance,
		})
	}
	req.Retire = a.retire
	req.Preferred = a.preferred
	if req.RequestID == "" {
		id, err := domain.NewUUID()
		if err != nil {
			return req, err
		}
		req.RequestID = id
	}
	return req, nil
}

func parseTitleArgs(args []string) (*titleArgs, error) {
	a := &titleArgs{titleKind: domain.TitleOriginal}
	rest, err := a.parseCommon(args)
	if err != nil {
		return nil, err
	}
	if len(rest) < 4 {
		return nil, fmt.Errorf("media-core title: usage: media-core title work|edition add|retire|prefer <id> <flags>")
	}
	entity, verb, target := rest[0], rest[1], rest[2]
	if entity != "work" && entity != "edition" {
		return nil, fmt.Errorf("media-core title: unknown entity %q (work, edition)", entity)
	}
	a.entity, a.target = entity, target
	switch verb {
	case "add":
	case "retire":
	case "prefer":
	default:
		return nil, fmt.Errorf("media-core title: unknown action %q (add, retire, prefer)", verb)
	}
	for i := 3; i < len(rest); i++ {
		needValue := func() (string, error) {
			if i+1 >= len(rest) {
				return "", fmt.Errorf("media-core title: %s requires a value", rest[i])
			}
			i++
			return rest[i], nil
		}
		switch rest[i] {
		case "--revision":
			v, err := needValue()
			if err != nil {
				return nil, err
			}
			n, err := strconv.Atoi(v)
			if err != nil {
				return nil, fmt.Errorf("media-core title: revision is not a number: %v", err)
			}
			a.revision = int64(n)
		case "--text":
			v, err := needValue()
			if err != nil {
				return nil, err
			}
			a.text = v
		case "--title-kind":
			v, err := needValue()
			if err != nil {
				return nil, err
			}
			k := domain.TitleKind(v)
			if !k.Valid() {
				return nil, fmt.Errorf("media-core title: unknown title kind %q", v)
			}
			a.titleKind = k
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
		case "--title-id":
			v, err := needValue()
			if err != nil {
				return nil, err
			}
			if verb == "add" {
				return nil, fmt.Errorf("media-core title: --title-id is not valid with add")
			}
			if verb == "retire" {
				a.retire = append(a.retire, v)
			} else {
				a.preferred = &v
			}
		case "--request-id":
			v, err := needValue()
			if err != nil {
				return nil, err
			}
			a.requestID = v
		default:
			return nil, fmt.Errorf("media-core title: unknown argument %q", rest[i])
		}
	}
	switch verb {
	case "add":
		if a.text == "" {
			return nil, fmt.Errorf("media-core title: add requires --text")
		}
		if a.provenance == "" {
			return nil, fmt.Errorf("media-core title: add requires --provenance")
		}
	case "retire":
		if len(a.retire) == 0 {
			return nil, fmt.Errorf("media-core title: retire requires at least one --title-id")
		}
	case "prefer":
		if a.preferred == nil {
			return nil, fmt.Errorf("media-core title: prefer requires --title-id")
		}
	}
	return a, nil
}

// runTitle applies one title correction: add, retire or prefer on a work or
// an edition.
func runTitle(args []string, out io.Writer) error {
	a, err := parseTitleArgs(args)
	if err != nil {
		return &runtime.ExitCodeError{Exit: ExitUsage, Err: err}
	}
	svc, db, err := a.service()
	if err != nil {
		return libExitError(err)
	}
	defer db.Close()
	req, err := a.request()
	if err != nil {
		return &runtime.ExitCodeError{Exit: ExitError, Err: err}
	}
	ctx := context.Background()
	if a.entity == "work" {
		view, err := svc.CorrectWorkTitles(ctx, req)
		if err != nil {
			return writeEnvelope(out, nil, libTyped(err))
		}
		return writeEnvelope(out, view, nil)
	}
	view, err := svc.CorrectEditionTitles(ctx, req)
	if err != nil {
		return writeEnvelope(out, nil, libTyped(err))
	}
	return writeEnvelope(out, view, nil)
}

// editionArgs is one edition metadata or archive command.
type editionArgs struct {
	libCommand
	verb      string
	target    string
	revision  int64
	kind      *domain.EditionKind
	language  *string
	label     *string
	requestID string
}

func parseEditionArgs(args []string) (*editionArgs, error) {
	a := &editionArgs{}
	rest, err := a.parseCommon(args)
	if err != nil {
		return nil, err
	}
	if len(rest) < 3 {
		return nil, fmt.Errorf("media-core edition: usage: media-core edition update|archive|unarchive <id> <flags>")
	}
	verb, target := rest[0], rest[1]
	switch verb {
	case "update", "archive", "unarchive":
	default:
		return nil, fmt.Errorf("media-core edition: unknown action %q (update, archive, unarchive)", verb)
	}
	a.verb, a.target = verb, target
	for i := 2; i < len(rest); i++ {
		needValue := func() (string, error) {
			if i+1 >= len(rest) {
				return "", fmt.Errorf("media-core edition: %s requires a value", rest[i])
			}
			i++
			return rest[i], nil
		}
		switch rest[i] {
		case "--revision":
			v, err := needValue()
			if err != nil {
				return nil, err
			}
			n, err := strconv.Atoi(v)
			if err != nil {
				return nil, fmt.Errorf("media-core edition: revision is not a number: %v", err)
			}
			a.revision = int64(n)
		case "--kind":
			v, err := needValue()
			if err != nil {
				return nil, err
			}
			k := domain.EditionKind(v)
			if !k.Valid() {
				return nil, fmt.Errorf("media-core edition: unknown edition kind %q", v)
			}
			a.kind = &k
		case "--language":
			v, err := needValue()
			if err != nil {
				return nil, err
			}
			a.language = &v
		case "--label":
			v, err := needValue()
			if err != nil {
				return nil, err
			}
			a.label = &v
		case "--request-id":
			v, err := needValue()
			if err != nil {
				return nil, err
			}
			a.requestID = v
		default:
			return nil, fmt.Errorf("media-core edition: unknown argument %q", rest[i])
		}
	}
	if verb == "update" && a.kind == nil && a.language == nil && a.label == nil {
		return nil, fmt.Errorf("media-core edition: update requires --kind, --language or --label")
	}
	return a, nil
}

// runEdition updates edition metadata or archives and unarchives an edition.
func runEdition(args []string, out io.Writer) error {
	a, err := parseEditionArgs(args)
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
	if a.verb == "update" {
		view, err := svc.UpdateEdition(ctx, app.UpdateEditionRequest{
			MutationBase: base,
			EditionID:    a.target,
			Kind:         a.kind,
			Language:     a.language,
			OtherLabel:   a.label,
		})
		if err != nil {
			return writeEnvelope(out, nil, libTyped(err))
		}
		return writeEnvelope(out, view, nil)
	}
	archived := a.verb == "archive"
	view, err := svc.ArchiveEdition(ctx, base, a.target, archived)
	if err != nil {
		return writeEnvelope(out, nil, libTyped(err))
	}
	return writeEnvelope(out, view, nil)
}
