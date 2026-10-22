/*
Command enshroudedbonker builds Enshrouded EML mods into the game archive.

This binary is built CGO_ENABLED=0. It shells out to exactly one external
program, upstream's emm, which on Linux runs under Wine.
*/
package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/ushineko/enshroudedbonker/internal/cli"
)

func main() {
	// Ctrl-C and SIGTERM cancel the running operation, so emm and its Wine
	// prefix are stopped rather than orphaned.
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	err := cli.Root().ExecuteContext(ctx)
	stop()
	if err != nil {
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
