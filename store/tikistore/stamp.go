package tikistore

import (
	"bytes"
	"errors"
	"fmt"
	"io/fs"
	"log/slog"
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"strings"
	"time"

	"github.com/boolean-maybe/ruki/idfmt"
	"github.com/boolean-maybe/tiki/document"
	"github.com/boolean-maybe/tiki/store"
	"github.com/boolean-maybe/tiki/store/internal/git"
	"github.com/boolean-maybe/tiki/tiki"
	"gopkg.in/yaml.v3"
)

type historyState int

const (
	historyUnknown historyState = iota
	// the load read the history, and its paths line up with the files
	historyRead
	// the folder is known not to be in a repository
	historyNone
)

// A variable so tests can stand in for a file whose times the user may not set.
var chtimes = os.Chtimes

// A variable so tests can make an outside write land between the stat and the read.
var readTikiFile = os.ReadFile

var errStampSkipped = errors.New("createdAt stamp skipped")

var createdAtKeyLine = regexp.MustCompile(`^["']?createdAt["']?\s*:`)

// Outside git counts only when the folder is known not to be a repository:
// a repository whose history can't be read trusts nothing,
// so an error never turns into a mass rewrite.
func (s *TikiStore) historyStateFor(lastCommitMap map[string]time.Time) historyState {
	switch {
	case lastCommitMap != nil:
		return historyRead
	case insideGitWorkTree(s.dir) || os.Getenv("GIT_DIR") != "":
		return historyUnknown
	}
	return historyNone
}

// Only a file git provably can't date may have its own times written into it.
func (s *TikiStore) trustFileTimes(tk *tiki.Tiki, state historyState, lastCommitMap map[string]time.Time) {
	if tk.CreatedAtSource() != tiki.CreatedAtGuess || s.inNestedRepo(tk.Path()) {
		return
	}
	switch state {
	case historyNone:
	case historyRead:
		key, err := s.gitUtil.RepoRelPath(tk.Path())
		if err != nil {
			return
		}
		if _, committed := lastCommitMap[key]; committed {
			return
		}
	default:
		return
	}
	tk.SetCreatedAtSource(tiki.CreatedAtFileTimes)
}

// historyKeysMatch guards against git's paths and tiki's not lining up:
// every committed tiki would then look uncommitted.
func (s *TikiStore) historyKeysMatch(lastCommitMap map[string]time.Time, docPaths []string) bool {
	root, err := filepath.Abs(s.dir)
	if err != nil {
		return false
	}
	prefix, err := s.gitUtil.RepoRelPath(root)
	if err != nil || !prefixMatchesDisk(root, prefix) {
		return false
	}
	if len(lastCommitMap) == 0 {
		return true
	}
	walked := make(map[string]bool, len(docPaths))
	for _, p := range docPaths {
		abs, err := filepath.Abs(p)
		if err != nil {
			continue
		}
		walked[abs] = true
		if key, err := s.gitUtil.RepoRelPath(abs); err == nil {
			if _, ok := lastCommitMap[key]; ok {
				return true
			}
		}
	}
	// No tiki has history, which is normal when none is committed,
	// but not when tiki walked a history path under another key, or none exists on disk.
	onDisk := false
	for key := range lastCommitMap {
		rel := key
		if prefix != "." {
			var ok bool
			if rel, ok = strings.CutPrefix(key, prefix+"/"); !ok {
				continue
			}
		}
		path := filepath.Join(root, filepath.FromSlash(rel))
		if walked[path] {
			return false
		}
		if _, err := os.Lstat(path); err == nil {
			onDisk = true
		}
	}
	return onDisk
}

// GIT_DIR without GIT_WORK_TREE makes git take the working directory as the top level,
// so git's idea of where root sits must agree with the .git entry on disk.
func prefixMatchesDisk(root, prefix string) bool {
	top := os.Getenv("GIT_WORK_TREE")
	if top == "" {
		var ok bool
		if top, ok = dirWithGitEntry(root); !ok {
			return false
		}
	}
	top, err := filepath.Abs(top)
	if err != nil {
		return false
	}
	rel, err := filepath.Rel(top, root)
	if err != nil {
		return false
	}
	return filepath.ToSlash(rel) == prefix
}

func dirWithGitEntry(dir string) (string, bool) {
	for p := dir; ; {
		if _, err := os.Lstat(filepath.Join(p, ".git")); err == nil {
			return p, true
		}
		parent := filepath.Dir(p)
		if parent == p {
			return "", false
		}
		p = parent
	}
}

// origin is where a moved tiki came from, with the createdAt it had at HEAD (zero if none).
type origin struct {
	path      string
	createdAt time.Time
}

