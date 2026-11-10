// Package buildinfo carries the build identity of the media-core binary.
package buildinfo

import (
	"fmt"
	"runtime"
)

// Identity fields keep truthful development defaults and are meant to be
// overridden at link time, for example:
//
//	go build -ldflags "-X github.com/HaidaDaniel/media-core/internal/buildinfo.Version=1.2.3 \
//	  -X github.com/HaidaDaniel/media-core/internal/buildinfo.Commit=$(git rev-parse --short HEAD) \
//	  -X github.com/HaidaDaniel/media-core/internal/buildinfo.BuildTime=$(date -u +%Y-%m-%dT%H:%M:%SZ)" \
//	  ./cmd/media-core
var (
	Version   = "dev"
	Commit    = "unknown"
	BuildTime = "unknown"
)

// Info is a snapshot of the build identity.
type Info struct {
	Version   string
	Commit    string
	BuildTime string
	GoVersion string
}

// Get returns the current build identity.
func Get() Info {
	return Info{
		Version:   Version,
		Commit:    Commit,
		BuildTime: BuildTime,
		GoVersion: runtime.Version(),
	}
}

// Render returns one line of human-readable build identity.
func (i Info) Render() string {
	return fmt.Sprintf("media-core %s (commit %s, built %s, go %s)", i.Version, i.Commit, i.BuildTime, i.GoVersion)
}
