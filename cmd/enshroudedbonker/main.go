/*
Command enshroudedbonker builds Enshrouded EML mods into the game archive.

This binary is built CGO_ENABLED=0. It shells out to exactly one external
program, upstream's emm, and to git and cargo when asked to build it.
*/
package main

import (
	"errors"
	"fmt"
	"os"

	"github.com/ushineko/enshroudedbonker/internal/cli"
)

func main() {
	if err := cli.Root().Execute(); err != nil {
		// The command tree silences cobra's own reporting so this is the single
		// place a failure is printed; leaving it on prints every failure twice.
		fmt.Fprintln(os.Stderr, "enshroudedbonker:", err)
		var usage *cli.UsageError
		if errors.As(err, &usage) {
			// A misuse of the command line is exit 2, distinct from a
			// command that ran and failed, so scripts can tell them apart.
			os.Exit(2)
		}
		os.Exit(1)
	}
}
