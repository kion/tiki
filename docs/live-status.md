# Live status

An AI agent working on a tiki can publish a live status: who is working and a short message. tiki
adds a progress animation and how long it has been since the last update. Boards show the status in
the bottom border of the tiki's card, and the detail view under the tiki's fields, while
[live reload](config.md#live-reload) is on:

```
┌──────────────────────────────┐
│X7F4K2                        │
│Status animation              │
│priority high  points 3       │
└─ ✻ Claude ✢ 3m Implementing ─┘
```

The agent chooses its name or glyph and the message, so any harness can use it. The animation is
tiki's own and the same for every status (`·✢✳✶✻✽`), so statuses from different agents look alike
and take the theme's colours. The bundled [AI skill](ai.md) tells agents when to post updates.

## Publishing a status

```sh
# first update: who is working (kept for later updates)
tiki live set X7F4K2 --agent "✻ Claude" --message "Researching"

# later updates need only the message
tiki live set X7F4K2 --message "Implementing parser"

# done, stopped, or handing back to a person
tiki live clear X7F4K2
```

| Option | Meaning |
|---|---|
| `--message <text>` | What is happening now, in 1–5 words. Required; single line, at most 60 characters. |
| `--agent <text>` | Who is working: a glyph and/or a short name, at most 16 characters. |

`--agent` is kept between updates. Pass an empty value (`--agent=`) to clear it. Every `set`
records the current time. Once a minute has passed, the card shows the age of the status in whole
minutes: `1m`, `3m`, `2h 5m`, `3d 4h`. A fresher status shows no age.

`tiki live set` checks that the id belongs to a tiki under the current directory, so run it from
the same directory as `tiki exec`. `tiki live clear` works for any well-formed id, including a tiki
that has since been deleted. Both exit with `0` on success, `1` if the status file can't be
written or removed, and `2` for a usage or input error.

Prefer text-style symbols (`✻`, `◆`, `✦`) to emoji for `--agent`: emoji keep their own colours
rather than the status colour, and terminals disagree on how wide some are, which can shift the rest
of the card border.

## Where it is stored

Each status is a small YAML file at `.tiki/live/<ID>.yaml` under the working directory:

```yaml
agent: ✻ Claude
message: Implementing parser
updatedAt: "2026-10-11T18:22:05Z"
```

The tiki file itself is never changed: a status update doesn't touch git, doesn't change the tiki's
`updatedAt`, and can't conflict with an edit you have open. `.tiki/` contains a `.gitignore` that
ignores everything in it, so statuses never reach git.

Other tools may write these files directly instead of calling `tiki live`. Only `message` is
required; `updatedAt` defaults to the file's modification time, and unknown keys are ignored. Write
to a temporary file in the same directory and rename it, so tiki never reads a half-written file.

## On the board and in the detail view

- Statuses show only while live reload is on: with it off, tiki can't see updates, so a status
  would be out of date. Toggle live reload with `Ctrl-L`.
- Updates appear within about a second, and the age keeps counting between updates.
- The agent and glyph are orange and the message a dimmer orange. The default themes use the
  orange of Claude Code's status line; the others use their own orange.
- When the card is too narrow, the status gives way part by part so the message stays readable:
  first the agent, then the age. Only the message is ever cut short with `…`, never the age. On the
  last visible card of a long lane the status also stops before the `▼ N more` hint.
- To clear a status yourself, select its card and run **Clear Live Status** from the action
  palette (`Ctrl-A`). It works in every workflow.
- A status that hasn't been updated for 15 minutes is dimmed and stops animating. Its age keeps
  growing until it is updated or cleared. Change the threshold with `liveStatus.staleAfter` in
  [config.yaml](config.md), or the `TIKI_LIVESTATUS_STALEAFTER` environment variable.

## Clearing on completion

tiki has no built-in notion of "done", so the bundled workflows clear a status with a trigger when a
tiki reaches their closing status: `done` in kanban and todo, `verified` or `wontFix` in
bug-tracker.

```yaml
triggers:
  - description: clear the agent live status when a tiki is done
    ruki: >
      after update
        where new.status = "done" and old.status != "done"
        run("\"${TIKI_BIN:-tiki}\" live clear " + new.id)
```

tiki sets `TIKI_BIN` to its own executable for the commands it runs, so the trigger uses the same
tiki even when another version comes first on your `PATH`. Like every `run()`, it needs `sh`, so it
doesn't run on Windows; there, clear statuses with `tiki live clear` or the "Clear Live Status"
action.

A workflow you installed before this feature existed doesn't have the trigger. Add it to your
`workflow.yaml`, or reinstall the bundled workflow with `tiki workflow reset`.

## Limitations

- Cards with a single-row layout (no border), such as the todo board, don't show live statuses.
- The detail view shows the status under the tiki's fields, so it's hidden while the description is
  fullscreen.
- A tiki has one status. If two agents work on the same tiki, the last update wins, and an update
  that sends only `--message` keeps the agent the other agent set.
- An agent working in another clone or git worktree writes its status there, so it doesn't reach a
  board running in your checkout.
