package cli

import (
	"context"
	"fmt"
	"io"
	"strconv"
	"strings"

	"github.com/HaidaDaniel/media-core/internal/app"
	"github.com/HaidaDaniel/media-core/internal/domain"
	"github.com/HaidaDaniel/media-core/internal/runtime"
)

func init() {
	runtime.RegisterCommand("link", runLink)
	runtime.RegisterCommand("origin", runOrigin)
	runtime.RegisterCommand("related", runRelated)
}

const linkUsage = "usage: media-core link add <from-id> <to-id> --scope <work|edition> --type <t> --provenance <text> [--state proposed|confirmed] [--confidence <f>] [--replaces <relation-id>] --revision <n> [--request-id] | media-core link confirm <relation-id> --revision <n> [--request-id] | media-core link revoke <relation-id> --revision <n> [--request-id]"

const originUsage = "usage: media-core origin set <edition-id> --kind <original|anime_original|unknown> --provenance <text> [--state proposed|confirmed] [--replaces <claim-id>] --revision <n> [--request-id] | media-core origin confirm <edition-id> --claim-id <id> --revision <n> [--request-id] | media-core origin revoke <edition-id> --claim-id <id> --revision <n> [--request-id]"

const relatedUsage = "usage: media-core related <entity-id> --scope <work|edition> [--depth <1-3>]"

// linkArgs is one relation add, confirm or revoke.
type linkArgs struct {
	libCommand
	verb       string
	from       string
	to         string
	relationID string
	scope      domain.RelationScope
	typ        domain.RelationType
	state      domain.ClaimState
	provenance string
	confidence *float64
	replacesID *string
	revision   int64
	requestID  string
}

func parseLinkArgs(args []string) (*linkArgs, error) {
	a := &linkArgs{state: domain.ClaimProposed}
	rest, err := a.parseCommon(args)
	if err != nil {
		return nil, err
	}
	if len(rest) < 2 {
		return nil, fmt.Errorf("media-core link: %s", linkUsage)
	}
	verb := rest[0]
	var start int
	switch verb {
	case "add":
		if len(rest) < 4 {
			return nil, fmt.Errorf("media-core link: %s", linkUsage)
		}
		a.from, a.to = rest[1], rest[2]
		start = 3
	case "confirm", "revoke":
		if len(rest) < 3 {
			return nil, fmt.Errorf("media-core link: %s", linkUsage)
		}
		a.relationID = rest[1]
		start = 2
	default:
		return nil, fmt.Errorf("media-core link: unknown action %q (add, confirm, revoke)", verb)
	}
	a.verb = verb
	for i := start; i < len(rest); i++ {
		needValue := func() (string, error) {
			if i+1 >= len(rest) {
				return "", fmt.Errorf("media-core link: %s requires a value", rest[i])
			}
			i++
			return rest[i], nil
		}
		switch rest[i] {
		case "--scope":
			v, err := needValue()
			if err != nil {
				return nil, err
			}
			sc := domain.RelationScope(v)
			if !sc.Valid() {
				return nil, fmt.Errorf("media-core link: unknown scope %q (work, edition)", v)
			}
			a.scope = sc
		case "--type":
			v, err := needValue()
			if err != nil {
				return nil, err
			}
			typ := domain.RelationType(v)
			if !typ.Valid() {
				return nil, fmt.Errorf("media-core link: unknown type %q (adaptation_of, translation_of, alternative_version, same_underlying_work, sequel_of, side_story_of, related)", v)
			}
			a.typ = typ
		case "--state":
			v, err := needValue()
			if err != nil {
				return nil, err
			}
			st := domain.ClaimState(v)
			if st != domain.ClaimProposed && st != domain.ClaimConfirmed {
				return nil, fmt.Errorf("media-core link: unknown state %q (proposed, confirmed)", v)
			}
			a.state = st
		case "--provenance":
			v, err := needValue()
			if err != nil {
				return nil, err
			}
			a.provenance = v
		case "--confidence":
			v, err := needValue()
			if err != nil {
				return nil, err
			}
			f, err := strconv.ParseFloat(v, 64)
			if err != nil {
				return nil, fmt.Errorf("media-core link: confidence is not a number: %v", err)
			}
			a.confidence = &f
		case "--replaces":
			v, err := needValue()
			if err != nil {
				return nil, err
			}
			a.replacesID = &v
		case "--revision":
			v, err := needValue()
			if err != nil {
				return nil, err
			}
			n, err := strconv.Atoi(v)
			if err != nil {
				return nil, fmt.Errorf("media-core link: revision is not a number: %v", err)
			}
			a.revision = int64(n)
		case "--request-id":
			v, err := needValue()
			if err != nil {
				return nil, err
			}
			a.requestID = v
		default:
			return nil, fmt.Errorf("media-core link: unknown argument %q", rest[i])
		}
	}
	if a.verb == "add" {
		if a.scope == "" {
			return nil, fmt.Errorf("media-core link: add requires --scope")
		}
		if !a.typ.Valid() {
			return nil, fmt.Errorf("media-core link: add requires --type")
		}
		if strings.TrimSpace(a.provenance) == "" {
			return nil, fmt.Errorf("media-core link: add requires --provenance")
		}
	}
	return a, nil
}

