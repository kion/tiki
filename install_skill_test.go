package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"testing"
)

var skillDirs = map[string]string{
	"claude-code": ".claude/skills/tiki",
	"codex":       ".agents/skills/tiki",
	"gemini":      ".gemini/skills/tiki",
	"opencode":    ".opencode/skills/tiki",
}

func runInstallSkill(t *testing.T, args ...string) (string, int) {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("install-skill.sh needs bash")
	}
	bash, err := exec.LookPath("bash")
	if err != nil {
		t.Skip("bash not found")
	}
	script, err := filepath.Abs("install-skill.sh")
	if err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command(bash, append([]string{script}, args...)...) //nolint:gosec // G204: test runs the repo's own script
	cmd.Dir = t.TempDir()
	out, err := cmd.CombinedOutput()
	if err == nil {
		return string(out), 0
	}
	exitErr, ok := err.(*exec.ExitError)
	if !ok {
		t.Fatalf("run install-skill.sh: %v", err)
	}
	return string(out), exitErr.ExitCode()
}

func newProject(t *testing.T, markers ...string) string {
	t.Helper()
	dir := t.TempDir()
	for _, m := range markers {
		path := filepath.Join(dir, m)
		if strings.HasSuffix(m, "/") {
			if err := os.MkdirAll(path, 0750); err != nil {
				t.Fatal(err)
			}
			continue
		}
		if err := os.WriteFile(path, []byte("x\n"), 0600); err != nil {
			t.Fatal(err)
		}
	}
	return dir
}

// installedTools lists the tools whose skill directory holds a complete copy of the skill.
func installedTools(t *testing.T, project string) []string {
	t.Helper()
	var tools []string
	for tool, rel := range skillDirs {
		dest := filepath.Join(project, rel)
		if _, err := os.Stat(dest); os.IsNotExist(err) {
			continue
		}
		for _, f := range []string{"SKILL.md", "references/points.md"} {
			want, err := os.ReadFile(filepath.Join("ai/skills/tiki", f))
			if err != nil {
				t.Fatal(err)
			}
			got, err := os.ReadFile(filepath.Join(dest, f))
			if err != nil || string(got) != string(want) {
				t.Errorf("%s: %s is missing or differs from the bundled skill (err %v)", tool, f, err)
			}
		}
		tools = append(tools, tool)
	}
	sort.Strings(tools)
	return tools
}

func TestInstallSkill_Targets(t *testing.T) {
	tests := []struct {
		name    string
		markers []string
		flags   []string
		want    []string
	}{
		{"detects a Claude Code directory", []string{".claude/"}, nil, []string{"claude-code"}},
		{"detects CLAUDE.md", []string{"CLAUDE.md"}, nil, []string{"claude-code"}},
		{"detects AGENTS.md as Codex", []string{"AGENTS.md"}, nil, []string{"codex"}},
		{"detects a .codex directory", []string{".codex/"}, nil, []string{"codex"}},
		{"detects an existing .agents directory", []string{".agents/"}, nil, []string{"codex"}},
		{"detects GEMINI.md", []string{"GEMINI.md"}, nil, []string{"gemini"}},
		{"detects a .gemini directory", []string{".gemini/"}, nil, []string{"gemini"}},
		{"detects an .opencode directory", []string{".opencode/"}, nil, []string{"opencode"}},
		{"detects opencode.json", []string{"opencode.json"}, nil, []string{"opencode"}},
		{"detects every harness present", []string{".claude/", "AGENTS.md", ".gemini/", ".opencode/"}, nil,
			[]string{"claude-code", "codex", "gemini", "opencode"}},
		{"a flag installs into an empty project", nil, []string{"--codex"}, []string{"codex"}},
		{"several flags", nil, []string{"--claude-code", "--gemini"}, []string{"claude-code", "gemini"}},
		{"flags replace detection", []string{".claude/"}, []string{"--opencode"}, []string{"opencode"}},
		{"--all", nil, []string{"--all"}, []string{"claude-code", "codex", "gemini", "opencode"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			project := newProject(t, tt.markers...)
			out, code := runInstallSkill(t, append(tt.flags, project)...)
			if code != 0 {
				t.Fatalf("exit %d, want 0:\n%s", code, out)
			}
			got := installedTools(t, project)
			if strings.Join(got, ",") != strings.Join(tt.want, ",") {
				t.Errorf("installed for %v, want %v:\n%s", got, tt.want, out)
			}
			for _, tool := range tt.want {
				if !strings.Contains(out, skillDirs[tool]) {
					t.Errorf("output should name %s:\n%s", skillDirs[tool], out)
				}
			}
		})
	}
}

