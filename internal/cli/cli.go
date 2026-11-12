// Package cli adapts command-line invocations to runtime modes.
package cli

import (
	"errors"
	"fmt"
	"io"
	"os"

	"github.com/HaidaDaniel/media-core/internal/buildinfo"
	"github.com/HaidaDaniel/media-core/internal/runtime"
)

// Exit codes for process termination, per docs/application-contract.md:
// 2 covers both invalid input and conflict-class failures.
const (
	ExitOK            = 0
	ExitError         = 1
	ExitUsage         = 2
	ExitConflict      = ExitUsage
	ExitPartialImport = 3
	ExitLocked        = 4
)

const usage = `usage: media-core <command>

commands:
  version  print build identity
  init     initialize the database (first setup)
  migrate  bring an existing database to the current schema
  add      add a library entry with explicit duplicate resolution
  list     search and list the library as work cards
  show     show one work or edition detail view
  title    add, retire or prefer a work or edition title
  edition  update edition metadata; archive or unarchive an edition
  status   set an edition's personal consumption status
  intention set an edition's personal intention
  progress record or retract edition reading progress by unit
  tag      attach or detach a tag on a work or edition
  note     add, revise or retract a plain-text note on a work or edition
  serve    reserved: HTTP API + embedded UI (not implemented yet)
  mcp      reserved: MCP stdio (not implemented yet)

Run "media-core version" to print the build identity.
`

// Usage writes the command usage text to w.
func Usage(w io.Writer) {
	fmt.Fprint(w, usage)
}

// Run executes argv (excluding the program name) and returns a process exit
// code. Missing and unsupported commands print usage and return ExitUsage;
// reserved commands fail with ExitError; capability commands return the exit
// code they carry (runtime.ExitCodeError) or ExitUsage when unclassified.
func Run(args []string, out, err io.Writer) int {
	if len(args) == 0 {
		fmt.Fprintln(err, "media-core: missing command")
		Usage(err)
		return ExitUsage
	}
	if args[0] == "help" || args[0] == "-h" || args[0] == "--help" {
		Usage(out)
		return ExitOK
	}
	if runErr := runtime.Dispatch(args, out); runErr != nil {
		fmt.Fprintln(err, runErr)
		var reserved runtime.ErrReserved
		if errors.As(runErr, &reserved) {
			return ExitError
		}
		var exitErr *runtime.ExitCodeError
		if errors.As(runErr, &exitErr) {
			return exitErr.Exit
		}
		Usage(err)
		return ExitUsage
	}
	return ExitOK
}

// Main is the process entry point.
func Main() int {
	return Run(os.Args[1:], os.Stdout, os.Stderr)
}

// VersionLine exposes the rendered build identity for adapters and tests.
func VersionLine() string {
	return buildinfo.Get().Render()
}
