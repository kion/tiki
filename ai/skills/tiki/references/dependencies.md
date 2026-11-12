# Dependencies

A field of type `list<ref>` holds the ids of other tikis. The examples use `dependsOn`, the usual
name for "this tiki is blocked by those"; check the schema for the project's field. `<field>` and
`<value>` stand for a field and a value from the schema, such as the status field and its closing
value.

```sh
# a tiki's dependencies
tiki exec 'select id, title, dependsOn where id = "X7F4K2"'

# what depends on a tiki (reverse lookup)
tiki exec 'select id, title where dependsOn any id = "X7F4K2"'

# blocked tikis: any dependency that is not finished
tiki exec 'select id, title where dependsOn any <field> != "<value>"'

# blocked tikis and what blocks them, in one call: match the ids in dependsOn against the second list
tiki exec 'select id, title, dependsOn where dependsOn any <field> != "<value>"'; tiki exec 'select id, title where <field> != "<value>"'

# unblocked tikis: every dependency finished (true when there are none)
tiki exec 'select id, title where dependsOn all <field> = "<value>"'

# add and remove a dependency
tiki exec 'update where id = "X7F4K2" set dependsOn=dependsOn + ["ABC123"]'
tiki exec 'update where id = "X7F4K2" set dependsOn=dependsOn - ["ABC123"]'
```

- In the two-list recipe, a blocker is still open when its id appears in the second list. Ids in
  `dependsOn` that are missing from it are finished.
- Each entry must be the id of a tiki that exists, or the update is rejected.
- `any` and `all` test the condition after them against each referenced tiki.
- Triggers often act on dependencies, for example refusing to close a tiki whose dependencies are
  open, or removing a deleted tiki's id from other tikis. The schema's trigger list says which apply
  here. Tell the user when a statement you ran set one off.
