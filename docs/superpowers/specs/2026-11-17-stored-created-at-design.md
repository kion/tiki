# Stored creation time and local-time display

Status: design, awaiting approval.
Builds on `1aae750` (creation and update times come from git history again).

## Why

A tiki's creation time is used to sort boards ("newest first within a priority").
Today it is never stored: tiki recomputes it on every load,
from the commit that added the file, or from the file's modification time when there is no such commit.
That loses information in two ways:

- A tiki created two days ago and committed today shows today as its creation time.
- Before the first commit, the value is the file's modification time,
  so editing an uncommitted tiki and restarting tiki moves its creation time.

The user wants the real creation time kept, stored with the tiki so every machine sees the same value,
and every timestamp tiki shows displayed in the viewer's local time zone.

## Current behaviour (as of 1aae750)

- `createdAt` is derived on load (`store/tikistore/persistence.go`, `loadTikiFile`):
  author date of the commit that added the file (`git log --diff-filter=A`), else file mtime.
  A `createdAt:` key in a file's frontmatter is ignored on load and dropped on the next save.
- `updatedAt` is derived: the later of the file's mtime and its last commit.
- `activeAt` is computed: the later of `updatedAt` and the tiki's live status time (`tiki/tiki.go`).
- `createdAt`, `updatedAt`, `createdBy`, `filepath` are immutable in ruki; `activeAt` is computed.
- Saving (`saveTiki`) re-serializes the whole frontmatter (`marshalTikiFrontmatter`).
- Display:
  - TUI detail view formats timestamps with `value.DateTimeFormat` ("2006-01-02 15:04")
    in whatever zone the `time.Time` carries:
    mtime and `time.Now()` values are local, git values carry the committer's UTC offset.
  - `tiki exec` table output (`internal/ruki/runtime/format.go`, `renderTimestamp`) prints UTC RFC3339.
  - `tiki exec --format json` (`jsonTimestamp`) prints UTC RFC3339.
- Workflow-declared `datetime` fields (e.g. `dueBy`) are parsed zone-less (`value.ParseDateTime`):
  they hold a wall-clock time as typed, stored in a UTC `time.Time`.
- The tiki AI skill (`ai/skills/tiki`) has agents create tikis only through `tiki exec 'create …'`;
  notes-only tikis may be edited directly with Read/Edit.

## Goals

1. `createdAt` is stored in the tiki file's frontmatter, in UTC, and a stored value wins over anything derived.
2. A tiki created in tiki (TUI or `tiki exec create`) gets `createdAt` at creation.
3. Every save tiki makes writes `createdAt`.
4. An uncommitted tiki that arrives from outside tiki (editor, agent, copy) gets `createdAt` written
   at the TUI's next full load: startup or full reload (`r`).
5. When no stored value and no commit exist, the fallback is the file's creation (birth) time where the OS
   and filesystem record it, and the modification time otherwise.
6. tiki's own timestamps (`createdAt`, `updatedAt`, `activeAt`) are shown in the machine's local time zone
   in the TUI and in `tiki exec` table output.
7. The AI skill and the docs describe the new behaviour.

## Non-goals

- Storing `updatedAt` or `activeAt`. `updatedAt` stays derived as today:
  storing it would conflict in git whenever two branches edit the same tiki,
  and would make tiki rewrite files that others just edited.
- Storing `createdBy`.
- Changing how workflow-declared `datetime` fields are parsed, stored or shown.
  They are wall-clock values with no zone; converting them would shift them.
- A batch backfill of existing tikis. The user does that separately per project.
- Making `createdAt` assignable through ruki. It stays immutable.

## Anti-goals (a failure even if it works)

- tiki rewriting or reformatting files it did not otherwise change.
  Stamping a file inserts exactly one line and leaves every other byte as it was.
- In a git repository, a first launch that rewrites every existing tiki (one big diff).
- Stamping counted as an edit: `updatedAt` / `activeAt` jumping to the moment of stamping,
  which would fill the Recent lane and restart the time-based triggers in the bundled workflows
  (todo's one-day cleanup of done tasks, bug-tracker's stale demotion and purge).
