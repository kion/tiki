# Stored createdAt and Local-Time Display Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Store each tiki's creation time in its file as `createdAt` (UTC), stamp it into uncommitted tikis at full loads, fall back to the file's birth time, and show tiki's own timestamps in local time.

**Architecture:** Resolution and stamping live in `store/tikistore` (load, save, a new stamping file, per-OS birth time).
A store option enables stamping; only the TUI bootstrap passes it.
Display changes are two small seams, the TUI detail renderers and the `tiki exec` table formatter,
both keyed on one shared field-name predicate and given the display zone explicitly.

**Tech Stack:** Go 1.25 module, `gopkg.in/yaml.v3`, `golang.org/x/sys/unix` (already in go.mod, becomes direct), git via `store/internal/git`.

**Spec:** `docs/superpowers/specs/2026-11-17-stored-created-at-design.md` (read it before starting).

## Global Constraints

- File format, exact: `createdAt: 2026-01-01T10:00:00Z` (RFC 3339, UTC, whole seconds, unquoted).
- Resolution order on load: valid stored `createdAt` → author date of the commit that added the file → file birth time → file mtime.
- A stored value is valid if yaml decodes it to a timestamp (`2025-06-01T08:00:00Z`, `2025-06-01T10:00:00+02:00`, `2025-06-01 08:00:00`, `2025-06-01`)
  or it is a string in RFC 3339; it is normalized to UTC.
- After tiki writes `createdAt` (save or stamp), the in-memory value equals exactly what was written (UTC, whole seconds).
- `createdBy` resolution is unchanged: git author, else current tiki identity.
- `updatedAt` stays derived (later of mtime and last commit); `activeAt` unchanged.
- Stamping on load: TUI full loads only (startup, `r`).
  Never in live reload (`ReloadPaths`, `Reconcile`), `ReloadTiki`, `tiki exec` or pipe create.
- Stamp only a tiki whose frontmatter has no `createdAt` key at all (an invalid value waits for the next save),
  that is not a symlink, and that is uncommitted, or any such tiki when the folder is known not to be a git repo.
  A repo whose history cannot be read stamps nothing.
