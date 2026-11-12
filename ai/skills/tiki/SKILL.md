---
name: tiki
description: Use when working with tikis (Markdown tasks, docs and notes that carry an `id` in their frontmatter) - finding, creating, updating, deleting or estimating them, managing their dependencies, or implementing one, e.g. "implement X7F4K2"
allowed-tools: Read, Grep, Glob, Write, WebFetch, Bash(tiki exec:*), Bash(tiki live:*), Bash(tiki workflow schema:*), Bash(git log:*), Bash(git blame:*), Bash(jq:*)
---

# tiki

A tiki is a Markdown file under the current directory with an `id` and a `title` in its frontmatter.
Every other field is declared by the project's workflow, so the fields differ from project to project.

Read and change tikis with `tiki exec '<statement>'`, which validates the change, runs the workflow's
triggers and writes the file. Statements are SQL-like: `select`, `create`, `update`, `delete`.

## First: learn this project's fields

Run this once per session, before the first statement that names a field other than `id` or `title`:

```sh
tiki workflow schema
```

It lists each field's type, allowed values and default, and the triggers that refuse a change or
change other tikis. Use only those fields and values: any other field name is an `unknown field`
error. Pass enum values as strings (`"3"`, `"done"`).

If tiki answers `unknown workflow command`, it is older than this skill: read `workflow.yaml` in the
current directory, or `~/.config/tiki/workflow.yaml` when there is none.

In the examples below, `<field>` and `<value>` stand for a field and a value from the schema.

## Query

```sh
tiki exec 'select id, title'                                          # every tiki
tiki exec 'select id, title, <field> where <field> = "<value>"'       # filter
tiki exec 'select id, title where "login" in title order by updatedAt desc limit 10'
tiki exec 'select title, description where id = "X7F4K2"'             # read one tiki's body
tiki exec 'count(select where <field> = "<value>")'                   # a number
```

- Name the columns you need. A bare `select` returns every field except `description`, the Markdown
  body. `select *` also returns the body of every tiki it matches, so give it a `where`.
- Output is a table, with line breaks inside a cell shown as `\n`. Add `--format json` before the
  statement when you parse it.
- A tiki that lacks a field fails `<field> = "x"` and passes `<field> != "x"`. Add `has(<field>)`
  when that matters.

## Create

```sh
tiki exec 'create title="Fix login" description="What is wrong and how to see it." <field>="<value>"'
```

Prints `created <ID>`. Unset fields get their schema default. If the schema lists a `points` field,
set it in the same statement: read `references/points.md` first.

## Update

```sh
tiki exec 'update where id = "X7F4K2" set <field>="<value>" <field2>="<value2>"'
tiki exec 'update where id = "X7F4K2" set <list field>=<list field> + ["x"]'      # - removes
tiki exec 'update where id = "X7F4K2" set <field>=empty'                          # clear
```

Prints `updated N tikis`. A trigger that refuses prints `updated 0 tikis (1 failed)` with the reason
and exits with code 4: meet its condition, often in the same statement, and run it again.

## Delete

```sh
tiki exec 'delete where id = "X7F4K2"'
```

Prints `deleted N tikis`. A trigger can refuse this too.

## Implementing a tiki

When the user approves implementing a tiki, move it to the workflow's work-in-progress value and
show what you are doing on their board with a live status.

Never spend a call on a live status alone. Put it in front of a command you were going to run anyway,
joined with `;`:

```sh
# start, in one call: who you are, the status change, and the tiki's content
tiki live set X7F4K2 --agent "✻ Claude" --message "Researching"; tiki exec 'update where id = "X7F4K2" set <status field>="<in-progress value>"'; tiki exec 'select title, description where id = "X7F4K2"'

# each new phase: in front of that phase's first command
tiki live set X7F4K2 --message "Running tests"; go test ./...

# finished, stopped, or handing back to the user: in front of your last command
tiki live clear X7F4K2; git status --short
```

- If a trigger in the schema requires another field for that status change, set it in the same
  statement. For a field of type `user`, use `user()`, the current user, unless the user names
  someone: `set <status field>="<in-progress value>" <user field>=user()`.
- Use `;`, not `&&`: a failed status update must not block the real command. `tiki live` prints
  nothing on success, so don't redirect its output.
- `--message`: the current activity in 1–5 words, one line, at most 60 characters.
- `--agent`: a text glyph and/or short name for your own harness or model, at most 16 characters.
  Send it with the first update only; later updates keep it.
- Update once per phase (researching, planning, implementing, testing, reviewing), not per step.
  A status goes stale after 15 minutes, so refresh it during a long phase.
- Always clear it when you finish, stop, or hand control back. If your last command changes the
  tiki and a trigger could refuse it, clear after it instead: `tiki exec '...' && tiki live clear X7F4K2`.

## Rules

- Change a tiki that has workflow fields only through `tiki exec`, never by editing its file.
- An id is 6 characters of `[A-Z0-9]`, assigned by tiki. Take ids from a query or from
  `created <ID>`; never invent or reformat one.
- `tiki exec` never touches git. Don't stage or commit tiki files unless the user asks.
- Exit codes: 0 ok, 2 usage error, 3 startup failure, 4 statement failed or refused.

## Modules

Read one of these files, in this skill's directory, only when its case comes up, and only after
you have seen the schema:

| When | Read |
|---|---|
| Creating a tiki, or starting work on one with no points, and the schema lists `points` | `references/points.md` |
| The work involves blockers, or a field of type `list<ref>` such as `dependsOn` | `references/dependencies.md` |
| Creating a tiki from a file, or writing a long or quoted `description` | `references/long-text.md` |
| A query needs dates, sorting on several fields, lists, or subqueries | `references/queries.md` |
| Editing a notes-only tiki by hand, or asked where tiki files live | `references/files.md` |
| Asked who created or changed a tiki | `references/provenance.md` |
