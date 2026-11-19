// Package idseq keeps the per-prefix number sequences behind prefixed tiki ids.
//
// One file per repository lives outside the repository, in the user's state dir,
// so every clone, branch and worktree on the machine draws from the same counters.
// Each read-increment-write runs under an exclusive OS file lock,
// so concurrent tiki processes never issue the same number.
//
// A file is named <label>-<hash of the repository key>.yaml.
// It is found by the hash alone, so the readable label can follow a renamed folder
// without the counters ever being lost or split.
package idseq

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

// lockTimeout bounds how long a create waits for another tiki process.
const lockTimeout = 5 * time.Second

// File is one repository's counter file.
type File struct {
	dir        string
	key        string
	label      string
	lastUsedBy string
}

type fileData struct {
	LastUsedBy string         `yaml:"lastUsedBy"` // for humans: which checkout the file belongs to
	Counters   map[string]int `yaml:"counters"`
}

func keyHash(key string) string {
	sum := sha256.Sum256([]byte(key))
	return hex.EncodeToString(sum[:])[:8]
}

// FileName names a repository's counter file: a readable label for humans,
// plus a hash of its key, which is what finds the file.
func FileName(label, key string) string {
	return sanitize(label) + "-" + keyHash(key) + ".yaml"
}

func sanitize(s string) string {
	var b strings.Builder
	for _, r := range s {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9', r == '.', r == '-', r == '_':
			b.WriteRune(r)
		default:
			b.WriteRune('_')
		}
	}
	if b.Len() == 0 {
		return "repo"
	}
	return b.String()
}

// Open returns the counter file for key in dir.
// Writes name it after label and record lastUsedBy in it, both for humans.
func Open(dir, key, label, lastUsedBy string) *File {
	return &File{dir: dir, key: key, label: label, lastUsedBy: lastUsedBy}
}

// Path returns the file holding the counters, or where the first write will create it.
func (f *File) Path() (string, bool, error) {
	files, err := f.existing()
	if err != nil {
		return "", false, err
	}
	if len(files) == 0 {
		return f.wanted(), false, nil
	}
	return files[0], true, nil
}

// Counters returns every stored counter without writing anything.
func (f *File) Counters() (map[string]int, error) {
	d, _, err := f.read()
	return d.Counters, err
}

// Next returns the next number for prefix:
// one above the larger of the stored counter and floor,
// which callers set from the ids they can see.
func (f *File) Next(prefix string, floor int) (int, error) {
	var n int
	err := f.update(func(d *fileData) {
		n = max(d.Counters[prefix], floor) + 1
		d.Counters[prefix] = n
	})
	return n, err
}

// Raise makes sure prefix's counter is at least atLeast.
func (f *File) Raise(prefix string, atLeast int) error {
	return f.update(func(d *fileData) {
		d.Counters[prefix] = max(d.Counters[prefix], atLeast)
	})
}

// Peek returns prefix's stored counter without writing anything.
func (f *File) Peek(prefix string) (int, error) {
	d, _, err := f.read()
	if err != nil {
		return 0, err
	}
	return d.Counters[prefix], nil
}

func (f *File) wanted() string {
	return filepath.Join(f.dir, FileName(f.label, f.key))
}

// existing lists the files holding this key's counters, whatever their label.
func (f *File) existing() ([]string, error) {
	files, err := filepath.Glob(filepath.Join(f.dir, "*-"+keyHash(f.key)+".yaml"))
	if err != nil {
		return nil, err
	}
	sort.Strings(files)
	return files, nil
}

func (f *File) update(change func(*fileData)) error {
	if err := os.MkdirAll(f.dir, 0o750); err != nil {
		return fmt.Errorf("create id sequence dir: %w", err)
	}
	// named by the key alone, so checkouts with different labels still take turns
	unlock, err := lockFile(filepath.Join(f.dir, "."+keyHash(f.key)+".lock"), lockTimeout)
	if err != nil {
		return err
	}
	defer unlock()

	d, files, err := f.read()
	if err != nil {
		return err
	}
	change(&d)
	d.LastUsedBy = f.lastUsedBy
	wanted := f.wanted()
	if err := write(wanted, d); err != nil {
		return err
	}
	for _, old := range files {
		if old != wanted {
			_ = os.Remove(old)
		}
		// lock files of the earlier per-file locking
		_ = os.Remove(old + ".lock")
	}
	return nil
}

// read merges every file holding this key, taking the larger counter for each prefix:
// two files only appear when checkouts with different labels raced to create the first one.
// A missing or unreadable file counts as empty: callers' floors rebuild from the ids they can see.
func (f *File) read() (fileData, []string, error) {
	d := fileData{Counters: map[string]int{}}
	files, err := f.existing()
	if err != nil {
		return d, nil, err
	}
	for _, path := range files {
		data, err := os.ReadFile(path)
		if errors.Is(err, os.ErrNotExist) {
			continue
		}
		if err != nil {
			return d, nil, fmt.Errorf("read id sequence file: %w", err)
		}
		var one fileData
		if err := yaml.Unmarshal(data, &one); err != nil {
			slog.Warn("id sequence file unreadable, rebuilding it from loaded ids", "path", path, "error", err)
			continue
		}
		for prefix, n := range one.Counters {
			d.Counters[prefix] = max(d.Counters[prefix], n)
		}
	}
	return d, files, nil
}

func write(path string, d fileData) error {
	data, err := yaml.Marshal(d)
	if err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), filepath.Base(path)+".tmp-*")
	if err != nil {
		return fmt.Errorf("write id sequence file: %w", err)
	}
	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		_ = os.Remove(tmp.Name())
		return fmt.Errorf("write id sequence file: %w", err)
	}
	if err := tmp.Close(); err != nil {
		_ = os.Remove(tmp.Name())
		return fmt.Errorf("write id sequence file: %w", err)
	}
	if err := os.Rename(tmp.Name(), path); err != nil {
		_ = os.Remove(tmp.Name())
		return fmt.Errorf("write id sequence file: %w", err)
	}
	return nil
}
