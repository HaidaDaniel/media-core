package cli

import (
	"fmt"
	"io"

	"github.com/HaidaDaniel/media-core/internal/config"
	"github.com/HaidaDaniel/media-core/internal/httpapi"
	"github.com/HaidaDaniel/media-core/internal/runtime"
)

func init() {
	runtime.RegisterCommand("owner", runOwner)
}

const ownerUsage = "usage: media-core owner <generate|show> [--state-dir <dir>]"

// ownerArgs is one owner-secret management request.
type ownerArgs struct {
	action   string
	stateDir string
}

func parseOwnerArgs(args []string) (*ownerArgs, error) {
	a := &ownerArgs{}
	var rest []string
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--state-dir":
			if i+1 >= len(args) {
				return nil, fmt.Errorf("media-core owner: --state-dir requires a value")
			}
			i++
			a.stateDir = args[i]
		default:
			rest = append(rest, args[i])
		}
	}
	if len(rest) != 1 {
		return nil, fmt.Errorf("media-core owner: %s", ownerUsage)
	}
	a.action = rest[0]
	if a.action != "generate" && a.action != "show" {
		return nil, fmt.Errorf("media-core owner: %s", ownerUsage)
	}
	return a, nil
}

// runOwner generates or displays the owner secret. The secret lives in a
// 0600 file inside the state directory; this command is the explicit local
// way to create and read it. The value is printed to stdout for the local
// operator and is never written to any log or diagnostic.
func runOwner(args []string, out io.Writer) error {
	a, err := parseOwnerArgs(args)
	if err != nil {
		return &runtime.ExitCodeError{Exit: ExitUsage, Err: err}
	}
	cfg, err := config.Resolve(config.Options{StateDir: a.stateDir})
	if err != nil {
		return &runtime.ExitCodeError{Exit: ExitError, Err: err}
	}
	switch a.action {
	case "generate":
		secret, err := httpapi.GenerateOwnerSecret(cfg.StateDir)
		if err != nil {
			return &runtime.ExitCodeError{Exit: ExitError, Err: fmt.Errorf("media-core owner: %w", err)}
		}
		fmt.Fprintln(out, secret)
	case "show":
		secret, err := httpapi.ReadOwnerSecretHex(cfg.StateDir)
		if err != nil {
			return &runtime.ExitCodeError{Exit: ExitError, Err: fmt.Errorf("media-core owner: %w", err)}
		}
		fmt.Fprintln(out, secret)
	}
	return nil
}
