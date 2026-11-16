// Package webassets embeds the staged production build of the web UI so
// the single media-core binary serves the SPA same-origin with no runtime
// Node or separate static server. The tracked dist holds a placeholder
// index.html so the module always compiles; scripts/build-release.sh
// replaces the staging with the real Vite build before linking the release
// binary, and the staged files other than the placeholder are git-ignored.
package webassets

import (
	"embed"
	"io/fs"
)

// dist is the staged production build. The all: prefix keeps dotfiles so a
// build that emits hidden entries is never silently dropped.
//
//go:embed all:dist
var dist embed.FS

// FS is the staged build rooted at its dist directory, so the serving
// handler sees index.html and assets/ at the root. It is computed once;
// the embedded FS is immutable.
var FS fs.FS = subOrPanic()

func subOrPanic() fs.FS {
	sub, err := fs.Sub(dist, "dist")
	if err != nil {
		panic("webassets: " + err.Error())
	}
	return sub
}