- A read-only command (`tiki exec 'select …'`) writing files.
- tiki fighting an outside writer: stamping a file while an editor or agent is still writing it,
  or treating its own stamp as an external edit (conflict warnings, reload loops).
- `tiki exec --format json` output changing shape or zone; scripts and agents read it.

## Approaches considered

Where the creation time is kept:

- **A. In the tiki file's frontmatter (picked by the user).**
  Travels through git, so every machine and every clone sees the same value.
  Costs: tiki adds a line to files; diverges from upstream,
  which keeps these fields derived, so future upstream merges may conflict in the load/save code.
- **B. In tiki's local, git-ignored `.tiki/` folder.**
  Leaves files untouched, but the value exists on one machine only;
  a clone or a teammate falls back to the commit time.

When tiki writes it into a file it did not create:

- **A. Only when tiki saves the tiki anyway.** Misses tikis an agent or editor creates
  and the user commits without editing them in tiki.
- **B. Every tiki, the first time tiki sees it.** Complete, but the first launch rewrites every existing tiki.
- **C. On sight, only for tikis not yet committed (picked by the user).**
  Those are exactly the tikis git would get wrong.
  Committed tikis without the line keep deriving from their first commit,
  which is the best information left for them, and no mass rewrite happens.

Known gap in every option: a tiki created and committed while tiki is not running gets the commit time.

## Design

### The field

