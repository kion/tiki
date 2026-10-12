package main

import (
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"github.com/boolean-maybe/tiki/config"
	"github.com/boolean-maybe/tiki/document"
	"github.com/boolean-maybe/tiki/internal/livestatus"
)

func runLive(args []string) int {
	return runLiveIn(config.GetDocDir(), args, os.Stdout, os.Stderr, time.Now())
}

func runLiveIn(root string, args []string, stdout, stderr io.Writer, now time.Time) int {
	if len(args) == 0 {
		return liveUsageError(stderr, errors.New("missing command"))
	}
	switch args[0] {
	case "-h", "--help":
		printLiveUsage(stdout)
		return exitOK
	case "set":
		return liveSet(root, args[1:], stdout, stderr, now)
	case "clear":
		return liveClear(root, args[1:], stdout, stderr)
	default:
		return liveUsageError(stderr, fmt.Errorf("unknown live command %q", args[0]))
	}
}

// nil means the flag wasn't given, so the stored value is kept.
type liveSetArgs struct {
	id      string
	message *string
	agent   *string
}

func parseLiveSetArgs(args []string) (liveSetArgs, error) {
	var out liveSetArgs
	flags := map[string]**string{
		"--message": &out.message,
		"--agent":   &out.agent,
	}
	for i := 0; i < len(args); i++ {
		arg := args[i]
		if arg == "-h" || arg == "--help" {
			return liveSetArgs{}, errHelpRequested
		}
		if !strings.HasPrefix(arg, "--") {
			if out.id != "" {
				return liveSetArgs{}, fmt.Errorf("more than one tiki id: %q and %q", out.id, arg)
			}
			out.id = arg
			continue
		}
		name, value, inline := strings.Cut(arg, "=")
		target, ok := flags[name]
		if !ok {
			return liveSetArgs{}, fmt.Errorf("unknown flag %s", name)
		}
		if !inline {
			i++
			if i >= len(args) {
				return liveSetArgs{}, fmt.Errorf("%s needs a value", name)
			}
			value = args[i]
		}
		*target = &value
	}
	if out.id == "" {
		return liveSetArgs{}, errors.New("missing tiki id")
	}
	if out.message == nil {
		return liveSetArgs{}, errors.New("--message is required")
	}
	return out, nil
}

func liveSet(root string, args []string, stdout, stderr io.Writer, now time.Time) int {
	a, err := parseLiveSetArgs(args)
	if errors.Is(err, errHelpRequested) {
		printLiveUsage(stdout)
		return exitOK
	}
	if err != nil {
		return liveUsageError(stderr, err)
	}
	id := document.NormalizeID(a.id)
	if err := document.ValidateID(id); err != nil {
		return liveInputError(stderr, err)
	}

	// A malformed status is replaced rather than blocking the agent's update.
	st, _, err := livestatus.Read(root, id)
	if err != nil {
		st = livestatus.Status{}
	}
	if err := applyLiveSetArgs(&st, a); err != nil {
		return liveInputError(stderr, err)
	}
	st.UpdatedAt = now

	_, found, err := document.FindIDPath(root, id)
	if err != nil {
		_, _ = fmt.Fprintln(stderr, "error:", err)
		return exitInternal
	}
	if !found {
		return liveInputError(stderr, fmt.Errorf("no tiki with id %s under %s", id, root))
	}
	if err := livestatus.Write(root, id, st); err != nil {
		_, _ = fmt.Fprintln(stderr, "error:", err)
		return exitInternal
	}
	return exitOK
}

func applyLiveSetArgs(st *livestatus.Status, a liveSetArgs) error {
	message := strings.TrimSpace(*a.message)
	if err := livestatus.ValidateMessage(message); err != nil {
		return err
	}
	st.Message = message
	if a.agent != nil {
		agent := strings.TrimSpace(*a.agent)
		if err := livestatus.ValidateAgent(agent); err != nil {
			return err
		}
		st.Agent = agent
	}
	return nil
}

// clear checks only the id's format,
// so a status left behind by a deleted tiki can still be removed.
func liveClear(root string, args []string, stdout, stderr io.Writer) int {
	if len(args) != 1 {
		return liveUsageError(stderr, errors.New("clear takes exactly one tiki id"))
	}
	arg := args[0]
	if arg == "-h" || arg == "--help" {
		printLiveUsage(stdout)
		return exitOK
	}
	id := document.NormalizeID(arg)
	if err := document.ValidateID(id); err != nil {
		return liveInputError(stderr, err)
	}
	if err := livestatus.Remove(root, id); err != nil {
		_, _ = fmt.Fprintln(stderr, "error:", err)
		return exitInternal
	}
	return exitOK
}

func liveUsageError(stderr io.Writer, err error) int {
	_, _ = fmt.Fprintln(stderr, "error:", err)
	printLiveUsage(stderr)
	return exitUsage
}

func liveInputError(stderr io.Writer, err error) int {
	_, _ = fmt.Fprintln(stderr, "error:", err)
	return exitUsage
}

func printLiveUsage(w io.Writer) {
	_, _ = fmt.Fprint(w, `Usage: tiki live set <ID> --message <text> [--agent <text>]
       tiki live clear <ID>

Publish what an agent is doing on a tiki. Boards show it in the tiki's card
border while live reload is on. It is stored in .tiki/live/<ID>.yaml, which
git ignores; the tiki file itself is never changed.

Options for set:
  --message <text>  What is happening now, in 1-5 words (required)
  --agent <text>    Who is working: a glyph and/or a short name

--agent is kept between updates, so later updates need only --message.
An empty value (--agent=) clears it.

Examples:
  tiki live set X7F4K2 --agent "✻ Claude" --message "Researching"
  tiki live set X7F4K2 --message "Implementing parser"
  tiki live clear X7F4K2
`)
}

// run() commands find tiki through this,
// so a trigger can't reach an older tiki that happens to be first on PATH.
const tikiBinEnv = "TIKI_BIN"

func exportTikiBin() {
	if os.Getenv(tikiBinEnv) != "" {
		return
	}
	if exe, err := os.Executable(); err == nil {
		_ = os.Setenv(tikiBinEnv, exe)
	}
}
