// Package idseq keeps the per-prefix number sequences behind prefixed tiki ids.
//
// One file per repository lives outside the repository, in the user's state dir,
// so every clone, branch and worktree on the machine draws from the same counters.
// Each read-increment-write runs under an exclusive OS file lock,
// so concurrent tiki processes never issue the same number.
package idseq

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

// lockTimeout bounds how long a create waits for another tiki process.
const lockTimeout = 5 * time.Second

// File is one repository's counter file.
type File struct {
	path       string
	lastUsedBy string
}

type fileData struct {
	LastUsedBy string         `yaml:"lastUsedBy"` // for humans: which checkout the file belongs to
	Counters   map[string]int `yaml:"counters"`
}

// FileName names a repository's counter file: its folder name for humans,
// plus a hash of its key so two folders with the same name never share counters.
func FileName(folder, key string) string {
	sum := sha256.Sum256([]byte(key))
	return sanitize(folder) + "-" + hex.EncodeToString(sum[:])[:8] + ".yaml"
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

// Open returns the counter file at path; each write records lastUsedBy in it for humans.
func Open(path, lastUsedBy string) *File {
	return &File{path: path, lastUsedBy: lastUsedBy}
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
	d, err := f.read()
	if err != nil {
		return 0, err
	}
	return d.Counters[prefix], nil
}

func (f *File) update(change func(*fileData)) error {
	if err := os.MkdirAll(filepath.Dir(f.path), 0o750); err != nil {
		return fmt.Errorf("create id sequence dir: %w", err)
	}
	unlock, err := lockFile(f.path+".lock", lockTimeout)
	if err != nil {
		return err
	}
	defer unlock()

	d, err := f.read()
	if err != nil {
		return err
	}
	change(&d)
	d.LastUsedBy = f.lastUsedBy
	return f.write(d)
}

// read treats a missing or unreadable file as empty:
// callers' floors rebuild the counters from the ids they can see.
func (f *File) read() (fileData, error) {
	d := fileData{Counters: map[string]int{}}
	data, err := os.ReadFile(f.path)
	if errors.Is(err, os.ErrNotExist) {
		return d, nil
	}
	if err != nil {
		return d, fmt.Errorf("read id sequence file: %w", err)
	}
	if err := yaml.Unmarshal(data, &d); err != nil {
		slog.Warn("id sequence file unreadable, rebuilding it from loaded ids", "path", f.path, "error", err)
		return fileData{Counters: map[string]int{}}, nil
	}
	if d.Counters == nil {
		d.Counters = map[string]int{}
	}
	return d, nil
}

func (f *File) write(d fileData) error {
	data, err := yaml.Marshal(d)
	if err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(f.path), filepath.Base(f.path)+".tmp-*")
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
	if err := os.Rename(tmp.Name(), f.path); err != nil {
		_ = os.Remove(tmp.Name())
		return fmt.Errorf("write id sequence file: %w", err)
	}
	return nil
}
