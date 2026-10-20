package cli

import (
	"fmt"
	"io"
	"strconv"
	"strings"

	"github.com/spf13/cobra"

	"github.com/ushineko/enshroudedbonker/internal/core"
	"github.com/ushineko/enshroudedbonker/internal/tweaks"
)

// jsonCmd wires the --json flag every listing takes.
func jsonCmd(cmd *cobra.Command, asJSON *bool) *cobra.Command {
	cmd.Flags().BoolVar(asJSON, "json", false, "print the result as JSON")
	return cmd
}

func newStatusCmd() *cobra.Command {
	var asJSON bool
	return jsonCmd(&cobra.Command{
		Use: "status", Short: "Report the game, its build, the tools and what is deployed", Args: noArgs(),
		RunE: func(cmd *cobra.Command, _ []string) error {
			res, err := core.Status(cmd.Context(), core.StatusRequest{Request: request()})
			if err != nil {
				return err
			}
			if asJSON {
				return writeJSON(cmd.OutOrStdout(), res)
			}
			printStatus(cmd.OutOrStdout(), res)
			return nil
		},
	}, &asJSON)
}

func printStatus(w io.Writer, r core.StatusResult) {
	fact(w, "enshroudedbonker", fmt.Sprintf("%s (%s)", r.Version, r.Commit))
	fact(w, "config", r.ConfigPath)
	fact(w, "workspace", r.Paths.Workspace)
	if r.Emm.Installed {
		fact(w, "emm", fmt.Sprintf("%s (%s)", r.Emm.Version, r.Emm.Source))
	} else {
		fact(w, "emm", "not installed: "+r.Emm.Error)
	}
	say(w, "")
	if !r.Install.Found {
		fact(w, "game", "not found ("+r.Install.Error+")")
		say(w, "Run `enshroudedbonker detect` to see every place that was examined.")
		return
	}
	fact(w, "game dir", r.Install.Dir)
	fact(w, "found by", r.Install.Source)
	fact(w, "steam build", r.Install.BuildID)
	if r.Fingerprint != nil {
		fact(w, "game build", r.Fingerprint.Short())
	}
	fact(w, "compatibility", r.Gate.Verdict+": "+r.Gate.Reason)
	if r.Install.RunningKnown {
		fact(w, "game running", r.Install.Running)
	}
	switch {
	case r.Pristine.Present:
		fact(w, "pristine copy", r.Pristine.Source+", "+r.Pristine.Captured)
	case len(r.Pristine.Patched) > 0:
		fact(w, "pristine copy", "none, and the install looks patched ("+strings.Join(r.Pristine.Patched, "; ")+
			"); restore it or use `pristine import <dir>`")
	default:
		fact(w, "pristine copy", "none yet; run `enshroudedbonker pristine capture`")
	}
	fact(w, "install state", r.State)
	if r.Deployed != nil {
		fact(w, "deployed", fmt.Sprintf("%s: %s", r.Deployed.When.Format("2006-01-02 15:04"),
			strings.Join(r.Deployed.Mods, ", ")))
	}
	if r.LastBuild.Present {
		cur := ""
		if !r.LastBuild.Current {
			cur = " (for a different game build)"
		}
		fact(w, "last build", fmt.Sprintf("%s, %d patches, %s%s", r.LastBuild.Created, r.LastBuild.Applied,
			strings.Join(r.LastBuild.Mods, ", "), cur))
	}
	fact(w, "enabled", fmt.Sprintf("%d of %d built-in + %d library: %s", len(r.Enabled), r.Tweaks, r.LibraryMod,
		strings.Join(r.Enabled, ", ")))
}

func newDetectCmd() *cobra.Command {
	var asJSON bool
	return jsonCmd(&cobra.Command{
		Use: "detect", Short: "Show every place the game was looked for", Args: noArgs(),
		RunE: func(cmd *cobra.Command, _ []string) error {
			res, err := core.Detect(cmd.Context(), core.DetectRequest{Request: request()})
			if err != nil {
				return err
			}
			if asJSON {
				return writeJSON(cmd.OutOrStdout(), res)
			}
			t := table{}
			t.header("STEAM LIBRARY", "GAME DIR", "VERDICT")
			for _, c := range res.Candidates {
				lib := c.LibraryDir
				if lib == "" {
					lib = c.Root
				}
				verdict := "ok"
				if c.Reason != "" {
					verdict = c.Reason
				}
				t.row(lib, c.GameDir, verdict)
			}
			t.write(cmd.OutOrStdout())
			if !res.Found {
				return fmt.Errorf("%s", res.Error)
			}
			return nil
		},
	}, &asJSON)
}