- Stamping inserts one line, writes in place, keeps the original mtime; every other byte stays as it was.
  If the mtime cannot be kept (the user doesn't own the file), don't stamp.
- `tiki exec --format json` output is unchanged (UTC RFC 3339). Scalar results of top-level expressions stay UTC.
- Workflow-declared `datetime` fields are unchanged everywhere.
- Tests never assign `time.Local`: the display zone is passed in (race detector, background goroutines read `time.Local`).
- No new modules. `golang.org/x/sys` moves from indirect to direct via `go mod tidy`.
- Code comments follow the user's rule: only when needed, short, *why* not *what*,
  one sentence or clause per line (break after a colon, before "so …"/"but …"/"because …").
- Commit on `main`, no branches, one commit per task, message style as in `git log`
  (lowercase outcome sentence, then `-` bullets broken at clause boundaries).
- Never launch GUI apps. TUI behaviour checks only in a private, detached tmux session (Task 7).
- Don't read anything outside `/stor/devel/tiki` (the scratchpad is fine). Never print environment values.
- Every store test does `t.Chdir(project)` before constructing the store:
  the store's git util uses the working directory, which is otherwise tiki's own repo.
- Done means: `gofmt -l .` empty, `golangci-lint run` 0 issues, `go test -race ./...` passes.

## Review Focus

1. A tiki file with CRLF line endings: the stamp line uses CRLF and nothing else changes. Test in Task 4 (`TestInsertCreatedAtLine`).
2. A stored `createdAt` written by hand: quoted, unquoted, offset, date-only, invalid, empty. Tests in Task 2 (`TestLoad_StoredCreatedAtForms`) and Task 4 (`TestFullLoad_InvalidStoredCreatedAtIsNotStamped`).
3. A file that changes between load and stamp: it is not stamped and the outside write survives. Test in Task 4 (`TestStampCreatedAt_SkipsFileChangedSinceLoad`).
4. A git repo whose history cannot be read, or reached through `GIT_DIR`: nothing committed is stamped. Tests in Task 4 (`TestReload_NoStampWhenHistoryUnreadable`, `TestStampPolicyFor`).
5. tiki's own stamp seen later by live reload or a save: no conflict, no reload change. Test in Task 4 (`TestStamp_NotSeenAsExternalEdit`).
6. A committed tiki under a non-ASCII path: matched to its history, not stamped. Tests in Task 0 and Task 4.

Accepted risk: stamping truncates and rewrites a file in place, as every save does;
killing tiki mid-write could leave that file empty.

---

### Task 0: Non-ASCII paths match their git history

Existing bug, needed by Task 4: git escapes non-ASCII bytes in `--name-only` output by default (`"\303\234ber/caf\303\251.md"`),
so those keys never match `RepoRelPath` (raw UTF-8). Such tikis get file times instead of git times today, and Task 4 would stamp them as uncommitted.

**Files:**
- Modify: `store/internal/git/internal/shell/timestamps.go` (`AllLastCommitTimes`), `store/internal/git/internal/shell/users.go` (`AllAuthors`), `store/internal/git/internal/shell/versions.go` (`AllFileVersionsSince`)
- Test: `store/internal/git/parity_test.go`, `store/tikistore/git_times_test.go`

- [ ] **Step 1: Write the failing tests.**
  - `TestParity_NonASCIIPath`: a repo with a committed `Über/café.md`; shell and go-git `AllAuthors(dir)` and `AllLastCommitTimes(dir)` both contain the key `Über/café.md`,
    and shell `RepoRelPath(<abs path>)` returns `Über/café.md`.
  - Add a case to `TestLoadTakesTimesFromGitHistory`: project in subfolder `Über` → git times as in the other cases.
- [ ] **Step 2: Run** `go test ./store/internal/git/ ./store/tikistore/ -run 'NonASCII|TakesTimesFromGitHistory' -count=1`. Expected: FAIL for the shell backend and the `Über` case.
- [ ] **Step 3: Implement.** Pass `-c core.quotePath=false` to the three shell `git log` calls.
- [ ] **Step 4: Run** the Step 2 command, then `go test ./store/... -count=1`. Expected: PASS.
- [ ] **Step 5: Commit** (`tikis under non-ASCII paths get their git times`).

### Task 1: File birth time

**Files:**
- Create: `store/tikistore/birthtime.go`, `birthtime_linux.go`, `birthtime_darwin.go`, `birthtime_windows.go`, `birthtime_other.go` (all in `store/tikistore/`)
- Modify: `go.mod`, `go.sum` (via `go mod tidy`)
- Test: `store/tikistore/birthtime_test.go`

**Interfaces:**
- Produces: `func fileBirthTime(path string, info os.FileInfo) (time.Time, bool)` (per-OS);
  `var birthTimeOf = fileBirthTime` in `birthtime.go`, so tests can force the fallback.

- [ ] **Step 1: Write the failing test** `TestFileBirthTime_ReportsCreationNotModification`:
  record `before := time.Now()`, write a file, record `after`, set its mtime to 2020-01-01 with `os.Chtimes`,
  `os.Stat` it, call `fileBirthTime`. If `!ok`, `t.Skip("filesystem does not record birth time")`.
  Assert the result is within `[before-1s, after+1s]` and not equal to the 2020 mtime.
- [ ] **Step 2: Run** `go test ./store/tikistore/ -run TestFileBirthTime -count=1`. Expected: build failure (undefined `fileBirthTime`).
- [ ] **Step 3: Implement** per OS:
  Linux `unix.Statx(unix.AT_FDCWD, path, unix.AT_STATX_SYNC_AS_STAT, unix.STATX_BTIME, &stx)`, ok only when `stx.Mask&unix.STATX_BTIME != 0`;
  darwin `info.Sys().(*syscall.Stat_t).Birthtimespec`;
  windows `info.Sys().(*syscall.Win32FileAttributeData).CreationTime`;
  `//go:build !linux && !darwin && !windows` returns `time.Time{}, false`.
  Then `go mod tidy`.
- [ ] **Step 4: Run** the test (Expected: PASS on this machine's tmpfs, which records birth time) and
  `GOOS=darwin GOARCH=arm64 go build ./store/tikistore/ && GOOS=windows go build ./store/tikistore/ && GOOS=freebsd go build ./store/tikistore/` (Expected: no output).
- [ ] **Step 5: Commit** (`a file's birth time is readable on Linux, macOS and Windows`).

### Task 2: Resolve createdAt on load, stored value first

**Files:**
- Modify: `tiki/tiki.go` (flag + accessors + `Clone`), `store/tikistore/persistence.go` (`loadTikiFile`), `store/tikistore/reload_paths.go` (`carryForwardGitMeta`)
- Create: `store/tikistore/created_at.go`
- Test: `store/tikistore/created_at_test.go`, a test file in package `tiki` for `Clone`

**Interfaces:**
- Consumes: `birthTimeOf` (Task 1).
- Produces: `func (t *Tiki) CreatedAtStored() bool`, `func (t *Tiki) SetCreatedAtStored(v bool)` (unexported field `createdAtStored`, copied by `Clone`);
  `func parseStoredCreatedAt(v interface{}) (time.Time, bool)` in `created_at.go`: accepts `time.Time` (yaml.v3 decodes unquoted timestamps to it) or an RFC 3339 string; returns the value in UTC.

- [ ] **Step 1: Write the failing tests.**
  - `TestLoad_StoredCreatedAtForms` (table, non-git temp dir, `birthTimeOf` forced to 2024-03-03T03:03:03Z): frontmatter value → want `CreatedAt`, `CreatedAtStored`:
    `2025-06-01T08:00:00Z` → 2025-06-01 08:00:00 UTC, true;
    `"2025-06-01T08:00:00Z"` → same, true;
    `2025-06-01T10:00:00+02:00` → 08:00:00 UTC with `Location() == time.UTC`, true;
    `2025-06-01 08:00:00` → 08:00:00 UTC, true;
    `2025-06-01` → 2025-06-01 00:00:00 UTC, true;
    `not a time` → 2024-03-03T03:03:03Z, false;
    empty (`createdAt:`) → 2024-03-03T03:03:03Z, false.
  - `TestLoad_StoredCreatedAtWinsOverGit`: git repo (as in `git_times_test.go`), file committed with `--date 2026-01-01T10:00:00Z` and carrying `createdAt: 2025-06-01T08:00:00Z` → `CreatedAt` = 2025-06-01 08:00 UTC.
  - `TestLoad_FallsBackToBirthTimeThenMtime` (non-git dir): `birthTimeOf` returning 2024-03-03T03:03:03Z → that value;
    `birthTimeOf` returning `false` and mtime set to 2023-05-05 → 2023-05-05. Both with `CreatedAtStored() == false`.
  - `TestLoad_CreatedByFallsBackWhenCreatedAtStored` (non-git): stored `createdAt` present, no git author → `createdBy` is present and non-empty.
  - `TestReloadPaths_StoredCreatedAtBeatsCarriedValue`: load via `newReloadStore`, set the old tiki's `CreatedAt` to 2020-01-02,
    `writeExternal` the file with `createdAt: 2025-06-01T08:00:00Z` added, `mustReload` → `CreatedAt` = 2025-06-01 08:00 UTC.
  - `TestClone_CopiesCreatedAtStored` in package `tiki`.
  - Restore `birthTimeOf` with `t.Cleanup`.
- [ ] **Step 2: Run** `go test ./tiki/ ./store/tikistore/ -run 'StoredCreatedAt|FallsBackToBirth|CreatedByFallsBack|CopiesCreatedAtStored' -count=1`. Expected: build failure, then FAIL on values once stubs exist.
- [ ] **Step 3: Implement.**
  In `loadTikiFile`, read `parsed.raw["createdAt"]` through `parseStoredCreatedAt`; a present-but-invalid value logs `slog.Warn` with the file and falls through.
  Apply the resolution order from Global Constraints and set the stored flag only for a valid stored value.
  Drive the `createdBy` fallback from "no git author found", not from `CreatedAt().IsZero()` (a stored value makes it non-zero).
  In `carryForwardGitMeta`, keep `loaded`'s `createdAt` when `loaded.CreatedAtStored()`; `createdBy` and `updatedAt` carry as today.
- [ ] **Step 4: Run** the Step 2 command, then `go test ./store/... ./tiki/... -count=1`. Expected: PASS, including the existing `TestReloadPaths_ModifyCarriesGitMetadata` and `TestLoadTakesTimesFromGitHistory`.
- [ ] **Step 5: Commit** (`a createdAt stored in a tiki's file wins over git and file times`).

### Task 3: Every save writes createdAt

**Files:**
- Modify: `store/tikistore/tiki_bridge.go` (`marshalTikiFrontmatter` and its doc comment), `store/tikistore/persistence.go` (`saveTiki`)
- Test: `store/tikistore/created_at_test.go`, a new `exec_created_at_test.go` in package `main`

**Interfaces:**
- Consumes: `CreatedAtStored`/`SetCreatedAtStored` (Task 2).
- Produces: saved files carry `createdAt:` as the line after `title:`;
  after a successful save with a non-zero `CreatedAt`, `tk.CreatedAt()` equals the written value and `tk.CreatedAtStored()` is true.

- [ ] **Step 1: Write the failing tests.**
  - `TestMarshal_WritesCreatedAtAfterTitleInUTC`: tiki `ABC123`, title `x`, `CreatedAt` 2025-06-01T10:00:00.5+02:00 → output has prefix `"id: ABC123\ntitle: x\ncreatedAt: 2025-06-01T08:00:00Z\n"`.
  - `TestMarshal_OmitsZeroCreatedAt`: zero `CreatedAt` → no `createdAt` in output.
  - `TestSave_NormalizesStoredCreatedAtToUTC`: load a file with `createdAt: 2025-06-01T10:00:00+02:00`, `UpdateTiki` a title change →
    file contains `createdAt: 2025-06-01T08:00:00Z` exactly once; after `Reload`, same `CreatedAt` and `CreatedAtStored() == true`.
  - `TestSave_ReplacesInvalidStoredCreatedAt`: file with `createdAt: garbage`, `UpdateTiki` → file has no `garbage`, and one line matching `^createdAt: \d{4}-\d\d-\d\dT\d\d:\d\d:\d\dZ$` (multiline regexp).
  - `TestSave_InMemoryCreatedAtMatchesFile`: `birthTimeOf` forced to 2024-03-03T03:03:03.7+01:00 in a non-git dir, `UpdateTiki` → `GetTiki(...).CreatedAt()` equals 2024-03-03T02:03:03Z exactly (UTC, no fraction).
  - `TestRunExec_CreateWritesCreatedAt` (package `main`, pattern of `exec_strings_test.go`): `runExec` a `create` → the new file has a line matching the regexp above.
- [ ] **Step 2: Run** `go test ./store/tikistore/ -run 'Marshal_|Save_.*CreatedAt' -count=1` and `go test . -run TestRunExec_CreateWritesCreatedAt -count=1`. Expected: FAIL (no `createdAt` line).
- [ ] **Step 3: Implement.** Emit the line after `title` when `CreatedAt` is non-zero, as UTC truncated to the second, unquoted.
  In `saveTiki`, after the write succeeds, set `CreatedAt` to the written value and the stored flag.
- [ ] **Step 4: Run** the Step 2 commands, then `go test -race ./... -count=1`.
  Expected: PASS. A pre-existing test that compares a saved file's exact bytes may now see the extra `createdAt:` line:
  update only that expectation, and report each such test by name. Any other failure is a bug in this task.
- [ ] **Step 5: Commit** (`every save writes createdAt, in UTC`).

### Task 4: Stamp uncommitted tikis at full loads

**Files:**
- Create: `store/tikistore/stamp.go`
- Modify: `store/tikistore/store.go` (options), `store/tikistore/persistence.go` (`loadLocked`), `internal/bootstrap/stores.go`, `internal/bootstrap/init.go`
- Test: `store/tikistore/stamp_test.go` (fakes next to `countingGit` in `reload_paths_test.go` or in `stamp_test.go`), `exec_created_at_test.go`

**Interfaces:**
- Consumes: `birthTimeOf` (Task 1), `CreatedAtStored`/`SetCreatedAtStored` (Task 2), `git.GitOps.RepoRelPath` (commit `1aae750`).
- Produces:
  - `type Option func(*TikiStore)`; `func WithCreatedAtStamping() Option`; `func NewTikiStore(dir string, opts ...Option) (*TikiStore, error)` (variadic, existing callers unchanged).
  - `func InitStores(opts ...tikistore.Option) (*tikistore.TikiStore, store.Store, error)`; `Bootstrap` calls `InitStores(tikistore.WithCreatedAtStamping())`;
    `main.go` exec and `internal/pipe/create.go` keep calling `InitStores()`.
  - `func insertCreatedAtLine(content []byte, created time.Time) ([]byte, bool)`:
    requires the file to start exactly with `---\n` or `---\r\n`;
    finds the closing delimiter the way `store.ParseFrontmatter` does (first `\n---` after the opening);
    refuses (false) when the frontmatter already has a line starting with `createdAt:`;
    inserts after a single-line `title:` entry (a `title:` line whose next line does not start with whitespace), otherwise just before the closing delimiter;
    uses the opening line's line ending.
  - `func insideGitWorkTree(dir string) bool`: makes `dir` absolute, then checks it and each parent for a `.git` entry (file or directory), stopping when `filepath.Dir(p) == p`. No git command.
  - `type stampPolicy int` with `stampNone`, `stampUncommitted`, `stampAll`;
    `func (s *TikiStore) stampPolicyFor(lastCommitMap map[string]time.Time) stampPolicy`, in this order:
    option off → `stampNone`; `lastCommitMap != nil` (history fetched) → `stampUncommitted`;
    `insideGitWorkTree(s.dir)` or `GIT_DIR` set → `stampNone`; otherwise → `stampAll`.
  - `var chtimes = os.Chtimes` in `stamp.go` (test seam for an mtime that cannot be set).
  - `func stampCreatedAt(tk *tiki.Tiki) error`, in this order:
    `Lstat` the file, skip if it is a symlink or its mtime or size differ from `tk.LoadedMtime`/`tk.LoadedSize`;
    probe `chtimes(path, time.Time{}, info.ModTime())` and skip if it fails;
    read, `insertCreatedAtLine` with `tk.CreatedAt()`, skip if it refuses;
    write in place (`os.WriteFile` keeps the inode), restore mtime with `chtimes(path, time.Time{}, oldMtime)`;
    after any successful write, always re-stat and record `tk.LoadedSize`/`tk.LoadedMtime`,
    set `tk.CreatedAt` to the written value (UTC, whole seconds) and `tk.SetCreatedAtStored(true)`, even if restoring the mtime failed.
    Every skip returns an error naming the reason.

- [ ] **Step 1: Write the failing tests.**
  - `TestInsertCreatedAtLine` (table, `created` = 2024-03-03T05:03:03.9+02:00):
    `"---\nid: AAAAAA\ntitle: a\n---\nbody\n"` → `"---\nid: AAAAAA\ntitle: a\ncreatedAt: 2024-03-03T03:03:03Z\n---\nbody\n"`, true;
    `"---\r\nid: AAAAAA\r\ntitle: a\r\nstatus: x\r\n---\r\nbody\r\n"` → `"---\r\nid: AAAAAA\r\ntitle: a\r\ncreatedAt: 2024-03-03T03:03:03Z\r\nstatus: x\r\n---\r\nbody\r\n"`, true;
    `"---\nid: AAAAAA\nstatus: x\n---\n"` (no title) → `"---\nid: AAAAAA\nstatus: x\ncreatedAt: 2024-03-03T03:03:03Z\n---\n"`, true;
    `"---\nid: AAAAAA\ntitle: |\n  long\n---\n"` (block title) → `"---\nid: AAAAAA\ntitle: |\n  long\ncreatedAt: 2024-03-03T03:03:03Z\n---\n"`, true;
    `"---\nid: AAAAAA\ncreatedAt: x\n---\n"` → false;
    `"\n---\nid: AAAAAA\n---\n"` → false; `"---\nid: AAAAAA\n"` (no closing) → false; `"no frontmatter\n"` → false.
  - `TestInsideGitWorkTree`: temp dir without `.git` → false; `.git` directory at the parent of the queried subdir → true; `.git` file → true; relative `"."` after `t.Chdir` into a non-repo temp dir → false (and returns).
  - `TestStampPolicyFor` (table): option off → none; option on with non-nil map → uncommitted; nil map with `.git` present → none;
    nil map, no `.git`, `GIT_DIR` set via `t.Setenv` → none; nil map, no `.git`, no `GIT_DIR` → all.
  - `TestFullLoad_StampsUncommittedTikiKeepingMtime`: git repo with committed `c.md` and untracked `n.md`; `birthTimeOf` forced to 2024-03-03T03:03:03Z; `n.md` mtime set to 2024-04-04;
    `NewTikiStore(project, WithCreatedAtStamping())` →
    `n.md` bytes equal the original with `createdAt: 2024-03-03T03:03:03Z` inserted after its `title:` line; `n.md` mtime still 2024-04-04;
    `c.md` bytes unchanged; the `n` tiki has `CreatedAtStored() == true` and `UpdatedAt` = 2024-04-04.
  - `TestFullLoad_WithoutOptionDoesNotStamp`: same setup without the option → `n.md` unchanged.
  - `TestFullLoad_StampsEveryTikiOutsideGit`: non-git dir, two files → both stamped.
  - `TestFullLoad_CommittedRenameIsNotStamped`: commit `a.md`, `git mv a.md b.md`, commit → `b.md` unchanged.
  - `TestFullLoad_CommittedNonASCIIPathIsNotStamped`: committed `Über/x.md` → unchanged.
  - `TestFullLoad_InvalidStoredCreatedAtIsNotStamped`: untracked file with `createdAt: garbage` → bytes unchanged, and after `Reload` the tiki still loads.
  - `TestFullLoad_SymlinkIsNotStamped`: non-git dir, `link.md` a symlink to a tiki file in another temp dir → the target's bytes unchanged.
  - `TestReload_StampsTikiAddedSinceStartup`: non-git dir, store with option, write a new file, `s.Reload()` → stamped.
  - `TestReload_NoStampWhenHistoryUnreadable`: `git init` the dir, construct the store with the option (no tikis yet),
    set `s.gitUtil` to a fake whose `AllAuthors`/`AllLastCommitTimes` return an error, write an uncommitted file, `s.Reload()` → file unchanged.
    Positive control in the same test: switch to a fake returning empty maps, `s.Reload()` → file stamped.
  - `TestLiveReloadPathsDoNotStamp`, one sequence in a non-git dir with a store built with the option (so stamping is enabled):
    write `a.md` after construction, `ReloadPaths([]string{a})` → no `createdAt:` in `a.md`;
    modify `a.md` externally, `ReloadTiki("AAAAAA")` → still none;
    write `b.md`, `Reconcile()` → no `createdAt:` in `b.md`.
  - `TestStampCreatedAt_SkipsFileChangedSinceLoad`: load a tiki, `writeExternal` new content, `stampCreatedAt(tk)` → returns an error and the file holds the external content.
  - `TestStampCreatedAt_SkipsWhenMtimeCannotBeKept`: `chtimes` replaced by a func returning an error → `stampCreatedAt` returns an error and the file is unchanged.
  - `TestStamp_NotSeenAsExternalEdit`: after a stamping load, `ReloadPaths` of the stamped path reports `Changed == false`,
    and `UpdateTiki` of a clone with a new title succeeds (no `ErrConflict`).
  - `TestRunExec_SelectDoesNotStamp` (package `main`): in a non-git temp dir, write a tiki file without `createdAt`, `runExec` a `select` → the file's bytes and mtime are unchanged.
- [ ] **Step 2: Run** `go test ./store/tikistore/ -run 'InsertCreatedAtLine|InsideGitWorkTree|StampPolicyFor|FullLoad_|Reload_.*Stamp|LiveReloadPathsDoNotStamp|StampCreatedAt_|Stamp_' -count=1`
  and `go test . -run TestRunExec_SelectDoesNotStamp -count=1`.
  Expected: build failure, then FAIL; `TestRunExec_SelectDoesNotStamp` and `TestLiveReloadPathsDoNotStamp` may already pass and must stay passing.
- [ ] **Step 3: Implement.** In `loadLocked`, compute the policy once after the history fetch.
  Stamp a tiki only after `admitDocument` admitted it (`s.tikis[tk.ID()] == tk`), when its parsed frontmatter has no `createdAt` key,
  and either the policy is `stampAll`, or it is `stampUncommitted` and its `RepoRelPath` key is absent from `lastCommitMap`.
  A `RepoRelPath` error means "treat as committed".
  `loadTikiFile` must expose whether the key was present (e.g. a second unexported flag on the result, or recompute from the parsed map); keep it out of `tiki.Tiki`'s public API.
  A stamping skip or error logs at `slog.Debug` (skips) or `slog.Warn` (I/O errors) and leaves the tiki loaded.
- [ ] **Step 4: Run** the Step 2 command, then `go test -race ./... -count=1`. Expected: PASS.
- [ ] **Step 5: Commit** (`tiki writes createdAt into uncommitted tikis when the TUI loads them`).

### Task 5: Show tiki's timestamps in local time

`activeAt` has no detail-view row (`fieldRegistry` has only `createdBy`, `createdAt`, `updatedAt`, `title`),
so in the TUI only `createdAt` and `updatedAt` change; the exec table converts all three.

**Files:**
- Modify: `tiki/fields.go`, `view/tikidetail/render_helpers.go` (`FieldRenderContext`), `view/tikidetail/field_registry.go` (`renderDateTimeValue`, the `TypeTimestamp` branch of `genericFieldValueString`), `internal/ruki/runtime/format.go` (`TableFormatter`, `formatCell`)
- Test: a test file in package `tiki`, `view/tikidetail/configurable_detail_view_test.go`, `internal/ruki/runtime/format_test.go`, `format_json_test.go`

**Interfaces:**
- Produces:
  - `func IsAuditTimeField(name string) bool` in `tiki/fields.go`: true for `createdAt`, `updatedAt`, `activeAt` only.
  - `FieldRenderContext.Location *time.Location` (nil means `time.Local`).
  - `TableFormatter.loc *time.Location` (unexported; nil means `time.Local`; `NewTableFormatter()` leaves it nil). `formatCell` gains access to it (make it a method or pass the location).

- [ ] **Step 1: Write the failing tests.** Use `tst := time.FixedZone("TST", -5*3600)` passed through the new fields; never assign `time.Local`.
  - `TestIsAuditTimeField`: true for the three names; false for `dueBy`, `due`, `createdBy`, `title`.
  - `TestTableFormatterTimestampFormatting` (existing, update): `&TableFormatter{loc: tst}`, `createdAt` 2025-06-01T19:30:00Z → cell `2025-06-01T14:30:00-05:00`.
  - `TestTableFormatterCustomDatetimeUnchanged`: `&TableFormatter{loc: tst}`, a registered `datetime` field `dueBy` = 2025-06-01T14:30:00 UTC → cell `2025-06-01T14:30:00Z`.
  - `TestJSONFormatterAuditTimestampsStayUTC`: `createdAt` 2025-06-01T19:30:00Z → JSON value `"2025-06-01T19:30:00Z"`.
  - `TestGenericFieldValueString_AuditTimeShowsLocal`: `FieldRenderContext{Location: tst}`: `createdAt` 2025-06-01T19:30:00Z → `2025-06-01 14:30`;
    `dueBy` 2025-06-01T14:30:00 UTC → still `2025-06-01 14:30`.
  - `renderDateTimeValue` with `FieldRenderContext{FieldName: "createdAt", Location: tst}` shows `2025-06-01 14:30` for 19:30Z.
    If reading text from the returned primitive is awkward, extract `func dateTimeDisplay(fieldName string, t time.Time, loc *time.Location) string`,
    test it, and have `renderDateTimeValue` call it.
- [ ] **Step 2: Run** `go test ./tiki/ ./view/tikidetail/ ./internal/ruki/runtime/ -count=1`. Expected: build failure on the new fields, then FAIL on the local-time expectations.
- [ ] **Step 3: Implement.** Convert with `.In(loc)` before formatting when `tiki.IsAuditTimeField(name)`;
  the table renders those fields as `t.In(loc).Format(time.RFC3339)`. JSON, scalar results and other types untouched.
- [ ] **Step 4: Run** the Step 2 command, then `go test -race ./... -count=1`. Expected: PASS.
- [ ] **Step 5: Commit** (`tiki's own timestamps show in local time`).

### Task 6: Docs, schema notes and AI skill

**Files:**
- Modify: `docs/tiki-format.md` ("Derived fields"), `docs/command-line.md` (`exec`), `docs/config.md` (full-reload bullet and the "Git metadata" limitation near lines 223-230),
  `cmd_workflow_schema.go` (`intrinsicFieldNotes["createdAt"]`), `ai/skills/tiki/references/files.md`, `ai/skills/tiki/references/provenance.md`
- Test: `cmd_workflow_schema_test.go` only if it pins the note text

- [ ] **Step 1: Docs.**
  - `tiki-format.md`: `createdAt` becomes a stored field: format, accepted forms, resolution order, when tiki writes it
    (creation, every save, full loads for uncommitted tikis or outside git, mtime kept; not symlinks, not files with an invalid value), and that hand-edited values are honoured.
    `updatedAt` and `createdBy` stay under derived fields.
  - `command-line.md`: table output shows `createdAt`, `updatedAt`, `activeAt` in local time with the offset;
    `--format json` and top-level expression results show UTC.
  - `config.md`: the full reload (`r`) also writes `createdAt` into tikis that lack it (link to tiki-format).
    Rewrite the "Git metadata" limitation: a tiki that carries `createdAt` shows it at once;
    one without it shows its file's birth time until the next `r`, and a save before then writes that value into the file.
- [ ] **Step 2: Schema note.** `intrinsicFieldNotes["createdAt"]`: `"stored in the file by tiki; never set or edit it"`.
  Run `go test . -run Schema -count=1` (Expected: PASS, updating a pinned expectation if one exists).
- [ ] **Step 3: Skill.**
  - `files.md`, notes-only section: keep the `id:` and `createdAt:` lines unchanged when editing the file directly.
  - `provenance.md`: the creation time is stored in the file once set; where it comes from before that.
  - Agents create tikis only through `tiki exec 'create …'`, which writes `createdAt`: add no instruction to set it.
- [ ] **Step 4: Re-trial the skill** (project memory `retrial-skill-after-edits`):
  build tiki into the scratchpad, wrap it in a script that sets `XDG_CONFIG_HOME` to a scratch dir,
  copy `ai/skills/tiki` beside a scratch project (from `tiki demo` files) with a workflow,
  and give a fresh general-purpose agent user-style requests:
  create a tiki; add a sentence to a notes-only tiki by editing its file; say when a given tiki was created.
  Ask it to list every command it ran and file it read; don't say what is being checked.
  Pass: it never writes or changes `createdAt`, keeps the line when editing the notes file, and answers from `select createdAt`.
  If the skill needs another edit, re-run the trial.
- [ ] **Step 5: Commit** (`docs, schema notes and skill: createdAt is stored, timestamps show in local time`).

### Task 7: Final verification

- [ ] **Step 1:** `gofmt -l .` (Expected: no output), `golangci-lint run` (Expected: `0 issues.`), `go test -race ./... -count=1` (Expected: every package `ok`).
- [ ] **Step 2: Behaviour check in tmux** (project memory `tui-testing-via-tmux`):
  build into the scratchpad; scratch project from `tiki demo` files, `git init` and commit half of them, leave the rest untracked;
  private server `tmux -L createdat -f /dev/null`, `window-size manual`, isolated `XDG_CONFIG_HOME`.
  Give the user the read-only attach command (`tmux -L createdat attach -r -t <session>`) **before** starting.
  Check: untracked files gained `createdAt:` with mtimes unchanged, committed files are untouched, the detail view's Created/Updated rows show local time,
  and `tiki exec` table vs JSON output differ only in zone.
- [ ] **Step 3:** Independent implementation review (the user's process): a fresh agent reviews the full diff since `f51e7e8` against the spec and this plan;
  fix what it confirms, re-run Step 1.
- [ ] **Step 4:** Hand the user the look-and-feel check on their own terminal: what changed, what to look at, what to expect.
