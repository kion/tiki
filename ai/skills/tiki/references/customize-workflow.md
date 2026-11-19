# Changing a project's workflow

The workflow file declares the project's fields, views, actions and triggers. First check that tiki is
new enough: `tiki --help` must list `tiki config`. If it doesn't, never run `tiki config` (an older tiki
opens its viewer on the word "config"); the workflow is `./workflow.yaml`, or else
`~/.config/tiki/workflow.yaml`.

## Which file

`tiki config` marks the workflow in use as `loaded`; `tiki workflow schema` prints its path first.

- Workflows don't merge: one file holds the whole workflow. If the one in use is global (in the user's
  config directory), ask the user whether the change is for this project only. If it is, copy the whole
  global file to `tiki-workflow.yaml` in the current directory and edit the copy. Never edit the global
  workflow unless the user asks.
- To start a project from a bundled workflow (`kanban`, `todo`, `bug-tracker`), run
  `tiki workflow install <name> --current`. It overwrites the project's workflow, so ask first when one
  exists. `tiki workflow describe <name>` shows what a bundled workflow is for.
- After every edit, run `tiki workflow schema`. It loads the fields, triggers, views and actions the way
  tiki does at startup and prints the first error. Fix it and run it again until it succeeds; a broken
  workflow stops tiki from starting. A running tiki picks up changes on its next start.

## Fields

```yaml
fields:
  - name: severity
    type: enum
    values:
      - {value: low, label: Low}
      - {value: medium, label: Medium, default: true}
      - {value: high, label: High}
  - name: due
    type: date
```

- Types: `enum`, `text`, `user`, `integer`, `boolean`, `date`, `datetime`, `stringList`, `tikiIdList`,
  `recurrence`.
- Optional per field: `default:`, `required: true` (set on create), `immutable: true` (never changes
  after create). An enum value can carry a `label` and a `visual` (an emoji or symbol).
- Keep the `status` and `type` fields.
- Lane filters and actions, other views and triggers can name a field's values. When you add, rename
  or remove a value, search the file for its uses and update them too.
- Rename or remove an enum value in three steps, so no tiki is left holding a value the workflow no
  longer has: add the new value, move the tikis
  (`tiki exec 'update where <field> = "old" set <field>="new"'`), then remove the old value.

## Readable ids

```yaml
prefix: ACME           # new tikis get ACME-1, ACME-2, ...
prefixSeparator: "-"   # "-" (ACME-12), "_" (ACME_12) or "" (ACME12)
```

- An enum field's values can add their own segment, such as `{value: bug, prefix: BUG}` for ids like
  `BUG-12`. That field must be `required: true` and `immutable: true`.
- Existing tikis keep their ids. `tiki ids migrate --dry-run` shows how they would be renamed and
  writes nothing; run `tiki ids migrate` only when the user asks for it.

## Views

```yaml
views:
  - name: Review
    kind: board
    key: "F5"
    description: "Tikis waiting for review"
    layout: |
      id + " " + title
    lanes:
      - name: Waiting
        filter: select where status = "review" order by updatedAt desc
        action: update where id = id() set status="review"
```

- `kind`: `board`, `list`, `wiki`, `detail` or `search`. A board needs `layout:` and at least one lane.
- A lane's `filter` is a ruki `select`; its `action` is the `update` applied when a tiki is moved into
  the lane. Add a value to `status` before a lane uses it.
- To add a column to an existing board, add a lane to that view.

## Actions

```yaml
actions:
  - key: "a"
    label: "Assign to me"
    action: update where id = id() set assignee=user()
```

Top-level `actions:` work in every view; a view's own `actions:` work only there and win on the same
key. `kind: view` actions open another view, and `require:` greys out an action until its condition
holds: read the docs for those.

## Triggers

`triggers:` holds ruki rules that refuse a change or change other tikis when something happens. Read the
triggers doc before writing one.

## Docs

Fetch these only when this file doesn't cover the case:

- https://github.com/kion/tiki/blob/main/docs/customization/customization.md (views, actions, layouts)
- https://github.com/kion/tiki/blob/main/docs/customization/custom-fields.md
- https://github.com/kion/tiki/blob/main/docs/workflow-format.md
- https://github.com/kion/tiki/blob/main/docs/ruki/triggers.md
