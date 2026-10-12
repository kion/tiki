package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/boolean-maybe/tiki/internal/livestatus"
)

var liveNow = time.Date(2026, 10, 11, 18, 22, 5, 0, time.UTC)

func newLiveRoot(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	tiki := "---\nid: X7F4K2\ntitle: Status animation\n---\nbody\n"
	if err := os.WriteFile(filepath.Join(root, "status-animation.md"), []byte(tiki), 0o600); err != nil {
		t.Fatal(err)
	}
	return root
}

func runLiveTest(t *testing.T, root string, now time.Time, args ...string) (int, string) {
	t.Helper()
	var stdout, stderr bytes.Buffer
	code := runLiveIn(root, args, &stdout, &stderr, now)
	return code, stderr.String()
}

func readLive(t *testing.T, root string) livestatus.Status {
	t.Helper()
	st, ok, err := livestatus.Read(root, "X7F4K2")
	if err != nil || !ok {
		t.Fatalf("live status for X7F4K2: ok=%v err=%v", ok, err)
	}
	return st
}

func TestLiveSet_WritesTheStatus(t *testing.T) {
	root := newLiveRoot(t)
	code, stderr := runLiveTest(t, root, liveNow, "set", "X7F4K2",
		"--message", "Researching", "--agent", "✻ Claude")
	if code != exitOK {
		t.Fatalf("exit %d, stderr %q", code, stderr)
	}
	st := readLive(t, root)
	if st.Message != "Researching" || st.Agent != "✻ Claude" || !st.UpdatedAt.Equal(liveNow) {
		t.Fatalf("status = %+v", st)
	}
}

func TestLiveSet_KeepsTheAgentBetweenUpdates(t *testing.T) {
	root := newLiveRoot(t)
	runLiveTest(t, root, liveNow, "set", "X7F4K2", "--message", "Researching", "--agent", "✻ Claude")

	later := liveNow.Add(3 * time.Minute)
	if code, stderr := runLiveTest(t, root, later, "set", "X7F4K2", "--message", "Implementing"); code != exitOK {
		t.Fatalf("exit %d, stderr %q", code, stderr)
	}
	st := readLive(t, root)
	if st.Message != "Implementing" || st.Agent != "✻ Claude" || !st.UpdatedAt.Equal(later) {
		t.Fatalf("status = %+v", st)
	}
}

func TestLiveSet_EmptyAgentClearsIt(t *testing.T) {
	root := newLiveRoot(t)
	runLiveTest(t, root, liveNow, "set", "X7F4K2", "--message", "Researching", "--agent", "✻ Claude")
	runLiveTest(t, root, liveNow, "set", "X7F4K2", "--message", "Researching", "--agent=")
	if st := readLive(t, root); st.Agent != "" {
		t.Fatalf("status = %+v, want the agent cleared", st)
	}
}

func TestLiveSet_NormalizesTheID(t *testing.T) {
	root := newLiveRoot(t)
	if code, stderr := runLiveTest(t, root, liveNow, "set", "x7f4k2", "--message", "Researching"); code != exitOK {
		t.Fatalf("exit %d, stderr %q", code, stderr)
	}
	readLive(t, root)
}

func TestLiveSet_ReplacesAMalformedStatus(t *testing.T) {
	root := newLiveRoot(t)
	if err := os.MkdirAll(livestatus.Dir(root), 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(livestatus.Path(root, "X7F4K2"), []byte("message: [unclosed\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if code, stderr := runLiveTest(t, root, liveNow, "set", "X7F4K2", "--message", "Researching"); code != exitOK {
		t.Fatalf("exit %d, stderr %q", code, stderr)
	}
	if st := readLive(t, root); st.Message != "Researching" {
		t.Fatalf("status = %+v", st)
	}
}

func TestLiveSet_RejectsBadInput(t *testing.T) {
	tests := []struct {
		name string
		args []string
		want string
	}{
		{name: "unknown tiki", args: []string{"set", "QQQQQQ", "--message", "x"}, want: "QQQQQQ"},
		{name: "malformed id", args: []string{"set", "TASK-1", "--message", "x"}, want: "TASK-1"},
		{name: "missing message", args: []string{"set", "X7F4K2"}, want: "message"},
		{name: "missing id", args: []string{"set", "--message", "x"}, want: "id"},
		{name: "frames are tiki's own", args: []string{"set", "X7F4K2", "--message", "x", "--frames", "·✢"}, want: "--frames"},
		{name: "so is the interval", args: []string{"set", "X7F4K2", "--message", "x", "--interval", "80ms"}, want: "--interval"},
		{name: "long agent", args: []string{"set", "X7F4K2", "--message", "x", "--agent", strings.Repeat("a", 17)}, want: "agent"},
		{name: "unknown flag", args: []string{"set", "X7F4K2", "--message", "x", "--color", "red"}, want: "--color"},
		{name: "flag without value", args: []string{"set", "X7F4K2", "--message"}, want: "--message"},
		{name: "unknown subcommand", args: []string{"show", "X7F4K2"}, want: "show"},
		{name: "no subcommand", args: nil, want: "usage"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			root := newLiveRoot(t)
			code, stderr := runLiveTest(t, root, liveNow, tt.args...)
			if code != exitUsage {
				t.Fatalf("exit %d, want %d (stderr %q)", code, exitUsage, stderr)
			}
			if !strings.Contains(strings.ToLower(stderr), strings.ToLower(tt.want)) {
				t.Fatalf("stderr %q does not mention %q", stderr, tt.want)
			}
			if _, err := os.Stat(livestatus.StateDir(root)); !os.IsNotExist(err) {
				t.Fatalf("a rejected command created %s", livestatus.StateDir(root))
			}
		})
	}
}

func TestLiveClear(t *testing.T) {
	root := newLiveRoot(t)
	runLiveTest(t, root, liveNow, "set", "X7F4K2", "--message", "Researching")
	if code, stderr := runLiveTest(t, root, liveNow, "clear", "X7F4K2"); code != exitOK {
		t.Fatalf("exit %d, stderr %q", code, stderr)
	}
	if _, ok, _ := livestatus.Read(root, "X7F4K2"); ok {
		t.Fatal("status still present after clear")
	}
	if code, _ := runLiveTest(t, root, liveNow, "clear", "X7F4K2"); code != exitOK {
		t.Fatalf("clearing an absent status: exit %d", code)
	}
}

func TestLiveClear_WorksForADeletedTiki(t *testing.T) {
	root := newLiveRoot(t)
	if err := livestatus.Write(root, "QQQQQQ", livestatus.Status{Message: "orphan"}); err != nil {
		t.Fatal(err)
	}
	if code, stderr := runLiveTest(t, root, liveNow, "clear", "QQQQQQ"); code != exitOK {
		t.Fatalf("exit %d, stderr %q", code, stderr)
	}
}

func TestLiveClear_RejectsAMalformedID(t *testing.T) {
	if code, _ := runLiveTest(t, newLiveRoot(t), liveNow, "clear", "TASK-1"); code != exitUsage {
		t.Fatalf("exit %d, want %d", code, exitUsage)
	}
}

func TestLiveHelp(t *testing.T) {
	var stdout, stderr bytes.Buffer
	if code := runLiveIn(t.TempDir(), []string{"--help"}, &stdout, &stderr, liveNow); code != exitOK {
		t.Fatalf("exit %d", code)
	}
	if !strings.Contains(stdout.String(), "tiki live set") {
		t.Fatalf("help = %q", stdout.String())
	}
}
