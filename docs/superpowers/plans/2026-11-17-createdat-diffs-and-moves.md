# createdAt for Changed Files and Moved Tikis Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** At full loads, stamp `createdAt` into tiki files that are already a git diff when tiki knows the correct value, and recognise moved tikis by id so they keep their original creation time.

**Architecture:** Two new read-only git operations (changed files from `git status`, file contents at HEAD) in both backends, with parity tests.
The store uses them once per full load: committed-and-changed files get their commit date stamped; gone `.md` paths at HEAD give an id → old path map that dates moved tikis.

**Tech Stack:** Go, shell git and go-git backends in `store/internal/git`.

**Spec:** `docs/superpowers/specs/2026-11-17-stored-created-at-design.md`, section "Revision 2".

## Global Constraints

- Everything from the first plan's Global Constraints (`docs/superpowers/plans/2026-11-17-stored-created-at.md`) still holds:
  file format, comment rule, commits on main, TDD, `gofmt`/`golangci-lint` (no new findings)/`go test -race ./...`, no GUI, stay inside the project.
- Both backends return repo-relative, slash-separated, unescaped paths (shell: `-z`, `core.quotePath=false`).
- The new git work runs only when the full load's history state is `historyRead`.
- A committed file with no changes is never stamped. A moved tiki whose old path has no add commit stays `CreatedAtGuess`.
- If `ChangedFiles` fails, no untracked file is upgraded to `CreatedAtFileTimes` in that load.

## Review Focus

1. A tiki moved with plain `mv`, with `git mv`, and moved plus edited: all keep the old path's commit date. Task 2.
2. A modified committed tiki that the user also staged (`MM`): stamped once, with the commit date. Task 2.
3. Two gone paths with the same id (corrupt history or copy-then-delete): no crash, the tiki is left as a guess. Task 2.
4. Non-ASCII gone paths: matched. Task 1 parity, Task 2.
5. `ChangedFiles` failing in a repo: nothing untracked is stamped or written. Task 2.

---

### Task 1: Git operations: changed files and contents at HEAD

**Files:**
- Modify: `store/internal/git/internal/types/types.go`, `store/internal/git/types.go`, `store/internal/git/ops.go`, `store/internal/git/autoselect.go`,
  `store/internal/git/internal/shell/` (new file `status.go`), `store/internal/git/internal/gogit/` (new file `status.go`)
- Modify (fakes): `store/tikistore/reload_paths_test.go` (`countingGit`), `store/tikistore/identity_test.go` (`fakeGitOps`)
- Test: `store/internal/git/parity_test.go`

**Interfaces:**
- Produces:
  - `type FileChange uint8` with `FileChanged` (tracked file differing from HEAD in the index or working tree and still present) and `FileGone` (tracked at HEAD, missing from the working tree or the index), aliased in `store/internal/git`.
  - `ChangedFiles(dirPattern string) (map[string]FileChange, error)` on `GitOps`: untracked files excluded; a staged rename gives the new path `FileChanged` and the old path `FileGone`.
    Shell: `git -c core.quotePath=false status --porcelain=v1 -z --untracked-files=no -- <dirPattern>`, parsing `XY path\0` and `R? new\0old\0`.
    go-git: `Worktree.Status()`, skipping untracked and unmodified; `Deleted` in staging or worktree → `FileGone`, anything else → `FileChanged`.
  - `HeadContents(paths []string) (map[string][]byte, error)` on `GitOps`: contents at HEAD for repo-relative paths; missing paths are left out; empty input returns an empty map without running git.
    Shell: one `git cat-file --batch`, input lines `HEAD:<path>`. go-git: HEAD commit's tree `File(path).Contents()`.

