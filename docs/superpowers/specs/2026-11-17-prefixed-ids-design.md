# Prefixed sequential tiki IDs — design

## Context

Tiki ids are 6 random characters (`X7F4K2`), which makes them collision-free across branches but hard to read and remember.
The user wants readable ids like `MEM-DB-35`: prefixes taken from field values, plus a sequential number.
They asked whether tiki supports this (it doesn't), and whether it belongs in tiki or in each project.
It has to be in tiki: tiki rejects any id that isn't `^[A-Z0-9]{6}$`, so a project can't do this on its own.

Outcome: a workflow can opt in to prefixed ids; new tikis get a readable id that never changes,
and every clone, branch and worktree of a repo on the same machine draws numbers from one locked counter, so they never clash.
Existing projects migrate with `tiki ids rename` / `tiki ids migrate`, guided by two agent prompts.

## Goals, non-goals, anti-goals

- Goals: readable ids for new tikis that never change; per-prefix sequences; no local clashes across clones/branches/worktrees;
  opt-in per workflow; a supported way to migrate existing projects (rename + bulk migrate commands, plus agent prompts).
- Non-goals (v1): coordination across machines; repairing a duplicate id the loader skipped;
  resolving former ids in links or ruki; reading other branches to rebuild a lost counter;
  static checks that lane actions don't set immutable fields; turning the feature on in shipped workflows.
- Anti-goals: ids that change after creation (outside an explicit rename/migrate);
  a number issued twice in normal operation within one repository on one machine (all its clones, branches and worktrees);
  any behaviour change for workflows without a prefix; making git mandatory.

## Approaches considered

- **Chosen — the prefixed code is the real id** (`id: MEM-DB-35`).
  Prefix fields are required and immutable, so the id never changes on its own.
  One identifier everywhere (frontmatter, `dependsOn`, `[[links]]`, ruki, Copy ID).
  Costs: the id format widens (ruki fork + tiki), the id is assigned at save, a clash from another machine is a duplicate id.
- Rejected — code beside a hidden random id (`code: MEM-DB-35`):
  only worthwhile when codes change; with codes that never change it just leaves two identifiers and random ids in `dependsOn`.
- Rejected — the id changes when a tiki moves between lists, with old ids resolving:
  every move becomes a multi-file rename and claims a new number. The user chose ids that never change instead.
- Rejected — projects own numbering via a script hook: tiki would still do the format, validation and display work;
  only the counter moves out, and every project would solve the branch problem alone.
- Rejected counters: a counter file committed to the repo (a merge conflict on every parallel create, and the duplicates exist anyway);
  ruki triggers only (ruki has `count()` but no `max()`, so numbers get reused).

## Design

### Workflow configuration (user's design; one key name, `prefix`, at both levels — user's choice)

```yaml
prefix: TIK                        # the project prefix: turns prefixed ids on; used when no field value adds a segment
fields:
  - name: area
    type: enum
    required: true
    immutable: true
    values:
      - {value: feature, prefix: "FEAT", label: "Feature wish"}
      - {value: memory,  prefix: "MEM",  label: "Memory & archive"}
      - {value: misc,                    label: "Misc"}        # no prefix → no segment
  - name: layer
    type: enum
    required: true
    immutable: true
    values:
      - {value: frontend, prefix: "FE", label: "Front-End/UI"}
      - {value: database, prefix: "DB", label: "Database"}
      - {value: cross,                  label: "Cross-cutting"}
```

Gives `FEAT-FE-15`, `FEAT-16` (layer=cross), `DB-33` (area=misc), `TIK-7` (misc + cross).

- Prefixed ids are on exactly when the workflow sets the top-level `prefix` (my call, 2026-11-18).
  The top-level `prefix` alone gives a plain project-wide sequence (`TIK-1`, `TIK-2`) with no immutable fields;
  value-level `prefix:` refines it and requires the top-level one.
- A value-level `prefix:` is optional; a value without one adds no segment (user's choice).
- Segments join with `-` in field declaration order; with no segments, the top-level `prefix` is used (user's choice).
- Each distinct joined prefix (`FEAT-FE`, `FEAT`, `DB`, `TIK`) has its own sequence (user's design).
- Load-time validation (my calls): every prefix matches `^[A-Z][A-Z0-9]*$`;
  a value prefix is unique across all fields and differs from the top-level one, so every joined prefix maps to one combination;
  a field with any value prefix must be `type: enum`, `required: true` and `immutable: true`;
  the top-level `prefix` is mandatory once any value prefix exists.
- Stored ids are never recomputed: editing prefixes in workflow.yaml affects only tikis created afterwards.

### Field attributes `required` and `immutable` (general, any field type)

- `required: true` — a create is rejected unless the field has a value after defaults are applied;
  an update that clears a set value is rejected.
  A field with a default satisfies it, so quick capture and piped input keep working.
- `immutable: true` — write-once (my call, revised from "fixed at creation"):
  once set, the value can't be changed or cleared.
  For tikis created under the rule the value always exists from creation, so it's fixed;
  legacy tikis with an empty field can fill it in once.
- An update to a legacy tiki that lacks a required field is *not* rejected for that reason alone.
- Prefix fields freeze when a tiki gets a prefixed id (at creation, rename or migrate), empty values included
  (my call after the second review): write-once filling is allowed only while the tiki still has a legacy id.
  So a prefixed id always matches the prefix its fields give.
- Enforced by validators on `service.TikiMutationGate` (`service/tiki_mutation_gate.go`, `OnCreate`/`OnUpdate`),
  which every mutation path goes through (TUI, ruki, triggers, pipe). Errors name the field: `area is immutable`.
- TUI: immutable fields with a value are read-only when editing; the new-tiki form can't save without required fields.
- Hand edits to files can't be blocked, and there is no "id no longer matches its fields" flag (dropped after review):
  a mismatch also arises legitimately when prefixes are edited in workflow.yaml, and nothing on disk tells the two apart.

### ID format

- Valid id = legacy `^[A-Z0-9]{6}$` **or** prefixed `^[A-Z][A-Z0-9]*(-[A-Z][A-Z0-9]*)*-[1-9][0-9]*$`.
  Prefixed ids always contain `-`, legacy ids never do, so the two can't be confused.
- Single source of truth stays `idfmt.IsValidID` in the ruki fork (`/stor/devel/ruki/idfmt/idfmt.go`);
  its only ruki callers are `executor.go:490` and `:528` (error text mentions 6 chars — reword).
- Tiki sites to update: `document/id.go` (`ValidateID` message), `store/tikistore/tiki_bridge.go:62`,
  `store/tikistore/crud.go:211`, `service/validators.go:133,149`, `view/markdown/wikilink.go:27` (hard-coded `{6}`),
  `view/tikidetail/content_provider.go:64`, `view/tikidetail/render_key.go:73`, `internal/livestatus/file.go:143`,
  `store/tikistore/stamp.go:245`, `workflow/fields.go:22` (comment).
- Note: `TIKI-<digits>` becomes a valid prefixed id; harmless, accepted (my call).

### Number allocation

- Counter file, one per repo, outside the repo (user's design; location and key are my calls the user accepted):
  - Linux: `$XDG_STATE_HOME/tiki/sequences/` (default `~/.local/state/tiki/sequences/`);
    macOS and Windows follow the conventions in `config/paths.go` (Windows under `%LOCALAPPDATA%`, never Roaming).
  - Keyed by the repo's root commit (`git rev-list --max-parents=0 HEAD`; the smallest hash if there are several),
    which every clone, branch and worktree of the repo shares (changed after review: the shared git dir differs per clone).
    Fallbacks: a repo with no commits → its shared git dir (`--git-common-dir`); no git → tiki's project root (the cwd,
    `config/paths.go` `getProjectRoot`; running in a subfolder of a non-git project is a separate counter — documented limitation).
    Both git backends (CLI and go-git, `store/internal/git`) must answer the same.
    File name: `<folder>-<8 hex of sha256(key)>.yaml`.
  - Content: the path it belongs to (for humans) and `counters: {FEAT-FE: 15, DB: 33, …}`.
- Locking: OS advisory lock via `golang.org/x/sys` (already in go.mod; `unix.Flock` / `windows.LockFileEx`),
  held only for read → increment → write; write via temp file + rename; bounded wait, then a clear error.
- Floor rule (accepted): `next = max(counter[prefix], highest number for that exact prefix among loaded tikis) + 1`.
  Covers a lost file and tikis merged in from another machine. Missing/corrupt file → treated as empty, logged, rewritten.
  The floor only sees this checkout (known gap: lost file + unmerged local branch can repeat a number; the duplicate check catches it).
- Assigned at save, in the store's create path (`store/tikistore/crud.go` `storeNewDocumentLocked`), after validation.
  A failed save leaves a gap; gaps are fine.
- Today every create path pre-assigns a random id in `NewTikiTemplate` (`store/tikistore/template.go:53`), used by
  `internal/pipe/create.go:108`, `internal/ruki/runtime/runner.go:79`, `service/trigger_engine.go:225,428`,
  `controller/plugin.go:206`. With a prefix configured, templates leave the id empty and the store assigns it;
  without one, behaviour is unchanged.
- Clashes from other machines: the existing duplicate-id load diagnostic reports them, and `admitDocument`
  (`store/tikistore/persistence.go`) skips the later file, so one tiki is hidden until fixed by hand in v1.

### Ordering

- Add `idfmt.Compare(a, b string) int` in the ruki fork: natural order for prefixed ids
  (prefix, then number numerically), plain string order otherwise, so `MEM-9` precedes `MEM-10`.
- ruki `order by id` uses it: `sortKeyForField` (`/stor/devel/ruki/executor.go:1728`) gets a case for id/ref fields
  (today ids sort as plain strings via `compareForSort`). Lanes like `order by priority, id` (`docs/config.md:325`) depend on this.
  Comparison operators (`id < "…"`) stay plain string comparisons.
- `GetAllTikis` (`store/tikistore/crud.go`) uses the same function.

### Migration commands (user's choice: rename + bulk migrate)

New subcommand group beside `workflow`/`exec`/`live` (dispatch in `main.go`), same exit codes as `tiki exec` (0/2/3/4):
- `tiki ids rename <OLD> <NEW>` — explicit target, e.g. keeping a custom scheme's numbers (`B55` → `B-55`).
  NEW must be a valid id, not taken, and not in any tiki's `formerIds`.
  If NEW is prefixed, its prefix must equal the one the tiki's fields give (mistake guard, my call);
  the counter file is raised to at least NEW's number so no clone or worktree issues it later.
- `tiki ids migrate [--dry-run]` — every tiki with a legacy 6-char id (plain docs included) gets `<prefix from its fields>-<next>`,
  allocated oldest first (createdAt, then id) so numbers follow creation order. Requires the top-level `prefix`.
  An empty prefix field adds no segment, as a value without a prefix does; since prefix fields then freeze,
  the output lists the tikis that will keep an empty prefix field so they can be filled first.
  `--dry-run` prints the map and the files it would change, and writes nothing, counter included.
- Both:
  - print one `OLD -> NEW` line per rename;
  - rewrite every reference: values of `tikiIdList`/`list<ref>` and `ref` fields in every tiki,
    and both link forms — `[[OLD]]` and a Markdown link whose target is the id, `[text](OLD)` — in every scanned Markdown file,
    tikis and id-less wiki pages alike (the wiki view resolves links in both: `view/wiki_view.go:176`, `extractTikiID` in
    `view/tikidetail/content_provider.go`). Id-less pages are rewritten as text with an atomic write, since the store never saves them.
    Plain-text mentions are left alone (my call after the second review);
  - append OLD to the renamed tiki's `formerIds` frontmatter list (plain key: findable by grep; not resolved by links, not a ruki field in v1);
  - keep `createdAt`: store it in frontmatter before the id changes if it isn't stored yet,
    because creation-time stamping matches files to git history by id (`store/tikistore/stamp.go`);
  - move the live-status file for OLD to NEW (`internal/livestatus`);
  - run under one store lock, write through the store's save path, and don't fire triggers (a rename changes no field);
  - refuse when OLD is unknown or NEW is invalid/taken, with a clear message.

### AI skill (`ai/skills/tiki/`) — requested by the user

Agents learn a project's fields from `tiki workflow schema` (`cmd_workflow_schema.go`), so that output is the main channel:
it must show `required`, `immutable`, each value's `prefix`, and the top-level `prefix`.
`SKILL.md` changes:
- Rules (line 112, "An id is 6 characters of `[A-Z0-9]`"): an id is either 6 characters or a prefixed id like `MEM-DB-35`;
  tiki assigns it; never invent one, never guess the next number;
  never rename one unless the user asks, and then only with `tiki ids rename`.
- Create: set every required field (unless it has a default), or the create is refused.
  Fields marked immutable can't be changed later and, when they carry prefixes, decide the id:
  choose their values deliberately and ask the user when the right value is unclear.
- Update: an immutable field can't be changed or cleared (exit code 4 with the reason).
  If the user wants it changed, say so and let them decide (e.g. a new tiki replacing the old one); never edit the file to get around it.
- Examples and the description's "implement X7F4K2" gain a prefixed id alongside.
- `references/dependencies.md` / `files.md`: mention that `[[ID]]` and `list<ref>` accept both id forms.
- Add `Bash(tiki ids:*)` to `allowed-tools`.
- Re-run the fresh-agent trial after editing the skill (scratch tiki build, scratch `XDG_CONFIG_HOME`,
  a scratch project whose workflow uses prefixes; user-style requests to create, move and link tikis).

### Shipped workflows (user's choice: off, with commented examples)

- `config/workflows/kanban.yaml`, `bug-tracker.yaml`, `todo.yaml` keep random ids for new installs
  (existing users keep their own workflow copy regardless: `config.InstallDefaultWorkflow` never overwrites).
- Each gets a commented example: kanban `type` → `STORY`/`BUG`/`SPIKE`/`PROJ` with `required`/`immutable`, `prefix: TIK`;
  bug-tracker `type` → `BUG`/`REG`/`INC`, `prefix: TIK`; todo a top-level `prefix: TODO` alone.
  The example must stay valid YAML when uncommented (block-style values where a `prefix:` line is shown).
- Docs explain the trade-off: random ids never clash across machines; sequential ids can, so they suit solo use or one machine.

### Docs

- `docs/tiki-format.md` (id rules), `docs/workflow-format.md` (new version entry for `required`, `immutable`,
  value-level and top-level `prefix`), `docs/ruki/types-and-values.md`, `docs/customization/`, `docs/command-line.md`
  (`tiki ids`), README, `config/index.md`.
  `config/loader.go` `workflowFileData` must list the top-level `prefix` to survive round-trips.

### Migration prompts (deliverable, written after implementation)

Two prompts the user runs in separate sessions, generic (no project names), each asking the agent to work with the user,
start from a clean git state with no tiki TUI running on the project, check the installed tiki has `tiki ids`, and not commit unless asked:
1. Project with its own prefix + number scheme: find where the current codes live; map its prefixes onto an enum field
   (value `prefix:`, `required`, `immutable`) and a top-level `prefix`; fill empty prefix fields through `tiki exec`;
   `tiki ids rename` each ticket to its existing code in the new format (`B55` → `B-55`), `tiki ids migrate` for the rest;
   retire the old scheme's scripts or fields only with the user's approval; verify.
2. Project with random ids: agree prefixes with the user; update workflow.yaml; fill prefix fields;
   `tiki ids migrate --dry-run`, review the map with the user, run it; offer to update old-id mentions outside tiki files; verify.
The final wording follows the commands as built.

## Left to the builder

- Audit everything that reads a template's id before save (TUI new-tiki form, `new.id` in before-create triggers, pipe output).
- Exact macOS/Windows state paths, lock-file naming, lock timeout, max id length.
- How the TUI shows read-only immutable fields.
- Whether ruki's `FieldSpec` also learns `required`/`immutable` for earlier errors (runtime enforcement in tiki is enough).

## Review outcome (builder checks, 2026-11-18)

- Separate clones didn't share a counter → key changed to the root commit.
- `order by id` would still sort `MEM-10` before `MEM-9` → `idfmt.Compare` used by ruki's sort.
- The id/fields mismatch flag would flood after prefix edits → dropped.
- Non-git project root ambiguity → the cwd, as tiki already defines the project root; documented limitation.
- Migration sections: empty prefix fields → no segment, and prefix fields freeze once the id is prefixed;
  links in id-less wiki pages and `[text](ID)` links → rewritten too.
- User: `idPrefix` renamed to a top-level `prefix`, matching the value-level key;
  `permanent` renamed to `immutable` (docs note the one-time fill for tikis created before the field became immutable).

## After approval

1. Write this design to `docs/superpowers/specs/2026-11-17-prefixed-ids-design.md` (uncommitted: commits only when the user asks).
2. Invoke `superpowers:writing-plans` for the implementation plan.
   ruki fork first (`/stor/devel/ruki`, branch `dev`): `idfmt` format + `Compare`, executor messages, `order by id`;
   tiki builds against it through an untracked `go.work`. Pushing the fork and bumping the `replace` pin wait for the user's go-ahead.
3. Implement test-first; then an independent agent reviews the implementation and found bugs get fixed;
   tmux sessions for what unit tests can't cover.
4. Report back with the two migration prompts.

## Verification

- Unit: `idfmt` both formats and `Compare`; prefix composition and fallback; workflow validation errors;
  required/immutable validators incl. write-once on legacy tikis and freezing on prefixed ids; floor rule;
  counter file under concurrent goroutines.
- Integration: a temp git repo with two worktrees and a second clone, and a prefixed workflow; parallel `tiki exec 'create …'`
  from all three → all ids unique and sequential per prefix; delete the counter file → numbering continues from the highest loaded.
- Behaviour in a `tiki demo` project: ruki `update … set area=` rejected; `[[MEM-DB-35]]` resolves; natural sort.
- `tiki ids migrate --dry-run` then a real run on a scratch copy of a demo project: references in `dependsOn`, `[[links]]`,
  `[text](ID)` links and wiki pages follow; `formerIds` recorded; createdAt unchanged; live status follows;
  the counter continues after the highest number; `tiki ids rename` refusals (taken, invalid, prefix mismatch).
- The uncommented shipped-workflow examples load and produce the documented ids.
- TUI behaviour via a private detached tmux session (approved); look and feel checked by the user.
- Fresh-agent trial of the edited AI skill on a prefixed workflow; re-run after any skill fix.
- `make` build, lint and full test suite; fresh-clone build after pushing.
