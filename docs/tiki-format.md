# Tiki format

`tiki` is a viewer over the current directory. Every `.md` file beneath the cwd that carries a frontmatter
`id` is a **tiki**: a Markdown file with YAML frontmatter, a bare `id`, and arbitrary body content. Beyond
`id` and `title`, frontmatter is an open field map — a tiki participates in board/list views when its
fields satisfy that view's `select` filter, typically via `has(...)` checks. A `.md` file with no `id` is
not store-indexed but is still wiki-viewable by path — plain prose coexists with tikis, no warning.

## Workspace layout

Tikis live anywhere under the current directory. Folder structure is user-controlled organization, not
identity.

```
./                          # the scan root (current working directory)
├── tiki-workflow.yaml      # workflow/view configuration (excluded from scan)
├── tiki-config.yaml        # appearance/display config (excluded from scan)
├── ABC123.md               # a tiki carrying status/type/priority fields
├── DEF456.md               # a tiki with only id + title
├── notes.md                # plain Markdown, no id — wiki-viewable by path, not a tiki
├── architecture/
│   ├── X7K2QM.md           # a design note tiki
│   └── diagrams/
│       └── 9LP4ZR.md       # a diagram-page tiki
└── assets/                 # shared images, svgs, binary assets
    └── logo.png
```

Rules:

- `./**/*.md` is scanned recursively and loaded; files with a frontmatter `id` become tikis.
- `.git/`, dotted directories, and paths matched by `.gitignore`/`.tikiignore` at the scan root are skipped.
- `tiki-workflow.yaml` and `tiki-config.yaml` are configuration files, not tikis.
- Non-Markdown files (images, svgs, binary assets) are not loaded as tikis; keep them under `assets/`
  or alongside the tikis that reference them.
- New tikis from `tiki demo`, piped input, and `ruki create` are named `./<slug>.md`, where `<slug>` is
  a lowercase-kebab transliteration of the title (e.g. "Fix Login Bug" → `fix-login-bug.md`). On a name
  collision a numeric suffix is appended (`fix-login-bug-2.md`). The filename is not the source of the
  `id` — the frontmatter `id` is — so you are free to rename or move files afterward; the `id` is stable.

## Required frontmatter

A tiki must declare an `id` to be store-indexed. A file with no `id` is skipped silently (it is plain
wiki content, not a managed tiki — never a diagnostic). A file that *has* an `id` but malformed is still
reported in diagnostics.

```yaml
---
id: ABC123
title: Implement search
---

Markdown body.
```

