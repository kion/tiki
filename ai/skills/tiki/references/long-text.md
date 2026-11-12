# Long and quoted text

A statement is a single-quoted shell argument that contains double-quoted ruki strings, so text goes
through two layers of quoting. Apply them in this order:

1. Inside the ruki string, write each backslash as `\\`, then each double quote as `\"`.
2. For the shell, write each single quote in the statement as `'\''`.

Line breaks can stay as they are: a ruki string may span lines.

```sh
tiki exec 'create title="Quote handling" description="She said \"it'\''s broken\".
Second paragraph, with a path: C:\\temp"'
```

## Creating a tiki from a file

1. Read the source file.
2. Summarize its content into a short title.
3. Use the file content as the `description`, escaped as above:
   `tiki exec 'create title="<summary>" description="<escaped content>"'`

To check the result, read the body back: `tiki exec 'select description where id = "<ID>"'`.