// runLink asserts, confirms or revokes one typed, scoped relation and
// prints the relation view of the canonical from entity.
func runLink(args []string, out io.Writer) error {
	a, err := parseLinkArgs(args)
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
	var view app.RelationView
	switch a.verb {
	case "add":
		view, err = svc.AddRelation(ctx, app.AddRelationRequest{
			MutationBase: base,
			Scope:        a.scope,
			Type:         a.typ,
			FromID:       a.from,
			ToID:         a.to,
			State:        a.state,
			Provenance:   a.provenance,
			Confidence:   a.confidence,
			ReplacesID:   a.replacesID,
		})
	case "confirm":
		view, err = svc.ConfirmRelation(ctx, app.ConfirmRelationRequest{
			MutationBase: base,
			RelationID:   a.relationID,
		})
	case "revoke":
		view, err = svc.RevokeRelation(ctx, app.RevokeRelationRequest{
			MutationBase: base,
			RelationID:   a.relationID,
		})
	}
	if err != nil {
		return writeEnvelope(out, nil, libTyped(err))
	}
	return writeEnvelope(out, view, nil)
}

// originArgs is one origin set, confirm or revoke.
type originArgs struct {
	libCommand
	verb       string
	edition    string
	claimID    string
	kind       domain.OriginKind
	state      domain.ClaimState
	provenance string
	replacesID *string
	revision   int64
	requestID  string
}

func parseOriginArgs(args []string) (*originArgs, error) {
	a := &originArgs{state: domain.ClaimProposed}
	rest, err := a.parseCommon(args)
	if err != nil {
		return nil, err
	}
	if len(rest) < 3 {
		return nil, fmt.Errorf("media-core origin: %s", originUsage)
	}
	verb, target := rest[0], rest[1]
	var start int
	switch verb {
	case "set":
		a.edition = target
		start = 2
	case "confirm", "revoke":
		a.edition = target
		start = 2
	default:
		return nil, fmt.Errorf("media-core origin: unknown action %q (set, confirm, revoke)", verb)
	}
	a.verb = verb
	for i := start; i < len(rest); i++ {
		needValue := func() (string, error) {
			if i+1 >= len(rest) {
				return "", fmt.Errorf("media-core origin: %s requires a value", rest[i])
			}
			i++
			return rest[i], nil
		}
		switch rest[i] {
		case "--kind":
			v, err := needValue()
			if err != nil {
				return nil, err
			}
			k := domain.OriginKind(v)
			if !k.Valid() {
				return nil, fmt.Errorf("media-core origin: unknown kind %q (original, anime_original, unknown)", v)
			}
			a.kind = k
		case "--state":
			v, err := needValue()
			if err != nil {
				return nil, err
			}
			st := domain.ClaimState(v)
			if st != domain.ClaimProposed && st != domain.ClaimConfirmed {
				return nil, fmt.Errorf("media-core origin: unknown state %q (proposed, confirmed)", v)
			}
			a.state = st
		case "--provenance":
			v, err := needValue()
			if err != nil {
				return nil, err
			}
			a.provenance = v
		case "--replaces":
			v, err := needValue()
			if err != nil {
				return nil, err
			}
			a.replacesID = &v
		case "--claim-id":
			v, err := needValue()
			if err != nil {
				return nil, err
			}
			a.claimID = v
		case "--revision":
			v, err := needValue()
			if err != nil {
				return nil, err
			}
			n, err := strconv.Atoi(v)
			if err != nil {
				return nil, fmt.Errorf("media-core origin: revision is not a number: %v", err)
			}
			a.revision = int64(n)
		case "--request-id":
			v, err := needValue()
			if err != nil {
				return nil, err
			}
			a.requestID = v
		default:
			return nil, fmt.Errorf("media-core origin: unknown argument %q", rest[i])
		}
	}
	if a.verb == "set" {
		if !a.kind.Valid() {
			return nil, fmt.Errorf("media-core origin: set requires --kind")
		}
		if strings.TrimSpace(a.provenance) == "" {
			return nil, fmt.Errorf("media-core origin: set requires --provenance")
		}
	}
	if a.verb == "confirm" || a.verb == "revoke" {
		if a.claimID == "" {
			return nil, fmt.Errorf("media-core origin: %s requires --claim-id", verb)
		}
	}
	return a, nil
}