func newToolsCmd() *cobra.Command {
	g := group("tools", "Build and inspect the external tools")
	g.AddCommand(&cobra.Command{
		Use: "build-emm", Short: "Build the pinned emm from source (needs git and cargo)", Args: noArgs(),
		RunE: func(cmd *cobra.Command, _ []string) error {
			res, err := core.BuildEmm(cmd.Context(), core.BuildEmmRequest{Request: request()})
			if err != nil {
				return err
			}
			fact(cmd.OutOrStdout(), "emm", res.Version)
			fact(cmd.OutOrStdout(), "installed", res.Path)
			return nil
		},
	})
	return g
}

func newCompatCmd() *cobra.Command {
	g := group("compat", "The schema gate: is this game build safe to patch")
	var asJSON bool
	g.AddCommand(jsonCmd(&cobra.Command{
		Use: "check", Short: "Dry-run every built-in against a scratch copy of the archive", Args: noArgs(),
		RunE: func(cmd *cobra.Command, _ []string) error {
			res, err := core.CompatCheck(cmd.Context(), core.CompatCheckRequest{Request: request()})
			if err != nil {
				return err
			}
			if asJSON {
				return writeJSON(cmd.OutOrStdout(), res)
			}
			t := table{}
			t.header("TWEAK", "RESULT", "DETAIL")
			for _, tw := range res.DryRun.Tweaks {
				verdict := "ok"
				if !tw.OK {
					verdict = "FAILED"
				}
				t.row(tw.Name, verdict, firstOf(tw.Problem, tw.Summary))
			}
			t.write(cmd.OutOrStdout())
			say(cmd.OutOrStdout(), "")
			fact(cmd.OutOrStdout(), "verdict", res.Decision.Verdict+": "+res.Decision.Reason)
			return nil
		},
	}, &asJSON))
	return g
}

func firstOf(a, b string) string {
	if a != "" {
		return a
	}
	return b
}

func newPristineCmd() *cobra.Command {
	g := group("pristine", "The vanilla copy of the archive every build starts from")
	g.AddCommand(&cobra.Command{
		Use: "capture", Short: "Copy the installed archive as the vanilla copy (refuses a patched one)",
		Args: noArgs(),
		RunE: func(cmd *cobra.Command, _ []string) error {
			res, err := core.PristineCapture(cmd.Context(), core.PristineCaptureRequest{Request: request()})
			if err != nil {
				return err
			}
			fact(cmd.OutOrStdout(), "pristine copy", res.Pristine.Dir)
			return nil
		},
	}, &cobra.Command{
		Use: "import <dir>", Short: "Take the vanilla copy from a backup holding enshrouded.kfc and .kfc_resources",
		Args: exactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			res, err := core.PristineImport(cmd.Context(), core.PristineImportRequest{Request: request(), Dir: args[0]})
			if err != nil {
				return err
			}
			fact(cmd.OutOrStdout(), "pristine copy", res.Pristine.Dir)
			return nil
		},
	})
	return g
}

func newTweaksCmd() *cobra.Command {
	g := group("tweaks", "The built-in mods and their parameters")
	var asJSON bool
	g.AddCommand(jsonCmd(&cobra.Command{
		Use: "list", Short: "List the built-in tweaks with their current values", Args: noArgs(),
		RunE: func(cmd *cobra.Command, _ []string) error {
			res, err := core.ListTweaks(cmd.Context(), core.ListTweaksRequest{Request: request()})
			if err != nil {
				return err
			}
			if asJSON {
				return writeJSON(cmd.OutOrStdout(), res)
			}
			w := cmd.OutOrStdout()
			for i, tw := range res.Tweaks {
				if i > 0 {
					say(w, "")
				}
				on := "off"
				if tw.Enabled {
					on = "ON"
				}
				say(w, "%s  [%s]  %s (%s)", tw.Name, on, tw.Title, tw.Group)
				if tw.Disabled != "" {
					say(w, "  unavailable: %s", tw.Disabled)
				}
				for _, p := range tw.Params {
					mark := ""
					if p.Overridden {
						mark = " *"
					}
					say(w, "  %-22s %s%s  (default %s, %s..%s)  %s", p.Name,
						tweaks.FormatValue(p.Current, p.Kind), mark, tweaks.FormatValue(p.Default, p.Kind),
						tweaks.FormatValue(p.Min, p.Kind), tweaks.FormatValue(p.Max, p.Kind), p.Label)
				}
			}
			return nil
		},
	}, &asJSON), &cobra.Command{
		Use: "set <tweak> <PARAM> <value>", Short: "Set a parameter (true/false for switches)", Args: exactArgs(3),
		RunE: func(cmd *cobra.Command, args []string) error {
			v, err := parseValue(args[2])
			if err != nil {
				return usagef("%s", err)
			}
			res, err := core.SetTweakParam(cmd.Context(), core.SetTweakParamRequest{Request: request(),
				Name: args[0], Param: args[1], Value: v})
			if err != nil {
				return err
			}
			msg := tweaks.FormatValue(res.New, res.Kind)
			if res.Clamped {
				msg += " (clamped to the allowed range)"
			}
			fact(cmd.OutOrStdout(), args[0]+" "+args[1], msg)
			return nil
		},
	}, &cobra.Command{
		Use: "reset <tweak> [PARAM]", Short: "Restore a parameter, or all of a tweak's, to the default",
		Args: rangeArgs(1, 2),
		RunE: func(cmd *cobra.Command, args []string) error {
			req := core.ResetTweakRequest{Request: request(), Name: args[0]}
			if len(args) == 2 {
				req.Param = args[1]
			}
			_, err := core.ResetTweak(cmd.Context(), req)
			return err
		},
	})
	return g
}

