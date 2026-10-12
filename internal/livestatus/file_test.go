package livestatus

import (
	"os"
	"path/filepath"
	"sort"
	"testing"
	"time"
)

func TestWriteThenRead(t *testing.T) {
	root := t.TempDir()
	want := Status{
		Agent:     "✻ Claude",
		Message:   "Implementing animation",
		UpdatedAt: time.Date(2026, 10, 11, 18, 22, 5, 0, time.UTC),
	}
	if err := Write(root, "X7F4K2", want); err != nil {
		t.Fatalf("Write: %v", err)
	}
	got, ok, err := Read(root, "X7F4K2")
	if err != nil || !ok {
		t.Fatalf("Read = %v, %v", ok, err)
	}
	if got != want {
		t.Fatalf("Read = %+v, want %+v", got, want)
	}
}

func TestWriteIgnoresTheWholeTikiDirForGit(t *testing.T) {
	root := t.TempDir()
	if err := Write(root, "X7F4K2", Status{Message: "Working"}); err != nil {
		t.Fatalf("Write: %v", err)
	}
	data, err := os.ReadFile(filepath.Join(root, ".tiki", ".gitignore"))
	if err != nil || string(data) != "*\n" {
		t.Fatalf(".gitignore = %q, %v; want %q", data, err, "*\n")
	}
}

func TestWriteKeepsAnExistingGitignore(t *testing.T) {
	root := t.TempDir()
	ignore := filepath.Join(root, ".tiki", ".gitignore")
	if err := os.MkdirAll(filepath.Dir(ignore), 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(ignore, []byte("live/\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := Write(root, "X7F4K2", Status{Message: "Working"}); err != nil {
		t.Fatalf("Write: %v", err)
	}
	if data, _ := os.ReadFile(ignore); string(data) != "live/\n" {
		t.Fatalf(".gitignore = %q, want it untouched", data)
	}
}

func TestWriteLeavesNoTempFiles(t *testing.T) {
	root := t.TempDir()
	for _, msg := range []string{"Researching", "Implementing"} {
		if err := Write(root, "X7F4K2", Status{Message: msg}); err != nil {
			t.Fatalf("Write: %v", err)
		}
	}
	entries, err := os.ReadDir(Dir(root))
	if err != nil {
		t.Fatal(err)
	}
	var names []string
	for _, e := range entries {
		names = append(names, e.Name())
	}
	if len(names) != 1 || names[0] != "X7F4K2.yaml" {
		t.Fatalf("live dir = %v, want only X7F4K2.yaml", names)
	}
}

func TestRemove(t *testing.T) {
	root := t.TempDir()
	if err := Remove(root, "X7F4K2"); err != nil {
		t.Fatalf("Remove of absent status: %v", err)
	}
	if _, err := os.Stat(filepath.Join(root, ".tiki")); !os.IsNotExist(err) {
		t.Fatalf("Remove created .tiki (stat err %v)", err)
	}
	if err := Write(root, "X7F4K2", Status{Message: "Working"}); err != nil {
		t.Fatal(err)
	}
	if err := Remove(root, "X7F4K2"); err != nil {
		t.Fatalf("Remove: %v", err)
	}
	if _, ok, _ := Read(root, "X7F4K2"); ok {
		t.Fatal("status still readable after Remove")
	}
}

func TestParse(t *testing.T) {
	mtime := time.Date(2026, 10, 11, 9, 0, 0, 0, time.UTC)
	tests := []struct {
		name    string
		data    string
		check   func(t *testing.T, st Status)
		wantErr bool
	}{
		{
			name: "missing updatedAt falls back to mtime",
			data: "message: Working\n",
			check: func(t *testing.T, st Status) {
				if !st.UpdatedAt.Equal(mtime) {
					t.Errorf("UpdatedAt = %v, want %v", st.UpdatedAt, mtime)
				}
			},
		},
		{
			name: "bad updatedAt falls back to mtime",
			data: "message: Working\nupdatedAt: yesterday\n",
			check: func(t *testing.T, st Status) {
				if !st.UpdatedAt.Equal(mtime) {
					t.Errorf("UpdatedAt = %v, want %v", st.UpdatedAt, mtime)
				}
			},
		},
		{
			name: "unknown keys are ignored",
			data: "message: Working\nmodel: opus\nframes: \"⚡ \"\n",
			check: func(t *testing.T, st Status) {
				if st.Message != "Working" {
					t.Errorf("Message = %q", st.Message)
				}
			},
		},
		{name: "empty message is malformed", data: "agent: x\n", wantErr: true},
		{name: "bad YAML is malformed", data: "message: [unclosed\n", wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			st, err := Parse([]byte(tt.data), mtime)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("Parse = %+v, want error", st)
				}
				return
			}
			if err != nil {
				t.Fatalf("Parse: %v", err)
			}
			tt.check(t, st)
		})
	}
}

func TestScan(t *testing.T) {
	root := t.TempDir()
	if err := Write(root, "AAAAAA", Status{Message: "One"}); err != nil {
		t.Fatal(err)
	}
	if err := Write(root, "BBBBBB", Status{Message: "Two"}); err != nil {
		t.Fatal(err)
	}
	dir := Dir(root)
	for name, body := range map[string]string{
		".CCCCCC.123.tmp": "message: half-written\n",
		"notes.txt":       "message: not a status\n",
		"bad-id.yaml":     "message: wrong name\n",
		"DDDDDD.yaml":     "agent: no message\n",
	} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}
	}

	statuses, problems := Scan(root)
	var ids []string
	for id := range statuses {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	if len(ids) != 2 || ids[0] != "AAAAAA" || ids[1] != "BBBBBB" {
		t.Fatalf("Scan ids = %v, want [AAAAAA BBBBBB]", ids)
	}
	if len(problems) != 1 || filepath.Base(problems[0].Path) != "DDDDDD.yaml" {
		t.Fatalf("Scan problems = %+v, want one for DDDDDD.yaml", problems)
	}
}

func TestScanWithoutLiveDir(t *testing.T) {
	statuses, problems := Scan(t.TempDir())
	if len(statuses) != 0 || len(problems) != 0 {
		t.Fatalf("Scan = %v, %v; want nothing", statuses, problems)
	}
}
