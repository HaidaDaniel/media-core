package cli

import (
	"context"
	"fmt"
	"io"

	"github.com/HaidaDaniel/media-core/internal/app"
	"github.com/HaidaDaniel/media-core/internal/domain"
	"github.com/HaidaDaniel/media-core/internal/importer"
	"github.com/HaidaDaniel/media-core/internal/runtime"
)

func init() {
	runtime.RegisterCommand("releases", runReleases)
}

const releasesUsage = `usage: media-core releases <subcommand>
  bind    --publication <id> --edition <id> [--namespace <ns>]
  rebind  --publication <id> --edition <id> [--namespace <ns>]
  revoke  --publication <id> [--namespace <ns>]
  list    --edition <id> [--limit N] [--cursor C]
  show    --publication <id> [--namespace <ns>]`

// releasesArgs is one release-binding or release-history request.
type releasesArgs struct {
	libCommand
	subcommand  string
	namespace   string
	publication string
	edition     string
	limit       int
	cursor      *string
}

func parseReleasesArgs(args []string) (*releasesArgs, error) {
	a := &releasesArgs{namespace: importer.DefaultNamespace}
	rest, err := a.parseCommon(args)
	if err != nil {
		return nil, err
	}
	if len(rest) < 1 {
		return nil, fmt.Errorf("media-core releases: %s", releasesUsage)
	}
	a.subcommand = rest[0]
	for i := 1; i < len(rest); i++ {
		needValue := func() (string, error) {
			if i+1 >= len(rest) {
				return "", fmt.Errorf("media-core releases: %s requires a value", rest[i])
			}
			i++
			return rest[i], nil
		}
		switch rest[i] {
		case "--publication":
			v, err := needValue()
			if err != nil {
				return nil, err
			}
			a.publication = v
		case "--edition":
			v, err := needValue()
			if err != nil {
				return nil, err
			}
			a.edition = v
		case "--namespace":
			v, err := needValue()
			if err != nil {
				return nil, err
			}
			a.namespace = v
		case "--limit":
			v, err := needValue()
			if err != nil {
				return nil, err
			}
			n, perr := parsePositiveInt(v)
			if perr != nil {
				return nil, fmt.Errorf("media-core releases: --limit: %v", perr)
			}
			a.limit = n
		case "--cursor":
			v, err := needValue()
			if err != nil {
				return nil, err
			}
			a.cursor = &v
		default:
			return nil, fmt.Errorf("media-core releases: unknown argument %q (%s)", rest[i], releasesUsage)
		}
	}
	switch a.subcommand {
	case "bind", "rebind":
		if a.publication == "" || a.edition == "" {
			return nil, fmt.Errorf("media-core releases %s: --publication and --edition are required; %s", a.subcommand, releasesUsage)
		}
	case "revoke", "show":
		if a.publication == "" {
			return nil, fmt.Errorf("media-core releases %s: --publication is required; %s", a.subcommand, releasesUsage)
		}
	case "list":
		if a.edition == "" {
			return nil, fmt.Errorf("media-core releases list: --edition is required; %s", releasesUsage)
		}
	default:
		return nil, fmt.Errorf("media-core releases: unknown subcommand %q; %s", a.subcommand, releasesUsage)
	}
	return a, nil
}

func releasesActor() app.Actor {
	return app.Actor{Principal: "local", Origin: domain.OriginUser}
}

// runReleases drives the release-binding vertical slice: explicit bind and
// rebind (correction) of a confirmed publication to an edition, revoke, and
// the bounded release-history listing. A failed mutation prints a failure
// envelope and exits with the code the stable error carries.
func runReleases(args []string, out io.Writer) error {
	a, err := parseReleasesArgs(args)
	if err != nil {
		return &runtime.ExitCodeError{Exit: ExitUsage, Err: err}
	}
	svc, db, err := a.service()
	if err != nil {
		return libExitError(err)
	}
	defer db.Close()
	ctx := context.Background()
	switch a.subcommand {
	case "bind", "rebind":
		req := app.BindReleaseRequest{
			MutationBase:  app.MutationBase{RequestID: "releases-" + a.subcommand, Actor: releasesActor()},
			Namespace:     a.namespace,
			PublicationID: a.publication,
			EditionID:     a.edition,
		}
		var view any
		if a.subcommand == "bind" {
			view, err = svc.BindRelease(ctx, req)
		} else {
			view, err = svc.RebindRelease(ctx, req)
		}
		if err != nil {
			return writeEnvelope(out, nil, libTyped(err))
		}
		return writeEnvelope(out, view, nil)
	case "revoke":
		req := app.RevokeReleaseRequest{
			MutationBase:  app.MutationBase{RequestID: "releases-revoke", Actor: releasesActor()},
			Namespace:     a.namespace,
			PublicationID: a.publication,
		}
		if err := svc.RevokeRelease(ctx, req); err != nil {
			return writeEnvelope(out, nil, libTyped(err))
		}
		return writeEnvelope(out, map[string]any{"status": "revoked", "publication_id": a.publication, "namespace": a.namespace}, nil)
	case "list":
		payload, err := svc.ListReleases(ctx, app.ReleaseQuery{EditionID: a.edition, Limit: a.limit, Cursor: a.cursor})
		if err != nil {
			return writeEnvelope(out, nil, libTyped(err))
		}
		return writeEnvelope(out, payload, nil)
	case "show":
		binding, err := svc.GetReleaseBinding(ctx, a.namespace, a.publication)
		if err != nil {
			return writeEnvelope(out, nil, libTyped(err))
		}
		history, err := svc.ListReleaseBindings(ctx, a.namespace, a.publication)
		if err != nil {
			return writeEnvelope(out, nil, libTyped(err))
		}
		return writeEnvelope(out, map[string]any{"active": binding, "history": history}, nil)
	}
	return &runtime.ExitCodeError{Exit: ExitUsage, Err: fmt.Errorf("media-core releases: %s", releasesUsage)}
}