func parseValue(s string) (float64, error) {
	switch strings.ToLower(s) {
	case "true", "on", "yes":
		return 1, nil
	case "false", "off", "no":
		return 0, nil
	}
	v, err := strconv.ParseFloat(s, 64)
	if err != nil {
		return 0, fmt.Errorf("%q is not a number or true/false", s)
	}
	return v, nil
}

func newModsCmd() *cobra.Command {
	g := group("mods", "Built-in tweaks and library mods: what is in the build")
	var asJSON, replace bool
	toggle := func(on bool) *cobra.Command {
		verb := "disable"
		if on {
			verb = "enable"
		}
		return &cobra.Command{
			Use: verb + " <name>...", Short: strings.ToUpper(verb[:1]) + verb[1:] + " mods for the next build",
			Args: minArgs(1),
			RunE: func(cmd *cobra.Command, args []string) error {
				_, err := core.SetModsEnabled(cmd.Context(), core.SetModsEnabledRequest{Request: request(),
					Names: args, Enabled: on})
				return err
			},
		}
	}
	imp := &cobra.Command{
		Use: "import <dir|zip>", Short: "Copy a third-party EML mod into the library (starts disabled)",
		Args: exactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			res, err := core.ImportMod(cmd.Context(), core.ImportModRequest{Request: request(), Path: args[0],
				Replace: replace})
			if err != nil {
				return err
			}
			fact(cmd.OutOrStdout(), "imported", res.Mod.Name+" ("+res.Mod.Title+" "+res.Mod.Version+")")
			if res.Mod.Unsupported != "" {
				fact(cmd.OutOrStdout(), "cannot be built", res.Mod.Unsupported)
			}
			return nil
		},
	}
	imp.Flags().BoolVar(&replace, "replace", false, "overwrite a library mod with the same id")
	g.AddCommand(jsonCmd(&cobra.Command{
		Use: "list", Short: "List built-in and library mods and whether each is enabled", Args: noArgs(),
		RunE: func(cmd *cobra.Command, _ []string) error {
			res, err := core.ListMods(cmd.Context(), core.ListModsRequest{Request: request()})
			if err != nil {
				return err
			}
			if asJSON {
				return writeJSON(cmd.OutOrStdout(), res)
			}
			t := table{}
			t.header("MOD", "KIND", "ENABLED", "TITLE", "NOTE")
			for _, m := range res.Mods {
				kind := "library"
				if m.Builtin {
					kind = "built-in"
				}
				t.row(m.Name, kind, strconv.FormatBool(m.Enabled), m.Title, m.Unsupported)
			}
			t.write(cmd.OutOrStdout())
			for _, p := range res.Problems {
				say(cmd.ErrOrStderr(), "enshroudedbonker: warn: %s", p)
			}
			return nil
		},
	}, &asJSON), toggle(true), toggle(false), imp, &cobra.Command{
		Use: "remove <name>", Short: "Delete a library mod", Args: exactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			_, err := core.RemoveMod(cmd.Context(), core.RemoveModRequest{Request: request(), Name: args[0]})
			return err
		},
	})
	return g
}

