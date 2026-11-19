package main

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/boolean-maybe/tiki/config"
	"github.com/boolean-maybe/tiki/document"
	"github.com/boolean-maybe/tiki/internal/bootstrap"
	"github.com/boolean-maybe/tiki/store/tikistore"
)

func runIDs(args []string) int {
	// usage problems are answered before config loading, which would try to read --help as a flag
	if _, err := parseIDsArgs(args); err != nil {
		return runIDsIn("", args, os.Stdout, os.Stderr)
	}
	cfg, err := bootstrap.LoadConfig()
	if err != nil {
		_, _ = fmt.Fprintf(os.Stderr, "error: load config: %v\n", err)
		return exitStartupFailure
	}
	bootstrap.InitCLILogging(cfg)
	if name := config.GetStoreName(); name != "tiki" {
		_, _ = fmt.Fprintf(os.Stderr, "error: unknown store backend: %q (supported: tiki)\n", name)
		return exitStartupFailure
	}
	if err := config.InstallDefaultWorkflow(); err != nil {
		_, _ = fmt.Fprintf(os.Stderr, "warning: install default workflow: %v\n", err)
	}
	return runIDsIn(config.GetDocDir(), args, os.Stdout, os.Stderr)
}

type idsCommand struct {
	name     string
	old, new string
	dryRun   bool
}

func parseIDsArgs(args []string) (idsCommand, error) {
	var cmd idsCommand
	var positional []string
	for _, a := range args {
		switch a {
		case "--help", "-h":
			return cmd, errHelpRequested
		case "--dry-run":
			cmd.dryRun = true
		default:
			positional = append(positional, a)
		}
	}
	if len(positional) == 0 {
		return cmd, errors.New("missing command")
	}
	cmd.name = positional[0]
	switch cmd.name {
	case "rename":
		if len(positional) != 3 {
			return cmd, errors.New("rename takes an old and a new id")
		}
		cmd.old, cmd.new = document.NormalizeID(positional[1]), document.NormalizeID(positional[2])
	case "migrate", "counter":
		if len(positional) != 1 {
			return cmd, fmt.Errorf("%s takes no arguments", cmd.name)
		}
	default:
		return cmd, fmt.Errorf("unknown ids command: %s", cmd.name)
	}
	return cmd, nil
}

func runIDsIn(root string, args []string, stdout, stderr io.Writer) int {
	cmd, err := parseIDsArgs(args)
	if errors.Is(err, errHelpRequested) {
		printIDsUsage(stdout)
		return exitOK
	}
	if err != nil {
		_, _ = fmt.Fprintln(stderr, "error:", err)
		printIDsUsage(stderr)
		return exitUsage
	}

	if err := config.LoadWorkflowFields(); err != nil {
		_, _ = fmt.Fprintf(stderr, "error: load workflow registries: %v\n", err)
		return exitStartupFailure
	}
	s, err := tikistore.NewTikiStore(root)
	if err != nil {
		_, _ = fmt.Fprintf(stderr, "error: initialize store: %v\n", err)
		return exitStartupFailure
	}

	var renames []tikistore.Rename
	switch cmd.name {
	case "counter":
		return printCounterFile(s, stdout, stderr)
	case "rename":
		if err := s.CheckRename(cmd.old, cmd.new); err != nil {
			_, _ = fmt.Fprintln(stderr, "error:", err)
			return exitQueryError
		}
		renames = []tikistore.Rename{{Old: cmd.old, New: cmd.new}}
	case "migrate":
		var missing []tikistore.MissingPrefix
		renames, missing, err = s.PlanMigration()
		if errors.Is(err, tikistore.ErrNoProjectPrefix) {
			_, _ = fmt.Fprintln(stderr, "error:", err)
			return exitUsage
		}
		if err != nil {
			_, _ = fmt.Fprintln(stderr, "error:", err)
			return exitQueryError
		}
		if len(missing) > 0 {
			printMissingPrefixes(stderr, missing)
			return exitQueryError
		}
		if len(renames) == 0 {
			// still run: references a failed earlier run left behind get healed
			if changed, err := s.ApplyRenames(nil, true); err == nil && len(changed) == 0 {
				_, _ = fmt.Fprintln(stdout, "nothing to migrate: every tiki already has a prefixed id")
				return exitOK
			}
		}
	}

	changed, err := s.ApplyRenames(renames, cmd.dryRun)
	if err != nil {
		_, _ = fmt.Fprintln(stderr, "error:", err)
		return exitQueryError
	}
	for _, r := range renames {
		_, _ = fmt.Fprintf(stdout, "%s -> %s\n", r.Old, r.New)
	}
	verb := "updated"
	if cmd.dryRun {
		verb = "would update"
	}
	_, _ = fmt.Fprintf(stdout, "%s %d files:\n", verb, len(changed))
	for _, path := range changed {
		if rel, err := filepath.Rel(root, path); err == nil {
			path = rel
		}
		_, _ = fmt.Fprintln(stdout, "  "+path)
	}
	if cmd.dryRun {
		_, _ = fmt.Fprintln(stdout, "dry run: nothing written")
	}
	return exitOK
}

func printMissingPrefixes(w io.Writer, missing []tikistore.MissingPrefix) {
	_, _ = fmt.Fprintln(w, "error: these tikis have no value in a prefix field, and prefix fields can't be filled once the id is prefixed:")
	for _, m := range missing {
		_, _ = fmt.Fprintf(w, "  %s: %s\n", m.ID, strings.Join(m.Fields, ", "))
	}
	_, _ = fmt.Fprintln(w, "Fill them first (tiki workflow schema lists the values), for example:")
	_, _ = fmt.Fprintln(w, "  "+tikistore.FillExample(missing[0].ID, missing[0].Fields[0]))
	_, _ = fmt.Fprintln(w, "then run tiki ids migrate again.")
}

func printCounterFile(s *tikistore.TikiStore, stdout, stderr io.Writer) int {
	path, counters, exists, err := s.CounterFile()
	if err != nil {
		_, _ = fmt.Fprintln(stderr, "error:", err)
		return exitQueryError
	}
	if !exists {
		_, _ = fmt.Fprintf(stdout, "counter file: %s (not created yet: the first prefixed id creates it)\n", path)
		return exitOK
	}
	_, _ = fmt.Fprintf(stdout, "counter file: %s\n", path)
	prefixes := make([]string, 0, len(counters))
	for prefix := range counters {
		prefixes = append(prefixes, prefix)
	}
	sort.Strings(prefixes)
	for _, prefix := range prefixes {
		_, _ = fmt.Fprintf(stdout, "  %s: %d\n", prefix, counters[prefix])
	}
	return exitOK
}

func printIDsUsage(w io.Writer) {
	_, _ = fmt.Fprint(w, `Usage: tiki ids <command> [--dry-run]

Commands:
  rename <OLD> <NEW>   Give a tiki a new id
  migrate              Give every tiki with a 6-character id a prefixed id, oldest first
  counter              Show this project's counter file and its numbers

Both rewrite every reference: id-list fields, and [[ID]] and [text](ID) links in all
Markdown files, wiki pages included. Plain-text mentions are left alone.
The old id is kept in the tiki's formerIds. --dry-run prints the changes without writing.

Examples:
  tiki ids migrate --dry-run
  tiki ids rename X7F4K2 FEAT-12
`)
}
