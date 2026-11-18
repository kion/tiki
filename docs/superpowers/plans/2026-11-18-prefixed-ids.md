# Prefixed Sequential IDs Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Workflows can opt in to readable, never-changing ids (`MEM-DB-35`) built from field-value prefixes plus a per-prefix sequence shared by every clone/branch/worktree on one machine, with `tiki ids rename|migrate` for existing projects.

**Architecture:** The id format lives in the ruki fork (`idfmt`). The workflow package holds the prefix scheme (top-level `prefix`, value `prefix:`, `required`, `immutable`) and composes a tiki's prefix. A small `internal/idseq` package owns the locked counter file; the store allocates at save, replacing provisional template ids. Validators on the mutation gate enforce `required`/`immutable`. A new `tiki ids` command group renames ids and rewrites references.

**Tech Stack:** Go 1.25, yaml.v3, golang.org/x/sys (flock / LockFileEx), go-git + git CLI backends, tview TUI.

**Spec:** `docs/superpowers/specs/2026-11-17-prefixed-ids-design.md`

## Global Constraints

- Valid id: legacy `^[A-Z0-9]{6}$` or prefixed `^[A-Z][A-Z0-9]*(-[A-Z][A-Z0-9]*)*-[1-9][0-9]{0,8}$`, at most 64 chars (`idfmt.MaxIDLength`).
- Prefix segment: `^[A-Z][A-Z0-9]*$`. Value prefixes unique across fields and different from the top-level `prefix`.
- Prefixed ids are on exactly when workflow.yaml sets a top-level `prefix`; a field with any value `prefix:` must be `type: enum`, `required: true`, `immutable: true`.
- Segments join with `-` in field declaration order; no segment → top-level `prefix`.
- Workflows without a top-level `prefix` behave exactly as before (random ids, no new checks unless `required`/`immutable` are declared).
- Counter file: `<state dir>/sequences/<folder>-<8 hex of sha256(key)>.yaml`; key = root commit (smallest if several) → `git --git-common-dir` → cwd.
  State dir: `$XDG_STATE_HOME/tiki`, else `~/.local/state/tiki` (Linux), `~/Library/Application Support/tiki/state` (macOS), `%LOCALAPPDATA%\tiki\state` (Windows).
