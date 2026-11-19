# Project identity + tiki-prefixed config files — design

## Context

The user runs tiki in several projects at once, one terminal tab or stacked window each.
Projects use similar prefixes, boards and workflows,
so it's hard to tell which tab or window holds which project.
Nothing in tiki shows the project today:
the header shows the view name, the key hints and a fixed dot logo,
and the bottom bar shows version, workflow scope, user and branch.

The user works in **tabs or stacked windows** and sees titles before switching,
so the terminal title is the main signal.
Their terminal shows titles that programs set.
Markers inside tiki confirm the choice and give the project some character.

Per-project settings need a home.
Today a project's `config.yaml` *replaces* the user's config (upstream `6375091`, whose message gives no reason;
the commit was mainly about workflow merging).
`./config.yaml` and `./workflow.yaml` are also generic names that many projects use for their own files.
Today another app's `config.yaml` silently replaces the user's tiki config,
another tool's `workflow.yaml` stops tiki from starting,
and `tiki workflow reset` deletes either one.
The user decided this design also covers how tiki finds, merges and reports its config files,
and updates the AI skill so agents can customise a project.

Outcome:
- a `<name> • tiki` terminal title;
- the project name in the header and the directory in the bottom bar;
- an optional colour, ASCII logo and theme per project;
- `tiki-config.yaml` and `tiki-workflow.yaml` as the default names, with the old names still working;
- user and project configs merged;
- a `tiki config` command that shows which files were used;
- `tiki workflow schema` that also checks views and actions;
- AI skill modules: one for config, one for workflow customisation.

## Goals, non-goals, anti-goals

- **Goals**
  - Tell projects apart from the tab or window title.
  - Confirm the project inside tiki: the name in the header, the directory in the bottom bar, the colour.
  - Per-project colour, ASCII logo and theme.
  - Default file names that don't clash with other projects' files.
  - Projects that already use `config.yaml` or `workflow.yaml` keep working.
  - A visible answer to "which files did tiki use, and why did it ignore that one?".
  - Agents can customise a project's config and the essentials of its workflow, from two separate skill modules,
    with the core skill kept small.
