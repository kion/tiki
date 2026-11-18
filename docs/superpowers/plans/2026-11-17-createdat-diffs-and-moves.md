# createdAt for Changed Files and Moved Tikis Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** At full loads, stamp `createdAt` into tiki files that are already a git diff when tiki knows the correct value, and recognise moved tikis by id so they keep their original creation time.

**Architecture:** Two new read-only git operations (changed files from `git status`, file contents at HEAD) in both backends, with parity tests.
The store uses them once per full load: committed-and-changed files get their commit date stamped; gone `.md` paths at HEAD give an id → old path map that dates moved tikis.

**Tech Stack:** Go, shell git and go-git backends in `store/internal/git`.

**Spec:** `docs/superpowers/specs/2026-11-17-stored-created-at-design.md`, section "Revision 2".

## Global Constraints

- The first plan's Global Constraints (`docs/superpowers/plans/2026-11-17-stored-created-at.md`) still hold
  (file format, comment rule, commits on main, TDD, `gofmt`/`golangci-lint` with no new findings/`go test -race ./...`, no GUI, stay inside the project),
  with two overrides: stamping now also covers changed committed files and recognised moves (not only uncommitted tikis),
  and `createdBy` of a recognised move comes from the old path's add commit.
- Both backends return repo-relative, slash-separated, unescaped paths (shell: `-z`, `core.quotePath=false`).
- The new git work runs only when the full load's history state is `historyRead`.
- A committed file with no changes is never stamped. A moved tiki whose old path has no add commit stays `CreatedAtGuess`.
- If `ChangedFiles` or `HeadContents` fails, no tiki is upgraded to `CreatedAtFileTimes` in that load.
- The new work is skipped when no admitted tiki is a candidate (a `CreatedAtGuess`, or with stamping on, a `CreatedAtGit` tiki), so most loads add no git call.
- Tikis inside a nested repository are never stamped, on every branch.
- "The commit that added the file" never counts a rename: both backends detect renames in `Author`/`AllAuthors`
  (shell `-M`; go-git `DetectRenames: true` for those two only), whatever the user's `diff.renames`.

## Review Focus

1. A tiki moved with plain `mv`, with `git mv`, and moved plus edited: all keep the old path's commit date. Task 2.
2. A modified committed tiki that the user also staged (`MM`): stamped once, with the commit date. Task 2.
3. Two gone paths with the same id: the tiki stays a guess, not stamped, not written. Task 2.
4. Non-ASCII gone paths: matched. Task 1 parity, Task 2.
5. `ChangedFiles`/`HeadContents` failing in a repo: nothing untracked is stamped or written. Task 2.
6. go-git on a filesystem without permission bits (`core.fileMode=false`): a chmod alone is not a change. Task 1.
7. A repo with no commits yet: no error, nothing gone. Task 1.

---

### Task 1: Git operations: changed files and contents at HEAD

**Files:**
- Modify: `store/internal/git/internal/types/types.go`, `store/internal/git/types.go`, `store/internal/git/ops.go`, `store/internal/git/autoselect.go`,
  `store/internal/git/internal/shell/` (new file `status.go`), `store/internal/git/internal/gogit/` (new file `status.go`)