- Next number = `max(counter[prefix], highest number for that exact prefix among loaded ids and formerIds) + 1`.
- Code comments: only *why*, short, clause per line (user's global rule). No commits unless the user asks; ruki changes stay local behind the untracked `go.work`.

## Review Focus

- A TUI new-tiki draft with a prefixed workflow: after Ctrl+S the board must select the saved tiki under its final id (not the provisional one).
- `tiki exec 'create …'` printing `created <ID>` must print the prefixed id the store assigned.
- Two processes allocating the same prefix at once must get different numbers (lock contention across file handles, not goroutines sharing one handle).
- Lane moves on a workflow where the lane action sets an immutable field: a clear status-line error, no silent failure.
- `tiki ids migrate` on a project with wiki pages and `[text](ID)` links: every link follows; plain-text mentions stay.

---

### Task 1: ruki fork — id format, natural order, messages  ✅ done

**Files:** `/stor/devel/ruki/idfmt/idfmt.go`, `idfmt_test.go`, `executor.go` (`sortKeyForField`, `compareForSort`, ref error text), `executor_test.go`, `phase5_test.go`; tiki `go.work` (untracked), `cmd_live_test.go` (malformed example `TASK-01`).

**Produces:** `idfmt.IsValidID`, `IsLegacyID`, `IsPrefixedID`, `SplitPrefixed(id) (prefix string, number int, ok bool)`, `IsValidPrefixSegment`, `Compare(a, b) int` (legacy before prefixed; prefixed by prefix then number), `MaxIDLength`. `order by id` sorts naturally.

- [x] Tests written, failed, implemented, passing; ruki `go test ./...` and `golangci-lint` clean; tiki suite green against it.

### Task 2: Workflow scheme model and parsing

**Files:**
- Modify: `workflow/fields.go` (EnumValue, FieldDef, deepCopy, ClearWorkflowFields), create `workflow/idscheme.go`, test `workflow/idscheme_test.go`
- Modify: `config/workflow_fields.go` (YAML keys, convert, LoadWorkflowFields, LoadWorkflowFieldsFromFile), `config/loader.go` (`workflowFileData.Prefix`), test `config/workflow_fields_test.go`

**Interfaces — Produces:**
- `workflow.EnumValue.Prefix string`; `workflow.FieldDef.Required bool`, `.Immutable bool`; `func (f FieldDef) HasPrefixes() bool`
- `func ValidateIDScheme(projectPrefix string, defs []FieldDef) error`
- `func SetProjectPrefix(p string)`, `func ProjectPrefix() string`, `func PrefixedIDsEnabled() bool` (cleared by `ClearWorkflowFields`)
- `func IDPrefixFor(value func(field string) string) string` — joined segments from registered workflow fields, or `ProjectPrefix()`
- `func PrefixFields() []FieldDef` — registered fields with `HasPrefixes()`
- config: `customFieldYAML.Required/Immutable` (`required`, `immutable` keys), enum value key `prefix`, `customFieldFileData.Prefix` (`prefix`)

- [ ] **Step 1: Failing tests** — `TestValidateIDScheme`: table of (projectPrefix, defs) → error substring: value prefix without project prefix (`requires a top-level prefix`), lowercase prefix, prefix field not required, not immutable, non-enum field with prefix, duplicate prefix across fields, value prefix equal to project prefix, valid scheme → nil. `TestIDPrefixFor`: area=feature,layer=frontend → `FEAT-FE`; layer=cross → `FEAT`; area=misc,layer=cross → `TIK`; area empty → `FE`. `TestLoadWorkflowFieldsReadsScheme` (config): a temp workflow.yaml with the spec's example → `ProjectPrefix()=="TIK"`, area `Required && Immutable`, feature prefix `FEAT`; unknown enum key error lists `prefix`; a scheme error fails `LoadWorkflowFields` and `LoadWorkflowFieldsFromFile`.
- [ ] **Step 2:** `go test ./workflow/ ./config/` → FAIL (undefined).
- [ ] **Step 3:** Implement; validation uses `idfmt.IsValidPrefixSegment`. `LoadWorkflowFields` sets the project prefix after registering fields; `LoadWorkflowFieldsFromFile` validates the scheme too (used by `tiki workflow install`).
- [ ] **Step 4:** `go test ./workflow/ ./config/` → PASS.

### Task 3: Tiki-side id sites

**Files:** `view/markdown/wikilink.go` (regex), `view/tikidetail/render_key.go` (`addIDTokens`), `store/tikistore/crud.go` (`GetAllTikis` sort, ref error text), `service/validators.go` and `document/id.go` and `store/tikistore/tiki_bridge.go` (messages), `workflow/fields.go:22` comment; tests beside each.

**Interfaces — Consumes:** Task 1 `idfmt`.

- [ ] **Step 1: Failing tests** — wikilink: `[[MEM-DB-35]]` resolves through a fake resolver, `[[mem-db-35]]` and `[[MEM-0]]` stay literal; render key: text `see MEM-DB-35 and X7F4K2` yields both ids; `GetAllTikis` over ids `MEM-10, MEM-9, ABC123` returns `ABC123, MEM-9, MEM-10`; loading a file with `id: MEM-3` admits it; `dependsOn: [MEM-3]` validates.
- [ ] **Step 2:** run the package tests → FAIL.
- [ ] **Step 3:** Wikilink regex captures `[A-Z0-9][A-Z0-9-]*` then filters with `IsValidID`; `addIDTokens` scans with a regex for both forms before tokenizing; `GetAllTikis` sorts with `idfmt.Compare`; messages say "expected 6 uppercase letters or digits, or a prefixed id such as ABC-12".
- [ ] **Step 4:** `go test ./view/... ./store/... ./service/... ./document/...` → PASS.

### Task 4: `required` / `immutable` validators

**Files:** create `service/field_rules.go`, test `service/field_rules_test.go`; modify `service/validators.go` (`RegisterFieldValidators` registers them); `controller/plugin.go:639-645` uses `rejectionMessage(err)`.

**Interfaces — Produces:** `func validateRequiredOnCreate(old, new *tikipkg.Tiki, all []*tikipkg.Tiki) *Rejection`, `func validateRequiredOnUpdate(...)`, `func validateImmutableOnUpdate(...)` (all `MutationValidator`).

- [ ] **Step 1: Failing tests** — create without a required field (no default) → `area is required`; update clearing a set required field → rejected; update of a legacy tiki lacking the required field but changing another field → allowed; immutable set value changed → `area is immutable`; cleared → rejected; legacy-id tiki filling an empty immutable field → allowed; prefixed-id tiki filling an empty *prefix* field → rejected; a non-prefix immutable field on a prefixed tiki filled once → allowed; workflow without these attributes → no rejections.
- [ ] **Step 2:** `go test ./service/ -run 'Required|Immutable'` → FAIL.
- [ ] **Step 3:** Implement; "has a value" = present and not empty string / empty list.
- [ ] **Step 4:** → PASS; `go test ./service/ ./controller/` green.

### Task 5: State dir and locked counter file

**Files:** `config/paths.go` (`getUserStateDir`, `PathManager.StateDir`, `GetStateDir`), test `config/paths_test.go`; create `internal/idseq/idseq.go`, `lock_unix.go`, `lock_windows.go`, test `internal/idseq/idseq_test.go`.

**Interfaces — Produces:**
- `config.GetStateDir() string`
- `idseq.FileName(folder, key string) string` → `<folder>-<8 hex>.yaml`
- `type idseq.File struct{...}`; `func Open(path, owner string) *File`
- `func (f *File) Next(prefix string, floor int) (int, error)`; `func (f *File) Raise(prefix string, atLeast int) error`; `func (f *File) Peek(prefix string) (int, error)`

- [ ] **Step 1: Failing tests** — `GetStateDir` honours `XDG_STATE_HOME`, defaults to `~/.local/state/tiki` on Linux; `Next` on a missing file returns floor+1 and creates the file with `owner` and `counters`; `Next` after `Next` increments; floor above the stored value wins; corrupt file → treated as empty and rewritten; `Raise` never lowers; `Peek` writes nothing; 8 goroutines × 50 `Next` calls each using **separate `Open` handles** → 400 distinct numbers 1..400.
- [ ] **Step 2:** `go test ./internal/idseq/ ./config/` → FAIL.
- [ ] **Step 3:** Implement: lock a sibling `<file>.lock` (unix `Flock LOCK_EX`, windows `LockFileEx`), retry with a 5 s bound then a clear error; write temp + rename.
- [ ] **Step 4:** → PASS (also `go vet` with `GOOS=windows go build ./internal/idseq/`).

### Task 6: Git repo key (both backends)

**Files:** `store/internal/git/ops.go`, `autoselect.go`, `internal/shell/…`, `internal/gogit/…`, `parity_test.go`; test fakes in `store/tikistore/identity_test.go`, `reload_paths_test.go`.

**Interfaces — Produces:** `GitOps.RootCommits() ([]string, error)` (sorted), `GitOps.CommonDir() (string, error)` (absolute).

- [ ] **Step 1: Failing test** — parity test: in a temp repo with one commit both backends return the same single hash and the same absolute common dir; a worktree (`git worktree add`) returns the same common dir; a repo with no commits → empty slice, no error.
- [ ] **Step 2:** `go test ./store/internal/git/` → FAIL.
- [ ] **Step 3:** shell: `git rev-list --max-parents=0 HEAD`, `git rev-parse --path-format=absolute --git-common-dir`; gogit: walk from HEAD collecting parentless commits, `repo.Storer` common dir.
- [ ] **Step 4:** → PASS.

### Task 7: Allocation in the store

**Files:** modify `tiki/tiki.go` (provisional-id flag, copied by `Clone`), `store/tikistore/template.go`, `store/tikistore/crud.go`, `store/tikistore/store.go` (option + key), create `store/tikistore/idalloc.go`, test `store/tikistore/idalloc_test.go`.

**Interfaces:**
- Consumes: Task 2 `IDPrefixFor`, `PrefixedIDsEnabled`; Task 5 `idseq`; Task 6 `RootCommits`, `CommonDir`.
- Produces: `func (t *Tiki) SetProvisionalID(bool)`, `func (t *Tiki) ProvisionalID() bool`; `type IDAllocator interface{ Next(prefix string, floor int) (int, error); Raise(prefix string, atLeast int) error; Peek(prefix string) (int, error) }`; `func WithIDAllocator(a IDAllocator) Option`; `func (s *TikiStore) highestNumberLocked(prefix string) int` (ids + `formerIds`).

- [ ] **Step 1: Failing tests** — with `prefix: TIK` and an in-memory allocator: two creates → `TIK-1`, `TIK-2`; with the spec's fields → `FEAT-FE-1`, then `FEAT-1` for layer=cross; a template id (provisional) is replaced and the caller's pointer shows the new id; loaded `TIK-7` → next create `TIK-8` even if the allocator holds 2; workflow without prefix → template id kept (random, 6 chars); the default allocator path builds `<state>/sequences/<folder>-<hash>.yaml` from the root commit (temp `XDG_STATE_HOME`, temp repo).
- [ ] **Step 2:** `go test ./store/tikistore/ -run Alloc` → FAIL.
- [ ] **Step 3:** Template marks its id provisional when `PrefixedIDsEnabled()`; `storeNewDocumentLocked` allocates when the id is empty or provisional (prefix from `IDPrefixFor` over the tiki's fields), retries on a taken id; default allocator created lazily from the repo key.
- [ ] **Step 4:** → PASS; full `go test ./...` green.

### Task 8: TUI — final id after commit, read-only immutable fields

**Files:** `controller/plugin.go` (`pendingSelectID` refresh after commit), `controller/detail_controller.go` (`selectedTikiID` after commit), `controller/tiki_edit_session.go` (return/record the committed id), `view/tikidetail/configurable_detail_view.go` (`isEditableLayoutField`); tests in `controller/…_test.go`, `view/tikidetail/…_test.go`.

**Interfaces — Consumes:** Task 7 provisional ids; Task 2 `FieldDef.Immutable`, `HasPrefixes`.

- [ ] **Step 1: Failing tests** — commit of a draft whose id the store replaces → `editSession` reports the final id and the plugin selects it; detail view on an existing tiki: immutable field with a value is not editable (focus skips it), empty immutable field on a legacy tiki is editable, prefix field on a prefixed tiki is not editable even when empty; a draft keeps every field editable.
- [ ] **Step 2:** `go test ./controller/ ./view/tikidetail/` → FAIL.
- [ ] **Step 3:** Implement.
- [ ] **Step 4:** → PASS; tmux check in Task 13.

### Task 9: `tiki workflow schema` output

**Files:** `cmd_workflow_schema.go` (`writeWorkflowSchema`, `fieldSchemaNote`, legend), `cmd_workflow_schema_test.go`.

- [ ] **Step 1: Failing test** — the spec's example workflow prints `prefix: TIK` after the `workflow:` line, the area row ends with `required; immutable`, enum values show their prefix as `feature=FEAT`; built-in schema still ≤ 2048 bytes.
- [ ] **Step 2–4:** fail → implement → `go test . -run WorkflowSchema` PASS.

### Task 10: `tiki ids rename` / `tiki ids migrate`

**Files:** create `cmd_ids.go`, `cmd_ids_test.go`, `store/tikistore/rename.go`, `rename_test.go`; modify `main.go` (dispatch, reserved names for `viewer.ParseViewerInput`, `printUsage`).

**Interfaces — Produces:**
- `type Rename struct{ Old, New string }`
- `func (s *TikiStore) PlanMigration() (renames []Rename, emptyPrefix []string, err error)` (oldest first: createdAt, then id; numbers reserved via `Peek` + floor, nothing written)
- `func (s *TikiStore) ApplyRenames(renames []Rename, dryRun bool) (changedFiles []string, err error)`
- `func (s *TikiStore) CheckRename(old, new string) error`
- `func runIDsIn(root string, args []string, stdout, stderr io.Writer) int`

- [ ] **Step 1: Failing tests** — rename: refs in `dependsOn`, `[[OLD]]`, `[t](OLD)` in tikis and an id-less page all become NEW; plain-text `OLD` stays; renamed file has `formerIds: [OLD]` and its createdAt unchanged; live-status file moved; counter raised to NEW's number; refusals with exit 4: unknown OLD, invalid NEW, taken NEW, NEW in someone's formerIds, prefix mismatch with fields. migrate: two legacy tikis → `TIK-1` (older) and `TIK-2`; `--dry-run` prints the map and changed files, writes nothing, counter untouched; tiki with an empty prefix field is listed; no top-level `prefix` → exit 2 with a message.
- [ ] **Step 2:** `go test . ./store/tikistore/ -run 'IDs|Rename|Migrat'` → FAIL.
- [ ] **Step 3:** Implement under one store lock; save tikis through `saveTiki`; id-less pages via `document.WalkDocuments` + atomic write; no triggers.
- [ ] **Step 4:** → PASS.

### Task 11: Shipped workflows

**Files:** `config/workflows/kanban.yaml`, `bug-tracker.yaml`, `todo.yaml`; test `config/shipped_workflows_test.go`.

- [ ] **Step 1: Failing test** — for each shipped workflow, uncommenting the lines marked `#id-example` yields a workflow that loads and gives `ProjectPrefix()` = `TIK`/`TIK`/`TODO`, kanban `type` prefixes `STORY/BUG/SPIKE/PROJ`, bug-tracker `BUG/REG/INC`; the files as shipped have no prefix.
- [ ] **Step 2–4:** fail → add commented examples (block-style values where a prefix line is shown) → PASS.

### Task 12: Docs and AI skill

**Files:** `docs/tiki-format.md`, `docs/workflow-format.md` (0.6.2 entry), `docs/customization/customization.md`, `docs/ruki/types-and-values.md`, `docs/command-line.md`, `README.md`, `config/index.md`, `ai/skills/tiki/SKILL.md`, `references/dependencies.md`, `references/files.md`.

- [ ] Write the docs per the spec (trade-off: sequential ids can clash across machines; one-time fill for tikis created before a field became immutable; non-git counter is per folder).
- [ ] Skill changes per the spec; fresh-agent trial on a scratch prefixed workflow; re-run after any fix.

### Task 13: End-to-end verification

- [ ] Temp repo + two worktrees + a second clone, prefixed workflow, `XDG_STATE_HOME` scratch: 3 × 20 parallel `tiki exec 'create …'` → 60 distinct sequential ids; delete counter → next create continues after the highest loaded.
- [ ] `tiki demo` copy: `tiki ids migrate --dry-run`, real run, links/dependsOn/live status checked; `tiki ids rename` refusals.
- [ ] tmux (private server, user gets the attach command first): new tiki gets a prefixed id and stays selected; immutable field not editable; lane move blocked by an immutable field shows an error.
- [ ] `make lint`, `go test -race ./...`, `GOOS=windows go build ./...`, `GOOS=darwin go build ./...`.
- [ ] Independent implementation review; fix confirmed bugs; write the two migration prompts.