- **Non-goals (this round)**
  - An image logo.
  - `--config` / `--workflow` flags, or a setting that names the project files (the user's choice: "neither for now").
  - Automatic colours.
  - Renaming existing files automatically.
  - Merging workflows (one workflow file still wins).
  - tmux window naming.
  - Workflow details beyond the essentials in the skill
    (detail-view layouts, input/choose actions, triggers); agents follow docs links for those.
  - Identity in the markdown viewer mode (`tiki file.md`).
- **Anti-goals**
  - Reading both the `tiki-` file and the old-name file from the same folder: one file per kind per folder.
  - Another project's `config.yaml` or `workflow.yaml` silently replacing the user's config,
    stopping tiki from starting, or being deleted by `tiki workflow reset`.
  - A real tiki workflow that has an error in it being skipped silently instead of reporting that error.
  - Extra output on stderr for every run (agents parse `tiki exec` output).
  - A cloned repo's tiki files injecting terminal escapes, hanging tiki, or getting writes redirected through symlinks.

## Approaches considered (where per-project settings live)

- **Chosen: A.** `config.yaml` merges again: the project file overrides only the keys it sets.
  The user's addition: the default name is `tiki-config.yaml`, falling back to `config.yaml`, never both.
  A per-project theme then needs no new code (`appearance.theme`).
- Rejected: B, a separate merging `config-override.yaml`.
  It leaves two project files that follow different rules.
- Rejected: C, everything in `workflow.yaml`.
  The user's principle is that `workflow.yaml` holds structure (fields, views, actions) and `config.yaml` holds looks and behaviour.

## Design

### Part 1: finding, merging and reporting config files

**Locations**: the user config folder ("global": `PathManager.ConfigDir()`, honours `XDG_CONFIG_HOME`),
then the project folder ("local": the cwd, `pm.projectRoot`).
- Two folders count as the same when `os.SameFile` says so, not when their absolute paths match,
  so a symlinked cwd or a differently-cased path isn't mistaken for a project folder.
- `findConfigFile`'s separate `ProjectConfigDir()` and `"."` candidates (`config/loader.go:127-134`)
  collapse into the one project folder. `TestLoadConfig_CwdOverridesProject` goes.
- `PathManager.ConfigFile()` has one caller (`loader.go:130`); delete it.
- Readers, writers and reset all take the folder explicitly.
  Today reset uses `os.Getwd` (`reset.go:77`) while the readers use `pm.projectRoot`.
- Output and docs use "global" and "local", matching the statusline label.

**Resolver**: a pure function per kind, taking a folder and whether that folder is the user's.
- It returns the chosen file plus every candidate's status (loaded, ignored, skipped or missing, with the reason).
- It is **not cached**. Writers run after resolution in the same process
  (`InstallDefaultWorkflow` → `LoadWorkflowFields` in bootstrap, `runExec`, `runWorkflowSchema` and `testutil.NewTestApp`),
  so a cache would serve them a stale "missing".
- It reports the file's on-disk spelling.

**Per folder, exactly one file of each kind:**
1. `tiki-config.yaml` / `tiki-workflow.yaml` if present. It is always used, and the old name is never read.
2. Otherwise `config.yaml` / `workflow.yaml`, if it passes the recognition check.
3. Otherwise nothing from this folder.

**File safety (all readers):** only regular files are read (after following symlinks), capped at 1 MiB,
the same limit as `config/install.go:12`.
Anything else (a FIFO, a device, an oversized file) is skipped with a reason.

**Recognition check**: runs only for old-name files in the **local** folder.
The global folder belongs to tiki, so its old-name files are accepted as they are.
- `workflow.yaml` counts as tiki's if it has at least one tiki-specific top-level key:
  `fields`, `views`, `statuses`, `types`, `triggers`, `prefix` or `prefixSeparator`.
  - Generic keys such as `version`, `description`, `actions` and `name` don't count.
  - A legacy `statuses:` file is therefore recognised and still gets its migration error (`config/workflow_fields.go:246`).
- `config.yaml` counts as tiki's only if **every** top-level key is a known tiki setting.
  - The known keys come from the `Config` struct's `mapstructure` tags
    (a complete list; the new `Project` field joins it).
  - Keys are compared case-insensitively, because viper lowercases them.
  - The cost is that a legacy local `config.yaml` with a misspelled top-level key is skipped as a whole.
    `tiki config` shows why; the fix is to rename it to `tiki-config.yaml`.
- **Invalid YAML counts as tiki's when reading**, so the user sees the parse error, as today.
  Another app's file is rarely broken, and a `tiki-` file means tiki never reads the old one.
- **Empty files, and files whose top level isn't a map, are not tiki's**, for both kinds.

**Merging**
- `config`: start from the built-in defaults, then merge the global file, then the local file
  (`viper.MergeConfig`, deep for maps), then `TIKI_*` env vars and flags.
  This restores the pre-`6375091` loop, with the new resolution.
  The other `LoadConfig` callers (markdown viewer, pipe, exec, ids, schema) pick up merging without changes.
  The go-git config read in `store/internal/git` isn't tiki config and doesn't change.
- `workflow`: unchanged. The local file wins over the global one.
  The `global`/`local` scope labels stay.
  Every reader goes through `findWorkflowFileWithScope` (`config/paths.go:364`),
  including "Edit Workflow" (`controller/input_router.go:629`) and `workflow schema`.

**Logging**: no buffer.
Bootstrap resolves the files again after `InitLogging` and logs the statuses at **debug** level.
Nothing is printed to stderr.

**`tiki config`** (new subcommand, read-only; the user's choice for making this visible):
- It prints, for config and then workflow, each folder (global, local) and each candidate file
  with its status and reason. For example:
  - `loaded`
  - `loaded (local wins)` / `overridden by local` for a workflow
  - `ignored: tiki-config.yaml takes precedence`
  - `skipped: not a tiki file (unknown keys: database, server)`
  - `skipped: no tiki workflow keys`
  - `skipped: not a regular file`
  - `missing`
- It flags personal settings (`identity`, `ai`, `logging`) found in a local file,
  because with merging they apply to everyone who clones the repo (my call; behaviour unchanged).
- Wiring: dispatched in `main.go` after `InitPaths` (`:57`) and before the piped-stdin check (`:82`),
  and added to the viewer's reserved names (`:93`).
  It doesn't call `InstallDefaultWorkflow`.
- It follows the `tiki ids counter` style, with an entry in `printUsage` (`main.go:365`),
  its own `--help`, and `docs/command-line.md`.

**`tiki workflow schema` checks views and actions** (review finding).
Today it runs `LoadWorkflowFields` and parses triggers only (`cmd_workflow_schema.go:32,246`),
while startup refuses bad views and actions (`plugin/loader.go:224-239`).
It will also run the view and action validation that `tiki workflow install` uses
(`validateWorkflowViews`, `cmd_workflow.go:247`), so a workflow that passes the check also starts.
It fails with that error and exit 3.

**Writers**
- `InstallDefaultWorkflow` (`config/system.go:69-90`):
  if either name already exists in the global folder, it does nothing; otherwise it creates `tiki-workflow.yaml`.
- `tiki workflow install` (`InstallWorkflowFromContent`, `config/install.go:41`):
  it overwrites the file the target folder resolves to.
  If that's none, or the local old-name file isn't tiki's, it creates the `tiki-` name.
- `tiki workflow reset` (`config/reset.go`) mostly deletes; the only write is the `--global` workflow reset.
  - It deletes **every tiki file** of that kind in the folder, under both names,
    so an old file can't quietly become active again.
  - Deleting needs a stricter rule than reading:
    a local old-name file is deleted only if it **parses and passes the key check**.
    Invalid YAML is never deleted, because templated files (Helm, Jinja `{{ }}`) are invalid YAML and common.
  - The `--global` workflow write goes to the file that resolves there, otherwise to `tiki-workflow.yaml`.
- `tiki demo` (`ensureDemoWorkflow`, `cmd_demo.go:71`): writes `tiki-workflow.yaml`, resolving in the demo folder explicitly.
- **Symlinks** (my call): in the local folder, writers refuse to write through a symlink and report it,
  so a cloned repo can't plant `tiki-workflow.yaml -> ~/.bashrc`.
  In the global folder, symlinks are followed, because dotfile managers symlink `~/.config/tiki/*`.
  Reset removes a symlink itself, never its target.

**Names**: `configFilename` / `defaultWorkflowFilename` (`config/paths.go:316-319`)
become the preferred and fallback name pairs.
`UserConfigWorkflowFile()` returns the path that resolves now, otherwise the `tiki-` path.

### Part 2: project identity

New `Project` struct on `Config`, with defaults in `setDefaults` so `TIKI_PROJECT_*` env vars bind:

```yaml
project:
  name: Tiki fork      # default: base name of the folder tiki was started in
  color: "#ff8800"     # optional: #rrggbb, #rgb, or a colour name tcell knows
  logo: |              # optional; replaces the dot logo
    …ASCII art…
appearance:
  theme: nord          # existing key; per project now that configs merge
```

`project.*` in the user's config applies to every project, by design.

**Sanitising** (all project values come from a possibly cloned file):
- `name` and `logo` have every control character removed (`unicode.IsControl`, covering C0 and C1) wherever they're used.
  Tabs in the logo are expanded to 8-column stops first.
- `color` is parsed with `tcell.GetColor`.
  `#rgb` is expanded to `#rrggbb` first.
  Unknown names, `default` and `reset` (which give `ColorDefault`) are rejected.
  Only the normalised `#rrggbb` ever goes into a tag.
  Invalid values get a debug log entry and behave as unset.
- Text going into tview is `tview.Escape`d.

**Features**
- **Name**: `project.name`, otherwise `filepath.Base(cwd)`. Exposed as a `config` getter.
  Worktrees therefore show their own folder name.
- **Terminal title**: `<name> • tiki`.
  - The separator is `•`, matching the header (my call; the earlier draft used `·`).
  - It uses tcell `Screen.SetTitle` (tcell 2.8 and later; v2.13.9 here) with no raw OSC fallback,
    set from a one-shot `SetBeforeDrawFunc` after `app.NewApp()` (`init.go:154`),
    the same pattern as `internal/viewer/markdown_viewer.go:182`.
  - It's set again after `SuspendAndEdit` / `SuspendAndRun` (`controller/navigation.go:144,169`);
    that's harmless if tcell already re-sends it.
  - Restoring on exit is left to tcell.
- **Header name** (`view/header/info.go` `SetViewInfo`):
  - The first line becomes `<project> • <view>`, separated by `•` (the user's choice).
  - The project name is drawn in the project colour, otherwise the theme's `LogoDot` colour.
    The `•` uses `TextMuted`, and the view name keeps its bold `StatusWarn`.
  - The usable width is 39 (`InfoWidth` 40 minus 1 column of left padding), and the widget word-wraps.
    So the line is fitted before it's set:
    if it's too long, the project name is truncated with `…`;
    if the view name alone fills the line, the project name is dropped.
  - Fix in passing: the `─` separator becomes 39 wide, not 40 (`info.go:37`).
- **Bottom bar directory block** (`internal/bootstrap/models.go:43` `InitStatuslineModel`):
  - A new first left segment, priority `-1` (the sort isn't stable, and Version is 0).
  - The value is stored raw, because `segmentsVisibleLen` measures it, and escaped only when drawn.
  - `~` replaces the home folder when the cwd is inside it (`filepath.Rel`, not a prefix match).
    On Windows, keep the drive letter and compare the home folder case-insensitively.
  - If it's longer than 40 columns, every middle folder is shortened to its first letter,
    keeping the dot for dot-folders (`~/s/d/.c/tiki`).
    The last folder is never shortened.
  - The other segments' alternating colours shift by one (accepted).
  - When `project.color` is set, the block's background is that colour,
    with black or white text, whichever contrasts better.
    Add optional fg/bg hex fields to `StatValue` (`model/header_config.go:22`) and `statSegment`,
    and add `SetLeftStatColored`.
    One helper returns a segment's colours, used both for the segment and for its neighbouring arrows
    (`view/statusline/statusline.go:131-136`).
- **Logo colour**: when `project.color` is set, `●` uses it, and `▓` / `▒` are mixed toward black in dark themes
  or white in light themes (`config.IsLightTheme()`), because themes have no background colour of their own.
  Otherwise, today's `LogoDot` / `LogoShade` / `LogoBorder` roles.
- **Custom logo**:
  - `project.logo`, sanitised and with the trailing newline trimmed, replaces `artDots`.
  - Colouring follows the glyph rule: `●` / `▓` / `▒` get the three logo colours,
    and every other character gets the dot colour.
  - At most 6 lines. Extra lines are cut, with a debug log.
    Pass the limit in, or move the logo rendering into `view/header`, to avoid a `config` ↔ `view/header` import cycle.
  - Short logos are top-aligned.
  - The width is the widest line, measured with `tview.TaggedStringWidth`.
    This replaces the fixed `LogoWidth` in `CalculateHeaderLayout`.
- **Narrow terminals**: the logo gives way to the key hints.
  - When the width is less than `InfoWidth` + logo width + `MinContextWidth` (40, unused today),
    the logo is hidden. This applies to the default logo too (below about 105 columns).
  - Integration tests use an 80-column simulation screen,
    so the logo is hidden there and the key hints get 40 columns instead of 15.
    Adjust the tests that depend on that.

### Docs

- `docs/config.md`:
  - The new names, the fallback and recognition rules, and the symlink and size rules.
  - Merging (replacing "no merging across files" for config).
  - `tiki config`.
  - The `project:` section.
  - A per-project theme example.
- `docs/themes.md`: a per-project theme note.
- `docs/command-line.md`: `tiki config`, plus the stricter `tiki workflow schema` check.
- Rename the file names across `docs/`, `ai/skills/tiki/`, `README.md` and `llms.txt`
  (about 122 mentions in 27 files; `llms.txt:33` also lists the new modules), noting that the old names still work.
- User-facing strings get the new names too.
  Where a resolved path exists, print it instead of a fixed name:
  - help text at `cmd_workflow.go:306-307,319`;
  - errors and messages at `workflow_fields.go:137,209`, `system.go:86`, `config/version.go:114`,
    `config/loader.go:222,226`, `store/tikistore/rename.go:29` and `plugin/parser.go:59`;
  - log lines at `plugin/loader.go:59,71,94`.
- Leave the demo content (`assets/demo/security/encryption.md`) alone.
  Don't rewrite the old `docs/superpowers` specs.

### AI skill (`ai/skills/tiki/`)

The user's rule for the split:
anything set in the workflow file goes in the workflow module, and anything set in the config file goes in the config module.
The config module keeps the generic name because the config file covers a mix of things:
looks (header, theme, gradients, image rows, `project:`),
behaviour (AI agent, live reload, live-status staleness), identity, logging and the store.
The core `SKILL.md` stays as small as it is now (7.2 KB), and its prefix *usage* lines stay
(the user's choice after discussion).

- **Version guard, in both modules**: if `tiki --help` doesn't list `tiki config`, tiki is older than the module.
  Never run `tiki config` there, because an older tiki opens its viewer on the word "config".
  Instead, find the files by the old rule: `./config.yaml` / `./workflow.yaml`, or the ones in `~/.config/tiki/`.
- **`references/customize-config.md`** covers every `config.yaml` setting. It's listed under
  "Asked to change a tiki setting: how this project looks (name, colour, logo, theme, header),
  the AI agent, live reload, identity, or which config file tiki reads". It covers:
  - running `tiki config` to see which files are used;
  - **global or local**:
    personal settings (`identity`, `ai.agent`, `logging`) go in the user config,
    never in a project file that is committed and applies to everyone who clones it.
    Project settings (`project:`, `appearance`, `header`, …) go in the project's `tiki-config.yaml`,
    or in an existing tiki `config.yaml` there; create `tiki-config.yaml` if there is neither;
  - the "never both" rule, and merging (set only what should differ);
  - each setting in one line, grouped as looks, behaviour, identity and other.
    The `project:` keys get an example.
    `appearance.theme` lists the valid theme names,
    and a Go test fails when that list, the `docs/config.md` list (`:130-134`) and the `config/themes.go` registry disagree;
  - the `TIKI_*` env vars for one-off runs;
  - that changes apply on the next start;
    for looks, the agent says what it changed and asks the user to look, because it can't see the screen.
  - The undocumented `appearance.codeBlock` and the unread `board.view` are left out.
- **`references/customize-workflow.md`** covers everything set in the workflow file, at essentials depth,
  with docs links for the rest.
  It's listed under "Asked to change this project's workflow: fields, statuses, types, views/boards, actions,
  triggers, the description, or id prefixes". It covers:
  - **Which file**: `tiki config` shows the workflow in use.
    Workflows don't merge, so if the project uses the global one, ask the user whether the change is for this project only.
    If it is, copy the whole global file to the project's `tiki-workflow.yaml` first, then edit the copy.
    Never edit the global workflow unless asked.
    `tiki workflow install <name> --current` starts a project from a bundled workflow.
  - **Fields**: adding a field (the main types, enum values, `default`, `required`, `immutable`).
    `status` and `type` must stay.
    Renaming or removing an enum value: add the new value, move the tikis with `tiki exec`, then remove the old one.
  - **Readable ids**: `prefix:`, per-value `prefix`, `prefixSeparator`, and required plus immutable prefix fields.
    Existing tikis move only with `tiki ids migrate`: `--dry-run` first, and only when the user asks.
  - **Views**: a board with lanes (a `filter` select plus an `action` update), its key, and its description.
  - **Actions**: global versus per-view, `key`, `label` and a ruki `action`. Input, choose and `require:` are named only.
  - **Triggers**: one line, plus the docs link.
  - **Check every edit**: run `tiki workflow schema`.
    It now validates fields, triggers, views and actions; fix any error and run it again.
    Changes appear in a running tiki on its next start.
  - **Docs**: links to the GitHub docs, fetched only when needed, the same pattern as `references/queries.md:46`:
    `docs/customization/customization.md`, `custom-fields.md`, `docs/workflow-format.md`, `docs/ruki/triggers.md`.
- **SKILL.md**:
  - Two module-table rows.
  - The frontmatter `description` adds customising a project's workflow or config.
  - `allowed-tools` adds `Bash(tiki config:*)` and `Bash(tiki --help)`.
    Not `tiki workflow install`, which overwrites workflows and should prompt.
  - The older-tiki fallback (lines 27-28) names `tiki-workflow.yaml` / `workflow.yaml`.
- `references/files.md` and the other mentions get the new names.
- **Trial** (memory: re-trial after skill edits):
  - Build tiki into the scratchpad, wrapped with `XDG_CONFIG_HOME` pointing at a scratch folder.
  - Copy the skill beside a scratch project.
  - Give a fresh general-purpose agent user-style requests without saying what's checked:
    - config: "make this project show as Acme in orange", "use nord only here", "put this ASCII logo in tiki",
      "set my name to Sam for tiki" (this should go to the user config);
    - workflow: "add a priority field with low/medium/high", "add a Review column to the board",
      "give new tikis ids like ACME-1".
  - Run them in three setups:
    a project on the global workflow (it should ask, then copy);
    a project with its own workflow;
    a project with a legacy `config.yaml`.
  - Check which files it reads and edits, that `tiki workflow schema` passes and tiki starts afterwards
    (a headless start check), and that the real user config is untouched.
  - Re-run the trial after every skill fix.

### Testing

- **Resolver**: pure table tests on temp folders, for each kind and folder:
  - `tiki-` only, old name only, both;
  - an unrecognised old-name file, invalid YAML, empty, non-map;
  - not a regular file, oversized;
  - global versus local (the check is skipped globally);
  - case-insensitive keys;
  - a symlinked cwd treated as the same folder.
- **`config/loader_test.go`**:
  - merging flips `TestLoadConfig_ProjectOverridesUser`;
  - `TestLoadConfig_CwdOverridesProject` goes;
  - the `project.*` getters and sanitising.
- **Fixtures that change** (review-corrected list):
  - Local files that are no longer recognised: `paths_test.go:344,382,469` (`version: 1`),
    `reset_test.go:113-114,152` (`custom\n`) and `cmd_demo_test.go:170`.
  - Assertions on the file name after a write into an empty folder:
    `install_test.go:53,77,104,183`, `cmd_workflow_test.go:270,283`, `reset_test.go:196`,
    `cmd_demo_test.go:70,149,181,211,288`.
  - Message text: `version_test.go:218`.
- **Writers**:
  - install targets the resolved file, or creates the `tiki-` name;
  - reset deletes both recognised names, never foreign files, never invalid YAML;
  - local symlinks are refused and global ones followed;
  - `InstallDefaultWorkflow` does nothing if either name exists;
  - `tiki demo` writes `tiki-workflow.yaml`;
  - the first run in a fresh folder works (the stale-cache regression).
- **Commands**:
  - `tiki config` output for each status, including the personal-settings flag;
  - `tiki workflow schema` fails on a bad view or action.
- **Units**:
  - the header info line (separator, truncation, drop);
  - the art (custom logo, sanitising, tabs, escaping, glyph colouring, line cut, width);
  - the logo hidden below the width threshold;
  - directory shortening (home via `filepath.Rel`, dot-folders, Windows-style paths);
  - title formatting and sanitising;
  - colour parsing (`#rgb`, names, rejected defaults), shade mixing and text contrast;
  - the statusline colour override reaching the arrows;
  - the theme-list drift test.
- **Simulation screen**:
  - the header line and the hidden logo through `testutil.NewTestApp` / `FindText`;
  - the directory block's wiring through `InitStatuslineModel` directly, because `NewTestApp` doesn't call it;
  - the title through the simulation screen, if it exposes one.
- **Behaviour**: a scripted run in a private detached tmux session on a `tiki demo` project (approved in memory).
  Read the title with `tmux display -p -t <session> '#{pane_title}'`.
- **The look** is checked by the user in their own terminal.

## Implementation steps (after approval)

1. Save this design to `docs/superpowers/specs/2026-11-19-project-identity-design.md` and commit it on `main`.
2. Invoke `superpowers:writing-plans` for the detailed task plan in `docs/superpowers/plans/`.
3. Build test-first, in this order:
   1. The pure resolver (recognition, file safety).
   2. Switch the readers; fix fixtures; full suite green.
   3. Merging.
   4. The writers (install, reset, seed, demo, symlinks).
   5. `tiki config`.
   6. The schema check for views and actions.
   7. Part 2: name and title, header line, directory block, colour, logo, narrow-width rule.
   8. Docs and strings.
   9. The skill modules and their trial.
4. Code review by a fresh agent; fix what it finds.
5. Verification (below), then a short report to the user with what to check in their terminal.

## Verification

- `go build ./... && go test ./...` and `golangci-lint run`.
- Fresh-clone build check (Actions are disabled on the fork).
- In the tmux session, in a demo project with only `tiki-config.yaml` (`project.name: Demo X`, `color`, `logo`):
  - check that the pane title is `Demo X • tiki`, the header shows `Demo X • Kanban`, and the bottom bar starts with `~/…`;
  - add a foreign `config.yaml` (unknown keys) and a foreign `workflow.yaml` (GitHub-Actions shape),
    then check that tiki starts on the user's settings and that `tiki config` reports both files as skipped, with reasons;
  - check that `tiki workflow reset` leaves both foreign files in place.
- The user checks the colours, the logo, the theme and narrow-width behaviour in their own terminal.
