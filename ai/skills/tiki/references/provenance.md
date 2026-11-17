# Who created or changed a tiki

The creation time and author are fields:

```sh
tiki exec 'select createdAt, createdBy, updatedAt where id = "X7F4K2"'
```

tiki stores `createdAt` in the tiki's frontmatter and fills it in itself, at creation or later. Until then
it comes from the commit that added the file, else from the file's own timestamps. Never set or edit it.
Table output shows times in local time; JSON shows UTC.

`ruki` does not read git history. For anything else, resolve the tiki's path from its id first, since
the file can be anywhere under the current directory, then use git:

```sh
path=$(tiki exec --format json 'select filepath where id = "X7F4K2"' | jq -r '.[0].filepath')

git log --follow --diff-filter=A -- "$path"    # who created it
git log --follow -- "$path"                    # its history
git blame "$path"                              # who last edited each line
```
