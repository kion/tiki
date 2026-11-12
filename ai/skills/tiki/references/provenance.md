# Who created or changed a tiki

The creation time and author are fields:

```sh
tiki exec 'select createdAt, createdBy, updatedAt where id = "X7F4K2"'
```

`ruki` does not read git history. For anything else, resolve the tiki's path from its id first, since
the file can be anywhere under the current directory, then use git:

```sh
path=$(tiki exec --format json 'select filepath where id = "X7F4K2"' | jq -r '.[0].filepath')

git log --follow --diff-filter=A -- "$path"    # who created it
git log --follow -- "$path"                    # its history
git blame "$path"                              # who last edited each line
```