- [ ] **Step 1: Write the failing tests.**
  - `TestParity_ChangedFiles`: with shell git, commit `sub/a.md`, `sub/b.md`, `sub/c.md`, `sub/d.md`, `sub/Über.md`;
    then append to `a.md`; `git mv sub/b.md sub/b2.md`; `mv sub/c.md sub/done/c.md` (plain); append to `d.md` and `git add` it; `mv sub/Über.md sub/Ünder.md`.
    Both backends' `ChangedFiles(dir)` equal exactly
    `{sub/a.md: FileChanged, sub/b2.md: FileChanged, sub/d.md: FileChanged, sub/b.md: FileGone, sub/c.md: FileGone, sub/Über.md: FileGone}`.
  - `TestParity_HeadContents`: both backends: `HeadContents(["sub/c.md", "sub/Über.md", "sub/missing.md"])` returns the committed bytes for the first two and no entry for the third;
    `HeadContents(nil)` returns an empty map.
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
    An id found under two gone paths is dropped (ambiguous). Read errors give an empty map.
  - In `loadLocked`, when the state is `historyRead`: one `ChangedFiles(s.dir)` call (on error: `changes` nil and a flag that blocks `trustFileTimes` for every tiki in this load),
    then `movedOrigins`. Per admitted tiki, before `trustFileTimes`:
    if its source is `CreatedAtGuess` and its id has an old path, it is a move:
    with an author entry for the old path, set `CreatedAt`, `createdBy` (as `loadTikiFile` does) and `CreatedAtGit`;
    without one, keep `CreatedAtGuess` and skip `trustFileTimes` for it.
    Stamp (when `stampOnLoad`) if the source is `CreatedAtFileTimes`, or it is `CreatedAtGit` and the tiki is a recognised move or its key is `FileChanged`.

- [ ] **Step 1: Write the failing tests** (real git unless noted; files committed with `--date 2020-01-01T00:00:00Z` and author `testuser`):
  - `TestFullLoad_StampsChangedCommittedTiki` (table): appended body (` M`); appended and staged (`M `); appended, staged, appended again (`MM`) →
    `createdAt: 2020-01-01T00:00:00Z` inserted, mtime kept (set to 2024-04-04 before loading).
  - `TestFullLoad_RecognisesMovedTiki` (table): plain `mv a.md done/a.md`; `git mv a.md b.md`; `mv` plus a body edit; a non-ASCII target folder `Ü/a.md` →
    with the option: the new file has `createdAt: 2020-01-01T00:00:00Z`; the tiki's `createdBy` is `testuser`.
  - `TestFullLoad_MovedTikiValueWithoutStamping`: plain `mv`, store without the option, `CreatedAt` = 2020-01-01 and a save writes `createdAt: 2020-01-01T00:00:00Z`.
  - `TestFullLoad_MovedTikiWithoutAddCommitStaysGuess` (fake git: `ChangedFiles` gone `old.md`, `HeadContents` returns a frontmatter with the tiki's id, empty author map, `relGit`-style `RepoRelPath`):
    the walked tiki is not stamped and a save leaves `createdAt` out.
  - `TestFullLoad_AmbiguousMovedIDStaysGuess` (fake git: two gone paths whose HEAD contents carry the same id): not stamped, no panic.
  - `TestFullLoad_ChangedFilesFailureTrustsNoNewFile` (fake git: `ChangedFiles` returns an error): an untracked new tiki is not stamped and a save leaves `createdAt` out.
  - `TestFullLoad_UnchangedCommittedTikiNotStamped` stays covered by `TestFullLoad_StampsUncommittedTikiKeepingMtime` (its committed `c.md` must stay byte-identical).
- [ ] **Step 2: Run** `go test ./store/tikistore/ -run 'ChangedCommitted|MovedTiki|AmbiguousMoved|ChangedFilesFailure' -count=1`. Expected: FAIL.
- [ ] **Step 3: Implement** as in the Interfaces block.
- [ ] **Step 4: Run** the Step 2 command, then `go test -race ./... -count=1`. Expected: PASS. Mutation-check each new branch (move recognition, ambiguous id, failure flag, the changed-file stamp condition).
- [ ] **Step 5: Docs** `docs/tiki-format.md` (when tiki writes `createdAt`: changed committed files and recognised moves), spec already updated. Commit (`moved tikis keep their creation time, and changed files get theirs stamped`).

### Task 3: Final verification

- [ ] `gofmt -l .`, `golangci-lint run` (no new findings), `go test -race ./... -count=1`.
- [ ] Behaviour check in a private tmux session (attach command to the user first): a scratch repo with a committed, a modified, a `mv`-moved and a `git mv`-moved tiki; startup stamps the modified and moved ones with their commit dates and leaves the unchanged one alone.
- [ ] Independent review of the whole change; fix what it confirms test-first; re-run the checks.
