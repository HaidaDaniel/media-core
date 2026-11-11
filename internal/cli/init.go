package cli

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"

	"github.com/HaidaDaniel/media-core/internal/app"
	"github.com/HaidaDaniel/media-core/internal/config"
	"github.com/HaidaDaniel/media-core/internal/runtime"
	"github.com/HaidaDaniel/media-core/internal/store"
)

func init() {
	runtime.RegisterCommand("init", runMaintenance)
	runtime.RegisterCommand("migrate", runMaintenance)
}

// MaintenanceResult is the data payload of the init and migrate commands.
type MaintenanceResult struct {
	Schema int `json:"schema"`
}

// runMaintenance resolves the configuration, opens the database under the
// exclusive maintenance lock, applies every pending migration, and prints
// exactly one JSON envelope to stdout. Repeated runs are safe no-ops; a
// ledger newer than the embedded set is rejected without modification.
func runMaintenance(args []string, out io.Writer) error {
	opts, err := parseMaintenanceArgs(args)
	if err != nil {
		return &runtime.ExitCodeError{Exit: ExitUsage, Err: err}
	}
	cfg, err := config.Resolve(opts)
	if err != nil {
		return &runtime.ExitCodeError{Exit: ExitError, Err: err}
	}
	db, err := runtime.OpenDatabaseForMaintenance(cfg)
	if err != nil {
		return maintenanceExitError(err)
	}
	version, migrateErr := store.RunMigrations(db.DB())
	closeErr := db.Close()
	if migrateErr != nil {
		return maintenanceExitError(migrateErr)
	}
	if closeErr != nil {
		return &runtime.ExitCodeError{Exit: ExitError, Err: closeErr}
	}
	env := app.Success(MaintenanceResult{Schema: version})
	b, err := json.Marshal(env)
	if err != nil {
		return &runtime.ExitCodeError{Exit: ExitError, Err: err}
	}
	fmt.Fprintln(out, string(b))
	return nil
}

// parseMaintenanceArgs accepts the state-dir flag; anything else is an
// invalid argument for the command.
func parseMaintenanceArgs(args []string) (config.Options, error) {
	var opts config.Options
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--state-dir":
			if i+1 >= len(args) {
				return opts, fmt.Errorf("media-core: --state-dir requires a value")
			}
			i++
			opts.StateDir = args[i]
		default:
			return opts, fmt.Errorf("media-core: unknown argument %q for this command", args[i])
		}
	}
	return opts, nil
}

// maintenanceExitError maps maintenance failures to contract exit codes:
// locked state exits 4, config and storage failures exit 1.
func maintenanceExitError(err error) error {
	if errors.Is(err, store.ErrLocked) {
		return &runtime.ExitCodeError{Exit: ExitLocked, Err: err}
	}
	return &runtime.ExitCodeError{Exit: ExitError, Err: err}
}