- Modify (fakes): `store/tikistore/reload_paths_test.go` (`countingGit`: new methods call `bump()` and return empty maps, nil error), `store/tikistore/identity_test.go` (`fakeGitOps`)
- Declare the `FileChanged`/`FileGone` constants in package `git` too (an alias doesn't re-export them).
- Test: `store/internal/git/parity_test.go`

**Interfaces:**
- Produces:
  - `type FileChange uint8` with `FileChanged` (tracked file differing from HEAD in the index or working tree and still present) and `FileGone` (tracked at HEAD, missing from the working tree or the index), aliased in `store/internal/git`.
  - `ChangedFiles(dirPattern string) (map[string]FileChange, error)` on `GitOps`: untracked files excluded; a staged rename gives the new path `FileChanged` and the old path `FileGone`.
    Shell: `git --no-optional-locks -c core.quotePath=false status --porcelain=v1 -z --untracked-files=no --no-renames --ignore-submodules=all -- <dirPattern>`
    (no index writes, single-path entries only); `D` in either column → `FileGone`, any other non-`?` code → `FileChanged`.
    go-git: `Worktree.Status()` filtered with `matchesPattern(path, toRelativePattern(dirPattern))`, skipping untracked and unmodified;
    `Deleted` in staging or worktree → `FileGone`, anything else → `FileChanged`, except a worktree-only change whose content hash equals the index entry's (mode-only).
  - Both backends: shell `Author`/`AllAuthors` gain `-M`; go-git `Author`/`AllAuthors` diff with `DetectRenames: true` (a new helper; `diffTrees` stays as is for the other callers).
  - `HeadContents(paths []string) (map[string][]byte, error)` on `GitOps`: contents at HEAD for repo-relative paths; missing paths are left out; empty input returns an empty map without running git.
    Shell: one `git cat-file --batch` fed through `cmd.Stdin`, input lines `HEAD:<path>` (paths containing a newline skipped);
    read each header, then exactly `size` bytes and the LF; `missing`/`ambiguous` lines leave the path out.
    go-git: HEAD commit's tree `File(path).Contents()`; an unborn HEAD returns an empty map, no error.

- [ ] **Step 1: Write the failing tests.**
  - `TestParity_ChangedFiles`: with shell git, commit `sub/a.md`, `sub/b.md`, `sub/c.md`, `sub/d.md`, `sub/Über.md`;
    then append to `a.md`; `git mv sub/b.md sub/b2.md`; `mv sub/c.md sub/done/c.md` (plain); append to `d.md` and `git add` it; `mv sub/Über.md sub/Ünder.md`.
    also change `other/o.txt` (outside `sub`) and, with `core.fileMode=false`, chmod a committed `sub/e.md`.
    Both backends opened at `sub` (as `TestParity_RepoRelPath` does), `ChangedFiles(<abs sub>)` equal exactly
    `{sub/a.md: FileChanged, sub/b2.md: FileChanged, sub/d.md: FileChanged, sub/b.md: FileGone, sub/c.md: FileGone, sub/Über.md: FileGone}`.
  - `TestParity_ChangedFilesUnbornHead`: a repo with `git add`ed files and no commit: both backends return no error.
  - `TestParity_CommittedRenameIsNotAnAdd`: commit `a.md`, `git mv a.md b.md`, commit, and set `diff.renames=false` in the repo config:
    both backends' `AllAuthors` contain `a.md` and not `b.md`.
  - `TestParity_HeadContents`: both backends: `HeadContents(["sub/c.md", "sub/Über.md", "sub/missing.md"])` returns the committed bytes for the first two and no entry for the third;
    `HeadContents(nil)` returns an empty map; on a repo with no commit, both return an empty map and no error.
- [ ] **Step 2: Run** `go test ./store/internal/git/ -run 'ChangedFiles|HeadContents' -count=1`. Expected: build failure.
- [ ] **Step 3: Implement** both operations in both backends, the selector methods, and the fakes (empty results).
- [ ] **Step 4: Run** the Step 2 command, then `go test ./store/... -count=1`. Expected: PASS.
- [ ] **Step 5: Commit** (`git layer reports changed files and reads files at HEAD`).

### Task 2: Stamp changed committed tikis and recognise moves

**Files:**
- Modify: `store/tikistore/stamp.go`, `store/tikistore/persistence.go` (`loadLocked`)
- Test: `store/tikistore/created_at_moves_test.go`

**Interfaces:**
- Consumes: `ChangedFiles`, `HeadContents` (Task 1); `trustFileTimes`, `historyStateFor`, `stampLogged`, `stampCreatedAt` (existing).
- Produces:
  - `func (s *TikiStore) movedOrigins(changes map[string]git.FileChange) map[string]string`: id → old repo-relative path,
    from gone `.md` paths read through `HeadContents`, ids parsed with `store.ParseFrontmatter`, yaml and `document.FrontmatterID`/`NormalizeID`.
    Gone paths count when they end in `.md` (case-insensitive) and their HEAD id passes `idfmt.IsValidID` after `NormalizeID`.
    An id found under two gone paths maps to `""` (ambiguous). Signature: `(map[string]string, error)`; a `HeadContents` error is returned.
  - In `loadLocked`, when the state is `historyRead` and some admitted tiki is a candidate: one `ChangedFiles(s.dir)`, then `movedOrigins`.
    Either failing sets a flag that keeps every tiki in this load from `trustFileTimes`.
    Per admitted tiki, before `trustFileTimes`: if its source is `CreatedAtGuess` and its id is in the map, it is a move:
    with a non-empty old path that has an author entry, set `CreatedAt`, `createdBy` (as `loadTikiFile` does) and `CreatedAtGit`;
    otherwise (ambiguous, or no add commit) keep `CreatedAtGuess` and skip `trustFileTimes` for it.
    Stamp (when `stampOnLoad`, and not `inNestedRepo`) if the source is `CreatedAtFileTimes`,
    or it is `CreatedAtGit` and the tiki is a recognised move or its key is `FileChanged`.
  - Since the admitted set is only known inside the loop, collect admitted tikis first, then run the git calls, then the per-tiki pass.

- [ ] **Step 1: Write the failing tests** (real git unless noted; files committed with `--date 2020-01-01T00:00:00Z` and author `alice` via `-c user.name=alice`, while the repo's configured user stays `testuser`;
  every fixture also commits a `keep.md` that stays put, so the history-key guard keeps the history):
  - `TestFullLoad_StampsChangedCommittedTiki` (table): appended body (` M`); appended and staged (`M `); appended, staged, appended again (`MM`) →
    `createdAt: 2020-01-01T00:00:00Z` inserted, mtime kept (set to 2024-04-04 before loading).
  - `TestFullLoad_RecognisesMovedTiki` (table): plain `mv a.md done/a.md`; `git mv a.md b.md`; `mv` plus a body edit; a move *from* a non-ASCII path `Ü/a.md` to `a.md` →
    with the option: the new file has `createdAt: 2020-01-01T00:00:00Z`; the tiki's `createdBy` is `alice`.
  - `TestFullLoad_MoveIntoNestedRepoNotStamped`: `mv a.md inner/a.md` where `inner` is its own repo → not stamped.
  - `TestFullLoad_MovedTikiValueWithoutStamping`: plain `mv`, store without the option, `CreatedAt` = 2020-01-01 and a save writes `createdAt: 2020-01-01T00:00:00Z`.
  - Fake-git tests: a `.git` directory in the temp root, a `relGit`-style fake (repo-relative `RepoRelPath`, non-nil maps with a committed `keep.md` on disk),
    and construct the store, swap the fake in, then `Reload()` (as `TestReload_NoStampWhenHistoryUnreadable` does):
    - `TestFullLoad_MovedTikiWithoutAddCommitStaysGuess`: gone `old.md` whose HEAD content carries the tiki's id, no author entry for `old.md` → not stamped, a save leaves `createdAt` out.
    - `TestFullLoad_AmbiguousMovedIDStaysGuess`: two gone paths carrying the same id, author entries with different dates for both → not stamped, a save leaves `createdAt` out.
    - `TestFullLoad_ChangedFilesFailureTrustsNoNewFile` and `..._HeadContentsFailure...`: the call returns an error → an untracked new tiki is not stamped and a save leaves `createdAt` out.
    - Positive control in each: the same fake without the failure stamps the new tiki.
  - Extend `TestFullLoad_StampsUncommittedTikiKeepingMtime`: touch the committed `c.md`'s mtime before loading; it must stay byte-identical (stat-dirty is not a change).
- [ ] **Step 2: Run** `go test ./store/tikistore/ -run 'ChangedCommitted|MovedTiki|AmbiguousMoved|ChangedFilesFailure' -count=1`. Expected: FAIL.
- [ ] **Step 3: Implement** as in the Interfaces block.
- [ ] **Step 4: Run** the Step 2 command, then `go test -race ./... -count=1`. Expected: PASS. Mutation-check each new branch (move recognition, ambiguous id, failure flag, the changed-file stamp condition).
- [ ] **Step 5: Docs** `docs/tiki-format.md` (when tiki writes `createdAt`: changed committed files and recognised moves), spec already updated. Commit (`moved tikis keep their creation time, and changed files get theirs stamped`).

### Task 3: Final verification

- [ ] `gofmt -l .`, `golangci-lint run` (no new findings), `go test -race ./... -count=1`.
- [ ] Behaviour check in a private tmux session (attach command to the user first): a scratch repo with a committed, a modified, a `mv`-moved and a `git mv`-moved tiki; startup stamps the modified and moved ones with their commit dates and leaves the unchanged one alone.
- [ ] Independent review of the whole change; fix what it confirms test-first; re-run the checks.
