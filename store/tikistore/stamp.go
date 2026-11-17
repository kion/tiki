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
	"time"

	"github.com/boolean-maybe/tiki/store"
	"github.com/boolean-maybe/tiki/tiki"
	"gopkg.in/yaml.v3"
)

type stampPolicy int

const (
	stampNone stampPolicy = iota
	stampUncommitted
	stampAll
)

// A variable so tests can stand in for a file whose times the user may not set.
var chtimes = os.Chtimes

var errStampSkipped = errors.New("createdAt stamp skipped")

var createdAtKeyLine = regexp.MustCompile(`^["']?createdAt["']?\s*:`)

// Everything is stamped only when the folder is known not to be a repository:
// a repository whose history can't be read stamps nothing, so an error never turns into a mass rewrite.
func (s *TikiStore) stampPolicyFor(lastCommitMap map[string]time.Time) stampPolicy {
	switch {
	case !s.stampOnLoad:
		return stampNone
	case lastCommitMap != nil:
		return stampUncommitted
	case insideGitWorkTree(s.dir) || os.Getenv("GIT_DIR") != "":
		return stampNone
	}
	return stampAll
}

func (s *TikiStore) needsStamp(tk *tiki.Tiki, policy stampPolicy, lastCommitMap map[string]time.Time) bool {
	if tk.CreatedAtStored() || s.inNestedRepo(tk.Path()) {
		return false
	}
	switch policy {
	case stampAll:
		return true
	case stampUncommitted:
		key, err := s.gitUtil.RepoRelPath(tk.Path())
		if err != nil {
			return false
		}
		_, committed := lastCommitMap[key]
		return !committed
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
	// Setting the unchanged time succeeds only where restoring it later will,
	// e.g. not on a file the user can write but doesn't own.
	if err := chtimes(path, time.Time{}, info.ModTime()); err != nil {
		return fmt.Errorf("%w: modification time can't be kept: %v", errStampSkipped, err)
	}
	content, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("read: %w", err)
	}
	if frontmatterHasCreatedAt(content) {
		return fmt.Errorf("%w: frontmatter already has createdAt", errStampSkipped)
	}
	stamped, ok := stampedContent(content, tk.CreatedAt())
	if !ok {
		return fmt.Errorf("%w: no place to insert the line safely", errStampSkipped)
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
	tk.SetCreatedAtStored(true)
	if restoreErr != nil {
		return fmt.Errorf("restore modification time: %w", restoreErr)
	}
	return nil
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
