package livestatus

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/boolean-maybe/ruki/idfmt"
	"gopkg.in/yaml.v3"
)

const (
	stateDirName = ".tiki"
	liveDirName  = "live"
	fileExt      = ".yaml"
)

type Problem struct {
	Path    string
	ModTime time.Time
	Err     error
}

type fileFormat struct {
	Agent     string `yaml:"agent,omitempty"`
	Message   string `yaml:"message"`
	UpdatedAt string `yaml:"updatedAt,omitempty"`
}

func StateDir(root string) string { return filepath.Join(root, stateDirName) }

func Dir(root string) string { return filepath.Join(root, stateDirName, liveDirName) }

func Path(root, id string) string { return filepath.Join(Dir(root), id+fileExt) }

// Parse is lenient because files may be written by hand or by other harnesses:
// only a missing message makes a status unusable.
func Parse(data []byte, mtime time.Time) (Status, error) {
	var f fileFormat
	if err := yaml.Unmarshal(data, &f); err != nil {
		return Status{}, err
	}
	if strings.TrimSpace(f.Message) == "" {
		return Status{}, errors.New("no message")
	}
	st := Status{Agent: f.Agent, Message: f.Message, UpdatedAt: mtime}
	if ts, err := time.Parse(time.RFC3339, f.UpdatedAt); err == nil {
		st.UpdatedAt = ts
	}
	return st, nil
}

func marshal(st Status) ([]byte, error) {
	f := fileFormat{Agent: st.Agent, Message: st.Message}
	if !st.UpdatedAt.IsZero() {
		f.UpdatedAt = st.UpdatedAt.UTC().Format(time.RFC3339)
	}
	return yaml.Marshal(f)
}

func Read(root, id string) (Status, bool, error) {
	path := Path(root, id)
	info, err := os.Stat(path)
	if errors.Is(err, fs.ErrNotExist) {
		return Status{}, false, nil
	}
	if err != nil {
		return Status{}, false, err
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return Status{}, false, err
	}
	st, err := Parse(data, info.ModTime())
	if err != nil {
		return Status{}, false, fmt.Errorf("%s: %w", path, err)
	}
	return st, true, nil
}

// Write replaces the file by rename,
// so the watching board never reads a half-written status.
func Write(root, id string, st Status) error {
	data, err := marshal(st)
	if err != nil {
		return err
	}
	if err := ensureDirs(root); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(Dir(root), "."+id+".*.tmp")
	if err != nil {
		return err
	}
	defer func() { _ = os.Remove(tmp.Name()) }()
	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmp.Name(), Path(root, id))
}

// The state dir ignores itself,
// so statuses never reach git whatever the project's own .gitignore says.
func ensureDirs(root string) error {
	if err := os.MkdirAll(Dir(root), 0o750); err != nil {
		return err
	}
	ignore := filepath.Join(StateDir(root), ".gitignore")
	if _, err := os.Stat(ignore); errors.Is(err, fs.ErrNotExist) {
		return os.WriteFile(ignore, []byte("*\n"), 0o600)
	}
	return nil
}

func Remove(root, id string) error {
	err := os.Remove(Path(root, id))
	if errors.Is(err, fs.ErrNotExist) {
		return nil
	}
	return err
}

func Scan(root string) (map[string]Status, []Problem) {
	entries, err := os.ReadDir(Dir(root))
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return map[string]Status{}, nil
		}
		return map[string]Status{}, []Problem{{Path: Dir(root), Err: err}}
	}
	statuses := make(map[string]Status, len(entries))
	var problems []Problem
	for _, e := range entries {
		id, ok := strings.CutSuffix(e.Name(), fileExt)
		if e.IsDir() || !ok || !idfmt.IsValidID(id) {
			continue
		}
		path := filepath.Join(Dir(root), e.Name())
		info, err := e.Info()
		if err != nil {
			// removed between ReadDir and now
			continue
		}
		data, err := os.ReadFile(path)
		if err == nil {
			var st Status
			if st, err = Parse(data, info.ModTime()); err == nil {
				statuses[id] = st
				continue
			}
		}
		if errors.Is(err, fs.ErrNotExist) {
			continue
		}
		problems = append(problems, Problem{Path: path, ModTime: info.ModTime(), Err: err})
	}
	return statuses, problems
}