type workingTree struct {
	changes map[string]git.FileChange
	// tiki id → where it was moved from; an empty path when two places carry the id
	origins map[string]origin
	// changed path holding the same tiki at HEAD → the createdAt it had there (zero if none);
	// only these may be stamped, since a path's history may belong to another tiki
	sameAtHead map[string]time.Time
	// changed path holding a different tiki at HEAD
	reused map[string]bool
	// status or HEAD unreadable: a moved tiki can't be told from a new one
	unknown bool
}

// git is asked only when its answer could change a creation time.
func (s *TikiStore) readWorkingTree(state historyState, admitted []*tiki.Tiki) workingTree {
	if state != historyRead || !s.anyWorkingTreeCandidate(admitted) {
		return workingTree{}
	}
	changes, err := s.gitUtil.ChangedFiles(s.dir)
	if err != nil {
		slog.Warn("can't read git status, so no new tiki's own times are trusted", "error", err)
		return workingTree{unknown: true}
	}
	wt := workingTree{changes: changes, origins: map[string]origin{},
		sameAtHead: map[string]time.Time{}, reused: map[string]bool{}}
	byKey := make(map[string]*tiki.Tiki, len(admitted))
	for _, tk := range admitted {
		if key, err := s.gitUtil.RepoRelPath(tk.Path()); err == nil {
			byKey[key] = tk
		}
	}
	var reads []string
	for path, change := range changes {
		if strings.EqualFold(filepath.Ext(path), ".md") && (change == git.FileGone || byKey[path] != nil) {
			reads = append(reads, path)
		}
	}
	if len(reads) == 0 {
		return wt
	}
	heads, err := s.gitUtil.HeadContents(reads)
	if err != nil {
		slog.Warn("can't read files at HEAD, so no new tiki's own times are trusted", "error", err)
		wt.unknown = true
		return wt
	}
	for path, content := range heads {
		id, createdAt, ok := headIdentity(content)
		if !ok {
			continue
		}
		if tk := byKey[path]; tk != nil && changes[path] == git.FileChanged {
			if tk.ID() == id {
				wt.sameAtHead[path] = createdAt
				continue
			}
			wt.reused[path] = true
		}
		wt.addOrigin(id, path, createdAt)
	}
	return wt
}

func (wt *workingTree) addOrigin(id, path string, createdAt time.Time) {
	if _, seen := wt.origins[id]; seen {
		wt.origins[id] = origin{}
		return
	}
	wt.origins[id] = origin{path: path, createdAt: createdAt}
}

func headIdentity(content []byte) (id string, createdAt time.Time, ok bool) {
	fm, ok := frontmatterMap(content)
	if !ok {
		return "", time.Time{}, false
	}
	raw, ok := document.FrontmatterID(fm)
	id = document.NormalizeID(raw)
	if !ok || !idfmt.IsValidID(id) {
		return "", time.Time{}, false
	}
	createdAt, _ = parseStoredCreatedAt(fm["createdAt"])
	return id, createdAt, true
}

func (s *TikiStore) anyWorkingTreeCandidate(admitted []*tiki.Tiki) bool {
	for _, tk := range admitted {
		switch tk.CreatedAtSource() {
		case tiki.CreatedAtGuess:
			return true
		case tiki.CreatedAtGit:
			if s.stampOnLoad {
				return true
			}
		}
	}
	return false
}

// Only what is already a diff and correctly dated gets stamped:
// a stamp alone would cost a commit, and a wrong date would win over git.
func (s *TikiStore) settleCreatedAt(tk *tiki.Tiki, state historyState, wt workingTree,
	authorMap map[string]*git.AuthorInfo, lastCommitMap map[string]time.Time) {
	key, keyErr := s.gitUtil.RepoRelPath(tk.Path())
	reused := keyErr == nil && wt.reused[key]
	moved := false
	if tk.CreatedAtSource() == tiki.CreatedAtGuess || reused {
		o, isMove := wt.origins[tk.ID()]
		switch {
		case isMove:
			if !dateFromOrigin(tk, o, authorMap) {
				s.guessFromFileTimes(tk)
				return
			}
			moved = true
		case reused:
			s.guessFromFileTimes(tk)
			return
		}
	}
	if !moved && tk.CreatedAtSource() == tiki.CreatedAtGit && keyErr == nil {
		if at := wt.sameAtHead[key]; !at.IsZero() {
			tk.SetCreatedAt(at)
		}
	}
	if !wt.unknown {
		s.trustFileTimes(tk, state, lastCommitMap)
	}
	if s.stampOnLoad && s.shouldStamp(tk, moved, key, keyErr == nil, wt) {
		s.stampLogged(tk)
	}
}

