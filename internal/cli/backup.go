package cli

import (
	"context"
	"fmt"
	"io"

	"github.com/HaidaDaniel/media-core/internal/app"
	"github.com/HaidaDaniel/media-core/internal/runtime"
)

func init() {
	runtime.RegisterCommand("backup", runBackup)
}

const backupUsage = "usage: media-core backup create [--dest <dir>] [--name <n>] [--force]"

// backupArgs is one snapshot archive creation request.
type backupArgs struct {
	libCommand
	dest  string
	name  string
	force bool
}

func parseBackupArgs(args []string) (*backupArgs, error) {
	a := &backupArgs{}
	rest, err := a.parseCommon(args)
	if err != nil {
		return nil, err
	}
	if len(rest) < 1 || rest[0] != "create" {
		return nil, fmt.Errorf("media-core backup: %s", backupUsage)
	}
	for i := 1; i < len(rest); i++ {
		needValue := func() (string, error) {
			if i+1 >= len(rest) {
				return "", fmt.Errorf("media-core backup: %s requires a value", rest[i])
			}
			i++
			return rest[i], nil
		}
		switch rest[i] {
		case "--dest":
			v, err := needValue()
			if err != nil {
				return nil, err
			}
			a.dest = v
		case "--name":
			v, err := needValue()
			if err != nil {
				return nil, err
			}
			a.name = v
		case "--force":
			a.force = true
		default:
			return nil, fmt.Errorf("media-core backup: unknown argument %q", rest[i])
		}
	}
	if a.dest == "" {
		return nil, fmt.Errorf("media-core backup: --dest is required; %s", backupUsage)
	}
	return a, nil
}

// runBackup publishes one consistent snapshot archive of the state
// database and prints the archive envelope.
func runBackup(args []string, out io.Writer) error {
	a, err := parseBackupArgs(args)
	if err != nil {
		return &runtime.ExitCodeError{Exit: ExitUsage, Err: err}
	}
	svc, db, err := a.service()
	if err != nil {
		return libExitError(err)
	}
	defer db.Close()
	payload, err := svc.CreateBackup(context.Background(), app.BackupRequest{
		DestDir:   a.dest,
		Name:      a.name,
		Overwrite: a.force,
	})
	if err != nil {
		return writeEnvelope(out, nil, libTyped(err))
	}
	return writeEnvelope(out, payload, nil)
}