// runOrigin asserts, confirms or revokes one explicit origin claim on an
// edition and prints the origin view.
func runOrigin(args []string, out io.Writer) error {
	a, err := parseOriginArgs(args)
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
	var view app.OriginView
	switch a.verb {
	case "set":
		view, err = svc.RecordOrigin(ctx, app.RecordOriginRequest{
			MutationBase: base,
			EditionID:    a.edition,
			Kind:         a.kind,
			State:        a.state,
			Provenance:   a.provenance,
			ReplacesID:   a.replacesID,
		})
	case "confirm":
		view, err = svc.ConfirmOrigin(ctx, app.ConfirmOriginRequest{
			MutationBase: base,
			EditionID:    a.edition,
			ClaimID:      a.claimID,
		})
	case "revoke":
		view, err = svc.RevokeOrigin(ctx, app.RevokeOriginRequest{
			MutationBase: base,
			EditionID:    a.edition,
			ClaimID:      a.claimID,
		})
	}
	if err != nil {
		return writeEnvelope(out, nil, libTyped(err))
	}
	return writeEnvelope(out, view, nil)
}

// relatedArgs is one bounded related-media traversal.
type relatedArgs struct {
	libCommand
	entity string
	scope  domain.RelationScope
	depth  int
}

func parseRelatedArgs(args []string) (*relatedArgs, error) {
	a := &relatedArgs{depth: 1}
	rest, err := a.parseCommon(args)
	if err != nil {
		return nil, err
	}
	if len(rest) < 2 {
		return nil, fmt.Errorf("media-core related: %s", relatedUsage)
	}
	a.entity = rest[0]
	for i := 1; i < len(rest); i++ {
		needValue := func() (string, error) {
			if i+1 >= len(rest) {
				return "", fmt.Errorf("media-core related: %s requires a value", rest[i])
			}
			i++
			return rest[i], nil
		}
		switch rest[i] {
		case "--scope":
			v, err := needValue()
			if err != nil {
				return nil, err
			}
			sc := domain.RelationScope(v)
			if !sc.Valid() {
				return nil, fmt.Errorf("media-core related: unknown scope %q (work, edition)", v)
			}
			a.scope = sc
		case "--depth":
			v, err := needValue()
			if err != nil {
				return nil, err
			}
			n, err := strconv.Atoi(v)
			if err != nil || n < 1 || n > 3 {
				return nil, fmt.Errorf("media-core related: depth must be 1, 2 or 3")
			}
			a.depth = n
		default:
			return nil, fmt.Errorf("media-core related: unknown argument %q", rest[i])
		}
	}
	if a.scope == "" {
		return nil, fmt.Errorf("media-core related: --scope is required")
	}
	return a, nil
}

// runRelated traverses the non-revoked relations of one scope in both
// directions with a bounded depth and prints the traversal view.
func runRelated(args []string, out io.Writer) error {
	a, err := parseRelatedArgs(args)
	if err != nil {
		return &runtime.ExitCodeError{Exit: ExitUsage, Err: err}
	}
	svc, db, err := a.service()
	if err != nil {
		return libExitError(err)
	}
	defer db.Close()
	view, err := svc.Related(context.Background(), a.scope, a.entity, a.depth)
	if err != nil {
		return writeEnvelope(out, nil, libTyped(err))
	}
	return writeEnvelope(out, view, nil)
}