// The createdAt a tiki had at HEAD is older information than its old path's add commit.
func dateFromOrigin(tk *tiki.Tiki, o origin, authorMap map[string]*git.AuthorInfo) bool {
	if o.path == "" {
		return false
	}
	author, dated := authorMap[o.path]
	switch {
	case !o.createdAt.IsZero():
		tk.SetCreatedAt(o.createdAt)
	case dated:
		tk.SetCreatedAt(author.Date)
	default:
		return false
	}
	tk.SetCreatedAtSource(tiki.CreatedAtGit)
	if dated {
		setCreatedBy(tk, author.Name, author.Email)
	}
	return true
}

// A path's history may belong to another tiki,
// so this one falls back to its own file times, as a guess that is never written.
func (s *TikiStore) guessFromFileTimes(tk *tiki.Tiki) {
	if info, err := os.Stat(tk.Path()); err == nil {
		if birth, ok := birthTimeOf(tk.Path(), info); ok {
			tk.SetCreatedAt(birth)
		} else {
			tk.SetCreatedAt(info.ModTime())
		}
	}
	tk.SetCreatedAtSource(tiki.CreatedAtGuess)
	if name, email, err := s.GetCurrentUser(); err == nil {
		tk.Delete("createdBy")
		setCreatedBy(tk, name, email)
	}
}

func (s *TikiStore) shouldStamp(tk *tiki.Tiki, moved bool, key string, keyOK bool, wt workingTree) bool {
	if s.inNestedRepo(tk.Path()) {
		return false
	}
	switch tk.CreatedAtSource() {
	case tiki.CreatedAtFileTimes:
		return true
	case tiki.CreatedAtGit:
		if moved {
			return true
		}
		_, verified := wt.sameAtHead[key]
		return keyOK && verified && wt.changes[key] == git.FileChanged
	}
	return false
}

func (s *TikiStore) stampLogged(tk *tiki.Tiki) {
	err := stampCreatedAt(tk)
	switch {
	case err == nil:
		slog.Debug("wrote createdAt into tiki", "tiki_id", tk.ID(), "file", tk.Path())
	case errors.Is(err, errStampSkipped):
		slog.Debug("not writing createdAt into tiki", "tiki_id", tk.ID(), "file", tk.Path(), "reason", err)
	default:
		slog.Warn("failed to write createdAt into tiki", "tiki_id", tk.ID(), "file", tk.Path(), "error", err)
	}
}

// insideGitWorkTree looks for a .git entry on disk rather than asking git,
// because git refuses some real repositories (e.g. "dubious ownership"),
// and treating one of those as a plain folder would stamp every tiki in it.
func insideGitWorkTree(dir string) bool {
	p, err := filepath.Abs(dir)
	if err != nil {
		return true
	}
	for {
		_, err := os.Lstat(filepath.Join(p, ".git"))
		if err == nil || !errors.Is(err, fs.ErrNotExist) {
			return true
		}
		parent := filepath.Dir(p)
		if parent == p {
			return false
		}
		p = parent
	}
}

// The load never reads a nested repository's history,
// so its committed files would look uncommitted.
func (s *TikiStore) inNestedRepo(path string) bool {
	root, err := filepath.Abs(s.dir)
	if err != nil {
		return true
	}
	for dir := filepath.Dir(path); dir != root; {
		_, err := os.Lstat(filepath.Join(dir, ".git"))
		if err == nil || !errors.Is(err, fs.ErrNotExist) {
			return true
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return true
		}
		dir = parent
	}
	return false
}

// The modification time is kept so the stamp doesn't count as an edit.
func stampCreatedAt(tk *tiki.Tiki) error {
	path := tk.Path()
	info, err := os.Lstat(path)
	if err != nil {
		return fmt.Errorf("stat: %w", err)
	}
	if info.Mode()&os.ModeSymlink != 0 {
		return fmt.Errorf("%w: symlink", errStampSkipped)
	}
	if !info.ModTime().Equal(tk.LoadedMtime) || info.Size() != tk.LoadedSize {
		return fmt.Errorf("%w: changed since load", errStampSkipped)
	}
	// An outside write must be caught before the probe below resets the mtime and hides it.
	content, err := readTikiFile(path)
	if err != nil {
		return fmt.Errorf("read: %w", err)
	}
	if int64(len(content)) != info.Size() || changedSince(path, info) {
		return fmt.Errorf("%w: changed while being read", errStampSkipped)
	}
	// Setting the unchanged time succeeds only where restoring it later will,
	// e.g. not on a file the user can write but doesn't own.
	if err := chtimes(path, time.Time{}, info.ModTime()); err != nil {
		return fmt.Errorf("%w: modification time can't be kept: %v", errStampSkipped, err)
	}
	if frontmatterHasCreatedAt(content) {
		return fmt.Errorf("%w: frontmatter already has createdAt", errStampSkipped)
	}
	stamped, ok := stampedContent(content, tk.CreatedAt())
	if !ok {
		return fmt.Errorf("%w: no place to insert the line safely", errStampSkipped)
	}
	// The probe put the old mtime back, so a write landing since then shows a newer one.
	if changedSince(path, info) {
		return fmt.Errorf("%w: changed before the write", errStampSkipped)
	}
	//nolint:gosec // G703: path is a tiki file the store just loaded
	if err := os.WriteFile(path, stamped, info.Mode().Perm()); err != nil {
		return fmt.Errorf("write: %w", err)
	}

	// The file changed whatever happens next, so tiki's view of it must follow,
	// or the next save would report a conflict with its own stamp.
	restoreErr := chtimes(path, time.Time{}, info.ModTime())
	if after, err := os.Stat(path); err == nil {
		tk.LoadedMtime = after.ModTime()
		tk.LoadedSize = after.Size()
	}
	tk.SetCreatedAt(writtenCreatedAt(tk.CreatedAt()))
	tk.SetCreatedAtSource(tiki.CreatedAtStored)
	if restoreErr != nil {
		return fmt.Errorf("restore modification time: %w", restoreErr)
	}
	return nil
}

