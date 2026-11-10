// Command media-core is the entry point for the Media Core binary.
package main

import (
	"os"

	"github.com/HaidaDaniel/media-core/internal/cli"
)

func main() {
	os.Exit(cli.Main())
}