func newBuildCmd() *cobra.Command {
	var force bool
	cmd := &cobra.Command{
		Use: "build", Short: "Build the enabled mods into a copy of the archive (the game is not touched)",
		Args: noArgs(),
		RunE: func(cmd *cobra.Command, _ []string) error {
			res, err := core.Build(cmd.Context(), core.BuildRequest{Request: request(), Force: force})
			if err != nil {
				return err
			}
			w := cmd.OutOrStdout()
			fact(w, "applied", fmt.Sprintf("%d patches", res.Manifest.Applied))
			for _, f := range res.Manifest.Files {
				fact(w, "  "+f.Name, fmt.Sprintf("%d bytes", f.Size))
			}
			fact(w, "output", res.Manifest.Dir)
			say(w, "Run `enshroudedbonker deploy` to install it (quit the game first).")
			return nil
		},
	}
	cmd.Flags().BoolVar(&force, "force", false, "build an unchecked game build without the dry run")
	return cmd
}

func newDeployCmd() *cobra.Command {
	return &cobra.Command{
		Use: "deploy", Short: "Install the last build into the game (quit the game first)", Args: noArgs(),
		RunE: func(cmd *cobra.Command, _ []string) error {
			res, err := core.Deploy(cmd.Context(), core.DeployRequest{Request: request()})
			if err != nil {
				return err
			}
			w := cmd.OutOrStdout()
			fact(w, "deployed", strings.Join(res.Deployed.Mods, ", "))
			if res.Deployed.Archive != "" {
				fact(w, "kept a copy of", res.Deployed.Archive)
			}
			say(w, "Stacks and chest sizes stay in your save even after a rollback.")
			return nil
		},
	}
}

func newRollbackCmd() *cobra.Command {
	return &cobra.Command{
		Use: "rollback", Short: "Put the vanilla archive back (quit the game first)", Args: noArgs(),
		RunE: func(cmd *cobra.Command, _ []string) error {
			res, err := core.Rollback(cmd.Context(), core.RollbackRequest{Request: request()})
			if err != nil {
				return err
			}
			fact(cmd.OutOrStdout(), "restored", strings.Join(res.Restored, ", "))
			if len(res.Removed) > 0 {
				fact(cmd.OutOrStdout(), "removed", strings.Join(res.Removed, ", "))
			}
			return nil
		},
	}
}

func newReportCmd() *cobra.Command {
	var asJSON bool
	return jsonCmd(&cobra.Command{
		Use: "report", Short: "Show what the last build did", Args: noArgs(),
		RunE: func(cmd *cobra.Command, _ []string) error {
			res, err := core.Report(cmd.Context(), core.ReportRequest{Request: request()})
			if err != nil {
				return err
			}
			if asJSON {
				return writeJSON(cmd.OutOrStdout(), res)
			}
			w, m := cmd.OutOrStdout(), res.Manifest
			fact(w, "built", m.Created.Format("2006-01-02 15:04")+" for "+m.Fingerprint.Short())
			fact(w, "applied", fmt.Sprintf("%d patches", m.Applied))
			fact(w, "emm order", strings.Join(m.Order, ", "))
			for _, mo := range m.Mods {
				say(w, "")
				say(w, "%s", mo.Name)
				for _, l := range mo.Lines {
					say(w, "  %s", l)
				}
			}
			for _, o := range m.Overlaps {
				say(w, "check: %s", o)
			}
			return nil
		},
	}, &asJSON)
}

func newConfigCmd() *cobra.Command {
	g := group("config", "Read and change settings")
	var asJSON bool
	g.AddCommand(jsonCmd(&cobra.Command{
		Use: "show", Short: "Print the settings", Args: noArgs(),
		RunE: func(cmd *cobra.Command, _ []string) error {
			res, err := core.ConfigShow(cmd.Context(), core.ConfigShowRequest{Request: request()})
			if err != nil {
				return err
			}
			if asJSON {
				return writeJSON(cmd.OutOrStdout(), res)
			}
			fact(cmd.OutOrStdout(), "file", res.Path)
			for _, k := range res.Keys {
				fact(cmd.OutOrStdout(), k, res.Settings[k])
			}
			return nil
		},
	}, &asJSON), &cobra.Command{
		Use: "set <key> <value>", Short: "Change a setting (empty value restores the default)", Args: exactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			res, err := core.ConfigSet(cmd.Context(), core.ConfigSetRequest{Request: request(), Key: args[0],
				Value: args[1]})
			if err != nil {
				return err
			}
			fact(cmd.OutOrStdout(), res.Key, res.Value)
			return nil
		},
	})
	return g
}