func changedSince(path string, info os.FileInfo) bool {
	now, err := os.Lstat(path)
	return err != nil || !now.ModTime().Equal(info.ModTime()) || now.Size() != info.Size()
}

// frontmatterHasCreatedAt catches every spelling of the key yaml accepts,
// because a second createdAt key would make the file fail to load.
func frontmatterHasCreatedAt(content []byte) bool {
	m, ok := frontmatterMap(content)
	if !ok {
		return true
	}
	_, present := m["createdAt"]
	return present
}

// stampedContent only accepts an insert that leaves every other frontmatter value as it was:
// a title wrapped onto a line at column 0 can fool the title rule and swallow the line.
// The end of the frontmatter is the fallback.
func stampedContent(content []byte, created time.Time) ([]byte, bool) {
	for _, afterTitle := range []bool{true, false} {
		stamped, ok := insertCreatedAtLineAt(content, created, afterTitle)
		if ok && stampKeepsFrontmatter(content, stamped, created) {
			return stamped, true
		}
	}
	return nil, false
}

func stampKeepsFrontmatter(before, after []byte, created time.Time) bool {
	was, ok := frontmatterMap(before)
	if !ok {
		return false
	}
	now, ok := frontmatterMap(after)
	if !ok {
		return false
	}
	got, ok := parseStoredCreatedAt(now["createdAt"])
	if !ok || !got.Equal(writtenCreatedAt(created)) {
		return false
	}
	delete(now, "createdAt")
	return reflect.DeepEqual(was, now)
}

func frontmatterMap(content []byte) (map[string]interface{}, bool) {
	fm, _, err := store.ParseFrontmatter(string(content))
	if err != nil {
		return nil, false
	}
	var m map[string]interface{}
	if err := yaml.Unmarshal([]byte(fm), &m); err != nil {
		return nil, false
	}
	return m, true
}

// insertCreatedAtLine puts the line after a single-line title, where saves put it,
// so the first save after a stamp doesn't move it.
func insertCreatedAtLine(content []byte, created time.Time) ([]byte, bool) {
	return insertCreatedAtLineAt(content, created, true)
}

func insertCreatedAtLineAt(content []byte, created time.Time, afterTitle bool) ([]byte, bool) {
	var eol string
	switch {
	case bytes.HasPrefix(content, []byte("---\r\n")):
		eol = "\r\n"
	case bytes.HasPrefix(content, []byte("---\n")):
		eol = "\n"
	default:
		return nil, false
	}
	// same rule as store.ParseFrontmatter: the first line starting with --- closes it
	idx := bytes.Index(content[3:], []byte("\n---"))
	if idx < 0 {
		return nil, false
	}
	closing := 3 + idx + 1

	at := closing
	lines := bytes.SplitAfter(content[3+len(eol):closing], []byte("\n"))
	offset := 3 + len(eol)
	for i, line := range lines {
		if createdAtKeyLine.Match(line) {
			return nil, false
		}
		if afterTitle && bytes.HasPrefix(line, []byte("title:")) {
			next := i+1 < len(lines) && len(lines[i+1]) > 0 && (lines[i+1][0] == ' ' || lines[i+1][0] == '\t')
			if !next {
				at = offset + len(line)
			}
		}
		offset += len(line)
	}

	out := make([]byte, 0, len(content)+40)
	out = append(out, content[:at]...)
	out = append(out, "createdAt: "+createdAtLineValue(created)+eol...)
	out = append(out, content[at:]...)
	return out, true
}
