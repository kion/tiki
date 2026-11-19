package main

import (
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
	"text/tabwriter"

	"github.com/boolean-maybe/tiki/config"
)

// runConfig implements `tiki config`: which config and workflow files tiki uses here, and why others are ignored.
// It only reads, so it never seeds the default workflow.
func runConfig(args []string) int {
	positional, err := parsePositionalOnly(args)
	if errors.Is(err, errHelpRequested) {
		printConfigUsage()
		return exitOK
	}
	if err == nil && positional != "" {
		err = fmt.Errorf("unexpected argument: %q", positional)
	}
	if err != nil {
		_, _ = fmt.Fprintln(os.Stderr, "error:", err)
		printConfigUsage()
		return exitUsage
	}

	if err := writeConfigReport(os.Stdout, config.ResolveFiles(config.KindConfig), config.ResolveFiles(config.KindWorkflow)); err != nil {
		_, _ = fmt.Fprintln(os.Stderr, "error:", err)
		return exitInternal
	}
	return exitOK
}

func writeConfigReport(w io.Writer, cfg, wf config.Resolution) error {
	var b strings.Builder
	b.WriteString("config\n")
	writeResolutionRows(&b, cfg, false)
	if cfg.Local != nil && cfg.Local.Chosen != "" {
		if keys := config.PersonalKeysIn(cfg.Local.Chosen); len(keys) > 0 {
			fmt.Fprintf(&b, "  note: %s in %s apply to everyone who uses this project\n", strings.Join(keys, ", "), config.StripControl(cfg.Local.Chosen))
		}
	}
	b.WriteString("\nworkflow\n")
	writeResolutionRows(&b, wf, true)
	_, err := io.WriteString(w, b.String())
	return err
}

// writeResolutionRows lists each folder's candidates.
// A workflow doesn't merge, so a local one makes the global one inactive.
func writeResolutionRows(b *strings.Builder, r config.Resolution, singleWinner bool) {
	tw := tabwriter.NewWriter(b, 0, 0, 2, ' ', 0)
	localWins := r.Local != nil && r.Local.Chosen != ""
	for _, c := range r.Global.Candidates {
		status := candidateStatus(c)
		if singleWinner && localWins && c.Status == config.StatusLoaded {
			status = "overridden by local"
		}
		_, _ = fmt.Fprintf(tw, "  global\t%s\t%s\n", config.StripControl(c.Path), status)
	}
	if r.Local == nil {
		_, _ = fmt.Fprintf(tw, "  local\t%s\t%s\n", config.StripControl(r.Global.Dir), "same folder as global")
	}
	if r.Local != nil {
		for _, c := range r.Local.Candidates {
			_, _ = fmt.Fprintf(tw, "  local\t%s\t%s\n", config.StripControl(c.Path), candidateStatus(c))
		}
	}
	_ = tw.Flush()
}

func candidateStatus(c config.Candidate) string {
	if c.Reason == "" {
		return string(c.Status)
	}
	return string(c.Status) + ": " + config.StripControl(c.Reason)
}

func printConfigUsage() {
	fmt.Print(`Usage: tiki config

Show which config and workflow files tiki uses in the current directory,
and why any other file there was ignored or skipped.

In each folder (global: your user config directory, local: the current directory)
tiki reads tiki-config.yaml and tiki-workflow.yaml, or else config.yaml and
workflow.yaml when they are recognised as tiki's. The local config merges over
the global one; a local workflow replaces the global one.
`)
}