func TestInstallSkill_FlagAfterProjectDir(t *testing.T) {
	project := newProject(t)
	if out, code := runInstallSkill(t, project, "--gemini"); code != 0 {
		t.Fatalf("exit %d, want 0:\n%s", code, out)
	}
	if got := installedTools(t, project); strings.Join(got, ",") != "gemini" {
		t.Errorf("installed for %v, want [gemini]", got)
	}
}

func TestInstallSkill_NothingDetected(t *testing.T) {
	project := newProject(t, "README.md")
	out, code := runInstallSkill(t, project)
	if code != 1 {
		t.Errorf("exit %d, want 1:\n%s", code, out)
	}
	if !strings.Contains(out, "--claude-code") {
		t.Errorf("output should point at the flags:\n%s", out)
	}
	if got := installedTools(t, project); len(got) != 0 {
		t.Errorf("nothing should be installed, got %v", got)
	}
}

func TestInstallSkill_UpdateDropsStaleFiles(t *testing.T) {
	project := newProject(t, ".claude/")
	dest := filepath.Join(project, skillDirs["claude-code"])
	if err := os.MkdirAll(dest, 0750); err != nil {
		t.Fatal(err)
	}
	for _, f := range []string{"SKILL.md", "removed-module.md"} {
		if err := os.WriteFile(filepath.Join(dest, f), []byte("old\n"), 0600); err != nil {
			t.Fatal(err)
		}
	}

	out, code := runInstallSkill(t, project)
	if code != 0 {
		t.Fatalf("exit %d, want 0:\n%s", code, out)
	}
	if got := installedTools(t, project); strings.Join(got, ",") != "claude-code" {
		t.Errorf("installed for %v, want [claude-code]", got)
	}
	if _, err := os.Stat(filepath.Join(dest, "removed-module.md")); !os.IsNotExist(err) {
		t.Errorf("a file the skill no longer ships should be gone (err %v)", err)
	}
	if !strings.Contains(out, "updated") {
		t.Errorf("output should say the copy was updated:\n%s", out)
	}
}

// The destination is replaced wholesale,
// so a directory that isn't a copy of the skill must not be mistaken for one.
func TestInstallSkill_RefusesToReplaceAForeignDirectory(t *testing.T) {
	project := newProject(t, ".claude/")
	dest := filepath.Join(project, skillDirs["claude-code"])
	if err := os.MkdirAll(dest, 0750); err != nil {
		t.Fatal(err)
	}
	notes := filepath.Join(dest, "my-notes.md")
	if err := os.WriteFile(notes, []byte("keep me\n"), 0600); err != nil {
		t.Fatal(err)
	}

	out, code := runInstallSkill(t, project)
	if code != 1 {
		t.Errorf("exit %d, want 1:\n%s", code, out)
	}
	if data, err := os.ReadFile(notes); err != nil || string(data) != "keep me\n" {
		t.Errorf("the foreign file must be left alone (err %v)", err)
	}
}

func TestInstallSkill_UsageErrors(t *testing.T) {
	project := newProject(t, ".claude/")
	tests := []struct {
		name string
		args []string
		want int
	}{
		{"no project", nil, 2},
		{"unknown flag", []string{"--cursor", project}, 2},
		{"two projects", []string{project, project}, 2},
		{"project is not a directory", []string{filepath.Join(project, "missing")}, 2},
		{"help", []string{"--help"}, 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			out, code := runInstallSkill(t, tt.args...)
			if code != tt.want {
				t.Errorf("exit %d, want %d:\n%s", code, tt.want, out)
			}
			if !strings.Contains(out, "Usage:") {
				t.Errorf("output should show usage:\n%s", out)
			}
			if got := installedTools(t, project); len(got) != 0 {
				t.Errorf("nothing should be installed, got %v", got)
			}
		})
	}
}
