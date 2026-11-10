// Package runtime owns process lifecycle and mode dispatch for media-core.
package runtime

import (
	"fmt"
	"io"

	"github.com/HaidaDaniel/media-core/internal/buildinfo"
)

// ErrMissingCommand is returned when no command is supplied.
var ErrMissingCommand = fmt.Errorf("media-core: missing command")

// ErrReserved is returned for commands that exist in the design but are not
// implemented yet.
type ErrReserved struct {
	Command string
}

func (e ErrReserved) Error() string {
	return fmt.Sprintf("media-core: command %q is reserved and not implemented yet", e.Command)
}

// ErrUnsupported is returned for commands outside the known set.
type ErrUnsupported struct {
	Command string
}

func (e ErrUnsupported) Error() string {
	return fmt.Sprintf("media-core: unsupported command %q", e.Command)
}

// Dispatch runs one mode. It performs no state access: version reports the
// build identity, and reserved or unknown commands fail before any state
// directory or server is created.
func Dispatch(args []string, out io.Writer) error {
	if len(args) < 1 {
		return ErrMissingCommand
	}
	switch args[0] {
	case "version":
		fmt.Fprintln(out, buildinfo.Get().Render())
		return nil
	case "serve", "mcp":
		return ErrReserved{Command: args[0]}
	default:
		return ErrUnsupported{Command: args[0]}
	}
}
