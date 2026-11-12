# More query patterns

`<field>` and `<value>` stand for a field and a value from `tiki workflow schema`. The field names in
the comments are only typical ones.

```sh
# dates: compare a date field with a date literal; guard with has() so unset dates are skipped
tiki exec 'select id, title, <date field> where has(<date field>) and <date field> < 2026-07-15'

# date arithmetic and durations (1day, 2week, 3hour)
tiki exec 'update where id = "X7F4K2" set <date field>=2026-04-01 + 2day'
tiki exec 'select id, title where now() - updatedAt < 24hour'

# lists: membership, several values, emptiness
tiki exec 'select id, title where "bug" in <list field>'            # e.g. tags
tiki exec 'select id, title where <field> in ["a", "b"]'
tiki exec 'select id, title where <field> not in ["a"]'
tiki exec 'select id, title where <field> is empty'                 # also: is not empty

# text: substring of a string field (not of an enum or id)
tiki exec 'select id, title where "login" in title'

# logic and grouping
tiki exec 'select id, title where not (<field> = "<value>" or <field2> = "<value2>")'

# sorting on several fields, and limiting
tiki exec 'select id, title order by <field> asc, createdAt desc limit 5'

# the current user, for a field of type user
tiki exec 'select id, title where <user field> = user()'

# scalars: a count or a yes/no instead of rows
tiki exec 'count(select where <field> = "<value>")'
tiki exec 'exists(select where <field> = "<value>")'

# recently active, including tikis an agent is working on
tiki exec 'select id, title, activeAt order by activeAt desc limit 10'
```

- Enum fields compare and sort by their order in the schema, so `<` and `order by` work on them.
- A date field takes a date literal (`2026-07-15`), not `now()`, which is a timestamp.
- A recurrence field is set with `daily()`, `weekly("monday")` or `monthly(1)`, and cleared with
  `empty`.
- `--format json` returns an array of row objects: pipe it to `jq` rather than parsing the table.
- The full language reference is at
  https://github.com/kion/tiki/blob/main/docs/ruki/index.md. Fetch it only when these
  patterns don't cover the case.
