# Configuration

## Configuration files

- `config-dir/config.yaml` main configuration file
- `config-dir/workflow.yaml` plugins/view configuration

## Configuration directories

`tiki` uses platform-standard directories for configuration while keeping tasks and documentation project-local:

**User Configuration** (global settings, plugins, templates):
- **Linux**: `~/.config/tiki` (or `$XDG_CONFIG_HOME/tiki`)
- **macOS**: `~/.config/tiki` (preferred) or `~/Library/Application Support/tiki` (fallback)
- **Windows**: `%APPDATA%\tiki`

Files stored here:
- `config.yaml` - User-global configuration
- `workflow.yaml` - Statuses and plugin/view definitions

**Environment Variables**:
- `XDG_CONFIG_HOME` - Override config directory location (all platforms)
- `XDG_CACHE_HOME` - Override cache directory location (all platforms)

Example: To use a custom config location on macOS:
```bash
export XDG_CONFIG_HOME=~/my-config
tiki  # Will use ~/my-config/tiki/ for configuration
```

### Overriding config via environment variables

Every setting in `config.yaml` can be overridden by a `TIKI_*` environment variable.
The mapping rule is mechanical:

1. Prefix with `TIKI_`
2. Replace each `.` in the config key with `_`
3. Uppercase the result

So `store.name` becomes `TIKI_STORE_NAME`, `logging.level` becomes `TIKI_LOGGING_LEVEL`,
and `appearance.theme` becomes `TIKI_APPEARANCE_THEME`.

Environment variables take precedence over every config file:

```bash
TIKI_LOGGING_LEVEL=debug tiki                   # temporarily verbose logs
TIKI_APPEARANCE_THEME=dracula tiki              # try a theme without editing files
```

The values are read during config load at the start of each `tiki` invocation, so
changes take effect on the next run — not retroactively for a running process.

## Precedence

`tiki` looks for configuration in two locations, from least specific to most specific:

1. **User config directory** (platform-specific, see [above](#configuration-directories)) - your personal defaults
2. **Current working directory** (`./`) - the scan root; per-project overrides committed alongside the repo

The single highest-priority file wins — no merging across files. This means each directory can have
its own workflow, statuses, and views that differ from your personal defaults. A design-team project
might use statuses like "Draft / Review / Approved" while an engineering project uses
"Backlog / In Progress / Done" — each defined in their own `./workflow.yaml`.

### config.yaml

The single highest-priority `config.yaml` found is loaded. Values not specified in that file fall
back to built-in defaults (not inherited from lower-priority files).

Search order: user config dir → `./config.yaml` (cwd). Last match wins.

### workflow.yaml

The single highest-priority `workflow.yaml` found is loaded. All workflow-backed sections (fields, views,
global actions, triggers) come from that one file. Lower-priority files are ignored entirely.
See [Workflow format versions](workflow-format.md) for schema evolution.

Search order: user config dir → `./workflow.yaml` (cwd). Last match wins. When neither file exists, the
embedded default (kanban) workflow is used.

On startup, tiki installs the bundled workflow only when the user config workflow does not already exist.
Existing installed workflows are not rewritten during upgrades. For example, an older `assignee: text`
declaration remains plain text until you edit it to `type: user` or explicitly reinstall/reset the workflow.

- Missing `fields:` entry `name: status` in the winning file is an error.
- Missing `fields:` entry `name: type` in the winning file is an error.
- Missing `views:` or explicit empty `views: []` means no views. The pre-0.6.0 `views: { plugins: [] }`
  wrapper is rejected by the parser.
- Missing `fields:` means no custom fields.
- Missing `triggers:` means no triggers.

Global actions are declared at the **top level** under `actions:` (not nested under `views:`) and apply to
every view. Per-view actions with the same key override globals for that view. See
[Workflow format versions](workflow-format.md) for the full 0.6.1 schema and the migration map from
pre-0.6.0 configs.

Actions can declare `require:` — a list of context attributes needed for the action to be enabled.
Actions with unmet requirements are visible but greyed out. See
[Action requirements](customization/customization.md#action-requirements) for details.

### config.yaml

Example `config.yaml` with available settings:

```yaml
# Header settings
header:
  visible: true             # Show/hide header: true, false

# Tiki settings
tiki:
  maxImageRows: 40          # Maximum rows for inline images (Kitty protocol)

# Logging settings
logging:
  level: error              # Log level: "debug", "info", "warn", "error"

# Appearance settings
appearance:
  theme: auto               # Theme: "auto" (detect from terminal), "dark", "light",
                            # or a named theme: "dracula", "tokyo-night", "gruvbox-dark",
                            # "catppuccin-mocha", "solarized-dark", "nord", "monokai",
                            # "one-dark", "catppuccin-latte", "solarized-light",
                            # "gruvbox-light", "github-light"
  gradientThreshold: 256    # Minimum terminal colors for gradient rendering
                            # Options: 16, 256, 16777216 (truecolor)
                            # Gradients disabled if terminal has fewer colors
                            # Default: 256 (works well on most terminals)

# AI agent integration
ai:
  agent: claude              # AI tool for chat: "claude", "gemini", "codex", "opencode"
                             # Enables AI collaboration features
                             # Omit or leave empty to disable

# Store backend configuration
store:
  name: tiki                 # Store engine name

# Live reload — refresh views when documents change outside tiki
liveReload:
  enabled: false             # Start with live reload on (default: false)
                             # Toggle at runtime with Ctrl-L or the action palette
                             # Flag: --live-reload; env: TIKI_LIVERELOAD_ENABLED

# Live status — what an AI agent is doing, shown on cards while live reload is on
liveStatus:
  staleAfter: 15m            # Dim a status and stop its animation after this long
                             # without an update (default: 15m; needs a unit, at least 1s)
                             # Env: TIKI_LIVESTATUS_STALEAFTER

# Tiki identity — used by `user()` and task attribution
identity:
  name: "Your Name"          # Display name for the current user
  email: "you@example.com"   # Email for the current user
                             # Both fields are optional. When unset, tiki falls
                             # back to git's user.name/user.email (when the scan
                             # root is a repo) and then to the OS account username.
                             # Environment overrides: TIKI_IDENTITY_NAME,
                             # TIKI_IDENTITY_EMAIL.
```

## Identity resolution

The `user()` ruki built-in and the "User" header stat resolve against a layered
identity, in order:

1. Configured `identity.name` / `identity.email` (or `TIKI_IDENTITY_NAME` /
   `TIKI_IDENTITY_EMAIL` environment variables).
2. Git user from `git config user.name` / `user.email`, when the scan root is a
   git repository and git is available.
3. OS account username from `os/user.Current()`, falling back to `$USER` /
   `$LOGNAME` / `$USERNAME`. The OS fallback never invents a display name —
   it returns the raw account username so behavior is predictable across
   machines.

Either `identity.name` or `identity.email` alone is sufficient — when only
the email is set, it is used as the display name so `user()` still resolves
consistently. The git layer is subject to the same promotion rule.

When none of those sources resolve, `user()` returns an "unavailable" error
and the "User" header stat displays `n/a`. Setting the `identity` block is
the recommended way to enable `user()` when the scan root is not a git repository.

## Live reload

Live reload keeps tiki's views up to date when documents change outside the app, so you don't
need to press `r`. Typical sources are an AI agent running `tiki exec`, an editor, or `git pull`.

**Turning it on and off.** It is off by default. Turn it on at startup with any of:

- the `--live-reload` flag
- `liveReload.enabled: true` in `config.yaml`
- the `TIKI_LIVERELOAD_ENABLED=true` environment variable

While tiki runs, toggle it with `Ctrl-L`, or pick "Live Reload" in the action palette (`Ctrl-A`).
Use the palette if your terminal setup takes `Ctrl-L` for itself (for example tmux with
vim-tmux-navigator). The statusline shows `● live` while it is on.

How it behaves:

- **Timing.** Changes show up about 0.2–1 s after the files settle.
- **Your own edits.** Saves you make in tiki are recognised and not reloaded a second time.
- **Selection.** The selected card stays selected when other tikis are added or removed around it.
- **Detail view.** It re-renders only when the tiki it shows, or a tiki it links to, changes.
- **Open overlays.** While the action palette, quick select, file tree, or a search or action input
  is open, changes wait until it closes.
- **Editing conflicts.** If the tiki you are editing changes on disk, the statusline warns you.
  Saving would conflict, so press `Esc` and reopen it.
- **Live status.** An AI agent's [live status](live-status.md) shows on cards only while live
  reload is on.
- **Full reload.** `r` still does a full reload. Unlike live reload, it also refreshes the creation
  time and author that tiki reads from git history.

Limitations:

- **Search results.** Active search results are a snapshot. Newly matching tikis appear after you
  search again.
- **Git metadata.** A tiki that is new to tiki (for example one that arrived with `git pull`) shows
  creation time and author from the file until the next `r`.
- **Not reloaded.** Changes to `workflow.yaml` and `config.yaml` still need a restart. Wiki views
  and the branch shown in the statusline are not refreshed.
- **Network drives.** Network filesystems and some WSL `/mnt/*` mounts don't report file changes,
  so live reload sees nothing there.
- **Large directory trees.** Live reload watches every directory tiki scans: all non-hidden,
  non-ignored directories under the working directory. In very large trees:
  - **Linux** may hit the inotify watch limit (`fs.inotify.max_user_watches`). Live reload then
    reports the error and stays off.
  - **macOS** keeps one open file per entry in each watched directory. A big tree can use up the
    open-file limit, and later saves or git commands then fail with "too many open files".
  - **Windows** holds a handle on every watched directory. While tiki runs, this may stop other
    programs from renaming or deleting those directories.

  Add large generated or vendored directories to `.gitignore` or `.tikiignore` to keep them out of
  the watch set, or leave live reload off in such trees.

If the system drops file events, for example during a very large checkout, tiki rescans the tree
and picks up the result.

### workflow.yaml

For detailed instructions see [Customization](customization/customization.md)

Example `workflow.yaml`:

```yaml
fields:
  - name: status
    type: enum
    values:
      - value: inbox
        label: Inbox
        visual: "📥"
        default: true
      - value: ready
        label: Ready
        visual: "📋"
      - value: inProgress
        label: "In Progress"
        visual: "⚙️"
      - value: done
        label: Done
        visual: "✅"
  - name: type
    type: enum
    values:
      - value: story
        label: Story
        default: true
      - value: bug
        label: Bug
      - value: project
        label: Project

actions:                        # top-level global actions (available from every view)
  - key: "a"
    kind: ruki
    label: "Assign to me"
    action: update where id = id() set assignee=user()
  - key: "A"
    kind: ruki
    label: "Assign to..."
    action: update where id = id() set assignee=input()
    input: string

views:                          # top-level list of views (no plugins: wrapper)
  - name: Kanban
    kind: board
    description: "Move documents to new status, search, create or delete"
    default: true
    key: "F1"
    lanes:
      - name: Inbox
        filter: select where status = "inbox" and type != "project" order by priority, createdAt
        action: update where id = id() set status="inbox"
      - name: Ready
        filter: select where status = "ready" and type != "project" order by priority, createdAt
        action: update where id = id() set status="ready"
      - name: In Progress
        filter: select where status = "inProgress" and type != "project" order by priority, createdAt
        action: update where id = id() set status="inProgress"
      - name: Done
        filter: select where status = "done" and type != "project" order by priority, createdAt
        action: update where id = id() set status="done"
  - name: Inbox
    kind: board
    description: "Tasks waiting to be picked up, sorted by priority"
    key: "F3"
    lanes:
      - name: Inbox
        columns: 4
        filter: select where status = "inbox" and type != "project" order by priority, id
    actions:
      - key: "b"
        label: "Add to board"
        action: update where id = id() set status="ready"
      - key: "e"
        label: "Link to project"
        action: update where id = choose(select where type = "project") set dependsOn = dependsOn + id()
  - name: Recent
    kind: board
    description: "Tasks changed in the last 24 hours, most recent first"
    key: Ctrl-R
    lanes:
      - name: Recent
        columns: 4
        filter: select where now() - updatedAt < 24hour order by updatedAt desc
  - name: Roadmap
    kind: board
    description: "Projects organized by Now, Next, and Later horizons"
    key: "F4"
    layout:                     # required — declares the tiki-box layout
      - ['type.visual + " " + id']
      - ["<highlight>title"]
      - ['"priority " + priority.visual + "  points " + points.visual']
    lanes:
      - name: Now
        columns: 1
        width: 25
        filter: select where type = "project" and status = "ready" order by priority, points desc
        action: update where id = id() set status="ready"
      - name: Next
        columns: 1
        width: 25
        filter: select where type = "project" and status = "inbox" and priority = "high" order by priority, points desc
        action: update where id = id() set status="inbox" priority="high"
      - name: Later
        columns: 2
        width: 50
        filter: select where type = "project" and status = "inbox" and priority > "high" order by priority, points desc
        action: update where id = id() set status="inbox" priority="medium-high"
    actions:
      - key: "l"
        label: "Add to project"
        action: update where id = id() set dependsOn = dependsOn + choose(select where type != "project")
  - name: Docs
    kind: wiki
    description: "Project notes and documentation files"
    path: "index.md"
    key: "F2"

triggers:
  - description: block completion with open dependencies
    ruki: >
      before update
        where new.status = "done" and new.dependsOn any status != "done"
        deny "cannot complete: has open dependencies"
  - description: tasks must pass through in-progress before completion
    ruki: >
      before update
        where new.status = "done" and old.status != "inProgress"
        deny "tasks must go through in-progress before marking done"
  - description: remove deleted task from dependency lists
    ruki: >
      after delete
        update where old.id in dependsOn set dependsOn=dependsOn - [old.id]
  - description: clean up completed tasks after 24 hours
    ruki: >
      every 1day
        delete where status = "done" and updatedAt < now() - 1day
  - description: tasks must have an assignee before starting
    ruki: >
      before update
        where new.status = "inProgress" and new.assignee is empty
        deny "assign someone before moving to in-progress"
  - description: auto-complete projects when all child tasks finish
    ruki: >
      after update
        where new.status = "done" and new.type != "project"
        update where type = "project" and new.id in dependsOn and dependsOn all status = "done"
        set status="done"
  - description: cannot delete tasks that are actively being worked
    ruki: >
      before delete
        where old.status = "inProgress"
        deny "cannot delete an in-progress task — move to inbox or done first"
```