- **`id`** — required, globally unique across the scan root, and assigned by tiki. Either 6 uppercase
  letters or digits (`X7F4K2`, the default), or a prefixed sequential id such as `FEAT-12` or `MEM-DB-35`
  when the workflow sets a `prefix` (`FEAT_12` or `B95` when its `prefixSeparator` is `_` or empty; see
  [Readable sequential ids](customization/customization.md#readable-sequential-ids)). The old `TIKI-XXXXXX`
  form is not recognized; edit such ids by hand, or with `tiki ids rename`.
- **`formerIds`** — optional, written by `tiki ids rename` and `tiki ids migrate`: the ids the tiki had
  before. They are never given out again, so a search for an old id still finds only this tiki.
- **`title`** — optional on disk; the loader accepts a missing or empty title and stores it as `""`.
  Required and non-empty (max 200 characters) on **create** and **update** paths via `tiki exec` —
  ruki rejects assignments that would leave the title blank. Recommended on every tiki, since the
  title is the display label across all views.

## Workflow-declared fields (optional)

Beyond `id` and `title`, frontmatter is an open field map. The fields below are **workflow-declared**
— the active `tiki-workflow.yaml fields:` lists them, so they get type coercion, validation, and any
typed UI treatment. They are all optional. Absence is preserved on disk: a tiki with no `status:`
key has no status, not a defaulted one. Default values apply only on explicit creation —
`ruki create`, piped capture, and the TUI's create action.

```yaml
---
id: X7F4K2
title: Fix login bug
type: bug
status: inProgress
assignee: alex
priority: medium-high
points: 5
tags:
  - frontend
  - urgent
dependsOn:
  - ABC123
  - DEF456
due: 2026-04-01
recurrence: 0 0 * * MON
---

Describe what is broken and how to reproduce it.
```

### `type`

Optional. The bundled workflows declare `type` as an enum, so its value must match that workflow
declaration. On explicit creation, the value marked `default: true` is applied. The stock enum values
include `story`, `bug`, `spike`, and `epic`; each can have its own label and emoji. The field name has no
built-in behavior: changing its declared type changes validation and editing like any other workflow field.

```yaml
type: bug
```

### `status`

Optional. The bundled workflows declare `status` as an enum, so its value must match that workflow
declaration. When absent, the tiki falls outside any view whose lane filter requires `status`. On explicit
creation, the value marked `default: true` is applied. The field name has no built-in behavior: changing its
declared type changes validation and editing like any other workflow field.

```yaml
status: inProgress
```

### `priority`

Optional. Priority is an `enum` field whose values are defined in `tiki-workflow.yaml` — tiki has no built-in
priority scale. The stock kanban workflow ships with five string values, most to least urgent:

| Value | TUI label | Emoji |
|-------|-----------|-------|
| `high` | High | 🔴 |
| `medium-high` | Medium High | 🟠 |
| `medium` | Medium | 🟡 |
| `medium-low` | Medium Low | 🟢 |
| `low` | Low | 🔵 |

The default on explicit creation is `medium`. The value stored in frontmatter is the enum key
(a string), not a number. Unknown values are retained as stale data for manual repair.

```yaml
priority: medium-high
```

### `points`

Optional. Like `priority`, points is an `enum` field defined in `tiki-workflow.yaml` — there is no built-in
numeric scale or `maxPoints` setting. The stock kanban workflow ships with a 1–10 estimation scale,
`1` (small, easy) through `10` (large, hard), stored as strings. It has no default, so a new tiki stays
unestimated until someone sets its points. Redefine the `points` field in your own workflow to use a
different scale.

```yaml
points: 3
```

### `assignee`

Optional free-form string, typically a username. Default: empty. The bundled workflows declare this field as
`type: user`, which changes the editor to a suggestion-enabled user picker but does not change the frontmatter
shape or ruki type.

```yaml
assignee: alex
```

### `tags`

Optional string list. Arbitrary labels attached to a document.
Values are trimmed, empty entries are dropped, and duplicates are removed.
Default: empty. Both YAML list formats are accepted:

```yaml
# block list
tags:
  - frontend
  - urgent

# inline list
tags: [frontend, urgent]
```

### `dependsOn`

Optional string list. Each entry must be the id of an existing document (`X7F4K2` or `FEAT-12`). Ids are uppercased, empty entries are dropped, and duplicates are removed. A dependency means
this document is blocked by the listed documents. Default: empty.

```yaml
# block list
dependsOn:
  - ABC123
  - DEF456

# inline list
dependsOn: [ABC123, DEF456]
```

Legacy `TIKI-ABC123`-style references are rejected by validation — edit them to the bare `ABC123` form.

### `due`

Optional date in `YYYY-MM-DD` format (date-only, no time component).
Represents when the document should be completed by. Empty string or omitted field means no due date.

```yaml
due: 2026-04-01
```

### `recurrence`

Optional cron string specifying a recurrence pattern. Displayed as English in the TUI.
This is metadata only — it does not auto-create documents on completion.

Supported patterns:

| Cron | Display |
|------|---------|
| (empty) | None |
| `0 0 * * *` | Daily |
| `0 0 * * MON` | Weekly on Monday |
| `0 0 * * TUE` | Weekly on Tuesday |
| `0 0 * * WED` | Weekly on Wednesday |
| `0 0 * * THU` | Weekly on Thursday |
| `0 0 * * FRI` | Weekly on Friday |
| `0 0 * * SAT` | Weekly on Saturday |
| `0 0 * * SUN` | Weekly on Sunday |
| `0 0 1 * *` | Monthly |

The cron string above is the on-disk form. In `ruki` (workflow rules and the command bar) you author these values with
the `daily()`, `weekly("<weekday>")`, and `monthly(<day>)` constructors rather than typing cron by hand — see the ruki
built-in functions reference.

```yaml
recurrence: 0 0 * * MON
```

## Project-specific fields

Any field declared under `tiki-workflow.yaml fields:` is round-tripped through save/load with type
coercion. Unknown frontmatter keys that the workflow does not declare are preserved as-is, so
workflow schema changes do not lose data. See [Custom fields](customization/custom-fields.md).

## Creation time (`createdAt`)

tiki stores a tiki's creation time in its frontmatter, in UTC:

```yaml
createdAt: 2026-01-01T10:00:00Z
```

- A value in the file always wins. By hand you can write any YAML timestamp, quoted or not (`2026-01-01`,
  `2026-01-01 10:00:00`, or one with an offset such as `+02:00`); the next save rewrites it in UTC.
  A value that isn't a timestamp is ignored and replaced on the next save.
- Without one, tiki uses the time of the commit that added the file, then the file's birth time where
  the filesystem records it, then its modification time.
- tiki writes it when it creates a tiki, and when it saves one whose time git can't contradict: a stored
  value, the commit date, or the file's own times for a file git has no record of. A time that is only a
  guess stays out of the file. For a tiki that appeared while tiki was running, that lasts until the next
  startup or `r` reads the history. For a tiki whose rename was committed, or one inside a git repository
  nested below the project, it lasts until the line is added some other way, e.g. by hand.
- At startup and on a full reload (`r`) tiki also adds it where it knows the right time and the file is
  already a change you will commit, or sits outside a git repository:
  - a new, uncommitted tiki gets its file's birth time;
  - a committed tiki you have modified or staged gets the date of the commit that added it;
  - a tiki you moved (with `mv` or `git mv`) and haven't committed yet is recognised by its id,
    and keeps the date of the commit that added it at its old path.

  A committed tiki with no changes is left alone. Stamping adds only the one line, and the file keeps its
  modification time, so it doesn't count as an edit.
- Nothing is added on load to symlinked files, to files with an invalid `createdAt`, to files whose
  modification time you can't set, to tikis inside a git repository nested below the project (such as a
  submodule), or anywhere in a repository whose history tiki can't read or whose paths don't line up with
  the files on disk.
  `tiki exec` and live reload never add it.

## Derived fields

These fields are not stored in the file:

- `created by`
- `updated at`

`updated at` is the later of the file's last commit and its mtime, so an uncommitted edit counts, and so
does any rewrite of the file, such as a fresh clone or a checkout that changes it. `created by` is
populated from git authorship when available; for uncommitted files or a non-repo scan root it falls back
to the current `tiki` identity (configured `identity.name` or `identity.email` → git user → OS account name).

## Sparse tikis (notes, docs, prompts)

A tiki with only `id` and `title` in its frontmatter is just a tiki with no workflow-declared
fields set — useful as a note, page, or reference. There is no separate "plain document" class in
the model; the absence of fields is the whole story. Such a tiki:

- is rendered by markdown, wiki, and detail views
- falls outside any view whose lane filter requires workflow-declared fields (typical board and
  list lanes)
- is queryable by `ruki` like any other tiki. Comparisons against absent fields are well-defined:
  `where status = "done"` evaluates `false` for tikis with no `status`, and `where status != "done"`
  evaluates `true`. Use `has(<field>)` when you need to filter explicitly by presence — for example,
  `select where has(status)` lists every tiki that carries a status, regardless of value.
- gains a field whenever you assign one. `update where id = "ABC123" set status = "inbox"` simply
  writes `status: inbox` into the frontmatter. There is no special "promotion" event — fields are
  added and removed like any other map entry.

```yaml
---
id: QR9XV2
title: Architecture overview
---

# System architecture

Entry points, component boundaries, and deployment diagrams.
```

## Cross-tiki links

Reference another tiki by its id inside double brackets (`[[X7F4K2]]`, `[[FEAT-12]]`):

```markdown
See the [[ABC123]] for background.
```

Wikilinks resolve through the tiki index, so they survive file moves and renames. Unknown ids render as
a literal `[[ABC123]]` with a `*(not found)*` marker — broken references stay visible instead of silently
disappearing.

Plain Markdown links to relative paths also work inside bodies; use them for anchored links to specific
sections within a tiki.

## How the body is displayed

The detail view renders a tiki body as Markdown, with one difference from a rendered `.md` file: the
line breaks you typed are kept, and so is the indentation of continuation lines. A hand-laid-out block
such as

```markdown
The options:
A. First option, wrapped by hand
   onto an indented line.
B. Second option.
```

shows as four rows, not as one joined paragraph. A line longer than the pane still wraps, and the
wrapped part keeps its line's indent. This applies to tiki bodies only: wiki pages, the standalone
Markdown viewer, and a doc page opened by following a link from a tiki body all join a paragraph's
lines as usual.

Press `v` in the detail view to switch the body between **rendered** and **source**. Source shows the
body text exactly as typed, with Markdown syntax colouring; links and images are not active there. The
mode you last used is remembered from tiki to tiki and across restarts (see `state.yaml` in
[Configuration](config.md)).
