# Project identity + tiki-prefixed config files Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Show which project a tiki instance belongs to (title, header, bottom bar, colour, logo, theme) and make tiki's own config files `tiki-config.yaml` / `tiki-workflow.yaml` with a recognised fallback to the old names, merged config, and a `tiki config` report.

**Architecture:** A pure per-folder resolver in `config/files.go` decides which file of each kind a folder contributes (with statuses for reporting); every reader, writer and `tiki config` goes through it. Project identity values are sanitised getters in `config/project.go`; the header, statusline and bootstrap consume them.

**Tech Stack:** Go, viper, yaml.v3, tview/tcell (forked tview), go-runewidth.

**Spec:** `docs/superpowers/specs/2026-11-19-project-identity-design.md`

## Global Constraints

- File names: preferred `tiki-config.yaml` / `tiki-workflow.yaml`; fallback `config.yaml` / `workflow.yaml`; never both from one folder.
- Folders: global = `PathManager.ConfigDir()`, local = `pm.projectRoot`; same folder when `os.SameFile`.
- Recognition only for old names in the local folder; workflow keys `fields views statuses types triggers prefix prefixSeparator`; config: every top-level key known (case-insensitive), from `Config` mapstructure tags.
- Invalid YAML counts as tiki's for reading, never for deleting; empty / non-map is not tiki's.
- Readers: regular files only, ≤ 1 MiB.
- No resolution cache. No stderr output from resolution; debug logs only.
- Separators: header `<project> • <view>`, title `<name> • tiki`.
- Code comments: only "why", short, clause-per-line (user rule).
- Commit on `main`, no branches.

## Review Focus

1. A project folder with another app's `config.yaml` (unknown keys) and a GitHub-Actions-shaped `workflow.yaml`: tiki starts on the user's settings; reset leaves both (Task 3, Task 4 tests).
2. First run on a fresh machine: seed writes `tiki-workflow.yaml`, the same process then loads it (Task 4 test `TestInstallDefaultWorkflow_ThenLoad`).
3. Hostile `tiki-config.yaml` from a cloned repo: control/escape chars in `project.name` / `project.logo`, `project.color: default` (Task 6 tests).
4. A cwd that is a symlink to the user config folder: treated as global, no "local" label (Task 1 test).
5. Narrow terminal (80 columns): logo hidden, key hints keep 40 columns, existing integration tests still pass (Task 7).

---

### Task 1: Per-folder resolver

**Files:** Create `config/files.go`, `config/files_test.go`; Modify `config/loader.go` (add `Project` to `Config`, defaults).

**Interfaces — Produces:**
```go
type FileKind int // KindConfig, KindWorkflow
const ConfigFileName, LegacyConfigFileName, WorkflowFileName, LegacyWorkflowFileName = "tiki-config.yaml", "config.yaml", "tiki-workflow.yaml", "workflow.yaml"
type CandidateStatus string // StatusLoaded "loaded", StatusIgnored "ignored", StatusSkipped "skipped", StatusMissing "missing"
type Candidate struct{ Path string; Status CandidateStatus; Reason string }
type FolderResolution struct{ Dir string; Global bool; Chosen string; Candidates []Candidate }
func ResolveInFolder(kind FileKind, dir string, global bool) FolderResolution
type Resolution struct{ Global FolderResolution; Local *FolderResolution } // Local nil when same folder
func ResolveFiles(kind FileKind) Resolution // uses the path manager
func IsDeletableTikiFile(kind FileKind, path string, global bool) bool
func knownConfigKeys() map[string]bool // lowercased top-level mapstructure tags
```

- [ ] Tests (`files_test.go`, table-driven on `t.TempDir()`): tiki-only → loaded; legacy-only global → loaded; legacy-only local recognised → loaded; both → tiki loaded + legacy `ignored` with reason `tiki-config.yaml takes precedence`; local legacy config with `database:` → skipped `not a tiki file (unknown keys: database)`; local legacy workflow `name/on/jobs` → skipped `no tiki workflow keys`; invalid YAML local → loaded; empty / `- a` list → skipped; FIFO → skipped `not a regular file`; 1 MiB+1 → skipped `larger than 1 MiB`; `Appearance:` key matches case-insensitively; neither → one `missing` candidate for the tiki- name; `IsDeletableTikiFile` false for invalid YAML legacy local, true for tiki- names; symlinked cwd to global dir → `Local == nil`.
- [ ] Run `go test ./config -run 'Resolve|Deletable'` → FAIL (undefined).
- [ ] Implement; `Project struct { Name, Color, Logo string }` with `mapstructure:"project"` and `setDefaults` entries `project.name/color/logo = ""`.
- [ ] Run → PASS. Commit `config: resolver for tiki-prefixed config files with recognised fallback`.

