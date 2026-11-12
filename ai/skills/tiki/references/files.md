# Tiki files

- Tikis are read from and written to the current directory, at any depth. A `.doc/` subdirectory is
  scanned too. No setup is needed.
- `create` writes `<cwd>/<slug of the title>.md`, for example `fix-login-bug.md`, adding a numeric
  suffix on a name collision.
- A tiki is identified by the `id:` in its frontmatter, not by its filename. Files can be renamed or
  moved into subfolders without breaking references, because `[[ID]]` wikilinks and `list<ref>`
  fields use ids.
- Get a tiki's path from its id, never by guessing a filename:
  `tiki exec 'select filepath where id = "X7F4K2"'`.
- An `id:` must be exactly 6 characters of `[A-Z0-9]`. A file with any other id (wrong length,
  lowercase, a prefix such as `task-1234`) fails to load and has to be fixed by hand.

## Notes-only tikis

A tiki with only `id` and `title` in its frontmatter is a plain note or document. It is reachable by
id and path, but it falls outside any view that filters on a workflow field.

- You may edit a notes-only tiki's file directly with Read and Edit. Keep its `id:` line unchanged.
- A tiki that has workflow fields is changed only through `tiki exec`, so validation and triggers
  run.
- Setting a workflow field on a notes-only tiki turns it into a tracked one:
  `tiki exec 'update where id = "X7F4K2" set <field>="<value>"'`.

## Git

`tiki exec` only writes files. A created tiki is left untracked and a deleted one shows as an
unstaged deletion. Leave staging and committing to the user unless they ask.