- Frontmatter key `createdAt` (decided: matches the ruki field name;
  lowerCamelCase is already used by `dependsOn`, `dueBy`, `reportedBy`;
  `createdAt` is already a reserved system field, so it cannot clash with a workflow's custom field).
- Written as RFC 3339 in UTC with seconds: `createdAt: 2026-01-01T10:00:00Z`.
- On read, any RFC 3339 value is accepted (any offset) and normalized to UTC in memory;
  the next tiki save writes it back in UTC. *(my call)*

### Resolving a tiki's creation time on load

In order, first match wins:

1. a valid stored `createdAt` in the frontmatter;
2. the author date of the commit that added the file (today's behaviour);
3. the file's birth time, if the OS and filesystem report it;
4. the file's modification time.

A stored value that does not parse is ignored for resolution (with a warning in the log) and the next
match is used. The next tiki save overwrites it with the resolved value. *(my call)*

`createdBy` resolution is unchanged.

### Writing it

- **tiki saves:** every save writes `createdAt` with the tiki's current value, whatever its source.
  For a committed tiki without the line, this freezes the first-commit time into the file
  on the first edit made in tiki. Two branches doing this write the same value, so git merges them cleanly.
  *(my call)*
- **Creation in tiki:** the creation time is the moment of creation (as today), and the first save writes it.
- **On sight (option C), at full loads only:** when the TUI does a full load
  (startup, or full reload with `r`) and finds a tiki file that
  - has no valid stored `createdAt`, and
  - is not committed: the project is not a git repository,
    or it is one and no commit in the history touches the path,
  tiki inserts `createdAt: <resolved value>` into the file's frontmatter.
  - Outside git, the first full load therefore stamps every tiki (decided by the user):
    there is no diff to worry about, and it keeps each file's birth time
    before a copy to another folder or machine resets it.
  - Stamp only on positive knowledge: the folder is known not to be a repository,
    or the history was fetched successfully. A repository whose history cannot be read
    (git error, failed history fetch) stamps nothing, so an error never turns into a mass rewrite.
  - Full loads already fetch the history (`AllLastCommitTimes`),
    so "committed" means the path is in that map: no new git calls.
    "Touches" rather than "added", so a renamed or moved tiki counts as committed.
    *(my call; the builder may refine)*
  - The value is resolved before writing, so it is the birth time (or mtime) of the file as found,
    not the moment of stamping.
  - Insert one line into the existing frontmatter; every other byte of the file stays as it was.
    Write the file in place rather than replacing it, so its birth time survives.
    Do not go through `saveTiki`, which re-serializes the frontmatter.
  - Restore the file's original modification time after writing (decided by the user),
    so the stamp is not an edit: `updatedAt` and `activeAt` keep their values across restarts.
  - Record the file's new size and restored mtime as tiki's loaded state,
    so the stamp is not seen as an external edit (no conflict warning on a later save, no reload loop
    if live reload is on).
  - No wait-for-writer mechanism: full loads rarely meet a file that is still being written,
    and a file that fails to parse is not stamped.
- **Not stamped on sight:**
  - during live reload (`ReloadPaths`, `Reconcile`), which never calls git (decided by the user).
    A tiki created by hand while tiki runs is stamped at the next startup or `r`.
  - by `tiki exec` (any statement). A read-only query does not write files.
    `tiki exec create` / `update` still write `createdAt` as part of their normal save. *(my call)*
  - files matched by the root `.gitignore` or `.tikiignore`: tiki does not load them at all.
- **Known limitations:**
  - An untracked tiki that is never committed (and not ignored) is stamped once, at the first full load.
    With the mtime kept, that is a one-line content change and nothing else.
  - An editor holding a stamped file open may not notice the change (the mtime is unchanged)
    and drop the line on its next save. The next full load stamps it again,
    with a later time if the editor replaced the file and reset its birth time.
  - A tiki created and committed while tiki is not running gets the commit time.

### Birth time

- Read the file's birth time where available (Linux `statx`, macOS and Windows equivalents);
  `golang.org/x/sys` is already a dependency.
- When the OS or filesystem does not report it, use the modification time.
- Applies to the derived fallback only (steps 3 and 4 above), including the value written on sight.

### Display in local time

- TUI: `createdAt`, `updatedAt` and `activeAt` are converted to the machine's local zone before formatting,
  wherever they are shown (detail view, and any other view or column that shows them).
  Each moment is converted with the zone rules in force at that moment,
  so a summer timestamp shows the summer offset.
- `tiki exec` table output: the same three fields show local time, as RFC 3339 with the local offset
  (e.g. `2026-01-01T05:00:00-05:00`). *(my call: keeps seconds and stays unambiguous)*
- `tiki exec --format json`: unchanged, UTC RFC 3339.
- Workflow-declared `datetime` fields: unchanged everywhere.
- Ruki comparisons (`now() - createdAt < 24hour`) work on moments and are unaffected by display zone.

### AI skill (`ai/skills/tiki`)

- Agents create tikis only through `tiki exec 'create …'`, which writes `createdAt`,
  so no instruction to set it is needed. Agents must not set or change `createdAt` themselves.
- `references/files.md`: when editing a notes-only tiki's file directly, keep its `createdAt:` line unchanged,
  as with `id:`.
- `references/provenance.md`: the creation time is stored in the file once set; describe where it comes from.
- After editing the skill, re-run the fresh-agent trial (scratch project, isolated config)
  before reporting it done.

### Docs

- `docs/tiki-format.md`: `createdAt` moves from "Derived fields" to a stored field, with its resolution order.
- Any ruki or workflow doc that calls `createdAt` derived or not stored.
- Note in the docs that `tiki exec` table output shows local time and JSON shows UTC.

## Decided by the user

- Store in the tiki file (approach A), key `createdAt`, value in UTC.
- Stamp on sight only uncommitted tikis (option C), at full loads only (startup and `r`), not in live reload.
- Restore the file's modification time after stamping.
- Outside a git repository, stamp every tiki without `createdAt` at full loads.
- Display tiki's timestamps in local time in the TUI and `tiki exec` table output; JSON stays UTC.
- Fall back to the file's birth time, not its modification time, where available.
- Update the AI skill if needed.
- Process: write a plan, have an independent agent review it, incorporate the feedback,
  implement with TDD, have an independent agent review the implementation, fix what it finds,
  pass the full suite, then call it done.

## Left to the builder

- Where the on-sight stamping hooks into the full load (`loadLocked`, used by startup and `Reload`),
  and how the TUI enables it while `tiki exec` does not (e.g. a store option).
- How the one-line insert finds the frontmatter and where in it the line goes.
- The birth-time code per OS, and how tests cover a filesystem without birth time.
- Test design, following the existing store and git test patterns
  (`store/tikistore/git_times_test.go`, `store/internal/git/parity_test.go`).

## Verification

- Unit and store tests for each rule above, written test-first.
- Behavioural checks may drive tiki in a detached private tmux session against a scratch project.
- The user checks the TUI's look and feel on their own display.
