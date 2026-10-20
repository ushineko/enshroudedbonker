/*
Package cli assembles the enshroudedbonker command tree.

Every command does the same three things: build a core request from flags, call
one core operation, and print the result. No command reaches past core, because
the window cannot either, and the two front ends may disagree only about
presentation (project rule: CLI/GUI parity).
*/
package cli

import (
	"encoding/json"
	"fmt"
	"io"
	"os"

	"github.com/spf13/cobra"

	"github.com/ushineko/enshroudedbonker/internal/buildinfo"
	"github.com/ushineko/enshroudedbonker/internal/core"
)

// UsageError marks a failure caused by how the command was invoked; main exits
// 2 for it, so a script can tell "typed it wrong" from "ran and failed".
type UsageError struct{ Err error }

func (e *UsageError) Error() string { return e.Err.Error() }
func (e *UsageError) Unwrap() error { return e.Err }

func usagef(format string, args ...any) error {
	return &UsageError{Err: fmt.Errorf(format, args...)}
}

type globalFlags struct {
	configPath string
	gameDir    string
	verbose    bool
}

// global holds the parsed persistent flags; cobra's binding needs a stable
// address.
//
//nolint:gochecknoglobals // cobra's flag binding requires a stable address
var global globalFlags

func request() core.Request {
	return core.Request{ConfigPath: global.configPath, GameDir: global.gameDir, Events: events()}
}

// events routes core's log lines to stderr. Debug lines (emm's own output) only
// under -v; the window is where progress is drawn.
func events() core.Events {
	return core.Events{
		Log: func(level core.Level, msg string) {
			if level == core.LevelDebug && !global.verbose {
				return
			}
			say(os.Stderr, "enshroudedbonker: %s: %s", level, msg)
		},
	}
}

func writeJSON(w io.Writer, v any) error {
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	if err := enc.Encode(v); err != nil {
		return fmt.Errorf("encode result as JSON: %w", err)
	}
	return nil
}

// Root builds the command tree.
func Root() *cobra.Command {
	root := &cobra.Command{
		Use:   "enshroudedbonker",
		Short: "Build Enshrouded EML mods into the game, on Linux",
		Long: "enshroudedbonker builds EML Lua mods -- its own tweaks and ones you import -- into\n" +
			"Enshrouded's archive with upstream's emm, then deploys and rolls back the result.",
		Version:       fmt.Sprintf("%s (%s)", buildinfo.Version, buildinfo.Commit),
		SilenceUsage:  true,
		SilenceErrors: true,
		Args:          cobra.ArbitraryArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) == 0 {
				return cmd.Help() //nolint:wrapcheck // cobra's own writer error
			}
			return usagef("unknown command %q for %q", args[0], cmd.CommandPath())
		},
	}
	root.SetFlagErrorFunc(func(_ *cobra.Command, err error) error { return &UsageError{Err: err} })
	pf := root.PersistentFlags()
	pf.StringVar(&global.configPath, "config", "",
		"settings file (default $XDG_CONFIG_HOME/enshroudedbonker/config.json)")
	pf.StringVar(&global.gameDir, "game-dir", "",
		"game directory, overriding the setting and $ENSHROUDEDBONKER_GAME_DIR")
	pf.BoolVarP(&global.verbose, "verbose", "v", false, "also print emm's own output")

	root.AddCommand(
		newStatusCmd(), newDetectCmd(),
		newToolsCmd(), newCompatCmd(), newPristineCmd(),
		newTweaksCmd(), newModsCmd(),
		newBuildCmd(), newDeployCmd(), newRollbackCmd(), newReportCmd(),
		newConfigCmd(), newVersionCmd(),
	)
	return root
}

func group(use, short string) *cobra.Command {
	return &cobra.Command{
		Use: use, Short: short, Args: cobra.ArbitraryArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) == 0 {
				return cmd.Help() //nolint:wrapcheck // cobra's own writer error
			}
			return usagef("unknown command %q for %q", args[0], cmd.CommandPath())
		},
	}
}

func exactArgs(n int) cobra.PositionalArgs {
	return func(cmd *cobra.Command, args []string) error {
		if len(args) != n {
			return usagef("%s takes exactly %d argument(s), got %d\n\nUsage:\n  %s",
				cmd.CommandPath(), n, len(args), cmd.UseLine())
		}
		return nil
	}
}

func noArgs() cobra.PositionalArgs {
	return func(cmd *cobra.Command, args []string) error {
		if len(args) > 0 {
			return usagef("unknown command %q for %q", args[0], cmd.CommandPath())
		}
		return nil
	}
}

func rangeArgs(lo, hi int) cobra.PositionalArgs {
	return func(cmd *cobra.Command, args []string) error {
		if len(args) < lo || len(args) > hi {
			return usagef("%s takes %d to %d argument(s), got %d\n\nUsage:\n  %s",
				cmd.CommandPath(), lo, hi, len(args), cmd.UseLine())
		}
		return nil
	}
}

func minArgs(n int) cobra.PositionalArgs {
	return func(cmd *cobra.Command, args []string) error {
		if len(args) < n {
			return usagef("%s takes at least %d argument(s)\n\nUsage:\n  %s",
				cmd.CommandPath(), n, cmd.UseLine())
		}
		return nil
	}
}

func newVersionCmd() *cobra.Command {
	return &cobra.Command{
		Use: "version", Short: "Print the version and commit this binary was built from", Args: noArgs(),
		RunE: func(cmd *cobra.Command, _ []string) error {
			say(cmd.OutOrStdout(), "enshroudedbonker %s (%s)", buildinfo.Version, buildinfo.Commit)
			return nil
		},
	}
}