### Task 2: Workflow readers use the resolver

**Files:** Modify `config/paths.go` (`findWorkflowFileWithScope`, `workflowCandidates` removed, `UserConfigWorkflowFile`, filename constants), fixtures in `config/paths_test.go`, `cmd_demo_test.go:170`.

**Interfaces — Consumes:** `ResolveFiles(KindWorkflow)`. **Produces:** unchanged `FindWorkflowFileWithScope() (string, Scope)`; `UserConfigWorkflowFile()` returns resolved global path or `<global>/tiki-workflow.yaml`.

- [ ] Tests: `TestFindWorkflowFileWithScope_TikiNameWins` (local both names → tiki- path, ScopeCurrent); `_ForeignLocalFallsBackToGlobal` (local GitHub-shaped `workflow.yaml` → global path, ScopeGlobal); update `version: 1` fixtures to carry `fields:`.
- [ ] Run → FAIL; implement; `go test ./...` → PASS. Commit.

### Task 3: Config loading merges global then local

**Files:** Modify `config/loader.go` (`LoadConfig`, delete `findConfigFile`, delete `PathManager.ConfigFile`), `config/loader_test.go`.

- [ ] Tests: flip `TestLoadConfig_ProjectOverridesUser` (header.visible false inherited from user, logging debug from project); delete `_CwdOverridesProject`; `TestLoadConfig_ForeignLocalConfigIgnored` (local `config.yaml` with `database:` + `logging:` → user's level kept); `TestLoadConfig_TikiConfigPreferred`.
- [ ] Implement with `viper.MergeConfig` per chosen file (global, then local). Run → PASS. Commit.

### Task 4: Writers

**Files:** Modify `config/install.go`, `config/reset.go`, `config/system.go`, `cmd_demo.go`; tests `config/install_test.go`, `config/reset_test.go`, `config/system_test.go`, `cmd_demo_test.go`, `cmd_workflow_test.go`.

**Interfaces — Produces:** `func WorkflowWriteTarget(dir string, global bool) (string, error)` — resolved chosen path, else `dir/tiki-workflow.yaml`; local symlink target → error `refusing to write through symlink <path>`.

- [ ] Tests: install into empty local → `tiki-workflow.yaml`; install with recognised legacy local → overwrites legacy; install with foreign legacy → writes tiki- beside it; local symlink → error; global symlink followed; reset `--current` deletes both recognised names, keeps foreign `config.yaml` / `workflow.yaml` and invalid-YAML legacy; `InstallDefaultWorkflow` no-op when global `workflow.yaml` exists, writes `tiki-workflow.yaml` otherwise; `TestInstallDefaultWorkflow_ThenLoad` (fresh XDG: seed then `LoadWorkflowFields` succeeds); demo writes `tiki-workflow.yaml`; update assertions listed in the spec's fixture list.
- [ ] Run → FAIL; implement; run → PASS. Commit.

### Task 5: `tiki config` and debug logging

**Files:** Create `cmd_config.go`, `cmd_config_test.go`; Modify `main.go` (dispatch after `InitPaths`, reserved name, usage), `internal/bootstrap/init.go` (debug-log statuses after `InitLogging`).

**Interfaces — Produces:** `func runConfig(args []string) int`; `func writeConfigReport(w io.Writer, cfg, wf config.Resolution, personal []string)`; `func config.PersonalKeysIn(path string) []string` (subset of `identity ai logging`).

- [ ] Tests: report rows for loaded/ignored/skipped/missing; workflow global row `overridden by local` when local chosen; personal-settings note `identity, ai set in a local file apply to everyone using this project`; `--help` exit 0; unknown arg exit 2.
- [ ] Implement; run → PASS. Commit.

### Task 6: Project values

**Files:** Create `config/project.go`, `config/project_test.go`.

**Interfaces — Produces:**
```go
func GetProjectName() string            // sanitised project.name, else filepath.Base(projectRoot)
func GetProjectColor() (string, bool)   // normalised "#rrggbb"
func ParseProjectColor(s string) (string, bool)
func GetProjectLogo() string            // sanitised, tabs expanded (8), trailing newlines trimmed
func ProjectDirLabel() string           // shortenDir(projectRoot, home, 40)
func TerminalTitle() string             // GetProjectName() + " • tiki"
func sanitizeLine(s string) string; func sanitizeBlock(s string) string; func shortenDir(path, home string, max int) string
```
- [ ] Tests: `"\x1b]0;evil\x07Acme\u009b"` → `"]0;evilAcme"` (controls removed); logo tab expansion; `#f80` → `#ff8800`, `orange` → `#ffa500`, `default`/`reset`/`nope` rejected; `shortenDir("/home/u/stor/devel/projects/.config/tiki", "/home/u", 20)` → `~/s/d/p/.c/tiki`; `/home/u2/x` with home `/home/u` → unchanged; name default = base of project root.
- [ ] Implement; PASS; commit.

### Task 7: Header — project name, logo, narrow rule

**Files:** Modify `view/header/info.go`, `view/header/header.go`, `view/header/header_layout.go`, `view/header/header_layout_test.go`; Create `view/header/logo.go`, `view/header/logo_test.go`, `view/header/info_test.go`; Modify `config/art.go` (export `DefaultLogoArt`, remove `GetArtTView`), `config/dimensions.go` comment.

**Interfaces — Produces:** `CalculateHeaderLayout(totalWidth, contextHelpWidth, logoWidth int) HeaderLayout{ContextWidth int; ShowLogo bool}`; `buildLogo(art string, c logoColors) (text string, width int)`; `projectLogoColors(hex string, light bool) logoColors`; `fitTitle(project, view string, width int) string` (returns the project part, possibly `…`-truncated or empty).

- [ ] Tests: layout `ShowLogo` false at 80 with logo 25, true at 105; `fitTitle("A very long project name here", "Kanban", 39)` truncated with `…` and total ≤ 39; view of 39 cols → project dropped; logo escaping (`[red]` stays literal), 7 lines → 6, width via `tview.TaggedStringWidth`; glyph colouring; separator 39 wide.
- [ ] Implement; full suite (integration tests at 80 cols) PASS; commit.

### Task 8: Bottom-bar directory block

**Files:** Modify `model/header_config.go` (`StatValue{Fg, Bg string}`), `model/statusline_config.go` (`SetLeftStatColored`), `view/statusline/statusline.go` (escape at render, per-segment colours incl. arrows), `internal/bootstrap/models.go`; tests in `view/statusline/statusline_test.go`, `internal/bootstrap/models_test.go`.

- [ ] Tests: coloured segment emits `[#000000:#ff8800] ~/x ` and the following arrow uses `#ff8800` as fg; value `a[red]b` rendered escaped but measured raw; `InitStatuslineModel` adds `Dir` at priority -1; contrast picks black for `#ffff00`, white for `#000080`.
- [ ] Implement; PASS; commit.

### Task 9: Terminal title

**Files:** Modify `internal/bootstrap/init.go`, `controller/navigation.go` (`SetAfterResume(func())`, called after `Suspend`), `internal/app/app.go` (`InstallTitle(app *tview.Application, title string) (rearm func())`).

- [ ] Test: navigation calls the after-resume hook after `SuspendAndEdit` / `SuspendAndRun` (existing test seams `editorOpener`, `commandRunner`); `InstallTitle` sets the title on the first draw of a `tcell.NewSimulationScreen` if the screen exposes it, else compile-only.
- [ ] Implement; PASS; commit.

### Task 10: Schema checks views and actions

**Files:** Modify `cmd_workflow_schema.go`, `cmd_workflow_schema_test.go`.

- [ ] Test: workflow with a lane filter referencing an unknown field → `runWorkflowSchema` exit 3, stderr names the view.
- [ ] Implement via `plugin.LoadPluginsAndGlobals(rukiRuntime.NewSchema())` after `LoadWorkflowFields`; PASS; commit.

### Task 11: Docs and user-facing strings

**Files:** `docs/config.md`, `docs/themes.md`, `docs/command-line.md`, mentions across `docs/`, `README.md`, `llms.txt`; strings listed in the spec's Docs section; `version_test.go:218`.

- [ ] Update; `go test ./...` PASS; commit.

### Task 12: AI skill modules

**Files:** Create `ai/skills/tiki/references/customize-config.md`, `ai/skills/tiki/references/customize-workflow.md`, theme-list drift test `config/skill_themes_test.go`; Modify `ai/skills/tiki/SKILL.md`, `ai/skills/tiki/references/files.md`.

- [ ] Drift test FAIL first (module missing), write modules, PASS; commit; run the fresh-agent trial (spec), fix, re-trial until clean; commit.

### Task 13: Review and verification

- [ ] Fresh-agent code review of the whole change; fix confirmed findings with tests.
- [ ] `go build ./... && go test ./... && golangci-lint run`; fresh-clone build; tmux behavioural checks from the spec's Verification section.
